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

var _ datasource.DataSource = &SDNZonesDataSource{}

type SDNZonesDataSource struct {
	client *Client
}

type sdnZonesDataSourceModel struct {
	ID    types.String               `tfsdk:"id"`
	Type  types.String               `tfsdk:"type"`
	Zones []sdnZonesDataSourceRecord `tfsdk:"zones"`
}

type sdnZonesDataSourceRecord struct {
	Zone    types.String `tfsdk:"zone"`
	Type    types.String `tfsdk:"type"`
	Nodes   types.List   `tfsdk:"nodes"`
	MTU     types.Int64  `tfsdk:"mtu"`
	Bridge  types.String `tfsdk:"bridge"`
	IPAM    types.String `tfsdk:"ipam"`
	State   types.String `tfsdk:"state"`
	Pending types.Map    `tfsdk:"pending"`
}

func NewSDNZonesDataSource() datasource.DataSource { return &SDNZonesDataSource{} }

func (d *SDNZonesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_zones"
}

func (d *SDNZonesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists staged SDN zones from `/cluster/sdn/zones`, sorted by `zone`. `state` and `pending` show what is still uncommitted, so a forgotten `proxmox_sdn_apply` stays visible.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"type": datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Restrict the result to one zone type. An empty value disables this filter."},
			"zones": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching zones, sorted by `zone`.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"zone":    datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Zone identifier."},
						"type":    datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Zone type."},
						"nodes":   datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Nodes the zone is created on."},
						"mtu":     datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Zone MTU, when set."},
						"bridge":  datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Bridge of a `vlan`/`qinq` zone."},
						"ipam":    datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "IPAM id, when set."},
						"state":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Staging state: `new` or `changed`."},
						"pending": datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Staged properties with `deleted` for removals."},
					},
				},
			},
		},
	}
}

func (d *SDNZonesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNZonesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnZonesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	zones, err := d.client.SDNZones(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN Zones", fmt.Sprintf("Unable to call `/cluster/sdn/zones`: %s", err))
		return
	}
	filter := stringValue(config.Type)
	records := make([]sdnZonesDataSourceRecord, 0, len(zones))
	for _, zone := range zones {
		if filter != "" && zone.Type != filter {
			continue
		}
		staged := sdnPendingMap(zone.Pending)
		nodes, nodeDiags := types.ListValueFrom(ctx, types.StringType, sdnStagedNodes(zone.Nodes, staged))
		resp.Diagnostics.Append(nodeDiags...)
		record := sdnZonesDataSourceRecord{
			Zone:    types.StringValue(zone.Zone),
			Type:    types.StringValue(zone.Type),
			Nodes:   nodes,
			MTU:     sdnStagedInt64(zone.MTU.Ptr(), staged["mtu"]),
			Bridge:  sdnStagedString(zone.Bridge, staged["bridge"]),
			IPAM:    sdnStagedString(zone.IPAM, staged["ipam"]),
			State:   stringOrNull(zone.State),
			Pending: types.MapNull(types.StringType),
		}
		if len(staged) > 0 {
			pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
			resp.Diagnostics.Append(pendingDiags...)
			record.Pending = pending
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Zone.ValueString() < records[j].Zone.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnZonesDataSourceModel{ID: types.StringValue("sdn_zones"), Type: config.Type, Zones: records})...)
}
