// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource = &ClusterLogDataSource{}
	_ datasource.DataSource = &ClusterTasksDataSource{}
	_ datasource.DataSource = &ClusterStatusDataSource{}
	_ datasource.DataSource = &ScheduleAnalyzeDataSource{}
	_ datasource.DataSource = &CPUFlagsDataSource{}
	_ datasource.DataSource = &CustomCPUModelsDataSource{}
	_ datasource.DataSource = &HAStatusDataSource{}
	_ datasource.DataSource = &HAManagerStatusDataSource{}
	_ datasource.DataSource = &HAGroupsDataSource{}
)

type ClusterLogDataSource struct{ client *Client }
type ClusterTasksDataSource struct{ client *Client }
type ClusterStatusDataSource struct{ client *Client }
type ScheduleAnalyzeDataSource struct{ client *Client }
type CPUFlagsDataSource struct{ client *Client }
type CustomCPUModelsDataSource struct{ client *Client }
type HAStatusDataSource struct{ client *Client }
type HAManagerStatusDataSource struct{ client *Client }
type HAGroupsDataSource struct{ client *Client }

func NewClusterLogDataSource() datasource.DataSource      { return &ClusterLogDataSource{} }
func NewClusterTasksDataSource() datasource.DataSource    { return &ClusterTasksDataSource{} }
func NewClusterStatusDataSource() datasource.DataSource   { return &ClusterStatusDataSource{} }
func NewScheduleAnalyzeDataSource() datasource.DataSource { return &ScheduleAnalyzeDataSource{} }
func NewCPUFlagsDataSource() datasource.DataSource        { return &CPUFlagsDataSource{} }
func NewCustomCPUModelsDataSource() datasource.DataSource { return &CustomCPUModelsDataSource{} }
func NewHAStatusDataSource() datasource.DataSource        { return &HAStatusDataSource{} }
func NewHAManagerStatusDataSource() datasource.DataSource { return &HAManagerStatusDataSource{} }
func NewHAGroupsDataSource() datasource.DataSource        { return &HAGroupsDataSource{} }

