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

var _ datasource.DataSource = &SDNSubnetDataSource{}

type SDNSubnetDataSource struct {
	client *Client
}

type sdnSubnetDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	Vnet          types.String `tfsdk:"vnet"`
	Subnet        types.String `tfsdk:"subnet"`
	Gateway       types.String `tfsdk:"gateway"`
	SNAT          types.Bool   `tfsdk:"snat"`
	DNSZonePrefix types.String `tfsdk:"dns_zone_prefix"`
	State         types.String `tfsdk:"state"`
	Pending       types.Map    `tfsdk:"pending"`
}

func NewSDNSubnetDataSource() datasource.DataSource { return &SDNSubnetDataSource{} }

func (d *SDNSubnetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_subnet"
}

func (d *SDNSubnetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads one staged SDN subnet from `/cluster/sdn/vnets/{vnet}/subnets/{subnet}`. Subnets are only addressable inside their VNet.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":              datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Subnet identifier (`<vnet>/<subnet>`)."},
			"vnet":            datasourceschema.StringAttribute{Required: true, MarkdownDescription: "VNet that owns the subnet."},
			"subnet":          datasourceschema.StringAttribute{Required: true, MarkdownDescription: "CIDR of the subnet to read."},
			"gateway":         datasourceschema.StringAttribute{Computed: true},
			"snat":            datasourceschema.BoolAttribute{Computed: true},
			"dns_zone_prefix": datasourceschema.StringAttribute{Computed: true},
			"state":           datasourceschema.StringAttribute{Computed: true},
			"pending":         datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *SDNSubnetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNSubnetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnSubnetDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vnet := config.Vnet.ValueString()
	subnet := config.Subnet.ValueString()
	entry, err := d.client.GetSDNSubnet(ctx, vnet, subnet)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN Subnet", fmt.Sprintf("Unable to call `/cluster/sdn/vnets/%s/subnets/%s`: %s", vnet, subnet, err))
		return
	}
	staged := sdnPendingMap(entry.Pending)
	pending := types.MapNull(types.StringType)
	if len(staged) > 0 {
		value, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		resp.Diagnostics.Append(pendingDiags...)
		pending = value
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnSubnetDataSourceModel{
		ID:            types.StringValue(vnet + "/" + subnet),
		Vnet:          types.StringValue(vnet),
		Subnet:        types.StringValue(subnet),
		Gateway:       sdnStagedString(entry.Gateway, staged["gateway"]),
		SNAT:          sdnStagedBool(entry.SNAT.Ptr(), staged["snat"]),
		DNSZonePrefix: sdnStagedString(entry.DNSZonePrefix, staged["dnszoneprefix"]),
		State:         stringOrNull(entry.State),
		Pending:       pending,
	})...)
}
