// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource = &CephServiceStartResource{}
	_ resource.Resource = &CephServiceStopResource{}
)

type CephServiceStartResource struct{ client *Client }
type CephServiceStopResource struct{ client *Client }

type cephServiceModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	Service  types.String `tfsdk:"service"`
	UPID     types.String `tfsdk:"upid"`
	Triggers types.Map    `tfsdk:"triggers"`
}

func NewCephServiceStartResource() resource.Resource { return &CephServiceStartResource{} }
func NewCephServiceStopResource() resource.Resource  { return &CephServiceStopResource{} }

func (r *CephServiceStartResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_start"
}
func (r *CephServiceStopResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_stop"
}

func cephServiceSchema(action string) schema.Schema {
	return schema.Schema{MarkdownDescription: "Starts a Ceph service through `POST /nodes/{node}/ceph/" + action + "`.", Attributes: map[string]schema.Attribute{
		"id":      schema.StringAttribute{Computed: true},
		"node":    schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"service": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"upid":    schema.StringAttribute{Computed: true},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType,
			PlanModifiers: []planmodifier.Map{mapplanmodifierRequiresReplace()}},
	}}
}

func (r *CephServiceStartResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = cephServiceSchema("start")
}
func (r *CephServiceStopResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = cephServiceSchema("stop")
}

func (r *CephServiceStartResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *CephServiceStopResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *CephServiceStartResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cephServiceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.run(ctx, plan, "start", &resp.State, &resp.Diagnostics)
}
func (r *CephServiceStartResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *CephServiceStartResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *CephServiceStartResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *CephServiceStopResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cephServiceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.run(ctx, plan, "stop", &resp.State, &resp.Diagnostics)
}
func (r *CephServiceStopResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *CephServiceStopResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *CephServiceStopResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *CephServiceStartResource) run(ctx context.Context, plan cephServiceModel, action string, state *tfsdk.State, diags *diag.Diagnostics) {
	upid, err := r.client.CephServiceAction(ctx, plan.Node.ValueString(), plan.Service.ValueString(), action)
	if err != nil {
		diags.AddError("Unable to "+action+" Ceph Service", err.Error())
		return
	}
	if upid != "" && !waitTask(ctx, r.client, upid, diags) {
		return
	}
	plan.ID = types.StringValue(plan.Node.ValueString() + "/" + plan.Service.ValueString())
	plan.UPID = stringOrNull(upid)
	diags.Append(state.Set(ctx, &plan)...)
}
func (r *CephServiceStopResource) run(ctx context.Context, plan cephServiceModel, action string, state *tfsdk.State, diags *diag.Diagnostics) {
	upid, err := r.client.CephServiceAction(ctx, plan.Node.ValueString(), plan.Service.ValueString(), action)
	if err != nil {
		diags.AddError("Unable to "+action+" Ceph Service", err.Error())
		return
	}
	if upid != "" && !waitTask(ctx, r.client, upid, diags) {
		return
	}
	plan.ID = types.StringValue(plan.Node.ValueString() + "/" + plan.Service.ValueString())
	plan.UPID = stringOrNull(upid)
	diags.Append(state.Set(ctx, &plan)...)
}
