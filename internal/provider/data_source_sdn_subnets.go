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

var _ datasource.DataSource = &SDNSubnetsDataSource{}

type SDNSubnetsDataSource struct {
	client *Client
}

type sdnSubnetsDataSourceModel struct {
	ID      types.String                 `tfsdk:"id"`
	VNet    types.String                 `tfsdk:"vnet"`
	Subnets []sdnSubnetsDataSourceRecord `tfsdk:"subnets"`
}

type sdnSubnetsDataSourceRecord struct {
	VNet    types.String `tfsdk:"vnet"`
	Subnet  types.String `tfsdk:"subnet"`
	Gateway types.String `tfsdk:"gateway"`
	State   types.String `tfsdk:"state"`
	Pending types.Map    `tfsdk:"pending"`
}

func NewSDNSubnetsDataSource() datasource.DataSource { return &SDNSubnetsDataSource{} }

func (d *SDNSubnetsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_subnets"
}

func (d *SDNSubnetsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists staged subnets of one SDN VNet, sorted by `subnet`. Requires `vnet` because subnets are only addressable per VNet.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"vnet": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "VNet whose subnets are listed."},
			"subnets": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching subnets, sorted by `subnet`.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"vnet":    datasourceschema.StringAttribute{Computed: true},
						"subnet":  datasourceschema.StringAttribute{Computed: true},
						"gateway": datasourceschema.StringAttribute{Computed: true},
						"state":   datasourceschema.StringAttribute{Computed: true},
						"pending": datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
					},
				},
			},
		},
	}
}

func (d *SDNSubnetsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNSubnetsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnSubnetsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vnet := config.VNet.ValueString()
	subnets, err := d.client.SDNSubnets(ctx, vnet)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN Subnets", fmt.Sprintf("Unable to call `/cluster/sdn/vnets/%s/subnets`: %s", vnet, err))
		return
	}
	records := make([]sdnSubnetsDataSourceRecord, 0, len(subnets))
	for _, subnet := range subnets {
		staged := sdnPendingMap(subnet.Pending)
		cidr := subnet.CIDR
		if cidr == "" {
			cidr = subnet.Subnet
		}
		subnetVnet := subnet.Vnet
		if subnetVnet == "" {
			subnetVnet = vnet
		}
		record := sdnSubnetsDataSourceRecord{
			VNet:    types.StringValue(subnetVnet),
			Subnet:  types.StringValue(cidr),
			Gateway: sdnStagedString(subnet.Gateway, staged["gateway"]),
			State:   stringOrNull(subnet.State),
			Pending: types.MapNull(types.StringType),
		}
		if len(staged) > 0 {
			pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
			resp.Diagnostics.Append(pendingDiags...)
			record.Pending = pending
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Subnet.ValueString() < records[j].Subnet.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnSubnetsDataSourceModel{ID: types.StringValue("sdn_subnets/" + vnet), VNet: config.VNet, Subnets: records})...)
}
