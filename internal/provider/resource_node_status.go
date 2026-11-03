// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
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

var _ resource.Resource = &NodeStatusCommandResource{}
var _ resource.ResourceWithValidateConfig = &NodeStatusCommandResource{}
var _ resource.Resource = &NodeStopAllResource{}
var _ resource.Resource = &NodeSuspendAllResource{}

type NodeStatusCommandResource struct{ client *Client }
type NodeStopAllResource struct{ client *Client }
type NodeSuspendAllResource struct{ client *Client }

type nodeStatusCommandModel struct {
	ID          types.String `tfsdk:"id"`
	Node        types.String `tfsdk:"node"`
	Command     types.String `tfsdk:"command"`
	WaitSeconds types.Int64  `tfsdk:"wait_seconds"`
	Triggers    types.Map    `tfsdk:"triggers"`
}

type nodePowerActionModel struct {
	ID         types.String `tfsdk:"id"`
	Node       types.String `tfsdk:"node"`
	VMs        types.Set    `tfsdk:"vms"`
	ForceStop  types.Bool   `tfsdk:"force_stop"`
	Timeout    types.Int64  `tfsdk:"timeout"`
	MaxWorkers types.Int64  `tfsdk:"max_workers"`
	Triggers   types.Map    `tfsdk:"triggers"`
	UPID       types.String `tfsdk:"upid"`
}

type nodeSuspendAllModel struct {
	ID         types.String `tfsdk:"id"`
	Node       types.String `tfsdk:"node"`
	VMs        types.Set    `tfsdk:"vms"`
	MaxWorkers types.Int64  `tfsdk:"max_workers"`
	Triggers   types.Map    `tfsdk:"triggers"`
	UPID       types.String `tfsdk:"upid"`
}

func NewNodeStatusCommandResource() resource.Resource { return &NodeStatusCommandResource{} }
func NewNodeStopAllResource() resource.Resource       { return &NodeStopAllResource{} }
func NewNodeSuspendAllResource() resource.Resource    { return &NodeSuspendAllResource{} }

func (r *NodeStatusCommandResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_status_command"
}

func (r *NodeStopAllResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_stopall"
}

func (r *NodeSuspendAllResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_suspendall"
}

