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

var _ datasource.DataSource = &SDNVnetDataSource{}

type SDNVnetDataSource struct {
	client *Client
}

type sdnVnetDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Vnet         types.String `tfsdk:"vnet"`
	Zone         types.String `tfsdk:"zone"`
	Alias        types.String `tfsdk:"alias"`
	Tag          types.Int64  `tfsdk:"tag"`
	VLANAware    types.Bool   `tfsdk:"vlanaware"`
	IsolatePorts types.Bool   `tfsdk:"isolate_ports"`
	State        types.String `tfsdk:"state"`
	Pending      types.Map    `tfsdk:"pending"`
}

func NewSDNVnetDataSource() datasource.DataSource { return &SDNVnetDataSource{} }

func (d *SDNVnetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_vnet"
}

func (d *SDNVnetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads one staged SDN VNet from `/cluster/sdn/vnets/{vnet}`. Staged values overlay the running config; fields without a typed counterpart stay in `pending`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":            datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "VNet identifier."},
			"vnet":          datasourceschema.StringAttribute{Required: true, MarkdownDescription: "VNet to read."},
			"zone":          datasourceschema.StringAttribute{Computed: true},
			"alias":         datasourceschema.StringAttribute{Computed: true},
			"tag":           datasourceschema.Int64Attribute{Computed: true},
			"vlanaware":     datasourceschema.BoolAttribute{Computed: true},
			"isolate_ports": datasourceschema.BoolAttribute{Computed: true},
			"state":         datasourceschema.StringAttribute{Computed: true},
			"pending":       datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *SDNVnetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNVnetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnVnetDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vnet := config.Vnet.ValueString()
	entry, err := d.client.GetSDNVnet(ctx, vnet)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN VNet", fmt.Sprintf("Unable to call `/cluster/sdn/vnets/%s`: %s", vnet, err))
		return
	}
	staged := sdnPendingMap(entry.Pending)
	pending := types.MapNull(types.StringType)
	if len(staged) > 0 {
		value, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		resp.Diagnostics.Append(pendingDiags...)
		pending = value
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnVnetDataSourceModel{
		ID:           types.StringValue(vnet),
		Vnet:         types.StringValue(entry.Vnet),
		Zone:         sdnStagedString(entry.Zone, staged["zone"]),
		Alias:        sdnStagedString(entry.Alias, staged["alias"]),
		Tag:          sdnStagedInt64(entry.Tag.Ptr(), staged["tag"]),
		VLANAware:    sdnStagedBool(entry.VLANAware.Ptr(), staged["vlanaware"]),
		IsolatePorts: sdnStagedBool(entry.IsolatePorts.Ptr(), staged["isolate-ports"]),
		State:        stringOrNull(entry.State),
		Pending:      pending,
	})...)
}
