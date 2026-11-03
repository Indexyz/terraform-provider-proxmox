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

var _ datasource.DataSource = &SDNVnetsDataSource{}

type SDNVnetsDataSource struct {
	client *Client
}

type sdnVnetsDataSourceModel struct {
	ID    types.String               `tfsdk:"id"`
	Zone  types.String               `tfsdk:"zone"`
	Vnets []sdnVnetsDataSourceRecord `tfsdk:"vnets"`
}

type sdnVnetsDataSourceRecord struct {
	Vnet      types.String `tfsdk:"vnet"`
	Zone      types.String `tfsdk:"zone"`
	Alias     types.String `tfsdk:"alias"`
	Tag       types.Int64  `tfsdk:"tag"`
	VLANAware types.Bool   `tfsdk:"vlanaware"`
	State     types.String `tfsdk:"state"`
	Pending   types.Map    `tfsdk:"pending"`
}

func NewSDNVnetsDataSource() datasource.DataSource { return &SDNVnetsDataSource{} }

func (d *SDNVnetsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_vnets"
}

func (d *SDNVnetsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists staged SDN VNets from `/cluster/sdn/vnets`, sorted by `vnet`. `state` and `pending` show what is still uncommitted, so a forgotten `proxmox_sdn_apply` stays visible.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"zone": datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Restrict the result to one zone. An empty value disables this filter."},
			"vnets": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching VNets, sorted by `vnet`.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"vnet":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "VNet identifier."},
						"zone":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Parent zone."},
						"alias":     datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Alias, when set."},
						"tag":       datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "VLAN tag or VNI, when set."},
						"vlanaware": datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether VLANs may pass through."},
						"state":     datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Staging state: `new` or `changed`."},
						"pending":   datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Staged properties with `deleted` for removals."},
					},
				},
			},
		},
	}
}

func (d *SDNVnetsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNVnetsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnVnetsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vnets, err := d.client.SDNVnets(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN VNets", fmt.Sprintf("Unable to call `/cluster/sdn/vnets`: %s", err))
		return
	}
	filter := stringValue(config.Zone)
	records := make([]sdnVnetsDataSourceRecord, 0, len(vnets))
	for _, vnet := range vnets {
		staged := sdnPendingMap(vnet.Pending)
		zone := sdnStagedString(vnet.Zone, staged["zone"])
		if filter != "" && zone.ValueString() != filter {
			continue
		}
		record := sdnVnetsDataSourceRecord{
			Vnet:      types.StringValue(vnet.Vnet),
			Zone:      zone,
			Alias:     sdnStagedString(vnet.Alias, staged["alias"]),
			Tag:       sdnStagedInt64(vnet.Tag.Ptr(), staged["tag"]),
			VLANAware: sdnStagedBool(vnet.VLANAware.Ptr(), staged["vlanaware"]),
			State:     stringOrNull(vnet.State),
			Pending:   types.MapNull(types.StringType),
		}
		if len(staged) > 0 {
			pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
			resp.Diagnostics.Append(pendingDiags...)
			record.Pending = pending
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Vnet.ValueString() < records[j].Vnet.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnVnetsDataSourceModel{ID: types.StringValue("sdn_vnets"), Zone: config.Zone, Vnets: records})...)
}