type clusterLogModel struct {
	Max     types.Int64            `tfsdk:"max"`
	Entries []clusterLogEntryModel `tfsdk:"entries"`
}
type clusterLogEntryModel struct {
	Message types.String `tfsdk:"message"`
	User    types.String `tfsdk:"user"`
	Time    types.Int64  `tfsdk:"time"`
	ID      types.String `tfsdk:"id"`
	Tag     types.String `tfsdk:"tag"`
	PRI     types.Int64  `tfsdk:"pri"`
	Node    types.String `tfsdk:"node"`
}
type clusterTasksModel struct {
	Tasks []clusterTaskModel `tfsdk:"tasks"`
}
type clusterTaskModel struct {
	UPID      types.String `tfsdk:"upid"`
	Node      types.String `tfsdk:"node"`
	Type      types.String `tfsdk:"type"`
	ID        types.String `tfsdk:"id"`
	Status    types.String `tfsdk:"status"`
	User      types.String `tfsdk:"user"`
	StartTime types.Int64  `tfsdk:"starttime"`
	EndTime   types.Int64  `tfsdk:"endtime"`
}
type clusterStatusModel struct {
	Nodes []clusterStatusNodeModel `tfsdk:"nodes"`
}
type clusterStatusNodeModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Type    types.String `tfsdk:"type"`
	IP      types.String `tfsdk:"ip"`
	Level   types.String `tfsdk:"level"`
	Local   types.Bool   `tfsdk:"local"`
	NodeID  types.Int64  `tfsdk:"nodeid"`
	Nodes   types.Int64  `tfsdk:"nodes"`
	Online  types.Bool   `tfsdk:"online"`
	Quorate types.Bool   `tfsdk:"quorate"`
	Version types.Int64  `tfsdk:"version"`
}
type scheduleAnalyzeModel struct {
	Schedule   types.String         `tfsdk:"schedule"`
	Iterations types.Int64          `tfsdk:"iterations"`
	StartTime  types.Int64          `tfsdk:"starttime"`
	Points     []schedulePointModel `tfsdk:"points"`
}
type schedulePointModel struct {
	Timestamp types.Int64  `tfsdk:"timestamp"`
	UTC       types.String `tfsdk:"utc"`
}
type cpuFlagsModel struct {
	Arch  types.String        `tfsdk:"arch"`
	Accel types.String        `tfsdk:"accel"`
	Flags []cpuFlagEntryModel `tfsdk:"flags"`
}
type cpuFlagEntryModel struct {
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	SupportedOn types.List   `tfsdk:"supported_on"`
}
type customCPUModelsModel struct {
	Models []customCPUModelEntry `tfsdk:"models"`
}
type customCPUModelEntry struct {
	CPUType       types.String `tfsdk:"cputype"`
	Digest        types.String `tfsdk:"digest"`
	Flags         types.String `tfsdk:"flags"`
	GuestPhysBits types.Int64  `tfsdk:"guest_phys_bits"`
	Hidden        types.Bool   `tfsdk:"hidden"`
	HVVendorID    types.String `tfsdk:"hv_vendor_id"`
	Level         types.Int64  `tfsdk:"level"`
	PhysBits      types.String `tfsdk:"phys_bits"`
	ReportedModel types.String `tfsdk:"reported_model"`
}
type haStatusModel struct {
	Entries []haStatusEntryModel `tfsdk:"entries"`
}
type haStatusEntryModel struct {
	Type          types.String `tfsdk:"type"`
	ID            types.String `tfsdk:"id"`
	Node          types.String `tfsdk:"node"`
	Status        types.String `tfsdk:"status"`
	State         types.String `tfsdk:"state"`
	RequestState  types.String `tfsdk:"request_state"`
	SID           types.String `tfsdk:"sid"`
	Quorate       types.Bool   `tfsdk:"quorate"`
	ArmedState    types.String `tfsdk:"armed_state"`
	Timestamp     types.Int64  `tfsdk:"timestamp"`
	MaxRelocate   types.Int64  `tfsdk:"max_relocate"`
	MaxRestart    types.Int64  `tfsdk:"max_restart"`
	AutoRebalance types.Bool   `tfsdk:"auto_rebalance"`
	Failback      types.Bool   `tfsdk:"failback"`
	CRMState      types.String `tfsdk:"crm_state"`
	ResourceMode  types.String `tfsdk:"resource_mode"`
}
type haManagerStatusModel struct {
	Status types.String `tfsdk:"status"`
}
type haGroupsModel struct {
	Migrated types.Bool          `tfsdk:"migrated"`
	Groups   []haGroupEntryModel `tfsdk:"groups"`
}
type haGroupEntryModel struct {
	Group      types.String `tfsdk:"group"`
	Comment    types.String `tfsdk:"comment"`
	Restricted types.Bool   `tfsdk:"restricted"`
	NoFailback types.Bool   `tfsdk:"nofailback"`
	Nodes      types.String `tfsdk:"nodes"`
}

func (d *ClusterLogDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_log"
}
func (d *ClusterTasksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_tasks"
}
func (d *ClusterStatusDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_status"
}
func (d *ScheduleAnalyzeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule_analyze"
}
func (d *CPUFlagsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cpu_flags"
}
func (d *CustomCPUModelsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_cpu_models"
}
func (d *HAStatusDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ha_status"
}
func (d *HAManagerStatusDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ha_manager_status"
}
func (d *HAGroupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ha_groups"
}

func clusterLogSchema() datasourceschema.Schema {
	return datasourceschema.Schema{
		MarkdownDescription: "Reads recent cluster log entries from `/cluster/log`, newest first.",
		Attributes: map[string]datasourceschema.Attribute{
			"max": datasourceschema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Maximum entries, default 100."},
			"entries": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"message": datasourceschema.StringAttribute{Computed: true},
				"user":    datasourceschema.StringAttribute{Computed: true},
				"time":    datasourceschema.Int64Attribute{Computed: true},
				"id":      datasourceschema.StringAttribute{Computed: true},
				"tag":     datasourceschema.StringAttribute{Computed: true},
				"pri":     datasourceschema.Int64Attribute{Computed: true},
				"node":    datasourceschema.StringAttribute{Computed: true},
			}}},
		},
	}
}

func (d *ClusterLogDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = clusterLogSchema()
}

