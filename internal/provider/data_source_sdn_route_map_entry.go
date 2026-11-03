// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &SDNRouteMapEntryDataSource{}

type SDNRouteMapEntryDataSource struct {
	client *Client
}

type sdnRouteMapEntryDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	RouteMapID types.String `tfsdk:"route_map_id"`
	Order      types.Int64  `tfsdk:"order"`
	Action     types.String `tfsdk:"action"`
	Set        types.List   `tfsdk:"set"`
	Match      types.List   `tfsdk:"match"`
	ExitAction types.String `tfsdk:"exit_action"`
	Call       types.String `tfsdk:"call"`
	State      types.String `tfsdk:"state"`
	Pending    types.Map    `tfsdk:"pending"`
}

func NewSDNRouteMapEntryDataSource() datasource.DataSource { return &SDNRouteMapEntryDataSource{} }

func (d *SDNRouteMapEntryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_route_map_entry"
}

func (d *SDNRouteMapEntryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads one staged SDN route map entry from `/cluster/sdn/route-maps/entries/{route_map_id}/{order}`. Entries stay Proxmox property strings.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":           datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Entry identifier (`<route_map_id>/<order>`)."},
			"route_map_id": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Route map that owns the entry."},
			"order":        datasourceschema.Int64Attribute{Required: true, MarkdownDescription: "Order of the entry inside the route map."},
			"action":       datasourceschema.StringAttribute{Computed: true},
			"set":          datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
			"match":        datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
			"exit_action":  datasourceschema.StringAttribute{Computed: true},
			"call":         datasourceschema.StringAttribute{Computed: true},
			"state":        datasourceschema.StringAttribute{Computed: true},
			"pending":      datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *SDNRouteMapEntryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNRouteMapEntryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnRouteMapEntryDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	routeMapID := config.RouteMapID.ValueString()
	order := config.Order.ValueInt64()
	entry, err := d.client.GetSDNRouteMapEntry(ctx, routeMapID, order)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN Route Map Entry", fmt.Sprintf("Unable to call `/cluster/sdn/route-maps/entries`: %s", err))
		return
	}
	staged := sdnPendingMap(entry.Pending)
	setEntries, setDiags := types.ListValueFrom(ctx, types.StringType, sdnStagedRawList(entry.Set, staged["set"]))
	resp.Diagnostics.Append(setDiags...)
	matchEntries, matchDiags := types.ListValueFrom(ctx, types.StringType, sdnStagedRawList(entry.Match, staged["match"]))
	resp.Diagnostics.Append(matchDiags...)
	pending := types.MapNull(types.StringType)
	if len(staged) > 0 {
		value, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		resp.Diagnostics.Append(pendingDiags...)
		pending = value
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnRouteMapEntryDataSourceModel{
		ID:         types.StringValue(fmt.Sprintf("%s/%d", routeMapID, order)),
		RouteMapID: types.StringValue(routeMapID),
		Order:      types.Int64Value(order),
		Action:     sdnStagedString(entry.Action, staged["action"]),
		Set:        setEntries,
		Match:      matchEntries,
		ExitAction: sdnStagedString(entry.ExitAction, staged["exit-action"]),
		Call:       sdnStagedString(entry.Call, staged["call"]),
		State:      stringOrNull(entry.State),
		Pending:    pending,
	})...)
}
