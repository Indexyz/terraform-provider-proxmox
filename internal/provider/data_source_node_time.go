// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &NodeTimeDataSource{}

type NodeTimeDataSource struct {
	client *Client
}

type NodeTimeDataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	Node      types.String `tfsdk:"node"`
	LocalTime types.Int64  `tfsdk:"local_time"`
	Time      types.Int64  `tfsdk:"time"`
	Timezone  types.String `tfsdk:"timezone"`
}

func NewNodeTimeDataSource() datasource.DataSource {
	return &NodeTimeDataSource{}
}

func (d *NodeTimeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_time"
}

func (d *NodeTimeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads server time and timezone settings from `/nodes/{node}/time`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster node name.",
			},
			"local_time": schema.Int64Attribute{Computed: true},
			"time":       schema.Int64Attribute{Computed: true},
			"timezone":   schema.StringAttribute{Computed: true},
		},
	}
}

func (d *NodeTimeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", err.Error())
		return
	}

	d.client = client
}

func (d *NodeTimeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NodeTimeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nodeTime, err := d.client.NodeTime(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Node Time", fmt.Sprintf("Unable to call `/nodes/%s/time`: %s", config.Node.ValueString(), err))
		return
	}

	state := NodeTimeDataSourceModel{
		ID:        config.Node,
		Node:      config.Node,
		LocalTime: types.Int64Value(nodeTime.LocalTime),
		Time:      types.Int64Value(nodeTime.Time),
		Timezone:  stringOrNull(nodeTime.Timezone),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
