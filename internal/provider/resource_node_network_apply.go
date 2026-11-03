// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &NodeNetworkApplyResource{}
var _ resource.ResourceWithImportState = &NodeNetworkApplyResource{}
var _ resource.ResourceWithValidateConfig = &NodeNetworkApplyResource{}

type NodeNetworkApplyResource struct {
	client *Client
}

type nodeNetworkApplyModel struct {
	ID             types.String `tfsdk:"id"`
	Node           types.String `tfsdk:"node"`
	Triggers       types.Map    `tfsdk:"triggers"`
	RegenerateFRR  types.Bool   `tfsdk:"regenerate_frr"`
	PendingChanges types.List   `tfsdk:"pending_changes"`
}

func NewNodeNetworkApplyResource() resource.Resource {
	return &NodeNetworkApplyResource{}
}

func (r *NodeNetworkApplyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_network_apply"
}

func (r *NodeNetworkApplyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Applies the staged network configuration of one Proxmox VE node through `PUT /nodes/{node}/network` and waits for the reload task. `proxmox_node_network_interface` only stages changes, so this resource is the explicit activation step for a whole node. Destroy is a no-op: an applied configuration stays applied. A failed reload keeps the instance in state for a retry.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true, MarkdownDescription: "Apply identifier (the node name).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"node": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node whose staged configuration is applied. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"triggers": schema.MapAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Arbitrary values that decide when a reload is due; changing any of them re-applies. Use a hash of the managed interfaces, for example `{ interfaces = sha1(jsonencode([for i in proxmox_node_network_interface.this : [i.id, i.address, i.bridge_ports]])) }`.",
				PlanModifiers:       []planmodifier.Map{mapplanmodifier.RequiresReplace()},
			},
			"regenerate_frr":  schema.BoolAttribute{Optional: true, MarkdownDescription: "Regenerate the FRR configuration during the reload. Proxmox only does so when this is set, which is required for SDN/FRR setups.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"pending_changes": schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Staged but not yet applied changes Proxmox currently reports for the node. Empty after a successful apply."},
		},
	}
}

func (r *NodeNetworkApplyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *NodeNetworkApplyResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeNetworkApplyModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateNodeNetworkApply(config)...)
}

func validateNodeNetworkApply(config nodeNetworkApplyModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if !config.Node.IsNull() && !config.Node.IsUnknown() && !proxmoxNodeNamePattern.MatchString(config.Node.ValueString()) {
		diags.AddAttributeError(path.Root("node"), "Invalid node name", "node must be an existing cluster node name.")
	}
	return diags
}

func (r *NodeNetworkApplyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nodeNetworkApplyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.Diagnostics, &resp.State)
}

func (r *NodeNetworkApplyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nodeNetworkApplyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *NodeNetworkApplyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan nodeNetworkApplyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.Diagnostics, &resp.State)
}

func (r *NodeNetworkApplyResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// A reload is never undone, so destroying the apply record leaves the applied
	// configuration in place.
}

func (r *NodeNetworkApplyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !proxmoxNodeNamePattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected a Proxmox node name as import identifier.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), req.ID)...)
}

// apply reloads the node and persists the plan together with the remaining
// pending changes. Triggers and regenerate_frr have no server-side echo, so the
// plan values stay in state.
func (r *NodeNetworkApplyResource) apply(ctx context.Context, plan *nodeNetworkApplyModel, diags *diag.Diagnostics, state *tfsdk.State) {
	node := plan.Node.ValueString()
	upid, err := r.client.ApplyNodeNetwork(ctx, node, boolPointerValue(plan.RegenerateFRR))
	if err != nil {
		diags.AddError("Unable to Apply Proxmox Node Network", err.Error())
		return
	}
	if err := r.client.waitForNodeTask(ctx, node, upid); err != nil {
		diags.AddError("Unable to Apply Proxmox Node Network", fmt.Sprintf("network reload task %q did not finish: %s", upid, err))
		return
	}
	refreshed, readDiags := r.readState(ctx, plan)
	diags.Append(readDiags...)
	if diags.HasError() {
		return
	}
	diags.Append(state.Set(ctx, &refreshed)...)
}

func (r *NodeNetworkApplyResource) readState(ctx context.Context, prior *nodeNetworkApplyModel) (nodeNetworkApplyModel, diag.Diagnostics) {
	_, changes, err := r.client.NodeNetworkInterfaces(ctx, prior.Node.ValueString())
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox Node Network", fmt.Sprintf("Unable to read staged network changes of node %q: %s", prior.Node.ValueString(), err))
		return nodeNetworkApplyModel{}, diags
	}
	state := *prior
	state.ID = types.StringValue(prior.Node.ValueString())
	state.PendingChanges = types.ListNull(types.StringType)
	if len(changes) > 0 {
		pending, listDiags := types.ListValueFrom(ctx, types.StringType, changes)
		if listDiags.HasError() {
			return nodeNetworkApplyModel{}, listDiags
		}
		state.PendingChanges = pending
	}
	return state, nil
}
