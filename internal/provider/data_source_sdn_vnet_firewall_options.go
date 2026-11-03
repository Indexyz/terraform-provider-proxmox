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

var _ datasource.DataSource = &SDNVnetFirewallOptionsDataSource{}

type SDNVnetFirewallOptionsDataSource struct {
	client *Client
}

type sdnVnetFirewallOptionsDataSourceModel struct {
	ID              types.String `tfsdk:"id"`
	Vnet            types.String `tfsdk:"vnet"`
	Enable          types.Bool   `tfsdk:"enable"`
	PolicyForward   types.String `tfsdk:"policy_forward"`
	LogLevelForward types.String `tfsdk:"log_level_forward"`
}

func NewSDNVnetFirewallOptionsDataSource() datasource.DataSource {
	return &SDNVnetFirewallOptionsDataSource{}
}

func (d *SDNVnetFirewallOptionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_vnet_firewall_options"
}

func (d *SDNVnetFirewallOptionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the firewall options of one SDN VNet from `/cluster/sdn/vnets/{vnet}/firewall/options`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":                datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "VNet identifier."},
			"vnet":              datasourceschema.StringAttribute{Required: true, MarkdownDescription: "VNet whose options are read."},
			"enable":            datasourceschema.BoolAttribute{Computed: true},
			"policy_forward":    datasourceschema.StringAttribute{Computed: true},
			"log_level_forward": datasourceschema.StringAttribute{Computed: true},
		},
	}
}

func (d *SDNVnetFirewallOptionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNVnetFirewallOptionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnVnetFirewallOptionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vnet := config.Vnet.ValueString()
	entry, err := d.client.GetSDNVnetFirewallOptions(ctx, vnet)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN VNet Firewall Options", fmt.Sprintf("Unable to call `/cluster/sdn/vnets/%s/firewall/options`: %s", vnet, err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnVnetFirewallOptionsDataSourceModel{
		ID:              types.StringValue(vnet),
		Vnet:            types.StringValue(vnet),
		Enable:          nodesBoolOrNull(entry.Enable),
		PolicyForward:   stringOrNull(entry.PolicyForward),
		LogLevelForward: stringOrNull(entry.LogLevelForward),
	})...)
}
