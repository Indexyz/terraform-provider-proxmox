// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &CephStatusDataSource{}
var _ datasource.DataSource = &CephFlagsDataSource{}
var _ datasource.DataSource = &CephOSDTreeDataSource{}
var _ datasource.DataSource = &CephMonitorsDataSource{}
var _ datasource.DataSource = &CephPoolsDataSource{}

type CephStatusDataSource struct{ client *Client }
type CephFlagsDataSource struct{ client *Client }
type CephOSDTreeDataSource struct{ client *Client }
type CephMonitorsDataSource struct{ client *Client }
type CephPoolsDataSource struct{ client *Client }

type cephStatusModel struct {
	Status types.String `tfsdk:"status"`
}

type cephFlagsModel struct {
	Flags []cephFlagEntry `tfsdk:"flags"`
}

type cephFlagEntry struct {
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Value       types.Bool   `tfsdk:"value"`
}

type cephOSDTreeModel struct {
	Node types.String `tfsdk:"node"`
	Tree types.String `tfsdk:"tree"`
}

type cephMonitorsModel struct {
	Node     types.String       `tfsdk:"node"`
	Monitors []cephMonitorEntry `tfsdk:"monitors"`
}

type cephMonitorEntry struct {
	Name        types.String `tfsdk:"name"`
	Address     types.String `tfsdk:"address"`
	Host        types.String `tfsdk:"host"`
	Rank        types.Int64  `tfsdk:"rank"`
	Quorum      types.Bool   `tfsdk:"quorum"`
	State       types.String `tfsdk:"state"`
	CephVersion types.String `tfsdk:"ceph_version"`
}

type cephPoolsModel struct {
	Node  types.String    `tfsdk:"node"`
	Pools []cephPoolEntry `tfsdk:"pools"`
}

type cephPoolEntry struct {
	Name            types.String  `tfsdk:"name"`
	PoolID          types.Int64   `tfsdk:"pool_id"`
	Size            types.Int64   `tfsdk:"size"`
	MinSize         types.Int64   `tfsdk:"min_size"`
	PGNum           types.Int64   `tfsdk:"pg_num"`
	PGNumMin        types.Int64   `tfsdk:"pg_num_min"`
	PGAutoscaleMode types.String  `tfsdk:"pg_autoscale_mode"`
	CrushRuleName   types.String  `tfsdk:"crush_rule_name"`
	BytesUsed       types.Int64   `tfsdk:"bytes_used"`
	PercentUsed     types.Float64 `tfsdk:"percent_used"`
	Type            types.String  `tfsdk:"type"`
}

func NewCephStatusDataSource() datasource.DataSource   { return &CephStatusDataSource{} }
func NewCephFlagsDataSource() datasource.DataSource    { return &CephFlagsDataSource{} }
func NewCephOSDTreeDataSource() datasource.DataSource  { return &CephOSDTreeDataSource{} }
func NewCephMonitorsDataSource() datasource.DataSource { return &CephMonitorsDataSource{} }
func NewCephPoolsDataSource() datasource.DataSource    { return &CephPoolsDataSource{} }

func (d *CephStatusDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ceph_status"
}
func (d *CephFlagsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ceph_flags"
}
func (d *CephOSDTreeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_osd_tree"
}
func (d *CephMonitorsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_monitors"
}
func (d *CephPoolsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_pools"
}

func (d *CephStatusDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the raw Ceph cluster status JSON from `/cluster/ceph/status`.",
		Attributes: map[string]datasourceschema.Attribute{
			"status": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "JSON object of the full Ceph status."},
		},
	}
}

func (d *CephFlagsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists Ceph cluster flags from `/cluster/ceph/flags`.",
		Attributes: map[string]datasourceschema.Attribute{
			"flags": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"name":        datasourceschema.StringAttribute{Computed: true},
				"description": datasourceschema.StringAttribute{Computed: true},
				"value":       datasourceschema.BoolAttribute{Computed: true},
			}}},
		},
	}
}

func (d *CephOSDTreeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the raw Ceph OSD tree JSON from `/nodes/{node}/ceph/osd`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"tree": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "JSON object with the CRUSH tree."},
		},
	}
}