func (r *NodeStatusCommandResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Runs `reboot` or `shutdown` on a node through `POST /nodes/{node}/status`. Every change replaces the instance, destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<command>/<node>` form."},
			"node":         schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"command":      schema.StringAttribute{Required: true, MarkdownDescription: "`reboot` or `shutdown`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"wait_seconds": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), MarkdownDescription: "After the command, wait up to this many seconds for the node to answer again (0 disables the wait).", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"triggers":     schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to run the command again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
		},
	}
}

func (r *NodeStopAllResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Stops all guests of a node through `POST /nodes/{node}/stopall` and waits for the worker. Every change replaces the instance, destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, MarkdownDescription: "Node name."},
			"node":        schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"vms":         schema.SetAttribute{Optional: true, ElementType: types.Int64Type, MarkdownDescription: "Only stop these VMIDs.", PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()}},
			"force_stop":  schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Force stop instead of a clean shutdown.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"timeout":     schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), MarkdownDescription: "Shutdown timeout in seconds.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"max_workers": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), MarkdownDescription: "Maximum concurrent tasks.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"triggers":    schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to stop again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"upid":        schema.StringAttribute{Computed: true, MarkdownDescription: "Worker task UPID."},
		},
	}
}

func (r *NodeSuspendAllResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Suspends all guests of a node through `POST /nodes/{node}/suspendall` and waits for the worker. Every change replaces the instance, destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, MarkdownDescription: "Node name."},
			"node":        schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"vms":         schema.SetAttribute{Optional: true, ElementType: types.Int64Type, MarkdownDescription: "Only suspend these VMIDs.", PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()}},
			"max_workers": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), MarkdownDescription: "Maximum concurrent tasks.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"triggers":    schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to suspend again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"upid":        schema.StringAttribute{Computed: true, MarkdownDescription: "Worker task UPID."},
		},
	}
}

func (r *NodeStatusCommandResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *NodeStopAllResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *NodeSuspendAllResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *NodeStatusCommandResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeStatusCommandModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateNodeNameResource(config.Node, resp)
	if isConfiguredValue(config.Command) && !slicesContains([]string{"reboot", "shutdown"}, config.Command.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("command"), "Invalid node command", "command must be reboot or shutdown.")
	}
}

func (r *NodeStatusCommandResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeStatusCommandResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeStatusCommandResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *NodeStatusCommandResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *NodeStatusCommandResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan nodeStatusCommandModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	node, command := plan.Node.ValueString(), plan.Command.ValueString()
	if err := r.client.NodeStatusCommand(ctx, node, command); err != nil {
		diags.AddError("Unable to Run Proxmox Node Command", err.Error())
		return
	}
	if plan.WaitSeconds.ValueInt64() > 0 {
		if err := r.client.WaitNodeOnline(ctx, node, time.Duration(plan.WaitSeconds.ValueInt64())*time.Second); err != nil {
			diags.AddError("Node Did Not Come Back", err.Error())
			return
		}
	}
	next := plan
	next.ID = types.StringValue(command + "/" + node)
	diags.Append(state.Set(ctx, &next)...)
}

func (r *NodeStopAllResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics, "stopall", true)
}

func (r *NodeStopAllResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics, "stopall", true)
}

func (r *NodeStopAllResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *NodeStopAllResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *NodeSuspendAllResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics, "suspendall")
}

func (r *NodeSuspendAllResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics, "suspendall")
}

func (r *NodeSuspendAllResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *NodeSuspendAllResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *NodeStopAllResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics, action string, withStopOptions bool) {
	runNodePowerAction(ctx, planGetter, state, diags, action, withStopOptions, r.client)
}

func (r *NodeSuspendAllResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics, action string) {
	var plan nodeSuspendAllModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	vms := vmIDSet(plan.VMs, diags)
	if diags.HasError() {
		return
	}
	node := plan.Node.ValueString()
	upid, err := r.client.NodePowerAction(ctx, node, action, vms, false, 0, plan.MaxWorkers.ValueInt64())
	if err != nil {
		diags.AddError("Unable to Run Proxmox Node Power Action", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, diags) {
		return
	}
	next := plan
	next.ID = types.StringValue(node)
	next.UPID = types.StringValue(upid)
	diags.Append(state.Set(ctx, &next)...)
}

func runNodePowerAction(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics, action string, withStopOptions bool, client *Client) {
	var plan nodePowerActionModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	vms := vmIDSet(plan.VMs, diags)
	if diags.HasError() {
		return
	}
	node := plan.Node.ValueString()
	var timeout, maxWorkers int64
	forceStop := false
	if withStopOptions {
		if plan.ForceStop.ValueBool() {
			forceStop = true
		}
		timeout = plan.Timeout.ValueInt64()
	}
	maxWorkers = plan.MaxWorkers.ValueInt64()
	upid, err := client.NodePowerAction(ctx, node, action, vms, forceStop, timeout, maxWorkers)
	if err != nil {
		diags.AddError("Unable to Run Proxmox Node Power Action", err.Error())
		return
	}
	if !nodeDiskWait(ctx, client, upid, diags) {
		return
	}
	next := plan
	next.ID = types.StringValue(node)
	next.UPID = types.StringValue(upid)
	diags.Append(state.Set(ctx, &next)...)
}

var _ = fmt.Sprintf
var _ = attr.Value(nil)

func vmIDSet(values types.Set, diags *diag.Diagnostics) string {
	if values.IsNull() || values.IsUnknown() {
		return ""
	}
	parts := make([]string, 0, len(values.Elements()))
	for _, element := range values.Elements() {
		value, ok := element.(types.Int64)
		if !ok {
			diags.AddAttributeError(path.Root("vms"), "Invalid VMID", "vms must contain only integers.")
			return ""
		}
		parts = append(parts, strconv.FormatInt(value.ValueInt64(), 10))
	}
	return strings.Join(parts, ",")
}
