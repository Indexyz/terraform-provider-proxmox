// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &NoCloudISOResource{}
var _ resource.ResourceWithValidateConfig = &NoCloudISOResource{}

// noCloudISOTasksKey holds the JSON-encoded UPIDs of API tasks this resource
// accepted but has not successfully awaited, so failed waits can be
// reconciled on retry instead of orphaning media or issuing duplicate cleanup.
const noCloudISOTasksKey = "nocloud-iso-managed-tasks"

type noCloudISOTasks struct {
	UploadTaskUPID string `json:"upload_task_upid,omitempty"`
	DeleteTaskUPID string `json:"delete_task_upid,omitempty"`
}

// noCloudISOFileNamePattern keeps the managed seed filename a safe exact
// `.iso` basename, unique to one resource and one generation.
var noCloudISOFileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.iso$`)

type NoCloudISOResource struct {
	client *Client
}

type noCloudISOModel struct {
	ID            types.String `tfsdk:"id"`
	Node          types.String `tfsdk:"node"`
	Storage       types.String `tfsdk:"storage"`
	Filename      types.String `tfsdk:"filename"`
	UserData      types.String `tfsdk:"user_data"`
	MetaData      types.String `tfsdk:"meta_data"`
	NetworkConfig types.String `tfsdk:"network_config"`
	VolumeID      types.String `tfsdk:"volume_id"`
}

func NewNoCloudISOResource() resource.Resource {
	return &NoCloudISOResource{}
}

func (r *NoCloudISOResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nocloud_iso"
}

func (r *NoCloudISOResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replaceString := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Generates a cloud-init NoCloud seed ISO labeled `CIDATA` with the given `user-data`, `meta-data`, and `network-config` root files, uploads it through `/nodes/{node}/storage/{storage}/upload`, and manages the resulting storage content item. All creation inputs require replacement. The filename must be unique for this resource; an existing destination is refused instead of adopted or overwritten, and the PVE upload API has no atomic create-if-absent, so callers must ensure cross-state and cross-generation uniqueness.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `node/storage/volume_id` form."},
			"node":           schema.StringAttribute{Required: true, MarkdownDescription: "Proxmox node that performs the upload. Changes require replacement.", PlanModifiers: replaceString},
			"storage":        schema.StringAttribute{Required: true, MarkdownDescription: "Destination storage identifier. Must be an enabled, active storage on the node that supports `iso` content. Changes require replacement.", PlanModifiers: replaceString},
			"filename":       schema.StringAttribute{Required: true, MarkdownDescription: "Exact `.iso` basename of the seed image. Use only letters, numbers, dots, underscores, and hyphens. Changes require replacement.", PlanModifiers: replaceString},
			"user_data":      schema.StringAttribute{Required: true, Sensitive: true, MarkdownDescription: "Raw cloud-init `user-data` content. Stored in Terraform state in plaintext despite being sensitive. Changes require replacement.", PlanModifiers: replaceString},
			"meta_data":      schema.StringAttribute{Required: true, Sensitive: true, MarkdownDescription: "Raw cloud-init `meta-data` content, including the per-generation `instance-id`. Stored in Terraform state in plaintext despite being sensitive. Changes require replacement.", PlanModifiers: replaceString},
			"network_config": schema.StringAttribute{Required: true, Sensitive: true, MarkdownDescription: "Raw cloud-init `network-config` content. Stored in Terraform state in plaintext despite being sensitive. Changes require replacement.", PlanModifiers: replaceString},
			"volume_id":      schema.StringAttribute{Computed: true, MarkdownDescription: "Proxmox volume identifier of the seed image in `storage:iso/filename` form."},
		},
	}
}

func (r *NoCloudISOResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *NoCloudISOResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config noCloudISOModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateNoCloudISOConfig(config)...)
}

func validateNoCloudISOConfig(config noCloudISOModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if !config.Filename.IsNull() && !config.Filename.IsUnknown() && !noCloudISOFileNamePattern.MatchString(config.Filename.ValueString()) {
		diags.AddAttributeError(path.Root("filename"), "Invalid NoCloud ISO filename", "filename must start with a letter or number and contain only letters, numbers, dots, underscores, and hyphens, ending with .iso")
	}
	return diags
}

func (r *NoCloudISOResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan noCloudISOModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	node := plan.Node.ValueString()
	storage := plan.Storage.ValueString()
	volumeID := storageFileVolumeID(storage, "iso", plan.Filename.ValueString())

	if err := ensureISOStorage(ctx, r.client, node, storage); err != nil {
		resp.Diagnostics.AddError("Unable to Use Proxmox ISO Storage", err.Error())
		return
	}

	exists, err := isoVolumeExists(ctx, r.client, node, storage, volumeID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Storage Content", err.Error())
		return
	}
	if exists {
		resp.Diagnostics.AddError(
			"Proxmox Storage File Already Exists",
			fmt.Sprintf("Storage content %q already exists on storage %q of node %q. The Proxmox upload API would silently overwrite it and has no atomic create-if-absent, so the resource refuses existing destinations; use a filename unique to this resource and generation.", volumeID, storage, node),
		)
		return
	}

	data, err := buildNoCloudISO([]byte(plan.UserData.ValueString()), []byte(plan.MetaData.ValueString()), []byte(plan.NetworkConfig.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Build NoCloud ISO", err.Error())
		return
	}

	upid, taskNode, err := r.client.UploadStorageFile(ctx, UploadStorageFileRequest{
		Node:     node,
		Storage:  storage,
		Content:  "iso",
		Filename: plan.Filename.ValueString(),
		Data:     data,
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to Upload Proxmox NoCloud ISO", err.Error())
		return
	}

	// The upload is now accepted by the API. Persist identity and the accepted
	// task before waiting, so a failed or cancelled wait keeps the resource
	// tracked with enough state to reconcile cleanup instead of orphaning the
	// media under an already-refused filename.
	plan.VolumeID = types.StringValue(volumeID)
	plan.ID = types.StringValue(storageFileDownloadID(node, storage, volumeID))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(writeNoCloudISOTasks(ctx, resp.Private, noCloudISOTasks{UploadTaskUPID: upid})...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.waitForNodeTask(ctx, taskNode, upid); err != nil {
		resp.Diagnostics.AddError("Unable to Upload Proxmox NoCloud ISO", err.Error())
		return
	}
	resp.Diagnostics.Append(writeNoCloudISOTasks(ctx, resp.Private, noCloudISOTasks{})...)
}

func (r *NoCloudISOResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state noCloudISOModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tasks, diags := readNoCloudISOTasks(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Reconcile retained accepted tasks before deciding absence: a file may be
	// transiently absent while an accepted task still runs, so a refresh must
	// not orphan it. Unknown task states are retryable errors, never absence.
	tasksChanged := false
	if tasks.UploadTaskUPID != "" {
		// The retained upload task must be polled on its actual owner, which
		// the upload endpoint's UPID encodes and which can differ from the
		// destination node.
		owner, err := taskOwnerNode(tasks.UploadTaskUPID)
		if err != nil {
			resp.Diagnostics.AddError("Unable to Read Proxmox NoCloud ISO Upload Task", err.Error())
			return
		}
		finished, err := r.taskFinished(ctx, owner, tasks.UploadTaskUPID)
		if err != nil {
			resp.Diagnostics.AddError("Unable to Read Proxmox NoCloud ISO Upload Task", err.Error())
			return
		}
		if !finished {
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
		tasks.UploadTaskUPID = ""
		tasksChanged = true
	}
	if tasks.DeleteTaskUPID != "" {
		owner, err := taskOwnerNode(tasks.DeleteTaskUPID)
		if err != nil {
			resp.Diagnostics.AddError("Unable to Read Proxmox NoCloud ISO Delete Task", err.Error())
			return
		}
		finished, err := r.taskFinished(ctx, owner, tasks.DeleteTaskUPID)
		if err != nil {
			resp.Diagnostics.AddError("Unable to Read Proxmox NoCloud ISO Delete Task", err.Error())
			return
		}
		if !finished {
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
		tasks.DeleteTaskUPID = ""
		tasksChanged = true
	}

	exists, err := isoVolumeExists(ctx, r.client, state.Node.ValueString(), state.Storage.ValueString(), state.VolumeID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Storage Content", err.Error())
		return
	}
	if !exists {
		resp.State.RemoveResource(ctx)
		return
	}
	if tasksChanged {
		resp.Diagnostics.Append(writeNoCloudISOTasks(ctx, resp.Private, tasks)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NoCloudISOResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state noCloudISOModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	exists, diags := r.existsInState(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !exists {
		resp.Diagnostics.AddError(
			"Proxmox NoCloud ISO No Longer Exists",
			fmt.Sprintf("NoCloud ISO %q no longer exists on storage %q of node %q.", state.VolumeID.ValueString(), state.Storage.ValueString(), state.Node.ValueString()),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NoCloudISOResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state noCloudISOModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	node := state.Node.ValueString()
	storage := state.Storage.ValueString()
	volumeID := state.VolumeID.ValueString()

	tasks, diags := readNoCloudISOTasks(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// An accepted upload may still be writing the managed file; cleanup must
	// not race it, and the existence check below only decides after it
	// settles. Terminal failed uploads permit the retryable cleanup. Both
	// retained tasks are polled on the owner node their UPID encodes, which
	// can differ from the destination node.
	if tasks.UploadTaskUPID != "" {
		owner, err := taskOwnerNode(tasks.UploadTaskUPID)
		if err != nil {
			resp.Diagnostics.AddError("Unable to Read Proxmox NoCloud ISO Upload Task", err.Error())
			return
		}
		finished, err := r.taskFinished(ctx, owner, tasks.UploadTaskUPID)
		if err != nil {
			resp.Diagnostics.AddError("Unable to Read Proxmox NoCloud ISO Upload Task", err.Error())
			return
		}
		if !finished {
			if err := r.client.waitForNodeTask(ctx, owner, tasks.UploadTaskUPID); err != nil {
				resp.Diagnostics.AddError("Unable to Await Proxmox NoCloud ISO Upload Task", err.Error())
				return
			}
		}
		tasks.UploadTaskUPID = ""
		resp.Diagnostics.Append(writeNoCloudISOTasks(ctx, resp.Private, tasks)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// A previous delete attempt may have accepted a task and then failed while
	// waiting. A retained task that already stopped (successfully or not) must
	// not block the retry; the existence check below decides what remains.
	if tasks.DeleteTaskUPID != "" {
		owner, err := taskOwnerNode(tasks.DeleteTaskUPID)
		if err != nil {
			resp.Diagnostics.AddError("Unable to Read Proxmox NoCloud ISO Delete Task", err.Error())
			return
		}
		finished, err := r.taskFinished(ctx, owner, tasks.DeleteTaskUPID)
		if err != nil {
			resp.Diagnostics.AddError("Unable to Read Proxmox NoCloud ISO Delete Task", err.Error())
			return
		}
		if !finished {
			if err := r.client.waitForNodeTask(ctx, owner, tasks.DeleteTaskUPID); err != nil {
				resp.Diagnostics.AddError("Unable to Delete Proxmox NoCloud ISO", err.Error())
				return
			}
		}
		tasks.DeleteTaskUPID = ""
		resp.Diagnostics.Append(writeNoCloudISOTasks(ctx, resp.Private, tasks)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	exists, err := isoVolumeExists(ctx, r.client, node, storage, volumeID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Storage Content", err.Error())
		return
	}
	if !exists {
		return
	}

	upid, err := r.client.deleteStorageFile(ctx, node, storage, volumeID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			// The volume is already gone; repeated destroys stay idempotent.
			return
		}
		resp.Diagnostics.AddError("Unable to Delete Proxmox NoCloud ISO", err.Error())
		return
	}

	// The no-delay content DELETE is asynchronous and must acknowledge with a
	// task UPID: PVE only returns null from this endpoint when the caller
	// supplies a `delay` parameter, which this client never sends. An empty or
	// malformed acknowledgement therefore leaves the deletion unverified, so
	// failing here keeps the resource in state for a retry instead of
	// reporting success while the media may still exist.
	taskNode, err := taskOwnerNode(upid)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox NoCloud ISO", err.Error())
		return
	}

	// Retain the accepted delete task before waiting so a failed or cancelled
	// wait keeps enough state for the retry to reconcile instead of issuing a
	// duplicate delete against a busy storage.
	tasks.DeleteTaskUPID = upid
	resp.Diagnostics.Append(writeNoCloudISOTasks(ctx, resp.Private, tasks)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.waitForNodeTask(ctx, taskNode, upid); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox NoCloud ISO", err.Error())
		return
	}
}

// taskFinished reports whether a retained task reached a terminal state.
// Only the known running/stopped states classify; unknown or missing status
// values are retryable errors, and a poll or transport failure is likewise an
// unknown condition, never success.
func (r *NoCloudISOResource) taskFinished(ctx context.Context, node, upid string) (bool, error) {
	status, err := r.client.GetNodeTaskStatus(ctx, node, upid)
	if err != nil {
		return false, fmt.Errorf("unable to poll retained task %q: %w", upid, err)
	}
	finished, err := nodeTaskFinished(status, upid)
	if err != nil {
		return false, err
	}
	return finished, nil
}

func readNoCloudISOTasks(ctx context.Context, private privateStateReader) (noCloudISOTasks, diag.Diagnostics) {
	var diags diag.Diagnostics
	var tasks noCloudISOTasks
	data, privateDiags := private.GetKey(ctx, noCloudISOTasksKey)
	diags.Append(privateDiags...)
	if diags.HasError() || len(data) == 0 {
		return tasks, diags
	}
	if err := json.Unmarshal(data, &tasks); err != nil {
		diags.AddError("Unable to Read Proxmox NoCloud ISO State", fmt.Sprintf("unable to decode retained task state: %v", err))
	}
	return tasks, diags
}

func writeNoCloudISOTasks(ctx context.Context, private privateStateWriter, tasks noCloudISOTasks) diag.Diagnostics {
	encoded, err := json.Marshal(tasks)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Unable to Store Proxmox NoCloud ISO State", fmt.Sprintf("unable to encode retained task state: %v", err))
		return diags
	}
	return private.SetKey(ctx, noCloudISOTasksKey, encoded)
}

// ensureISOStorage proves that the node sees the storage, and that it is
// enabled, active (accessible), and supports iso content. Both the NoCloud
// upload and the QEMU VM CD-ROM attachment must verify this on the node that
// will actually use the storage.
func ensureISOStorage(ctx context.Context, client *Client, node, storage string) error {
	storages, err := client.NodeStorages(ctx, node)
	if err != nil {
		return fmt.Errorf("unable to read storage status of node %q: %w", node, err)
	}

	index := slices.IndexFunc(storages, func(entry NodeStorage) bool { return entry.Storage == storage })
	if index < 0 {
		return fmt.Errorf("storage %q is not visible on node %q", storage, node)
	}
	entry := storages[index]

	if enabled := entry.Enabled.Ptr(); enabled == nil || !*enabled {
		return fmt.Errorf("storage %q on node %q is disabled", storage, node)
	}
	if active := entry.Active.Ptr(); active == nil || !*active {
		return fmt.Errorf("storage %q on node %q is not active", storage, node)
	}
	if !slices.Contains(splitProxmoxList(entry.Content), "iso") {
		return fmt.Errorf("storage %q on node %q does not support iso content (supports %q)", storage, node, entry.Content)
	}
	return nil
}

// existsInState proves the managed volume still exists through an
// authoritative collection read. The write-only content inputs are preserved
// from the prior state; they cannot be read back from Proxmox.
func (r *NoCloudISOResource) existsInState(ctx context.Context, state *noCloudISOModel) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	exists, err := isoVolumeExists(ctx, r.client, state.Node.ValueString(), state.Storage.ValueString(), state.VolumeID.ValueString())
	if err != nil {
		diags.AddError("Unable to Read Proxmox Storage Content", err.Error())
		return false, diags
	}
	if !exists {
		state.ID = types.StringNull()
		return false, diags
	}

	state.ID = types.StringValue(storageFileDownloadID(state.Node.ValueString(), state.Storage.ValueString(), state.VolumeID.ValueString()))
	return true, diags
}
