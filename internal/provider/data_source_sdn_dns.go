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

var _ datasource.DataSource = &SDNDNSDataSource{}

type SDNDNSDataSource struct {
	client *Client
}

type sdnDNSDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	DNS           types.String `tfsdk:"dns"`
	Type          types.String `tfsdk:"type"`
	URL           types.String `tfsdk:"url"`
	TTL           types.Int64  `tfsdk:"ttl"`
	ReverseMaskV6 types.Int64  `tfsdk:"reverse_mask_v6"`
	Fingerprint   types.String `tfsdk:"fingerprint"`
	State         types.String `tfsdk:"state"`
	Pending       types.Map    `tfsdk:"pending"`
}

func NewSDNDNSDataSource() datasource.DataSource { return &SDNDNSDataSource{} }

func (d *SDNDNSDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_dns"
}

func (d *SDNDNSDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads one staged SDN DNS provider from `/cluster/sdn/dns`. The provider key is not exposed, only its fingerprint.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":              datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "DNS provider identifier."},
			"dns":             datasourceschema.StringAttribute{Required: true, MarkdownDescription: "DNS provider to read."},
			"type":            datasourceschema.StringAttribute{Computed: true},
			"url":             datasourceschema.StringAttribute{Computed: true},
			"ttl":             datasourceschema.Int64Attribute{Computed: true},
			"reverse_mask_v6": datasourceschema.Int64Attribute{Computed: true},
			"fingerprint":     datasourceschema.StringAttribute{Computed: true},
			"state":           datasourceschema.StringAttribute{Computed: true},
			"pending":         datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *SDNDNSDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNDNSDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnDNSDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := config.DNS.ValueString()
	entry, err := d.client.GetSDNDNS(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN DNS Provider", fmt.Sprintf("Unable to call `/cluster/sdn/dns`: %s", err))
		return
	}
	staged := sdnPendingMap(entry.Pending)
	pending := types.MapNull(types.StringType)
	if len(staged) > 0 {
		value, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		resp.Diagnostics.Append(pendingDiags...)
		pending = value
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnDNSDataSourceModel{
		ID:            types.StringValue(name),
		DNS:           types.StringValue(entry.DNS),
		Type:          sdnStagedString(entry.Type, staged["type"]),
		URL:           sdnStagedString(entry.URL, staged["url"]),
		TTL:           sdnStagedInt64(entry.TTL.Ptr(), staged["ttl"]),
		ReverseMaskV6: sdnStagedInt64(entry.ReverseMaskV6.Ptr(), staged["reversemaskv6"]),
		Fingerprint:   sdnStagedString(entry.Fingerprint, staged["fingerprint"]),
		State:         stringOrNull(entry.State),
		Pending:       pending,
	})...)
}
