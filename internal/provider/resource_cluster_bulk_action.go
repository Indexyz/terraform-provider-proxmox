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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &ClusterBulkActionResource{}
var _ resource.ResourceWithValidateConfig = &ClusterBulkActionResource{}
var _ resource.Resource = &NotificationTargetTestResource{}

type ClusterBulkActionResource struct{ client *Client }
type NotificationTargetTestResource struct{ client *Client }

type clusterBulkActionModel struct {
	ID             types.String `tfsdk:"id"`
	Action         types.String `tfsdk:"action"`
	VMs            types.Set    `tfsdk:"vms"`
	Target         types.String `tfsdk:"target"`
	Online         types.Bool   `tfsdk:"online"`
	WithLocalDisks types.Bool   `tfsdk:"with_local_disks"`
	MaxWorkers     types.Int64  `tfsdk:"max_workers"`
	Triggers       types.Map    `tfsdk:"triggers"`
	UPID           types.String `tfsdk:"upid"`
}

type notificationTargetTestModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Triggers types.Map    `tfsdk:"triggers"`
}

func NewClusterBulkActionResource() resource.Resource      { return &ClusterBulkActionResource{} }
func NewNotificationTargetTestResource() resource.Resource { return &NotificationTargetTestResource{} }

func (r *ClusterBulkActionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_bulk_action"
}

func (r *NotificationTargetTestResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_target_test"
}

func (r *ClusterBulkActionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Runs a cluster-wide bulk action (`start`, `shutdown`, `suspend`, `migrate`) on a set of guests through `POST /cluster/bulk-action/guest/{action}` and waits for the bulk worker. Every change replaces the instance, destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<action>/<vmid,>` form."},
			"action":           schema.StringAttribute{Required: true, MarkdownDescription: "Bulk action: `start`, `shutdown`, `suspend` or `migrate`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"vms":              schema.SetAttribute{Required: true, ElementType: types.Int64Type, MarkdownDescription: "Guest VMIDs the action applies to.", PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()}},
			"target":           schema.StringAttribute{Optional: true, MarkdownDescription: "Target node, required for `migrate`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"online":           schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Live migration for VMs, restart migration for containers.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"with_local_disks": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Migrate local disks as well.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"max_workers":      schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(4), MarkdownDescription: "Maximum concurrent tasks for `start` and `migrate` (1-64).", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"triggers":         schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to run the action again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"upid":             schema.StringAttribute{Computed: true, MarkdownDescription: "Worker task UPID."},
		},
	}
}

func (r *NotificationTargetTestResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Sends one test notification to a notification target through `POST /cluster/notifications/targets/{name}/test`. Every change replaces the instance, destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true, MarkdownDescription: "Target name."},
			"name":     schema.StringAttribute{Required: true, MarkdownDescription: "Notification target name.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to test again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
		},
	}
}

func (r *ClusterBulkActionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *NotificationTargetTestResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *ClusterBulkActionResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config clusterBulkActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if isConfiguredValue(config.Action) && !slicesContains([]string{"start", "shutdown", "suspend", "migrate"}, config.Action.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("action"), "Invalid bulk action", "action must be start, shutdown, suspend or migrate.")
	}
	if isConfiguredValue(config.Action) && config.Action.ValueString() == "migrate" && !isConfiguredValue(config.Target) {
		resp.Diagnostics.AddAttributeError(path.Root("target"), "Missing migration target", "target is required for the migrate action.")
	}
	if isConfiguredValue(config.VMs) && len(config.VMs.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(path.Root("vms"), "Empty guest set", "vms must contain at least one VMID.")
	}
	if isConfiguredValue(config.MaxWorkers) && (config.MaxWorkers.ValueInt64() < 1 || config.MaxWorkers.ValueInt64() > 64) {
		resp.Diagnostics.AddAttributeError(path.Root("max_workers"), "Invalid worker count", "max_workers must be between 1 and 64.")
	}
}

func (r *ClusterBulkActionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *ClusterBulkActionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *ClusterBulkActionResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *ClusterBulkActionResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *ClusterBulkActionResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan clusterBulkActionModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	vmids := make([]int64, 0, len(plan.VMs.Elements()))
	for _, element := range plan.VMs.Elements() {
		value, ok := element.(types.Int64)
		if !ok {
			diags.AddAttributeError(path.Root("vms"), "Invalid VMID", "vms must contain only integers.")
			return
		}
		vmids = append(vmids, value.ValueInt64())
	}
	action := plan.Action.ValueString()
	upid, err := r.client.RunBulkAction(ctx, action, vmids, stringValue(plan.Target), plan.Online.ValueBool(), plan.WithLocalDisks.ValueBool(), plan.MaxWorkers.ValueInt64())
	if err != nil {
		diags.AddError("Unable to Run Proxmox Bulk Action", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, diags) {
		return
	}
	next := plan
	next.ID = types.StringValue(action + "/" + joinInt64(vmids))
	next.UPID = types.StringValue(upid)
	diags.Append(state.Set(ctx, &next)...)
}

func (r *NotificationTargetTestResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NotificationTargetTestResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NotificationTargetTestResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *NotificationTargetTestResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *NotificationTargetTestResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan notificationTargetTestModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	name := plan.Name.ValueString()
	if err := r.client.TestNotificationTarget(ctx, name); err != nil {
		diags.AddError("Unable to Test Proxmox Notification Target", err.Error())
		return
	}
	next := plan
	next.ID = types.StringValue(name)
	diags.Append(state.Set(ctx, &next)...)
}

func joinInt64(values []int64) string {
	result := ""
	for i, value := range values {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf("%d", value)
	}
	return result
}
