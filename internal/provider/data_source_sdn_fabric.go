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

var _ datasource.DataSource = &SDNFabricDataSource{}

type SDNFabricDataSource struct {
	client *Client
}

type sdnFabricDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	FabricID     types.String `tfsdk:"fabric_id"`
	Protocol     types.String `tfsdk:"protocol"`
	IPPrefix     types.String `tfsdk:"ip_prefix"`
	RouteMapIn   types.String `tfsdk:"route_map_in"`
	RouteMapOut  types.String `tfsdk:"route_map_out"`
	Redistribute types.List   `tfsdk:"redistribute"`
	State        types.String `tfsdk:"state"`
	Pending      types.Map    `tfsdk:"pending"`
}

func NewSDNFabricDataSource() datasource.DataSource { return &SDNFabricDataSource{} }

func (d *SDNFabricDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_fabric"
}

func (d *SDNFabricDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads one staged SDN fabric from `/cluster/sdn/fabrics/fabric/{fabric_id}`. Fields without a typed counterpart stay in `pending`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":            datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Fabric identifier."},
			"fabric_id":     datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Fabric to read."},
			"protocol":      datasourceschema.StringAttribute{Computed: true},
			"ip_prefix":     datasourceschema.StringAttribute{Computed: true},
			"route_map_in":  datasourceschema.StringAttribute{Computed: true},
			"route_map_out": datasourceschema.StringAttribute{Computed: true},
			"redistribute":  datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
			"state":         datasourceschema.StringAttribute{Computed: true},
			"pending":       datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *SDNFabricDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNFabricDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnFabricDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := config.FabricID.ValueString()
	entry, err := d.client.GetSDNFabric(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN Fabric", fmt.Sprintf("Unable to call `/cluster/sdn/fabrics/fabric/%s`: %s", id, err))
		return
	}
	staged := sdnPendingMap(entry.Pending)
	redistribute := types.ListNull(types.StringType)
	if list, listDiags := types.ListValueFrom(ctx, types.StringType, sdnStagedRawList(entry.Redistribute, staged["redistribute"])); listDiags.HasError() {
		resp.Diagnostics.Append(listDiags...)
	} else {
		redistribute = list
	}
	pending := types.MapNull(types.StringType)
	if len(staged) > 0 {
		value, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		resp.Diagnostics.Append(pendingDiags...)
		pending = value
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnFabricDataSourceModel{
		ID:           types.StringValue(id),
		FabricID:     types.StringValue(entry.ID),
		Protocol:     sdnStagedString(entry.Protocol, staged["protocol"]),
		IPPrefix:     sdnStagedString(entry.IPPrefix, staged["ip_prefix"]),
		RouteMapIn:   sdnStagedString(entry.RouteMapIn, staged["route_map_in"]),
		RouteMapOut:  sdnStagedString(entry.RouteMapOut, staged["route_map_out"]),
		Redistribute: redistribute,
		State:        stringOrNull(entry.State),
		Pending:      pending,
	})...)
}
