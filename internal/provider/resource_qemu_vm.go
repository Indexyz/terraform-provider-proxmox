// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &QemuVMResource{}
var _ resource.ResourceWithImportState = &QemuVMResource{}
var _ resource.ResourceWithValidateConfig = &QemuVMResource{}

// qemuVMPendingTaskKey holds the JSON-encoded UPID of a create or clone task
// the API accepted but whose completion has not been awaited, so a failed or
// cancelled wait can be reconciled on refresh or destroy instead of treating
// the guest as absent while the task still runs.
const qemuVMPendingTaskKey = "qemu-vm-pending-task"

type qemuVMPendingTask struct {
	UPID string `json:"upid,omitempty"`
}

type QemuVMResource struct {
	client *Client
}

func NewQemuVMResource() resource.Resource {
	return &QemuVMResource{}
}

func (r *QemuVMResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_vm"
}

func (r *QemuVMResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceschema.Schema{
		MarkdownDescription: "Manages a Proxmox VE QEMU virtual machine through `/nodes/{node}/qemu`, `/config`, and clone mode create flows.",
		Attributes:          qemuVMResourceAttributes(),
	}
}

func (r *QemuVMResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", err.Error())
		return
	}

	r.client = client
}

func (r *QemuVMResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config qemuVMModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(validateQemuVMRawConflicts(ctx, config)...)
	resp.Diagnostics.Append(validateQemuVMIDAllocation(config)...)
	resp.Diagnostics.Append(validateQemuVMNoCloudSlotConfig(config)...)
	resp.Diagnostics.Append(validateQemuVMPowerConfig(config)...)
}

