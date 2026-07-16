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

var _ resource.Resource = &HAResourceResource{}
var _ resource.ResourceWithImportState = &HAResourceResource{}
var _ resource.ResourceWithValidateConfig = &HAResourceResource{}

const haResourceManagedFieldsKey = "ha-resource-managed-fields"

var haResourceIDPattern = regexp.MustCompile(`^(vm|ct):[1-9][0-9]+$`)

type HAResourceResource struct {
	client *Client
}

type haResourcePrivateData interface {
	GetKey(context.Context, string) ([]byte, diag.Diagnostics)
	SetKey(context.Context, string, []byte) diag.Diagnostics
}

type haResourceModel struct {
	ID            types.String `tfsdk:"id"`
	ResourceID    types.String `tfsdk:"resource_id"`
	State         types.String `tfsdk:"state"`
	Comment       types.String `tfsdk:"comment"`
	Failback      types.Bool   `tfsdk:"failback"`
	AutoRebalance types.Bool   `tfsdk:"auto_rebalance"`
	MaxRestart    types.Int64  `tfsdk:"max_restart"`
	MaxRelocate   types.Int64  `tfsdk:"max_relocate"`
}

func NewHAResourceResource() resource.Resource {
	return &HAResourceResource{}
}

func (r *HAResourceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ha_resource"
}

func (r *HAResourceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Proxmox VE 9 high-availability enrollment and requested policy for an existing QEMU VM or LXC container through `/cluster/ha/resources`. Destroy sends `purge=0` and removes only HA management; it never deletes or stops the guest. Runtime placement, migration, recovery, and failover are not Terraform convergence targets.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "HA resource identifier (same as `resource_id`).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"resource_id":    schema.StringAttribute{Required: true, MarkdownDescription: "Canonical HA resource identifier in `vm:<vmid>` or `ct:<vmid>` form. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"state":          schema.StringAttribute{Required: true, MarkdownDescription: "Requested HA state: `started`, `stopped`, `disabled`, or `ignored`. This is explicit because changing it can cause Proxmox HA to start, stop, recover, or stop tracking the guest."},
			"comment":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "HA resource description."},
			"failback":       schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Allow Proxmox HA to migrate the resource toward a higher-priority node when it returns. The Proxmox default is `true`."},
			"auto_rebalance": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Allow PVE 9.2 dynamic CRS to migrate this resource during automatic rebalancing. The Proxmox default is `true`."},
			"max_restart":    schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Maximum local restart attempts before Proxmox HA tries relocation. The Proxmox default is `1`."},
			"max_relocate":   schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Maximum relocation attempts after a failed start. The Proxmox default is `1`."},
		},
	}
}

func (r *HAResourceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *HAResourceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config haResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateHAResourceConfig(config)...)
}

func validateHAResourceConfig(config haResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if !config.ResourceID.IsNull() && !config.ResourceID.IsUnknown() && !haResourceIDPattern.MatchString(config.ResourceID.ValueString()) {
		diags.AddAttributeError(path.Root("resource_id"), "Invalid HA resource identifier", "resource_id must use canonical vm:<vmid> or ct:<vmid> form, for example vm:120")
	}
	if !config.State.IsNull() && !config.State.IsUnknown() && !slices.Contains([]string{"started", "stopped", "disabled", "ignored"}, config.State.ValueString()) {
		diags.AddAttributeError(path.Root("state"), "Invalid HA resource state", "state must be started, stopped, disabled, or ignored")
	}
	if !config.Comment.IsNull() && !config.Comment.IsUnknown() && len(config.Comment.ValueString()) > 4096 {
		diags.AddAttributeError(path.Root("comment"), "Invalid HA resource comment", "comment must not exceed 4096 characters")
	}
	if !config.MaxRestart.IsNull() && !config.MaxRestart.IsUnknown() && config.MaxRestart.ValueInt64() < 0 {
		diags.AddAttributeError(path.Root("max_restart"), "Invalid HA restart limit", "max_restart must be at least 0")
	}
	if !config.MaxRelocate.IsNull() && !config.MaxRelocate.IsUnknown() && config.MaxRelocate.ValueInt64() < 0 {
		diags.AddAttributeError(path.Root("max_relocate"), "Invalid HA relocation limit", "max_relocate must be at least 0")
	}
	return diags
}

func haResourceRequestFromModels(config, plan haResourceModel) HAResourceRequest {
	req := HAResourceRequest{State: plan.State.ValueString()}
	if !config.Comment.IsNull() && !config.Comment.IsUnknown() {
		req.Comment = stringPointer(plan.Comment)
	}
	if !config.Failback.IsNull() && !config.Failback.IsUnknown() {
		req.Failback = boolPointerValue(plan.Failback)
	}
	if !config.AutoRebalance.IsNull() && !config.AutoRebalance.IsUnknown() {
		req.AutoRebalance = boolPointerValue(plan.AutoRebalance)
	}
	if !config.MaxRestart.IsNull() && !config.MaxRestart.IsUnknown() {
		req.MaxRestart = int64PointerValue(plan.MaxRestart)
	}
	if !config.MaxRelocate.IsNull() && !config.MaxRelocate.IsUnknown() {
		req.MaxRelocate = int64PointerValue(plan.MaxRelocate)
	}
	return req
}

