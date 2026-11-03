// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &NodeRRDDataDataSource{}
var _ datasource.DataSource = &QemuRRDDataDataSource{}

type NodeRRDDataDataSource struct{ client *Client }
type QemuRRDDataDataSource struct{ client *Client }

type nodeRRDDataModel struct {
	Node      types.String `tfsdk:"node"`
	Timeframe types.String `tfsdk:"timeframe"`
	CF        types.String `tfsdk:"cf"`
	Data      types.String `tfsdk:"data"`
}

type qemuRRDDataModel struct {
	Node      types.String `tfsdk:"node"`
	VMID      types.Int64  `tfsdk:"vmid"`
	Timeframe types.String `tfsdk:"timeframe"`
	Data      types.String `tfsdk:"data"`
}

func NewNodeRRDDataDataSource() datasource.DataSource { return &NodeRRDDataDataSource{} }
func NewQemuRRDDataDataSource() datasource.DataSource { return &QemuRRDDataDataSource{} }

func (d *NodeRRDDataDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_rrddata"
}

func (d *QemuRRDDataDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_rrddata"
}

func (d *NodeRRDDataDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads raw RRD metric points of a node from `GET /nodes/{node}/rrddata` and returns them as a JSON array.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":      datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"timeframe": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "`hour`, `day`, `week`, `month`, `year` or `decade`."},
			"cf":        datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Consolidation function: `AVERAGE` or `MAX`."},
			"data":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "JSON array of metric points."},
		},
	}
}

func (d *QemuRRDDataDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads raw RRD metric points of a QEMU VM from `GET /nodes/{node}/qemu/{vmid}/rrddata` and returns them as a JSON array.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":      datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"vmid":      datasourceschema.Int64Attribute{Required: true, MarkdownDescription: "QEMU VM ID."},
			"timeframe": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "`hour`, `day`, `week`, `month`, `year` or `decade`."},
			"data":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "JSON array of metric points."},
		},
	}
}

func (d *NodeRRDDataDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *QemuRRDDataDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *NodeRRDDataDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeRRDDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data, err := d.client.NodeRRDData(ctx, config.Node.ValueString(), config.Timeframe.ValueString(), stringValue(config.CF))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Node RRD Data", err.Error())
		return
	}
	config.Data = types.StringValue(data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *QemuRRDDataDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config qemuRRDDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data, err := d.client.QemuRRDData(ctx, config.Node.ValueString(), config.VMID.ValueInt64(), config.Timeframe.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox QEMU RRD Data", err.Error())
		return
	}
	config.Data = types.StringValue(data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
