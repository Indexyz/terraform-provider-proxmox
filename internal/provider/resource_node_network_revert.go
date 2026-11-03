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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &NodeNetworkRevertResource{}
var _ resource.ResourceWithImportState = &NodeNetworkRevertResource{}
var _ resource.ResourceWithValidateConfig = &NodeNetworkRevertResource{}

type NodeNetworkRevertResource struct {
	client *Client
}

type nodeNetworkRevertModel struct {
	ID             types.String `tfsdk:"id"`
	Node           types.String `tfsdk:"node"`
	Triggers       types.Map    `tfsdk:"triggers"`
	PendingChanges types.List   `tfsdk:"pending_changes"`
}

func NewNodeNetworkRevertResource() resource.Resource {
	return &NodeNetworkRevertResource{}
}

func (r *NodeNetworkRevertResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_network_revert"
}

func (r *NodeNetworkRevertResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Discards the staged network configuration of one Proxmox VE node through `DELETE /nodes/{node}/network`; Proxmox unlinks the staged file synchronously and answers without a worker task. Proxmox reverts the whole node at once, so this drops every staged interface change, not one interface. Destroy is a no-op: an applied configuration stays applied. A failed revert keeps the instance in state for a retry.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true, MarkdownDescription: "Revert identifier (the node name).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"node": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node whose staged configuration is discarded. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"triggers": schema.MapAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Values that decide when another revert is due; changing any of them reverts again. Use something that changes when the staged configuration does, for example `{ staged = sha1(jsonencode(proxmox_node_network_apply.this.pending_changes)) }`.",
				PlanModifiers:       []planmodifier.Map{mapplanmodifier.RequiresReplace()},
			},
			"pending_changes": schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Staged changes Proxmox still reports for the node. Empty after a successful revert."},
		},
	}
}

func (r *NodeNetworkRevertResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *NodeNetworkRevertResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeNetworkRevertModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateNodeNetworkRevert(config)...)
}

func validateNodeNetworkRevert(config nodeNetworkRevertModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.Node) && !proxmoxNodeNamePattern.MatchString(config.Node.ValueString()) {
		diags.AddAttributeError(path.Root("node"), "Invalid node name", "node must be an existing cluster node name.")
	}
	return diags
}

func (r *NodeNetworkRevertResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nodeNetworkRevertModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.revert(ctx, &plan, &resp.Diagnostics, &resp.State)
}

func (r *NodeNetworkRevertResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nodeNetworkRevertModel
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

func (r *NodeNetworkRevertResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan nodeNetworkRevertModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.revert(ctx, &plan, &resp.Diagnostics, &resp.State)
}

func (r *NodeNetworkRevertResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// A revert is never undone, so destroying the record leaves the node alone.
}

func (r *NodeNetworkRevertResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !proxmoxNodeNamePattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected a Proxmox node name as import identifier.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), req.ID)...)
}

// revert discards the staged configuration and persists the plan. Triggers have
// no server-side echo, so the plan is the only source for them.
func (r *NodeNetworkRevertResource) revert(ctx context.Context, plan *nodeNetworkRevertModel, diags *diag.Diagnostics, state *tfsdk.State) {
	node := plan.Node.ValueString()
	upid, err := r.client.RevertNodeNetwork(ctx, node)
	if err != nil {
		diags.AddError("Unable to Revert Proxmox Node Network", err.Error())
		return
	}
	if upid != "" {
		if err := r.client.waitForNodeTask(ctx, node, upid); err != nil {
			diags.AddError("Unable to Revert Proxmox Node Network", fmt.Sprintf("network revert task %q did not finish: %s", upid, err))
			return
		}
	}
	next, readDiags := r.readState(ctx, plan)
	diags.Append(readDiags...)
	if diags.HasError() {
		return
	}
	diags.Append(state.Set(ctx, &next)...)
}

func (r *NodeNetworkRevertResource) readState(ctx context.Context, prior *nodeNetworkRevertModel) (nodeNetworkRevertModel, diag.Diagnostics) {
	_, changes, err := r.client.NodeNetworkInterfaces(ctx, prior.Node.ValueString())
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox Node Network", fmt.Sprintf("Unable to read staged network changes of node %q: %s", prior.Node.ValueString(), err))
		return nodeNetworkRevertModel{}, diags
	}
	state := *prior
	state.ID = types.StringValue(prior.Node.ValueString())
	state.PendingChanges = types.ListNull(types.StringType)
	if len(changes) > 0 {
		list, diags := types.ListValueFrom(ctx, types.StringType, changes)
		state.PendingChanges = list
		return state, diags
	}
	return state, nil
}