func (d *ClusterTasksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists recent cluster tasks from `/cluster/tasks`.",
		Attributes: map[string]datasourceschema.Attribute{
			"tasks": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"upid":      datasourceschema.StringAttribute{Computed: true},
				"node":      datasourceschema.StringAttribute{Computed: true},
				"type":      datasourceschema.StringAttribute{Computed: true},
				"id":        datasourceschema.StringAttribute{Computed: true},
				"status":    datasourceschema.StringAttribute{Computed: true},
				"user":      datasourceschema.StringAttribute{Computed: true},
				"starttime": datasourceschema.Int64Attribute{Computed: true},
				"endtime":   datasourceschema.Int64Attribute{Computed: true},
			}}},
		},
	}
}

func (d *ClusterStatusDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads cluster membership and quorum state from `/cluster/status`.",
		Attributes: map[string]datasourceschema.Attribute{
			"nodes": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"id":      datasourceschema.StringAttribute{Computed: true},
				"name":    datasourceschema.StringAttribute{Computed: true},
				"type":    datasourceschema.StringAttribute{Computed: true},
				"ip":      datasourceschema.StringAttribute{Computed: true},
				"level":   datasourceschema.StringAttribute{Computed: true},
				"local":   datasourceschema.BoolAttribute{Computed: true},
				"nodeid":  datasourceschema.Int64Attribute{Computed: true},
				"nodes":   datasourceschema.Int64Attribute{Computed: true},
				"online":  datasourceschema.BoolAttribute{Computed: true},
				"quorate": datasourceschema.BoolAttribute{Computed: true},
				"version": datasourceschema.Int64Attribute{Computed: true},
			}}},
		},
	}
}

func (d *ScheduleAnalyzeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Analyzes a calendar schedule through `/cluster/jobs/schedule-analyze`.",
		Attributes: map[string]datasourceschema.Attribute{
			"schedule":   datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Calendar event specification."},
			"iterations": datasourceschema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Number of results, default 10."},
			"starttime":  datasourceschema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Start epoch."},
			"points": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"timestamp": datasourceschema.Int64Attribute{Computed: true},
				"utc":       datasourceschema.StringAttribute{Computed: true},
			}}},
		},
	}
}

func (d *CPUFlagsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists CPU flags known to `/cluster/qemu/cpu-flags`.",
		Attributes: map[string]datasourceschema.Attribute{
			"arch":  datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "CPU architecture."},
			"accel": datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Accelerator (`kvm` or `tcg`)."},
			"flags": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"name":         datasourceschema.StringAttribute{Computed: true},
				"description":  datasourceschema.StringAttribute{Computed: true},
				"supported_on": datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
			}}},
		},
	}
}

func (d *CustomCPUModelsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = customCPUModelSchema()
}

func (d *HAStatusDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the HA status from `/cluster/ha/status/current`.",
		Attributes: map[string]datasourceschema.Attribute{
			"entries": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"type":           datasourceschema.StringAttribute{Computed: true},
				"id":             datasourceschema.StringAttribute{Computed: true},
				"node":           datasourceschema.StringAttribute{Computed: true},
				"status":         datasourceschema.StringAttribute{Computed: true},
				"state":          datasourceschema.StringAttribute{Computed: true},
				"request_state":  datasourceschema.StringAttribute{Computed: true},
				"sid":            datasourceschema.StringAttribute{Computed: true},
				"quorate":        datasourceschema.BoolAttribute{Computed: true},
				"armed_state":    datasourceschema.StringAttribute{Computed: true},
				"timestamp":      datasourceschema.Int64Attribute{Computed: true},
				"max_relocate":   datasourceschema.Int64Attribute{Computed: true},
				"max_restart":    datasourceschema.Int64Attribute{Computed: true},
				"auto_rebalance": datasourceschema.BoolAttribute{Computed: true},
				"failback":       datasourceschema.BoolAttribute{Computed: true},
				"crm_state":      datasourceschema.StringAttribute{Computed: true},
				"resource_mode":  datasourceschema.StringAttribute{Computed: true},
			}}},
		},
	}
}

func (d *HAManagerStatusDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the raw HA manager status JSON from `/cluster/ha/status/manager_status`.",
		Attributes: map[string]datasourceschema.Attribute{
			"status": datasourceschema.StringAttribute{Computed: true},
		},
	}
}

