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

var _ datasource.DataSource = &ClusterMetricsServersDataSource{}

type ClusterMetricsServersDataSource struct {
	client *Client
}

type ClusterMetricsServersDataSourceModel struct {
	ID      types.String                            `tfsdk:"id"`
	Servers []ClusterMetricsServersDataSourceRecord `tfsdk:"servers"`
}

type ClusterMetricsServersDataSourceRecord struct {
	Disable types.Bool   `tfsdk:"disable"`
	ID      types.String `tfsdk:"id"`
	Port    types.Int64  `tfsdk:"port"`
	Server  types.String `tfsdk:"server"`
	Type    types.String `tfsdk:"type"`
}

func NewClusterMetricsServersDataSource() datasource.DataSource {
	return &ClusterMetricsServersDataSource{}
}

func (d *ClusterMetricsServersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_metrics_servers"
}

func (d *ClusterMetricsServersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists configured cluster metrics servers from `/cluster/metrics/server`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"servers": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"disable": schema.BoolAttribute{Computed: true},
						"id":      schema.StringAttribute{Computed: true},
						"port":    schema.Int64Attribute{Computed: true},
						"server":  schema.StringAttribute{Computed: true},
						"type":    schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *ClusterMetricsServersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ClusterMetricsServersDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	servers, err := d.client.ClusterMetricsServers(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Cluster Metrics Servers", fmt.Sprintf("Unable to call `/cluster/metrics/server`: %s", err))
		return
	}

	items := make([]ClusterMetricsServersDataSourceRecord, 0, len(servers))
	for _, server := range servers {
		items = append(items, ClusterMetricsServersDataSourceRecord{
			Disable: boolOrNull(server.Disable),
			ID:      stringOrNull(server.ID),
			Port:    int64OrNull(server.Port),
			Server:  stringOrNull(server.Server),
			Type:    stringOrNull(server.Type),
		})
	}

	state := ClusterMetricsServersDataSourceModel{
		ID:      types.StringValue("cluster_metrics_servers"),
		Servers: items,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
