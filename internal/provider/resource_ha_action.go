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

var _ resource.Resource = &HAResourceActionResource{}

type HAResourceActionResource struct{ client *Client }

type haResourceActionModel struct {
	ID       types.String `tfsdk:"id"`
	SID      types.String `tfsdk:"sid"`
	Action   types.String `tfsdk:"action"`
	Node     types.String `tfsdk:"node"`
	Result   types.String `tfsdk:"result"`
	Triggers types.Map    `tfsdk:"triggers"`
}

func NewHAResourceActionResource() resource.Resource { return &HAResourceActionResource{} }

func (r *HAResourceActionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ha_resource_migrate"
}

func (r *HAResourceActionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Triggers an HA resource migration through `POST /cluster/ha/resources/{sid}/migrate` or `/relocate`. `migrate` moves the resource without the recovery mode; `relocate` uses HA relocation. The response lists blocking and co-migrated resources and is exposed as JSON in `result`.", Attributes: map[string]schema.Attribute{
		"id":     schema.StringAttribute{Computed: true},
		"sid":    schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"action": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"node":   schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"result": schema.StringAttribute{Computed: true},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType,
			PlanModifiers: []planmodifier.Map{mapplanmodifierRequiresReplace()}},
	}}
}

func (r *HAResourceActionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *HAResourceActionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan haResourceActionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if plan.Action.ValueString() != "migrate" && plan.Action.ValueString() != "relocate" {
		resp.Diagnostics.AddError("Invalid HA Action", "action must be migrate or relocate")
		return
	}
	r.run(ctx, plan, &resp.State, &resp.Diagnostics)
}

func (r *HAResourceActionResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *HAResourceActionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan haResourceActionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if plan.Action.ValueString() != "migrate" && plan.Action.ValueString() != "relocate" {
		resp.Diagnostics.AddError("Invalid HA Action", "action must be migrate or relocate")
		return
	}
	r.run(ctx, plan, &resp.State, &resp.Diagnostics)
}

func (r *HAResourceActionResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *HAResourceActionResource) run(ctx context.Context, plan haResourceActionModel, state *tfsdk.State, diags *diag.Diagnostics) {
	result, err := r.client.HAResourceAction(ctx, plan.SID.ValueString(), plan.Action.ValueString(), plan.Node.ValueString())
	if err != nil {
		diags.AddError("Unable to "+plan.Action.ValueString()+" HA Resource", err.Error())
		return
	}
	plan.ID = types.StringValue(plan.SID.ValueString() + "/" + plan.Action.ValueString())
	plan.Result = stringOrNull(result)
	diags.Append(state.Set(ctx, &plan)...)
}

func (r *HAResourceActionResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config haResourceActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	action := config.Action
	if action.IsUnknown() || action.IsNull() {
		return
	}
	if action.ValueString() != "migrate" && action.ValueString() != "relocate" {
		resp.Diagnostics.AddError("Invalid HA Action", "action must be migrate or relocate")
	}
}