func (d *HAGroupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists legacy HA groups from `/cluster/ha/groups`. PVE 9 reports the endpoint as migrated; in that case `groups` is empty and `migrated` is true.",
		Attributes: map[string]datasourceschema.Attribute{
			"migrated": datasourceschema.BoolAttribute{Computed: true},
			"groups": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"group":      datasourceschema.StringAttribute{Computed: true},
				"comment":    datasourceschema.StringAttribute{Computed: true},
				"restricted": datasourceschema.BoolAttribute{Computed: true},
				"nofailback": datasourceschema.BoolAttribute{Computed: true},
				"nodes":      datasourceschema.StringAttribute{Computed: true},
			}}},
		},
	}
}

func (d *ClusterLogDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *ClusterTasksDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *ClusterStatusDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *ScheduleAnalyzeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CPUFlagsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CustomCPUModelsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *HAStatusDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *HAManagerStatusDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *HAGroupsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *ClusterLogDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config clusterLogModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	limit := int64(100)
	if !config.Max.IsNull() && !config.Max.IsUnknown() {
		limit = config.Max.ValueInt64()
	}
	entries, err := d.client.ClusterLog(ctx, limit)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Cluster Log", err.Error())
		return
	}
	records := make([]clusterLogEntryModel, 0, len(entries))
	for _, entry := range entries {
		records = append(records, clusterLogEntryModel{
			Message: stringOrNull(entry.Message), User: stringOrNull(entry.User), Time: types.Int64Value(entry.Time),
			ID: stringOrNull(entry.ID), Tag: stringOrNull(entry.Tag), PRI: types.Int64Value(entry.PRI), Node: stringOrNull(entry.Node),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &clusterLogModel{Max: types.Int64Value(limit), Entries: records})...)
}

func (d *ClusterTasksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config clusterTasksModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tasks, err := d.client.ClusterTasks(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Cluster Tasks", err.Error())
		return
	}
	records := make([]clusterTaskModel, 0, len(tasks))
	for _, task := range tasks {
		records = append(records, clusterTaskModel{
			UPID: types.StringValue(task.UPID), Node: stringOrNull(task.Node), Type: stringOrNull(task.Type), ID: stringOrNull(task.ID),
			Status: stringOrNull(task.Status), User: stringOrNull(task.User), StartTime: types.Int64Value(task.StartTime), EndTime: types.Int64Value(task.EndTime),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &clusterTasksModel{Tasks: records})...)
}

func (d *ClusterStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config clusterStatusModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	nodes, err := d.client.ClusterStatus(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Cluster Status", err.Error())
		return
	}
	records := make([]clusterStatusNodeModel, 0, len(nodes))
	for _, node := range nodes {
		records = append(records, clusterStatusNodeModel{
			ID: stringOrNull(node.ID), Name: stringOrNull(node.Name), Type: stringOrNull(node.Type), IP: stringOrNull(node.IP),
			Level: stringOrNull(node.Level), Local: nodesBoolOrNull(node.Local), NodeID: types.Int64Value(node.NodeID), Nodes: types.Int64Value(node.Nodes),
			Online: nodesBoolOrNull(node.Online), Quorate: nodesBoolOrNull(node.Quorate), Version: types.Int64Value(node.Version),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &clusterStatusModel{Nodes: records})...)
}

func (d *ScheduleAnalyzeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config scheduleAnalyzeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	iterations := int64(10)
	if !config.Iterations.IsNull() && !config.Iterations.IsUnknown() {
		iterations = config.Iterations.ValueInt64()
	}
	start := int64(0)
	if !config.StartTime.IsNull() && !config.StartTime.IsUnknown() {
		start = config.StartTime.ValueInt64()
	}
	points, err := d.client.AnalyzeSchedule(ctx, config.Schedule.ValueString(), iterations, start)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Analyze Schedule", err.Error())
		return
	}
	records := make([]schedulePointModel, 0, len(points))
	for _, point := range points {
		records = append(records, schedulePointModel{Timestamp: types.Int64Value(point.Timestamp), UTC: stringOrNull(point.UTC)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &scheduleAnalyzeModel{
		Schedule: config.Schedule, Iterations: types.Int64Value(iterations), StartTime: types.Int64Value(start), Points: records,
	})...)
}

func (d *CPUFlagsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cpuFlagsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	flags, err := d.client.CPUFlags(ctx, stringValue(config.Arch), stringValue(config.Accel))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read CPU Flags", err.Error())
		return
	}
	records := make([]cpuFlagEntryModel, 0, len(flags))
	for _, flag := range flags {
		supported, diags := types.ListValueFrom(ctx, types.StringType, flag.SupportedOn)
		resp.Diagnostics.Append(diags...)
		records = append(records, cpuFlagEntryModel{Name: stringOrNull(flag.Name), Description: stringOrNull(flag.Description), SupportedOn: supported})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cpuFlagsModel{Arch: config.Arch, Accel: config.Accel, Flags: records})...)
}

