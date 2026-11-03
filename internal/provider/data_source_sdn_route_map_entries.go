// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &SDNRouteMapEntriesDataSource{}

type SDNRouteMapEntriesDataSource struct {
	client *Client
}

type sdnRouteMapEntriesDataSourceModel struct {
	ID         types.String                         `tfsdk:"id"`
	RouteMapID types.String                         `tfsdk:"route_map_id"`
	Entries    []sdnRouteMapEntriesDataSourceRecord `tfsdk:"entries"`
}

type sdnRouteMapEntriesDataSourceRecord struct {
	RouteMapID types.String `tfsdk:"route_map_id"`
	Order      types.Int64  `tfsdk:"order"`
	Action     types.String `tfsdk:"action"`
	State      types.String `tfsdk:"state"`
	Pending    types.Map    `tfsdk:"pending"`
}

func NewSDNRouteMapEntriesDataSource() datasource.DataSource { return &SDNRouteMapEntriesDataSource{} }

func (d *SDNRouteMapEntriesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_route_map_entries"
}

func (d *SDNRouteMapEntriesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists staged SDN route map entries from `/cluster/sdn/route-maps/entries`, sorted by route map then order. Requires `route_map_id` because orders are only unique per route map.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":           datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"route_map_id": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Route map whose entries are listed."},
			"entries": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching entries, sorted by `order`.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"route_map_id": datasourceschema.StringAttribute{Computed: true},
						"order":        datasourceschema.Int64Attribute{Computed: true},
						"action":       datasourceschema.StringAttribute{Computed: true},
						"state":        datasourceschema.StringAttribute{Computed: true},
						"pending":      datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
					},
				},
			},
		},
	}
}

func (d *SDNRouteMapEntriesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNRouteMapEntriesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnRouteMapEntriesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entries, err := d.client.SDNRouteMapEntries(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN Route Map Entries", fmt.Sprintf("Unable to call `/cluster/sdn/route-maps/entries`: %s", err))
		return
	}
	filter := config.RouteMapID.ValueString()
	records := make([]sdnRouteMapEntriesDataSourceRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.RouteMapID != filter {
			continue
		}
		staged := sdnPendingMap(entry.Pending)
		record := sdnRouteMapEntriesDataSourceRecord{
			RouteMapID: types.StringValue(entry.RouteMapID),
			Order:      sdnStagedInt64(entry.Order.Ptr(), ""),
			Action:     sdnStagedString(entry.Action, staged["action"]),
			State:      stringOrNull(entry.State),
			Pending:    types.MapNull(types.StringType),
		}
		if len(staged) > 0 {
			pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
			resp.Diagnostics.Append(pendingDiags...)
			record.Pending = pending
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Order.ValueInt64() < records[j].Order.ValueInt64()
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnRouteMapEntriesDataSourceModel{ID: types.StringValue("sdn_route_map_entries/" + filter), RouteMapID: config.RouteMapID, Entries: records})...)
}
