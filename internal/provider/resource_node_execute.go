// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &NodeExecuteResource{}
var _ resource.ResourceWithValidateConfig = &NodeExecuteResource{}
var _ datasource.DataSource = &MetricsExportDataSource{}

type NodeExecuteResource struct{ client *Client }
type MetricsExportDataSource struct{ client *Client }

type nodeExecuteModel struct {
	ID       types.String       `tfsdk:"id"`
	Node     types.String       `tfsdk:"node"`
	Calls    []nodeCommandModel `tfsdk:"calls"`
	Results  types.String       `tfsdk:"results"`
	Triggers types.Map          `tfsdk:"triggers"`
}

type nodeCommandModel struct {
	Method types.String `tfsdk:"method"`
	Path   types.String `tfsdk:"path"`
	Args   types.Map    `tfsdk:"args"`
}

type metricsExportModel struct {
	NodeList types.String         `tfsdk:"node_list"`
	History  types.Bool           `tfsdk:"history"`
	Metrics  []clusterMetricModel `tfsdk:"metrics"`
}

type clusterMetricModel struct {
	ID        types.String  `tfsdk:"id"`
	Metric    types.String  `tfsdk:"metric"`
	Type      types.String  `tfsdk:"type"`
	Timestamp types.Int64   `tfsdk:"timestamp"`
	Value     types.Float64 `tfsdk:"value"`
}

func NewNodeExecuteResource() resource.Resource         { return &NodeExecuteResource{} }
func NewMetricsExportDataSource() datasource.DataSource { return &MetricsExportDataSource{} }

func (r *NodeExecuteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_execute"
}

func (d *MetricsExportDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_metrics_export"
}

func (r *NodeExecuteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Runs an ordered batch of node API calls through `POST /nodes/{node}/execute` and records their result JSON. Paths are relative to the node (for example `status` or `qemu/100/status/start`). Every change replaces the instance, destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true, MarkdownDescription: "Node name."},
			"node": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"calls": schema.ListNestedAttribute{Required: true, PlanModifiers: []planmodifier.List{listRequiresReplace()}, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"method": schema.StringAttribute{Required: true, MarkdownDescription: "`GET`, `POST`, `PUT` or `DELETE`."},
				"path":   schema.StringAttribute{Required: true, MarkdownDescription: "Node-relative API path."},
				"args":   schema.MapAttribute{Optional: true, ElementType: types.StringType, MarkdownDescription: "String arguments; booleans as `1`/`0`."},
			}}},
			"results":  schema.StringAttribute{Computed: true, MarkdownDescription: "JSON array of per-call results."},
			"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to run the batch again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
		},
	}
}

func (d *MetricsExportDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads cluster metric points from `GET /cluster/metrics/export`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node_list": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Comma separated node list."},
			"history":   datasourceschema.BoolAttribute{Optional: true, MarkdownDescription: "Also return historical points (the API limits history to roughly an hour)."},
			"metrics": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"id":        datasourceschema.StringAttribute{Computed: true},
				"metric":    datasourceschema.StringAttribute{Computed: true},
				"type":      datasourceschema.StringAttribute{Computed: true},
				"timestamp": datasourceschema.Int64Attribute{Computed: true},
				"value":     datasourceschema.Float64Attribute{Computed: true},
			}}},
		},
	}
}

func (r *NodeExecuteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (d *MetricsExportDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (r *NodeExecuteResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeExecuteModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateNodeNameResource(config.Node, resp)
	if len(config.Calls) == 0 {
		resp.Diagnostics.AddAttributeError(agentPathAttr("calls"), "Empty command batch", "calls must contain at least one call.")
	}
}

func (r *NodeExecuteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeExecuteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeExecuteResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *NodeExecuteResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *NodeExecuteResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan nodeExecuteModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	calls := make([]NodeCommandCall, 0, len(plan.Calls))
	for _, call := range plan.Calls {
		args := make(map[string]string, len(call.Args.Elements()))
		for key, value := range call.Args.Elements() {
			text, ok := value.(types.String)
			if !ok {
				diags.AddAttributeError(agentPathAttr("calls"), "Invalid argument", "args values must be strings.")
				return
			}
			args[key] = text.ValueString()
		}
		calls = append(calls, NodeCommandCall{Method: call.Method.ValueString(), Path: call.Path.ValueString(), Args: args})
	}
	node := plan.Node.ValueString()
	results, err := r.client.ExecuteCommands(ctx, node, calls)
	if err != nil {
		diags.AddError("Unable to Execute Proxmox Node Commands", err.Error())
		return
	}
	encoded, encodeErr := json.Marshal(results)
	if encodeErr != nil {
		diags.AddError("Unable to Encode Proxmox Command Results", encodeErr.Error())
		return
	}
	next := plan
	next.ID = types.StringValue(node)
	next.Results = types.StringValue(string(encoded))
	diags.Append(state.Set(ctx, &next)...)
}

func (d *MetricsExportDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config metricsExportModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	metrics, err := d.client.MetricsExport(ctx, config.NodeList.ValueString(), config.History.ValueBool(), 0)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Metrics Export", err.Error())
		return
	}
	records := make([]clusterMetricModel, 0, len(metrics))
	for _, metric := range metrics {
		records = append(records, clusterMetricModel{
			ID: stringOrNull(metric.ID), Metric: stringOrNull(metric.Metric), Type: stringOrNull(metric.Type),
			Timestamp: types.Int64Value(metric.Timestamp), Value: types.Float64Value(metric.Value),
		})
	}
	config.Metrics = records
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
