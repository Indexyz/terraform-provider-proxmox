// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &NodeTimeResource{}
var _ resource.ResourceWithImportState = &NodeTimeResource{}
var _ resource.Resource = &NodeDNSResource{}
var _ resource.ResourceWithImportState = &NodeDNSResource{}
var _ resource.ResourceWithValidateConfig = &NodeTimeResource{}

type NodeTimeResource struct{ client *Client }
type NodeDNSResource struct{ client *Client }

type nodeTimeModel struct {
	ID        types.String `tfsdk:"id"`
	Node      types.String `tfsdk:"node"`
	Timezone  types.String `tfsdk:"timezone"`
	Time      types.Int64  `tfsdk:"time"`
	Localtime types.Int64  `tfsdk:"localtime"`
}

type nodeDNSModel struct {
	ID     types.String `tfsdk:"id"`
	Node   types.String `tfsdk:"node"`
	DNS1   types.String `tfsdk:"dns1"`
	DNS2   types.String `tfsdk:"dns2"`
	DNS3   types.String `tfsdk:"dns3"`
	Search types.String `tfsdk:"search"`
}

func NewNodeTimeResource() resource.Resource { return &NodeTimeResource{} }
func NewNodeDNSResource() resource.Resource  { return &NodeDNSResource{} }

func (r *NodeTimeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_time"
}

func (r *NodeDNSResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_dns"
}

func (r *NodeTimeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Sets the node timezone through `PUT /nodes/{node}/time`. Import as the node name; destroy keeps the last timezone.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true, MarkdownDescription: "Node name."},
			"node":      schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"timezone":  schema.StringAttribute{Required: true, MarkdownDescription: "IANA timezone such as `Europe/Berlin`."},
			"time":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Current epoch time reported by the node."},
			"localtime": schema.Int64Attribute{Computed: true, MarkdownDescription: "Node-local epoch time."},
		},
	}
}

func (r *NodeDNSResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Sets the node DNS servers and search domain through `PUT /nodes/{node}/dns`. Import as the node name; destroy keeps the last values.",
		Attributes: map[string]schema.Attribute{
			"id":     schema.StringAttribute{Computed: true, MarkdownDescription: "Node name."},
			"node":   schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"dns1":   schema.StringAttribute{Optional: true, MarkdownDescription: "Primary DNS server."},
			"dns2":   schema.StringAttribute{Optional: true, MarkdownDescription: "Secondary DNS server."},
			"dns3":   schema.StringAttribute{Optional: true, MarkdownDescription: "Tertiary DNS server."},
			"search": schema.StringAttribute{Required: true, MarkdownDescription: "Search domain."},
		},
	}
}

func (r *NodeTimeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *NodeDNSResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *NodeTimeResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeTimeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateNodeNameResource(config.Node, resp)
	if isConfiguredValue(config.Timezone) && config.Timezone.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(path.Root("timezone"), "Missing timezone", "timezone must not be empty.")
	}
}

func (r *NodeTimeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeTimeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeTimeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nodeTimeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.refresh(ctx, state.Node.ValueString(), &resp.State, &resp.Diagnostics, nil)
}

func (r *NodeTimeResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *NodeTimeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), req.ID)...)
}

func (r *NodeTimeResource) apply(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan nodeTimeModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	node := plan.Node.ValueString()
	if err := r.client.SetNodeTimezone(ctx, node, plan.Timezone.ValueString()); err != nil {
		diags.AddError("Unable to Set Proxmox Node Timezone", err.Error())
		return
	}
	next := plan
	r.refresh(ctx, node, state, diags, &next)
}

func (r *NodeTimeResource) refresh(ctx context.Context, node string, state *tfsdk.State, diags *diag.Diagnostics, next *nodeTimeModel) {
	timeInfo, err := r.client.NodeTime(ctx, node)
	if err != nil {
		diags.AddError("Unable to Read Proxmox Node Time", err.Error())
		return
	}
	if next == nil {
		next = &nodeTimeModel{Node: types.StringValue(node)}
	}
	next.ID = types.StringValue(node)
	next.Timezone = types.StringValue(timeInfo.Timezone)
	next.Time = types.Int64Value(timeInfo.Time)
	next.Localtime = types.Int64Value(timeInfo.LocalTime)
	diags.Append(state.Set(ctx, next)...)
}

func (r *NodeDNSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeDNSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeDNSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nodeDNSModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.refresh(ctx, state.Node.ValueString(), &resp.State, &resp.Diagnostics)
}

func (r *NodeDNSResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *NodeDNSResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), req.ID)...)
}

func (r *NodeDNSResource) apply(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan nodeDNSModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	node := plan.Node.ValueString()
	err := r.client.SetNodeDNS(ctx, node, NodeDNS{
		DNS1: stringValue(plan.DNS1), DNS2: stringValue(plan.DNS2), DNS3: stringValue(plan.DNS3), Search: stringValue(plan.Search),
	})
	if err != nil {
		diags.AddError("Unable to Set Proxmox Node DNS", err.Error())
		return
	}
	plan.ID = types.StringValue(node)
	r.refresh(ctx, node, state, diags)
}

func (r *NodeDNSResource) refresh(ctx context.Context, node string, state *tfsdk.State, diags *diag.Diagnostics) {
	dns, err := r.client.NodeDNS(ctx, node)
	if err != nil {
		diags.AddError("Unable to Read Proxmox Node DNS", err.Error())
		return
	}
	next := nodeDNSModel{
		ID: types.StringValue(node), Node: types.StringValue(node), DNS1: types.StringValue(dns.DNS1),
		DNS2: stringOrNull(dns.DNS2), DNS3: stringOrNull(dns.DNS3), Search: stringOrNull(dns.Search),
	}
	diags.Append(state.Set(ctx, &next)...)
}