func (d *CephMonitorsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists Ceph monitors from `/nodes/{node}/ceph/mon`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"monitors": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"name":         datasourceschema.StringAttribute{Computed: true},
				"address":      datasourceschema.StringAttribute{Computed: true},
				"host":         datasourceschema.StringAttribute{Computed: true},
				"rank":         datasourceschema.Int64Attribute{Computed: true},
				"quorum":       datasourceschema.BoolAttribute{Computed: true},
				"state":        datasourceschema.StringAttribute{Computed: true},
				"ceph_version": datasourceschema.StringAttribute{Computed: true},
			}}},
		},
	}
}

func (d *CephPoolsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists Ceph pools from `/nodes/{node}/ceph/pool`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"pools": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"name":              datasourceschema.StringAttribute{Computed: true},
				"pool_id":           datasourceschema.Int64Attribute{Computed: true},
				"size":              datasourceschema.Int64Attribute{Computed: true},
				"min_size":          datasourceschema.Int64Attribute{Computed: true},
				"pg_num":            datasourceschema.Int64Attribute{Computed: true},
				"pg_num_min":        datasourceschema.Int64Attribute{Computed: true},
				"pg_autoscale_mode": datasourceschema.StringAttribute{Computed: true},
				"crush_rule_name":   datasourceschema.StringAttribute{Computed: true},
				"bytes_used":        datasourceschema.Int64Attribute{Computed: true},
				"percent_used":      datasourceschema.Float64Attribute{Computed: true},
				"type":              datasourceschema.StringAttribute{Computed: true},
			}}},
		},
	}
}

func (d *CephStatusDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephFlagsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephOSDTreeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephMonitorsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephPoolsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *CephStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephStatusModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	status, err := d.client.CephStatus(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph Status", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cephStatusModel{Status: types.StringValue(status)})...)
}

func (d *CephFlagsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephFlagsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	flags, err := d.client.CephFlags(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph Flags", err.Error())
		return
	}
	entries := make([]cephFlagEntry, 0, len(flags))
	for _, flag := range flags {
		entries = append(entries, cephFlagEntry{Name: types.StringValue(flag.Name), Description: stringOrNull(flag.Description), Value: nodesBoolOrNull(flag.Value)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cephFlagsModel{Flags: entries})...)
}

func (d *CephOSDTreeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephOSDTreeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tree, err := d.client.CephOSDTree(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph OSD Tree", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cephOSDTreeModel{Node: config.Node, Tree: types.StringValue(tree)})...)
}

func (d *CephMonitorsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephMonitorsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	monitors, err := d.client.CephMonitors(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph Monitors", err.Error())
		return
	}
	entries := make([]cephMonitorEntry, 0, len(monitors))
	for _, monitor := range monitors {
		entries = append(entries, cephMonitorEntry{
			Name: stringOrNull(monitor.Name), Address: stringOrNull(monitor.Addr), Host: stringOrNull(monitor.Host),
			Rank: types.Int64Value(monitor.Rank), Quorum: nodesBoolOrNull(monitor.Quorum), State: stringOrNull(monitor.State),
			CephVersion: stringOrNull(monitor.CephVersion),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cephMonitorsModel{Node: config.Node, Monitors: entries})...)
}

func (d *CephPoolsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephPoolsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	pools, err := d.client.CephPools(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		resp.Diagnostics.AddError("Unable to Read Ceph Pools", err.Error())
		return
	}
	entries := make([]cephPoolEntry, 0, len(pools))
	for _, pool := range pools {
		entries = append(entries, cephPoolEntry{
			Name: stringOrNull(pool.Name), PoolID: types.Int64Value(pool.PoolID), Size: types.Int64Value(pool.Size),
			MinSize: types.Int64Value(pool.MinSize), PGNum: types.Int64Value(pool.PGNum), PGNumMin: types.Int64Value(pool.PGNumMin),
			PGAutoscaleMode: stringOrNull(pool.PGAutoscaleMode), CrushRuleName: stringOrNull(pool.CrushRuleName),
			BytesUsed: types.Int64Value(pool.BytesUsed), PercentUsed: types.Float64Value(pool.PercentUsed), Type: stringOrNull(pool.Type),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cephPoolsModel{Node: config.Node, Pools: entries})...)
}