func (d *CustomCPUModelsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config customCPUModelsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	models, err := d.client.CustomCPUModels(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Custom CPU Models", err.Error())
		return
	}
	records := make([]customCPUModelEntry, 0, len(models))
	for _, model := range models {
		records = append(records, cpuModelEntry(model))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &customCPUModelsModel{Models: records})...)
}

func cpuModelEntry(model CustomCPUModel) customCPUModelEntry {
	return customCPUModelEntry{
		CPUType: stringOrNull(model.CPUType), Digest: stringOrNull(model.Digest), Flags: stringOrNull(model.Flags),
		GuestPhysBits: types.Int64Value(model.GuestPhysBits), Hidden: nodesBoolOrNull(model.Hidden), HVVendorID: stringOrNull(model.HVVendorID),
		Level: types.Int64Value(model.Level), PhysBits: stringOrNull(model.PhysBits), ReportedModel: stringOrNull(model.ReportedModel),
	}
}

func (d *HAStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config haStatusModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entries, err := d.client.HAStatus(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read HA Status", err.Error())
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	records := make([]haStatusEntryModel, 0, len(entries))
	for _, entry := range entries {
		records = append(records, haStatusEntryModel{
			Type: stringOrNull(entry.Type), ID: stringOrNull(entry.ID), Node: stringOrNull(entry.Node), Status: stringOrNull(entry.Status),
			State: stringOrNull(entry.State), RequestState: stringOrNull(entry.RequestState), SID: stringOrNull(entry.SID),
			Quorate: nodesBoolOrNull(entry.Quorate), ArmedState: stringOrNull(entry.ArmedState), Timestamp: types.Int64Value(entry.Timestamp),
			MaxRelocate: types.Int64Value(entry.MaxRelocate), MaxRestart: types.Int64Value(entry.MaxRestart),
			AutoRebalance: nodesBoolOrNull(entry.AutoRebalance), Failback: nodesBoolOrNull(entry.Failback),
			CRMState: stringOrNull(entry.CRMState), ResourceMode: stringOrNull(entry.ResourceMode),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &haStatusModel{Entries: records})...)
}

func (d *HAManagerStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config haManagerStatusModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	status, err := d.client.HAManagerStatus(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read HA Manager Status", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &haManagerStatusModel{Status: types.StringValue(status)})...)
}

func (d *HAGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config haGroupsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groups, migrated, err := d.client.HAGroups(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read HA Groups", err.Error())
		return
	}
	records := make([]haGroupEntryModel, 0, len(groups))
	for _, group := range groups {
		records = append(records, haGroupEntryModel{
			Group: stringOrNull(group.Group), Comment: stringOrNull(group.Comment),
			Restricted: nodesBoolOrNull(group.Restricted), NoFailback: nodesBoolOrNull(group.NoFailback), Nodes: stringOrNull(group.Nodes),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &haGroupsModel{Migrated: types.BoolValue(migrated), Groups: records})...)
}

func customCPUModelSchema() datasourceschema.Schema {
	return datasourceschema.Schema{
		MarkdownDescription: "Lists custom QEMU CPU models from `/cluster/qemu/custom-cpu-models`.",
		Attributes: map[string]datasourceschema.Attribute{
			"models": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"cputype":         datasourceschema.StringAttribute{Computed: true},
				"digest":          datasourceschema.StringAttribute{Computed: true},
				"flags":           datasourceschema.StringAttribute{Computed: true},
				"guest_phys_bits": datasourceschema.Int64Attribute{Computed: true},
				"hidden":          datasourceschema.BoolAttribute{Computed: true},
				"hv_vendor_id":    datasourceschema.StringAttribute{Computed: true},
				"level":           datasourceschema.Int64Attribute{Computed: true},
				"phys_bits":       datasourceschema.StringAttribute{Computed: true},
				"reported_model":  datasourceschema.StringAttribute{Computed: true},
			}}},
		},
	}
}
