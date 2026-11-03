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

var _ datasource.DataSource = &SDNZoneDataSource{}

type SDNZoneDataSource struct {
	client *Client
}

type sdnZoneDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	Zone       types.String `tfsdk:"zone"`
	Type       types.String `tfsdk:"type"`
	Bridge     types.String `tfsdk:"bridge"`
	MTU        types.Int64  `tfsdk:"mtu"`
	Nodes      types.List   `tfsdk:"nodes"`
	DNSZone    types.String `tfsdk:"dns_zone_prefix"`
	IPAM       types.String `tfsdk:"ipam"`
	ReverseDNS types.String `tfsdk:"reverse_dns"`
	State      types.String `tfsdk:"state"`
	Pending    types.Map    `tfsdk:"pending"`
}

func NewSDNZoneDataSource() datasource.DataSource { return &SDNZoneDataSource{} }

func (d *SDNZoneDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_zone"
}

func (d *SDNZoneDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads one staged SDN zone from `/cluster/sdn/zones/{zone}`. Staged values overlay the running config; fields without a typed counterpart stay in `pending`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":              datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Zone identifier."},
			"zone":            datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Zone to read."},
			"type":            datasourceschema.StringAttribute{Computed: true},
			"bridge":          datasourceschema.StringAttribute{Computed: true},
			"mtu":             datasourceschema.Int64Attribute{Computed: true},
			"nodes":           datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
			"dns_zone_prefix": datasourceschema.StringAttribute{Computed: true},
			"ipam":            datasourceschema.StringAttribute{Computed: true},
			"reverse_dns":     datasourceschema.StringAttribute{Computed: true},
			"state":           datasourceschema.StringAttribute{Computed: true},
			"pending":         datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *SDNZoneDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNZoneDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnZoneDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	zone := config.Zone.ValueString()
	entry, err := d.client.GetSDNZone(ctx, zone)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN Zone", fmt.Sprintf("Unable to call `/cluster/sdn/zones/%s`: %s", zone, err))
		return
	}
	staged := sdnPendingMap(entry.Pending)
	nodes, nodeDiags := types.ListValueFrom(ctx, types.StringType, sdnStagedNodes(entry.Nodes, staged))
	resp.Diagnostics.Append(nodeDiags...)
	pending := types.MapNull(types.StringType)
	if len(staged) > 0 {
		value, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		resp.Diagnostics.Append(pendingDiags...)
		pending = value
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnZoneDataSourceModel{
		ID:         types.StringValue(zone),
		Zone:       types.StringValue(entry.Zone),
		Type:       types.StringValue(entry.Type),
		Bridge:     sdnStagedString(entry.Bridge, staged["bridge"]),
		MTU:        sdnStagedInt64(entry.MTU.Ptr(), staged["mtu"]),
		Nodes:      nodes,
		DNSZone:    sdnStagedString(entry.DNSZone, staged["dnszone"]),
		IPAM:       sdnStagedString(entry.IPAM, staged["ipam"]),
		ReverseDNS: sdnStagedString(entry.ReverseDNS, staged["reversedns"]),
		State:      stringOrNull(entry.State),
		Pending:    pending,
	})...)
}
