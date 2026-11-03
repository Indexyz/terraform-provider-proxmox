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

var _ datasource.DataSource = &SDNDNSsDataSource{}

type SDNDNSsDataSource struct {
	client *Client
}

type sdnDNSsDataSourceModel struct {
	ID      types.String              `tfsdk:"id"`
	Type    types.String              `tfsdk:"type"`
	Servers []sdnDNSsDataSourceRecord `tfsdk:"servers"`
}

type sdnDNSsDataSourceRecord struct {
	DNS     types.String `tfsdk:"dns"`
	Type    types.String `tfsdk:"type"`
	URL     types.String `tfsdk:"url"`
	TTL     types.Int64  `tfsdk:"ttl"`
	State   types.String `tfsdk:"state"`
	Pending types.Map    `tfsdk:"pending"`
}

func NewSDNDNSsDataSource() datasource.DataSource { return &SDNDNSsDataSource{} }

func (d *SDNDNSsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_dns_providers"
}

func (d *SDNDNSsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists staged SDN DNS providers from `/cluster/sdn/dns`, sorted by `dns`. Provider keys are not exposed.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"type": datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Restrict the result to one DNS provider type. An empty value disables this filter."},
			"servers": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching DNS providers, sorted by `dns`.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"dns":     datasourceschema.StringAttribute{Computed: true},
						"type":    datasourceschema.StringAttribute{Computed: true},
						"url":     datasourceschema.StringAttribute{Computed: true},
						"ttl":     datasourceschema.Int64Attribute{Computed: true},
						"state":   datasourceschema.StringAttribute{Computed: true},
						"pending": datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
					},
				},
			},
		},
	}
}

func (d *SDNDNSsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNDNSsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnDNSsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	servers, err := d.client.SDNDNSs(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN DNS Providers", fmt.Sprintf("Unable to call `/cluster/sdn/dns`: %s", err))
		return
	}
	filter := stringValue(config.Type)
	records := make([]sdnDNSsDataSourceRecord, 0, len(servers))
	for _, server := range servers {
		if filter != "" && server.Type != filter {
			continue
		}
		staged := sdnPendingMap(server.Pending)
		record := sdnDNSsDataSourceRecord{
			DNS:     types.StringValue(server.DNS),
			Type:    types.StringValue(server.Type),
			URL:     sdnStagedString(server.URL, staged["url"]),
			TTL:     sdnStagedInt64(server.TTL.Ptr(), staged["ttl"]),
			State:   stringOrNull(server.State),
			Pending: types.MapNull(types.StringType),
		}
		if len(staged) > 0 {
			pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
			resp.Diagnostics.Append(pendingDiags...)
			record.Pending = pending
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].DNS.ValueString() < records[j].DNS.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnDNSsDataSourceModel{ID: types.StringValue("sdn_dns_providers"), Type: config.Type, Servers: records})...)
}
