// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &NodeSyslogDataSource{}
var _ datasource.DataSource = &NodeJournalDataSource{}

type NodeSyslogDataSource struct{ client *Client }
type NodeJournalDataSource struct{ client *Client }

type nodeSyslogModel struct {
	Node    types.String           `tfsdk:"node"`
	Limit   types.Int64            `tfsdk:"limit"`
	Service types.String           `tfsdk:"service"`
	Since   types.String           `tfsdk:"since"`
	Until   types.String           `tfsdk:"until"`
	Entries []nodeSyslogEntryModel `tfsdk:"entries"`
}

type nodeSyslogEntryModel struct {
	Line types.Int64  `tfsdk:"line"`
	Text types.String `tfsdk:"text"`
}

type nodeJournalModel struct {
	Node    types.String `tfsdk:"node"`
	Limit   types.Int64  `tfsdk:"limit"`
	Service types.String `tfsdk:"service"`
	Since   types.String `tfsdk:"since"`
	Until   types.String `tfsdk:"until"`
	Lines   types.List   `tfsdk:"lines"`
}

func NewNodeSyslogDataSource() datasource.DataSource  { return &NodeSyslogDataSource{} }
func NewNodeJournalDataSource() datasource.DataSource { return &NodeJournalDataSource{} }

func (d *NodeSyslogDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_syslog"
}

func (d *NodeJournalDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_journal"
}

func (d *NodeSyslogDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads node syslog entries from `GET /nodes/{node}/syslog`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":    datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"limit":   datasourceschema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Maximum entries, default 100."},
			"service": datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Only entries of this service."},
			"since":   datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Lower time bound."},
			"until":   datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Upper time bound."},
			"entries": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"line": datasourceschema.Int64Attribute{Computed: true},
				"text": datasourceschema.StringAttribute{Computed: true},
			}}},
		},
	}
}

func (d *NodeJournalDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads node journal lines from `GET /nodes/{node}/journal`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":    datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"limit":   datasourceschema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Maximum entries, default 100."},
			"service": datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Only entries of this service."},
			"since":   datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Lower time bound."},
			"until":   datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Upper time bound."},
			"lines":   datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *NodeSyslogDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *NodeJournalDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *NodeSyslogDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeSyslogModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	limit := int64(100)
	if !config.Limit.IsNull() && !config.Limit.IsUnknown() {
		limit = config.Limit.ValueInt64()
	}
	entries, err := d.client.NodeSyslog(ctx, config.Node.ValueString(), limit, stringValue(config.Service), stringValue(config.Since), stringValue(config.Until), 0)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Node Syslog", err.Error())
		return
	}
	records := make([]nodeSyslogEntryModel, 0, len(entries))
	for _, entry := range entries {
		records = append(records, nodeSyslogEntryModel{Line: types.Int64Value(entry.Line), Text: stringOrNull(entry.Text)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeSyslogModel{
		Node: config.Node, Limit: types.Int64Value(limit), Service: config.Service, Since: config.Since, Until: config.Until, Entries: records,
	})...)
}

func (d *NodeJournalDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeJournalModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	limit := int64(100)
	if !config.Limit.IsNull() && !config.Limit.IsUnknown() {
		limit = config.Limit.ValueInt64()
	}
	lines, err := d.client.NodeJournal(ctx, config.Node.ValueString(), limit, stringValue(config.Service), stringValue(config.Since), stringValue(config.Until))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Node Journal", err.Error())
		return
	}
	value, valueDiags := types.ListValueFrom(ctx, types.StringType, lines)
	resp.Diagnostics.Append(valueDiags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeJournalModel{
		Node: config.Node, Limit: types.Int64Value(limit), Service: config.Service, Since: config.Since, Until: config.Until, Lines: value,
	})...)
}
