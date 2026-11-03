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

var _ datasource.DataSource = &SDNControllerDataSource{}

type SDNControllerDataSource struct {
	client *Client
}

type sdnControllerDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Controller  types.String `tfsdk:"controller"`
	Type        types.String `tfsdk:"type"`
	ASN         types.Int64  `tfsdk:"asn"`
	Node        types.String `tfsdk:"node"`
	Peers       types.String `tfsdk:"peers"`
	Nodes       types.List   `tfsdk:"nodes"`
	Fabric      types.String `tfsdk:"fabric"`
	Loopback    types.String `tfsdk:"loopback"`
	RouteMapIn  types.String `tfsdk:"route_map_in"`
	RouteMapOut types.String `tfsdk:"route_map_out"`
	State       types.String `tfsdk:"state"`
	Pending     types.Map    `tfsdk:"pending"`
}

func NewSDNControllerDataSource() datasource.DataSource { return &SDNControllerDataSource{} }

func (d *SDNControllerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_controller"
}

func (d *SDNControllerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads one staged SDN controller from `/cluster/sdn/controllers`. Fields without a typed counterpart stay in `pending`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":            datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Controller identifier."},
			"controller":    datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Controller to read."},
			"type":          datasourceschema.StringAttribute{Computed: true},
			"asn":           datasourceschema.Int64Attribute{Computed: true},
			"node":          datasourceschema.StringAttribute{Computed: true},
			"peers":         datasourceschema.StringAttribute{Computed: true},
			"nodes":         datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
			"fabric":        datasourceschema.StringAttribute{Computed: true},
			"loopback":      datasourceschema.StringAttribute{Computed: true},
			"route_map_in":  datasourceschema.StringAttribute{Computed: true},
			"route_map_out": datasourceschema.StringAttribute{Computed: true},
			"state":         datasourceschema.StringAttribute{Computed: true},
			"pending":       datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *SDNControllerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNControllerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnControllerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := config.Controller.ValueString()
	entry, err := d.client.GetSDNController(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN Controller", fmt.Sprintf("Unable to call `/cluster/sdn/controllers`: %s", err))
		return
	}
	staged := sdnPendingMap(entry.Pending)
	nodes := types.ListNull(types.StringType)
	if list, listDiags := types.ListValueFrom(ctx, types.StringType, sdnStagedRawList(sdnZoneNodeList(entry.Nodes), staged["nodes"])); listDiags.HasError() {
		resp.Diagnostics.Append(listDiags...)
	} else {
		nodes = list
	}
	pending := types.MapNull(types.StringType)
	if len(staged) > 0 {
		value, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		resp.Diagnostics.Append(pendingDiags...)
		pending = value
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnControllerDataSourceModel{
		ID:          types.StringValue(name),
		Controller:  types.StringValue(entry.Controller),
		Type:        sdnStagedString(entry.Type, staged["type"]),
		ASN:         sdnStagedInt64(entry.ASN.Ptr(), staged["asn"]),
		Node:        sdnStagedString(entry.Node, staged["node"]),
		Peers:       sdnStagedString(entry.Peers, staged["peers"]),
		Nodes:       nodes,
		Fabric:      sdnStagedString(entry.Fabric, staged["fabric"]),
		Loopback:    sdnStagedString(entry.Loopback, staged["loopback"]),
		RouteMapIn:  sdnStagedString(entry.RouteMapIn, staged["route-map-in"]),
		RouteMapOut: sdnStagedString(entry.RouteMapOut, staged["route-map-out"]),
		State:       stringOrNull(entry.State),
		Pending:     pending,
	})...)
}