func (r *QemuVMResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan qemuVMModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// CD-ROM media (the NoCloud seed attachment path) is validated before any
	// guest or clone task exists: unsuitable slots, ambiguous double seeds, and
	// marked seed media that is unusable or absent on the VM's own node must
	// fail first.
	plannedCDROMs, diags := plannedQemuVMCDROMs(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	markerSlot := qemuVMNoCloudSlotValue(plan)
	if markerSlot != "" {
		disks, diskDiags := expandQemuVMDiskModelMap(ctx, plan.Disk)
		resp.Diagnostics.Append(diskDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := validateQemuVMNoCloudMarkerSlot(markerSlot, disks); err != nil {
			resp.Diagnostics.AddError("Invalid NoCloud CD-ROM Slot", err.Error())
			return
		}
	}
	// The marker scopes strict seed safety to this workflow, and the checks
	// must see every attachment the request will carry: planned raw
	// extra_config disk slots are merged in, so a raw second seed cannot pass
	// validation and reach the API beside the typed marked seed. Unmarked
	// workflows keep the typed-only set.
	plannedWireCDROMs, wireDiags := plannedQemuVMWireCDROMs(ctx, plan, plannedCDROMs, markerSlot)
	resp.Diagnostics.Append(wireDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(plannedWireCDROMs) > 0 {
		if err := r.validatePlannedQemuVMCDROMs(ctx, plan.Node.ValueString(), plannedWireCDROMs, markerSlot); err != nil {
			resp.Diagnostics.AddError("Unsafe Proxmox QEMU VM CD-ROM Attachment", err.Error())
			return
		}
	}

	if plan.VMID.IsNull() || plan.VMID.IsUnknown() {
		vmID, err := r.allocateVMID(ctx, qemuInt64Value(plan.VMIDStart))
		if err != nil {
			resp.Diagnostics.AddError("Unable to Allocate Proxmox VMID", err.Error())
			return
		}
		plan.VMID = types.Int64Value(vmID)
	}

	if !plan.Clone.IsNull() && !plan.Clone.IsUnknown() {
		cloneReq, diags := qemuVMCloneRequestFromModel(ctx, plan)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		upid, err := r.client.SubmitCloneQemuVM(ctx, cloneReq)
		if err != nil {
			resp.Diagnostics.AddError("Unable to Clone Proxmox QEMU VM", err.Error())
			return
		}

		// The clone is now accepted by the API. Persist identity, lifecycle
		// policy, and the accepted task before waiting, so a failed or cancelled
		// wait keeps the guest tracked and the pending task reconcilable on
		// refresh or destroy instead of losing ownership of the accepted clone
		// across a polling timeout. A rejected POST (a candidate newid that lost
		// a race) is never tracked or adopted.
		resp.Diagnostics.Append(persistQemuVMIdentity(ctx, &resp.State, plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
		resp.Diagnostics.Append(writeQemuVMPendingTask(ctx, resp.Private, qemuVMPendingTask{UPID: upid})...)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := r.awaitQemuVMPendingTask(ctx, upid); err != nil {
			resp.Diagnostics.AddError("Unable to Await Proxmox QEMU VM Clone Task", err.Error())
			return
		}
		resp.Diagnostics.Append(writeQemuVMPendingTask(ctx, resp.Private, qemuVMPendingTask{})...)
		if resp.Diagnostics.HasError() {
			return
		}

		// Clone-media safety: inspect the raw wire disk config inherited from
		// the template before applying a planned CD-ROM, so a hard disk is
		// never overwritten, a Proxmox-generated cloud-init drive is only
		// replaced explicitly in its own slot, and no second seed remains
		// attached. A failure here retains the clone identity for recovery.
		// Ownership inspection is marker-scoped: unmarked clones keep the
		// ordinary baseline update flow without an ownership read.
		if markerSlot != "" {
			inherited, err := r.client.GetQemuVMConfig(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64())
			if err != nil {
				resp.Diagnostics.AddError("Unable to Read Cloned Proxmox QEMU VM Disks", err.Error())
				return
			}
			if err := validateQemuVMCDROMOwnership(plan.VMID.ValueInt64(), plannedWireCDROMs, inherited.Disk, markerSlot); err != nil {
				resp.Diagnostics.AddError("Unsafe Proxmox QEMU VM CD-ROM Attachment", err.Error())
				return
			}
		}

		updateReq, diags := qemuVMUpdateRequestFromModel(ctx, plan)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if !updateReq.IsEmpty() {
			if err := r.client.UpdateQemuVM(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), updateReq); err != nil {
				resp.Diagnostics.AddError("Unable to Update Cloned Proxmox QEMU VM", err.Error())
				return
			}
		}
	} else {
		createReq, diags := qemuVMCreateRequestFromModel(ctx, plan)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		upid, err := r.client.SubmitCreateQemuVM(ctx, plan.Node.ValueString(), createReq)
		if err != nil {
			resp.Diagnostics.AddError("Unable to Create Proxmox QEMU VM", err.Error())
			return
		}

		// Same as the clone branch: the accepted create is tracked with its
		// pending task before waiting, so the created guest stays owned across
		// a failed or cancelled wait.
		resp.Diagnostics.Append(persistQemuVMIdentity(ctx, &resp.State, plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
		resp.Diagnostics.Append(writeQemuVMPendingTask(ctx, resp.Private, qemuVMPendingTask{UPID: upid})...)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := r.awaitQemuVMPendingTask(ctx, upid); err != nil {
			resp.Diagnostics.AddError("Unable to Await Proxmox QEMU VM Create Task", err.Error())
			return
		}
		resp.Diagnostics.Append(writeQemuVMPendingTask(ctx, resp.Private, qemuVMPendingTask{})...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Declarative `power` reconciles the desired state at create time: true
	// starts the guest after creation/clone and configuration succeed, false
	// leaves it stopped. Unset keeps the one-time `start_on_create` hook
	// behavior. A failure here leaves the tracked guest in state for
	// recovery, with its desired power retained for the retry.
	startGuest := false
	switch {
	case !plan.Power.IsNull() && !plan.Power.IsUnknown():
		startGuest = plan.Power.ValueBool()
	case !plan.StartOnCreate.IsNull() && !plan.StartOnCreate.IsUnknown():
		startGuest = plan.StartOnCreate.ValueBool()
	}
	if startGuest {
		if err := r.client.StartQemuVM(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64()); err != nil {
			resp.Diagnostics.AddError("Unable to Start Proxmox QEMU VM", err.Error())
			return
		}
	}

	state, diags := r.readQemuVMState(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *QemuVMResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state qemuVMModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Reconcile an accepted but unawaited create/clone task before deciding
	// absence: the guest may be transiently missing while the task still
	// runs, so a refresh must neither drop the resource nor act on it yet.
	pending, diags := readQemuVMPendingTask(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if pending.UPID != "" {
		settled, diags := r.reconcileQemuVMPendingTask(ctx, pending.UPID)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if !settled {
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
		resp.Diagnostics.Append(writeQemuVMPendingTask(ctx, resp.Private, qemuVMPendingTask{})...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	refreshed, diags := r.readQemuVMState(ctx, state.Node.ValueString(), state.VMID.ValueInt64(), &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if refreshed.ID.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *QemuVMResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan qemuVMModel
	var state qemuVMModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	current, err := r.client.GetQemuVMConfig(ctx, state.Node.ValueString(), state.VMID.ValueInt64())
	if err != nil {
		if errors.Is(err, errNotFound) {
			resp.Diagnostics.AddError(
				"Proxmox QEMU VM No Longer Exists",
				fmt.Sprintf("QEMU virtual machine %q no longer exists on node %q.", qemuVMID(state.Node.ValueString(), state.VMID.ValueInt64()), state.Node.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError("Unable to Read Current Proxmox QEMU VM", err.Error())
		return
	}

	// A marked in-place seed mutation must prove attachment safety against the
	// live wire configuration before any PUT: the marked slot must plan a real
	// ISO volume, the new medium must exist on the VM's node, and the mutation
	// may only replace the same medium, an empty bay, or the same-slot Proxmox
	// cloud-init drive. Refreshed state is observed reality, not proof of which
	// medium this resource attached, so it grants no replacement allowance:
	// any different real medium is refused in favor of VM replacement. Planned
	// raw extra_config disk slots join the effective attachment set, so a raw
	// second seed or raw target overwrite cannot slip past the guard.
	markerSlot := qemuVMNoCloudSlotValue(plan)
	if markerSlot != "" {
		disks, diskDiags := expandQemuVMDiskModelMap(ctx, plan.Disk)
		resp.Diagnostics.Append(diskDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := validateQemuVMNoCloudMarkerSlot(markerSlot, disks); err != nil {
			resp.Diagnostics.AddError("Invalid NoCloud CD-ROM Slot", err.Error())
			return
		}
		plannedCDROMs, diags := plannedQemuVMCDROMs(ctx, plan)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		plannedWireCDROMs, wireDiags := plannedQemuVMWireCDROMs(ctx, plan, plannedCDROMs, markerSlot)
		resp.Diagnostics.Append(wireDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := r.validatePlannedQemuVMCDROMs(ctx, plan.Node.ValueString(), plannedWireCDROMs, markerSlot); err != nil {
			resp.Diagnostics.AddError("Unsafe Proxmox QEMU VM CD-ROM Attachment", err.Error())
			return
		}
		if err := validateQemuVMCDROMOwnership(state.VMID.ValueInt64(), plannedWireCDROMs, current.Disk, markerSlot); err != nil {
			resp.Diagnostics.AddError("Unsafe Proxmox QEMU VM CD-ROM Attachment", err.Error())
			return
		}
	}

	// Declarative power reconciliation happens after the current-config and
	// CD-ROM safety checks and before the update request: a desired stop
	// applies to the still-running guest before the config PUT (config
	// changes belong to a stopped guest), a desired start runs after the PUT
	// (config-then-start, matching create order), and an already matching
	// state takes no power action. A status read failure aborts before any
	// mutation.
	var startAfterPut bool
	if !plan.Power.IsNull() && !plan.Power.IsUnknown() {
		status, err := r.client.GetQemuVMStatus(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Unable to Read Proxmox QEMU VM Status Before Power Reconcile", err.Error())
			return
		}
		switch {
		case !qemuVMPoweredOn(status) && plan.Power.ValueBool():
			startAfterPut = true
		case qemuVMPoweredOn(status) && !plan.Power.ValueBool():
			timeout := powerShutdownTimeoutDefault
			if !plan.PowerShutdownTimeout.IsNull() && !plan.PowerShutdownTimeout.IsUnknown() {
				timeout = int(plan.PowerShutdownTimeout.ValueInt64())
			}
			if err := r.client.ShutdownQemuVM(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), timeout); err != nil {
				resp.Diagnostics.AddError("Unable to Shut Down Proxmox QEMU VM", err.Error())
				return
			}
		}
	}

	updateReq, diags := qemuVMUpdateRequestFromModel(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Terraform fills unconfigured Optional+Computed disk and network maps
	// with prior observations, so the final plan re-carries inherited slots
	// (a template clone's system disk and NIC) that the configuration never
	// declared. Narrow the outgoing update request to slots that are new or
	// changed relative to the prior state: an unrelated update must not
	// resend observed inherited attachments as new mutation intent, while
	// deliberately added or changed slots are preserved. The full planned
	// and live typed+raw media validation above already ran on the complete
	// effective attachment set.
	priorReq, priorDiags := qemuVMConfigRequestFromModel(ctx, state)
	resp.Diagnostics.Append(priorDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	narrowedDisk := make(map[string]string, len(updateReq.Disk))
	for slot, value := range updateReq.Disk {
		if prior, ok := priorReq.Disk[slot]; !ok || prior != value {
			narrowedDisk[slot] = value
		}
	}
	updateReq.Disk = narrowedDisk
	narrowedNetwork := make(map[string]string, len(updateReq.Network))
	for slot, value := range updateReq.Network {
		if prior, ok := priorReq.Network[slot]; !ok || prior != value {
			narrowedNetwork[slot] = value
		}
	}
	updateReq.Network = narrowedNetwork

	if err := r.client.UpdateQemuVM(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), updateReq); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox QEMU VM", err.Error())
		return
	}

	if startAfterPut {
		if err := r.client.StartQemuVM(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64()); err != nil {
			resp.Diagnostics.AddError("Unable to Start Proxmox QEMU VM", err.Error())
			return
		}
	}

	// The post-update read projects the disk map onto the plan's intended
	// managed key set rather than the old state's: a newly declared managed
	// slot must survive the update in state, while a slot the plan no longer
	// declares drops out. Everything else still echoes the prior state.
	// Power mirrors observed reality through the plan's desired power as the
	// reconcile switch, and the timeout echoes the plan so an in-place
	// change converges in the same apply.
	planPrior := state
	planPrior.Disk = plan.Disk
	planPrior.Power = plan.Power
	planPrior.PowerShutdownTimeout = plan.PowerShutdownTimeout
	refreshed, diags := r.readQemuVMState(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), &planPrior)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Lifecycle hooks are Terraform-side policies; persist the plan values so
	// updating them never restarts the guest and applies on the next destroy.
	// Read remains observed-only for runtime state.
	refreshed.StartOnCreate = plan.StartOnCreate
	refreshed.StopOnDestroy = plan.StopOnDestroy

	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *QemuVMResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state qemuVMModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	node := state.Node.ValueString()
	vmID := state.VMID.ValueInt64()

	// A retained accepted create/clone task is reconciled before cleanup so
	// it can neither be raced nor wedge the destroy forever.
	pending, diags := readQemuVMPendingTask(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if pending.UPID != "" {
		// Cleanup must not race active work, but a task that already stopped
		// (successfully or not) no longer blocks: one owner-node poll first
		// classifies the task, a settled task lets the config read below decide
		// what actually remains, a still-running task must be awaited, and poll
		// failures or unknown states abort. A wait failure likewise aborts so
		// deletion is never premature; the retry classifies the settled task.
		settled, classifyDiags := r.reconcileQemuVMPendingTask(ctx, pending.UPID)
		resp.Diagnostics.Append(classifyDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if !settled {
			if err := r.awaitQemuVMPendingTask(ctx, pending.UPID); err != nil {
				resp.Diagnostics.AddError("Unable to Await Proxmox QEMU VM Pending Task", err.Error())
				return
			}
		}
		resp.Diagnostics.Append(writeQemuVMPendingTask(ctx, resp.Private, qemuVMPendingTask{})...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Discover a guest that no longer exists before any power action, so
	// repeated destroys stay idempotent; PVE reports missing QEMU configs as
	// HTTP 500, which the client classifies as errNotFound for this exact
	// node/VMID shape.
	if _, err := r.client.GetQemuVMConfig(ctx, node, vmID); err != nil {
		if errors.Is(err, errNotFound) {
			return
		}
		resp.Diagnostics.AddError("Unable to Read Proxmox QEMU VM Before Delete", err.Error())
		return
	}

	if state.StopOnDestroy.ValueBool() {
		status, err := r.client.GetQemuVMStatus(ctx, node, vmID)
		if err != nil && !errors.Is(err, errNotFound) {
			resp.Diagnostics.AddError("Unable to Read Proxmox QEMU VM Status Before Delete", err.Error())
			return
		}
		// A guest that vanished between the config and status reads is gone;
		// the delete below stays idempotent for that case.
		if err == nil && status.Status == "running" {
			// No delete after a failed or cancelled stop: the guest must remain
			// tracked so the destroy can be retried. A 404 while polling the
			// stop task proves nothing about the guest or the stop result, so
			// every stop error aborts; a later retry re-checks the guest by
			// config read.
			if err := r.client.StopQemuVM(ctx, node, vmID); err != nil {
				resp.Diagnostics.AddError("Unable to Stop Proxmox QEMU VM Before Delete", err.Error())
				return
			}
		}
	}

	if err := r.client.DeleteQemuVM(ctx, node, vmID); err != nil {
		// The authoritative absence classification is the config read above;
		// any errNotFound surfacing here can only come from the delete-task
		// status poll, and a missing task is not a missing VM, so the error
		// must keep the state for a retry instead of allowing dependent
		// cleanup on an unverified deletion.
		resp.Diagnostics.AddError("Unable to Delete Proxmox QEMU VM", err.Error())
	}
}

func (r *QemuVMResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, vmID, err := parseQemuVMImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Import Identifier", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), qemuVMID(node, vmID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vm_id"), vmID)...)
}

// persistQemuVMIdentity writes the resource identity and Terraform-side
// lifecycle policy into the response state right after the create or clone
// task succeeds, so a later post-create failure (config update, start) still
// leaves the created guest tracked with its destroy policy.
func persistQemuVMIdentity(ctx context.Context, state *tfsdk.State, plan qemuVMModel) diag.Diagnostics {
	var diags diag.Diagnostics

	diags.Append(state.SetAttribute(ctx, path.Root("id"), qemuVMID(plan.Node.ValueString(), plan.VMID.ValueInt64()))...)
	diags.Append(state.SetAttribute(ctx, path.Root("node"), plan.Node)...)
	diags.Append(state.SetAttribute(ctx, path.Root("vm_id"), plan.VMID)...)
	diags.Append(state.SetAttribute(ctx, path.Root("vm_id_start"), plan.VMIDStart)...)
	diags.Append(state.SetAttribute(ctx, path.Root("start_on_create"), plan.StartOnCreate)...)
	diags.Append(state.SetAttribute(ctx, path.Root("stop_on_destroy"), plan.StopOnDestroy)...)
	diags.Append(state.SetAttribute(ctx, path.Root("power"), plan.Power)...)
	diags.Append(state.SetAttribute(ctx, path.Root("power_shutdown_timeout"), plan.PowerShutdownTimeout)...)
	diags.Append(state.SetAttribute(ctx, path.Root("nocloud_cdrom_slot"), plan.NoCloudCDROMSlot)...)

	return diags
}

// awaitQemuVMPendingTask waits for an accepted create/clone task on the node
// that owns it: a clone task runs on the source node even when the guest
// lands on a different target node.
func (r *QemuVMResource) awaitQemuVMPendingTask(ctx context.Context, upid string) error {
	node, err := taskOwnerNode(upid)
	if err != nil {
		return err
	}
	return r.client.waitForNodeTask(ctx, node, upid)
}

// reconcileQemuVMPendingTask polls an accepted create/clone task once and
// reports whether it settled. A running task keeps the resource tracked
// without any absence decision; a settled task (successful or failed) is
// cleared so the ordinary read or destroy flow can decide from the guest
// itself. Poll failures and unknown task states are retryable errors, never
// absence or success.
func (r *QemuVMResource) reconcileQemuVMPendingTask(ctx context.Context, upid string) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	node, err := taskOwnerNode(upid)
	if err != nil {
		diags.AddError("Unable to Read Proxmox QEMU VM Pending Task", err.Error())
		return false, diags
	}
	status, err := r.client.GetNodeTaskStatus(ctx, node, upid)
	if err != nil {
		diags.AddError("Unable to Read Proxmox QEMU VM Pending Task", err.Error())
		return false, diags
	}
	finished, err := nodeTaskFinished(status, upid)
	if err != nil {
		diags.AddError("Unable to Read Proxmox QEMU VM Pending Task", err.Error())
		return false, diags
	}
	return finished, diags
}

func readQemuVMPendingTask(ctx context.Context, private privateStateReader) (qemuVMPendingTask, diag.Diagnostics) {
	var diags diag.Diagnostics
	var pending qemuVMPendingTask
	data, privateDiags := private.GetKey(ctx, qemuVMPendingTaskKey)
	diags.Append(privateDiags...)
	if diags.HasError() || len(data) == 0 {
		return pending, diags
	}
	if err := json.Unmarshal(data, &pending); err != nil {
		diags.AddError("Unable to Read Proxmox QEMU VM State", fmt.Sprintf("unable to decode pending task state: %v", err))
	}
	return pending, diags
}

func writeQemuVMPendingTask(ctx context.Context, private privateStateWriter, pending qemuVMPendingTask) diag.Diagnostics {
	encoded, err := json.Marshal(pending)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Unable to Store Proxmox QEMU VM State", fmt.Sprintf("unable to encode pending task state: %v", err))
		return diags
	}
	return private.SetKey(ctx, qemuVMPendingTaskKey, encoded)
}

// plannedQemuVMCDROMs returns planned CD-ROM media keyed by slot from the
// typed disk map. A slot counts as a CD-ROM when it declares
// `media = "cdrom"` or points at an `.iso` volume; the suffix is a safety
// heuristic so the attachment checks cannot miss a seed drive - PVE itself
// requires explicit `media=cdrom` for ISO images, which the marked NoCloud
// slot enforces.
func plannedQemuVMCDROMs(ctx context.Context, model qemuVMModel) (map[string]string, diag.Diagnostics) {
	disks, diags := expandQemuVMDiskModelMap(ctx, model.Disk)
	if diags.HasError() || len(disks) == 0 {
		return nil, diags
	}

	planned := map[string]string{}
	for slot, disk := range disks {
		volume := stringValue(disk.Volume)
		if !strings.EqualFold(stringValue(disk.Media), "cdrom") && !strings.HasSuffix(strings.ToLower(volume), ".iso") {
			continue
		}
		planned[slot] = volume
	}
	return planned, diags
}

// plannedQemuVMRawWireCDROMs returns planned raw.extra_config disk slots whose
// wire value describes CD-ROM media (an `.iso` volume or `media=cdrom`) or a
// positional pseudo medium (`none`/`cdrom`) that clears the slot without
// declaring media: the attachments qemuVMConfigRequestFromModel encodes beside
// the typed disk map. The only caller merges these under the NoCloud marker,
// so bare pseudo values join the inspected set without widening unmarked
// workflows. Values are reduced to their bare volume reference so they
// compare uniformly with typed planned volumes.
func plannedQemuVMRawWireCDROMs(ctx context.Context, model qemuVMModel) (map[string]string, diag.Diagnostics) {
	raw, diags := expandQemuVMRawModel(ctx, model.Raw)
	if diags.HasError() || raw.ExtraConfig.IsNull() || raw.ExtraConfig.IsUnknown() {
		return nil, diags
	}
	var extra map[string]string
	diags.Append(raw.ExtraConfig.ElementsAs(ctx, &extra, false)...)
	if diags.HasError() {
		return nil, diags
	}
	var planned map[string]string
	for slot, value := range extra {
		if !isQemuVMDiskKey(slot) {
			continue
		}
		volume := qemuVMWireDiskVolume(value)
		if qemuVMWireDiskIsCDROM(value) || qemuVMIsPseudoMedium(volume) {
			if planned == nil {
				planned = make(map[string]string, len(extra))
			}
			planned[slot] = volume
		}
	}
	return planned, diags
}

// plannedQemuVMWireCDROMs returns the effective planned CD-ROM attachment set:
// the typed planned CD-ROMs plus, when the NoCloud marker scopes the strict
// seed checks, planned raw extra_config CD-ROM slots and typed or raw slots
// holding positional `none`/`cdrom` pseudo media. Pseudo values clear a slot
// without declaring media, so they cannot bypass ownership inspection even
// though they never count as seed volumes. Unmarked workflows keep the
// typed-only set so ordinary raw passthrough stays unguarded.
func plannedQemuVMWireCDROMs(ctx context.Context, model qemuVMModel, typed map[string]string, markerSlot string) (map[string]string, diag.Diagnostics) {
	if markerSlot == "" {
		return typed, nil
	}
	raw, diags := plannedQemuVMRawWireCDROMs(ctx, model)
	if diags.HasError() {
		return nil, diags
	}
	disks, diskDiags := expandQemuVMDiskModelMap(ctx, model.Disk)
	if diskDiags.HasError() {
		return nil, diskDiags
	}
	merged := make(map[string]string, len(typed)+len(raw)+len(disks))
	maps.Copy(merged, typed)
	maps.Copy(merged, raw)
	for slot, disk := range disks {
		if volume := stringValue(disk.Volume); qemuVMIsPseudoMedium(volume) {
			merged[slot] = volume
		}
	}
	return merged, diags
}

// qemuVMIsPseudoMedium reports whether a reduced volume reference is one of
// PVE's positional pseudo media - the physical `cdrom` drive or an empty
// `none` bay - which clear a slot without naming a real volume.
func qemuVMIsPseudoMedium(volume string) bool {
	return volume == "none" || volume == "cdrom"
}

// validateQemuVMNoCloudSlotConfig checks the static `nocloud_cdrom_slot`
// grammar at configuration time; attachment semantics need the planned disk
// map and are checked before any HTTP call in create and update.
func validateQemuVMNoCloudSlotConfig(model qemuVMModel) diag.Diagnostics {
	var diags diag.Diagnostics

	if model.NoCloudCDROMSlot.IsNull() || model.NoCloudCDROMSlot.IsUnknown() {
		return diags
	}
	slot := strings.TrimSpace(model.NoCloudCDROMSlot.ValueString())
	if !qemuVMSlotSupportsCDROM(slot) {
		diags.AddAttributeError(
			path.Root("nocloud_cdrom_slot"),
			"Invalid NoCloud CD-ROM slot",
			fmt.Sprintf("`nocloud_cdrom_slot` %q cannot hold CD-ROM media: use an ide, sata, or scsi slot.", slot),
		)
	}
	return diags
}

// qemuVMNoCloudSlotValue returns the trimmed `nocloud_cdrom_slot` marker, or
// an empty string when unset; unmarked workflows keep ordinary CD-ROM
// behavior without seed layout restrictions.
func qemuVMNoCloudSlotValue(model qemuVMModel) string {
	if model.NoCloudCDROMSlot.IsNull() || model.NoCloudCDROMSlot.IsUnknown() {
		return ""
	}
	return strings.TrimSpace(model.NoCloudCDROMSlot.ValueString())
}

// validateQemuVMNoCloudMarkerSlot requires the marked slot to plan a real,
// explicitly typed ISO CD-ROM. PVE rejects ISO images without explicit
// `media=cdrom` ("explicit 'media=cdrom' is required for iso images"), so a
// bare `.iso` volume without media, or an explicit disk media value, cannot
// carry the NoCloud seed. Pseudo volumes (`none`, `cdrom`, unset) and
// unattached slots are refused as well.
func validateQemuVMNoCloudMarkerSlot(markerSlot string, disks map[string]qemuVMDiskModel) error {
	disk, ok := disks[markerSlot]
	if !ok {
		return fmt.Errorf("`nocloud_cdrom_slot` declares slot %q but the plan attaches no CD-ROM media to it; attach the NoCloud seed through disk[%q].volume or remove the marker (which requires replacement)", markerSlot, markerSlot)
	}
	volume := stringValue(disk.Volume)
	if !qemuVMIsVolumeReference(volume) {
		return fmt.Errorf("`nocloud_cdrom_slot` slot %q must plan a real ISO volume (for example `storage:iso/seed.iso`), not %q", markerSlot, volume)
	}
	if !strings.EqualFold(stringValue(disk.Media), "cdrom") {
		return fmt.Errorf("`nocloud_cdrom_slot` slot %q must set `media = \"cdrom\"` explicitly on the marked disk; Proxmox requires explicit media=cdrom for ISO images, so a bare .iso volume or a disk medium cannot carry the seed", markerSlot)
	}
	return nil
}

// validatePlannedQemuVMCDROMs checks plan-level CD-ROM safety before any VM
// exists: cdrom media requires an ide/sata/scsi slot for every planned
// attachment. Without the `nocloud_cdrom_slot` marker the attachment is an
// ordinary CD-ROM and Proxmox enforces storage reality at PUT time. With the
// marker, planned holds the full effective attachment set (typed CD-ROMs plus
// planned raw extra_config CD-ROM slots), the marked slot is the single seed:
// no other planned seed volume is allowed, the seed storage must be visible,
// active, and iso-capable on the VM's own node (VM disk storages need not
// serve ISO), and the exact seed volume must exist in the VM node's
// authoritative ISO collection, so a same-named storage on another node
// lacking the file fails before clone.
func (r *QemuVMResource) validatePlannedQemuVMCDROMs(ctx context.Context, node string, planned map[string]string, markerSlot string) error {
	for slot := range planned {
		if !qemuVMSlotSupportsCDROM(slot) {
			return fmt.Errorf("slot %q cannot hold CD-ROM media: use an ide, sata, or scsi slot", slot)
		}
	}
	if markerSlot == "" {
		return nil
	}

	var extraSeeds []string
	for slot, volume := range planned {
		if slot != markerSlot && qemuVMIsVolumeReference(volume) {
			extraSeeds = append(extraSeeds, slot)
		}
	}
	if len(extraSeeds) > 0 {
		sort.Strings(extraSeeds)
		return fmt.Errorf("plan attaches seed volumes to slots %s besides the marked NoCloud slot %q; at most one cloud-init seed drive may be attached", strings.Join(extraSeeds, ", "), markerSlot)
	}

	volume := planned[markerSlot]
	storage := qemuVMDiskStorageFromVolume(volume)
	if err := ensureISOStorage(ctx, r.client, node, storage); err != nil {
		return err
	}
	exists, err := isoVolumeExists(ctx, r.client, node, storage, volume)
	if err != nil {
		return fmt.Errorf("unable to verify seed volume %q on node %q: %w", volume, node, err)
	}
	if !exists {
		return fmt.Errorf("seed volume %q does not exist in storage %q on node %q; create it (for example with proxmox_nocloud_iso) before attaching", volume, storage, node)
	}
	return nil
}

// validateQemuVMCDROMOwnership refuses to overwrite foreign disk content in
// marked NoCloud workflows: the target slot may only already hold the same
// volume, an empty bay, or the cloud-init drive Proxmox generated for this
// VM. Refreshed Terraform state is observed reality, not proof of which
// medium this resource attached, so prior state grants no replacement
// allowance: swapping any different real medium in place is refused in favor
// of replacing the VM. planned holds the full effective attachment set (typed
// CD-ROMs plus planned raw extra_config CD-ROM slots); inherited holds raw
// `/config` wire values, including ones the typed disk parser does not fully
// understand. Unmarked workflows perform no ownership inspection: ordinary
// CD-ROM updates keep their baseline behavior, guarded only by Proxmox
// itself.
func validateQemuVMCDROMOwnership(vmID int64, planned map[string]string, inherited map[string]string, markerSlot string) error {
	if markerSlot == "" {
		return nil
	}
	for slot, volume := range planned {
		raw, ok := inherited[slot]
		if !ok {
			continue
		}
		currentVolume := qemuVMWireDiskVolume(raw)
		switch {
		case currentVolume == volume, currentVolume == "none":
			// The slot already holds the planned medium, or an empty CD-ROM
			// bay holds no medium to discard.
		case qemuVMIsPVECloudInitDrive(raw, vmID):
			// Replacing the Proxmox-generated cloud-init drive in its own
			// slot is the explicit, safe replacement path.
		default:
			return fmt.Errorf("slot %q currently holds %q; attaching %q would replace an existing disk or unknown medium - only the same volume, an empty bay, or this VM's Proxmox-generated cloud-init drive in its own slot may be replaced in place; swap the seed by replacing the VM (for example through lifecycle replace_triggered_by), not by adopting or overwriting the live medium", slot, currentVolume, volume)
		}
	}
	var extraSeeds []string
	for slot, volume := range planned {
		if slot != markerSlot && qemuVMIsVolumeReference(volume) {
			extraSeeds = append(extraSeeds, slot)
		}
	}
	for slot, raw := range inherited {
		if _, isPlanned := planned[slot]; isPlanned {
			continue
		}
		if !qemuVMWireDiskIsCDROM(raw) {
			continue
		}
		if !qemuVMIsVolumeReference(qemuVMWireDiskVolume(raw)) {
			continue
		}
		extraSeeds = append(extraSeeds, slot)
	}
	if len(extraSeeds) > 0 {
		sort.Strings(extraSeeds)
		return fmt.Errorf("attaching the marked NoCloud seed in %q would leave more than one ISO/cloud-init drive attached (%s); replace the inherited Proxmox cloud-init drive in its own slot or detach the other medium first", markerSlot, strings.Join(extraSeeds, ", "))
	}
	return nil
}

// qemuVMSlotSupportsCDROM reports whether a disk slot can hold CD-ROM media;
// Proxmox defines cdrom media for ide, sata, and scsi drives only.
func qemuVMSlotSupportsCDROM(slot string) bool {
	for _, prefix := range []string{"ide", "sata", "scsi"} {
		if strings.HasPrefix(slot, prefix) && len(slot) > len(prefix) && isDecimalString(slot[len(prefix):]) {
			return true
		}
	}
	return false
}

// qemuVMIsVolumeReference reports whether a CD-ROM volume is a real storage
// volume, as opposed to the `cdrom` (physical drive) and `none` (empty)
// pseudo volumes or an unset value.
func qemuVMIsVolumeReference(volume string) bool {
	return volume != "" && volume != "cdrom" && volume != "none"
}

// qemuVMCloudInitFormats mirrors the PVE qemu-server `$QEMU_FORMAT_RE`
// suffixes a file-backed cloudinit volume may carry.
var qemuVMCloudInitFormats = map[string]struct{}{"raw": {}, "qcow": {}, "qcow2": {}, "qed": {}, "vmdk": {}, "cloop": {}}

// qemuVMIsPVECloudInitDrive reports whether a raw `/config` disk value is the
// cloud-init drive Proxmox generated for this VM. Proxmox sets `media=cdrom`
// when generating the drive and, matching
// PVE::QemuServer::Drive::drive_is_cloudinit, accepts block storages
// (`<storage>:vm-<vmid>-cloudinit`) and file-backed storages
// (`<storage>:<vmid>/vm-<vmid>-cloudinit.<format>`); the owner VMID and the
// known grammar must match exactly, so foreign or lookalike files never
// count.
func qemuVMIsPVECloudInitDrive(raw string, vmID int64) bool {
	if !qemuVMWireDiskHasMediaCDROM(raw) {
		return false
	}
	_, name, ok := strings.Cut(qemuVMWireDiskVolume(raw), ":")
	if !ok || name == "" {
		return false
	}
	base := name
	if _, format, hasFormat := strings.Cut(name, "."); hasFormat {
		if _, known := qemuVMCloudInitFormats[format]; !known {
			return false
		}
		base = strings.TrimSuffix(name, "."+format)
	}
	if base == fmt.Sprintf("vm-%d-cloudinit", vmID) {
		return true
	}
	// File-backed (path-based) storages prefix the volume name with the VMID.
	owner, file, hasOwner := strings.Cut(base, "/")
	return hasOwner && owner == strconv.FormatInt(vmID, 10) && file == fmt.Sprintf("vm-%d-cloudinit", vmID)
}

// qemuVMWireDiskVolume returns the volume reference of a raw `/config` disk
// value: the positional segment before the options.
func qemuVMWireDiskVolume(raw string) string {
	volume, _, _ := strings.Cut(strings.TrimSpace(raw), ",")
	return strings.TrimSpace(volume)
}

// qemuVMWireDiskIsCDROM reports whether a raw `/config` disk value describes
// a CD-ROM medium, including values whose extra options the typed disk
// parser does not fully understand.
func qemuVMWireDiskIsCDROM(raw string) bool {
	if strings.HasSuffix(strings.ToLower(qemuVMWireDiskVolume(raw)), ".iso") {
		return true
	}
	return qemuVMWireDiskHasMediaCDROM(raw)
}

// qemuVMWireDiskHasMediaCDROM reports whether a raw `/config` disk value
// carries a `media=cdrom` option.
func qemuVMWireDiskHasMediaCDROM(raw string) bool {
	for segment := range strings.SplitSeq(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(segment), "=")
		if ok && strings.TrimSpace(key) == "media" && strings.EqualFold(strings.TrimSpace(value), "cdrom") {
			return true
		}
	}
	return false
}

// allocateVMID returns the next free cluster VMID through `GET /cluster/nextid`.
// The endpoint cannot express a floor, so when start is positive the provider
// proposes candidate IDs beginning at start and asserts availability until one
// is free (a taken candidate fails with HTTP 400 `VM <id> already exists`).
func (r *QemuVMResource) allocateVMID(ctx context.Context, start int64) (int64, error) {
	if start <= 0 {
		return r.client.GetNextVMID(ctx, nil)
	}

	for candidate := start; candidate <= qemuVMIDMaximum; candidate++ {
		vmID, err := r.client.GetNextVMID(ctx, &candidate)
		if err == nil {
			return vmID, nil
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest || !strings.Contains(apiErr.Body, "already exists") {
			return 0, err
		}
	}
	return 0, fmt.Errorf("no free VMID found in range [%d, %d]", start, qemuVMIDMaximum)
}

func (r *QemuVMResource) readQemuVMState(ctx context.Context, node string, vmID int64, prior *qemuVMModel) (qemuVMModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	config, err := r.client.GetQemuVMConfig(ctx, node, vmID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return qemuVMModel{ID: types.StringNull()}, diags
		}

		diags.AddError("Unable to Read Proxmox QEMU VM Config", err.Error())
		return qemuVMModel{}, diags
	}

	status, err := r.client.GetQemuVMStatus(ctx, node, vmID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return qemuVMModel{ID: types.StringNull()}, diags
		}

		diags.AddError("Unable to Read Proxmox QEMU VM Status", err.Error())
		return qemuVMModel{}, diags
	}

	state, stateDiags := qemuVMStateFromAPI(ctx, node, vmID, config, status, prior)
	diags.Append(stateDiags...)
	return state, diags
}
