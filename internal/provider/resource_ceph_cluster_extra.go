// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource = &CephHealthMuteResource{}
	_ resource.Resource = &ClusterCephRestartBulkResource{}
)

type CephHealthMuteResource struct{ client *Client }
type ClusterCephRestartBulkResource struct{ client *Client }

type cephHealthMuteModel struct {
	ID     types.String `tfsdk:"id"`
	Code   types.String `tfsdk:"code"`
	Value  types.Bool   `tfsdk:"value"`
	TTL    types.String `tfsdk:"ttl"`
	Sticky types.Bool   `tfsdk:"sticky"`
}

type clusterCephRestartBulkModel struct {
	ID           types.String `tfsdk:"id"`
	ServiceType  types.String `tfsdk:"service_type"`
	Force        types.Bool   `tfsdk:"force"`
	OnlyOutdated types.Bool   `tfsdk:"only_outdated"`
	Timeout      types.Int64  `tfsdk:"timeout"`
	UPID         types.String `tfsdk:"upid"`
	Triggers     types.Map    `tfsdk:"triggers"`
}

func NewCephHealthMuteResource() resource.Resource         { return &CephHealthMuteResource{} }
func NewClusterCephRestartBulkResource() resource.Resource { return &ClusterCephRestartBulkResource{} }

func (r *CephHealthMuteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_ceph_health_mute"
}
func (r *ClusterCephRestartBulkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_ceph_restart_bulk"
}

func (r *CephHealthMuteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Mutes one Ceph health check through `PUT /cluster/ceph/health-mute/{code}`. Destroy re-enables the check (value `false`). PVE 9.2 returns 501 for this path.", Attributes: map[string]schema.Attribute{
		"id":     schema.StringAttribute{Computed: true},
		"code":   schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"value":  schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
		"ttl":    schema.StringAttribute{Optional: true, Computed: true},
		"sticky": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
	}}
}

func (r *ClusterCephRestartBulkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Restarts outdated Ceph daemons cluster-wide through `POST /cluster/ceph/restart-bulk`. PVE 9.2 returns 501 for this path.", Attributes: map[string]schema.Attribute{
		"id":            schema.StringAttribute{Computed: true},
		"service_type":  schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"force":         schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"only_outdated": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"timeout":       schema.Int64Attribute{Optional: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"upid":          schema.StringAttribute{Computed: true},
		"triggers":      schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifierRequiresReplace()}},
	}}
}

func (r *CephHealthMuteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *ClusterCephRestartBulkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *CephHealthMuteResource) refresh(ctx context.Context, code string, state *tfsdk.State, diags *diag.Diagnostics) {
	items, err := r.client.CephHealthMutes(ctx)
	if err != nil {
		diags.AddError("Unable to Read Ceph Health Mutes", err.Error())
		return
	}
	for _, item := range items {
		if item.Code != code {
			continue
		}
		diags.Append(state.Set(ctx, &cephHealthMuteModel{ID: types.StringValue(code), Code: types.StringValue(code), Value: types.BoolValue(true), TTL: stringOrNull(item.TTL), Sticky: boolOrNull(item.Sticky.Ptr())})...)
		return
	}
	state.RemoveResource(ctx)
}

func (r *CephHealthMuteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cephHealthMuteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.SetCephHealthMute(ctx, plan.Code.ValueString(), plan.Value.ValueBool(), stringValue(plan.TTL), boolInt64(plan.Sticky.ValueBool())); err != nil {
		resp.Diagnostics.AddError("Unable to Mute Ceph Health Check", err.Error())
		return
	}
	r.refresh(ctx, plan.Code.ValueString(), &resp.State, &resp.Diagnostics)
}
func (r *CephHealthMuteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cephHealthMuteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.refresh(ctx, state.Code.ValueString(), &resp.State, &resp.Diagnostics)
}
func (r *CephHealthMuteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan cephHealthMuteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.SetCephHealthMute(ctx, plan.Code.ValueString(), plan.Value.ValueBool(), stringValue(plan.TTL), boolInt64(plan.Sticky.ValueBool())); err != nil {
		resp.Diagnostics.AddError("Unable to Update Ceph Health Mute", err.Error())
		return
	}
	r.refresh(ctx, plan.Code.ValueString(), &resp.State, &resp.Diagnostics)
}
func (r *CephHealthMuteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cephHealthMuteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.SetCephHealthMute(ctx, state.Code.ValueString(), false, "", 0); err != nil && !isNotFoundError(err) {
		resp.Diagnostics.AddError("Unable to Unmute Ceph Health Check", err.Error())
	}
}

func (r *ClusterCephRestartBulkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterCephRestartBulkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.CephRestartBulkCluster(ctx, stringValue(plan.ServiceType), plan.Force.ValueBool(), plan.OnlyOutdated.ValueBool(), plan.Timeout.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Restart Ceph Services", err.Error())
		return
	}
	if upid != "" && !waitTask(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	plan.ID = types.StringValue("cluster")
	plan.UPID = stringOrNull(upid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *ClusterCephRestartBulkResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *ClusterCephRestartBulkResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *ClusterCephRestartBulkResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func boolInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func (r *CephHealthMuteResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config cephHealthMuteModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.Code.IsUnknown() || config.Code.IsNull() {
		return
	}
	if config.Code.ValueString() == "" {
		resp.Diagnostics.AddError("Missing Health Check Code", "code must not be empty")
	}
}