func haResourceManagedFields(config haResourceModel) []string {
	var fields []string
	if !config.Comment.IsNull() && !config.Comment.IsUnknown() {
		fields = append(fields, "comment")
	}
	if !config.Failback.IsNull() && !config.Failback.IsUnknown() {
		fields = append(fields, "failback")
	}
	if !config.AutoRebalance.IsNull() && !config.AutoRebalance.IsUnknown() {
		fields = append(fields, "auto-rebalance")
	}
	if !config.MaxRestart.IsNull() && !config.MaxRestart.IsUnknown() {
		fields = append(fields, "max_restart")
	}
	if !config.MaxRelocate.IsNull() && !config.MaxRelocate.IsUnknown() {
		fields = append(fields, "max_relocate")
	}
	slices.Sort(fields)
	return fields
}

func haResourceDeleteKeys(config haResourceModel, previouslyManaged []string) []string {
	current := haResourceManagedFields(config)
	var deleted []string
	for _, key := range previouslyManaged {
		if !slices.Contains(current, key) {
			deleted = append(deleted, key)
		}
	}
	slices.Sort(deleted)
	return deleted
}

func (r *HAResourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config haResourceModel
	var plan haResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.CreateHAResource(ctx, plan.ResourceID.ValueString(), haResourceRequestFromModels(config, plan)); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox HA Resource", err.Error())
		return
	}
	state, diags := r.readState(ctx, plan.ResourceID.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *HAResourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state haResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.ResourceID.ValueString())
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

func (r *HAResourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config haResourceModel
	var plan haResourceModel
	var prior haResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManaged, diags := readHAResourceManagedFields(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetHAResource(ctx, prior.ResourceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Current Proxmox HA Resource", err.Error())
		return
	}
	updateReq := haResourceRequestFromModels(config, plan)
	updateReq.Delete = haResourceDeleteKeys(config, previouslyManaged)
	if current.Digest != "" {
		updateReq.Digest = &current.Digest
	}
	if err := r.client.UpdateHAResource(ctx, prior.ResourceID.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox HA Resource", err.Error())
		return
	}
	state, stateDiags := r.readState(ctx, prior.ResourceID.ValueString())
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *HAResourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state haResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteHAResource(ctx, state.ResourceID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox HA Resource", err.Error())
	}
}

func (r *HAResourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !haResourceIDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected a canonical Proxmox HA resource identifier in vm:<vmid> or ct:<vmid> form.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("resource_id"), req.ID)...)
}

func (r *HAResourceResource) readState(ctx context.Context, sid string) (haResourceModel, diag.Diagnostics) {
	resource, err := r.client.GetHAResource(ctx, sid)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return haResourceModel{ID: types.StringNull()}, nil
		}
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox HA Resource", fmt.Sprintf("Unable to read HA resource %q: %s", sid, err))
		return haResourceModel{}, diags
	}
	return haResourceStateFromAPI(resource), nil
}

func haResourceStateFromAPI(resource HAResource) haResourceModel {
	state := resource.State
	if state == "" || state == "enabled" {
		state = "started"
	}
	failback := true
	if resource.Failback.Ptr() != nil {
		failback = *resource.Failback.Ptr()
	}
	autoRebalance := true
	if resource.AutoRebalance.Ptr() != nil {
		autoRebalance = *resource.AutoRebalance.Ptr()
	}
	maxRestart := int64(1)
	if resource.MaxRestart.Ptr() != nil {
		maxRestart = *resource.MaxRestart.Ptr()
	}
	maxRelocate := int64(1)
	if resource.MaxRelocate.Ptr() != nil {
		maxRelocate = *resource.MaxRelocate.Ptr()
	}
	return haResourceModel{
		ID:            types.StringValue(resource.SID),
		ResourceID:    types.StringValue(resource.SID),
		State:         types.StringValue(state),
		Comment:       stringOrNull(resource.Comment),
		Failback:      types.BoolValue(failback),
		AutoRebalance: types.BoolValue(autoRebalance),
		MaxRestart:    types.Int64Value(maxRestart),
		MaxRelocate:   types.Int64Value(maxRelocate),
	}
}

func (r *HAResourceResource) storeManagedFields(ctx context.Context, config haResourceModel, private haResourcePrivateData, diags *diag.Diagnostics) {
	managedFields, err := json.Marshal(haResourceManagedFields(config))
	if err != nil {
		diags.AddError("Unable to Store HA Resource State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, haResourceManagedFieldsKey, managedFields)...)
}

func readHAResourceManagedFields(ctx context.Context, private haResourcePrivateData) ([]string, diag.Diagnostics) {
	value, diags := private.GetKey(ctx, haResourceManagedFieldsKey)
	if diags.HasError() || len(value) == 0 {
		return nil, diags
	}
	var fields []string
	if err := json.Unmarshal(value, &fields); err != nil {
		diags.AddError("Unable to Read HA Resource State", fmt.Sprintf("unable to decode managed fields: %v", err))
	}
	return fields, diags
}
