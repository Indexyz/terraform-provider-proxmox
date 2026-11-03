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

var _ datasource.DataSource = &SDNIPAMsDataSource{}

type SDNIPAMsDataSource struct {
	client *Client
}

type sdnIPAMsDataSourceModel struct {
	ID    types.String               `tfsdk:"id"`
	Type  types.String               `tfsdk:"type"`
	IPAMs []sdnIPAMsDataSourceRecord `tfsdk:"ipams"`
}

type sdnIPAMsDataSourceRecord struct {
	IPAM    types.String `tfsdk:"ipam"`
	Type    types.String `tfsdk:"type"`
	URL     types.String `tfsdk:"url"`
	Section types.Int64  `tfsdk:"section"`
	State   types.String `tfsdk:"state"`
	Pending types.Map    `tfsdk:"pending"`
}

func NewSDNIPAMsDataSource() datasource.DataSource { return &SDNIPAMsDataSource{} }

func (d *SDNIPAMsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_ipams"
}

func (d *SDNIPAMsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists staged SDN IPAMs from `/cluster/sdn/ipams`, sorted by `ipam`. Tokens are not exposed.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"type": datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Restrict the result to one IPAM type. An empty value disables this filter."},
			"ipams": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching IPAMs, sorted by `ipam`.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"ipam":    datasourceschema.StringAttribute{Computed: true},
						"type":    datasourceschema.StringAttribute{Computed: true},
						"url":     datasourceschema.StringAttribute{Computed: true},
						"section": datasourceschema.Int64Attribute{Computed: true},
						"state":   datasourceschema.StringAttribute{Computed: true},
						"pending": datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
					},
				},
			},
		},
	}
}

func (d *SDNIPAMsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNIPAMsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnIPAMsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ipams, err := d.client.SDNIPAMs(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN IPAMs", fmt.Sprintf("Unable to call `/cluster/sdn/ipams`: %s", err))
		return
	}
	filter := stringValue(config.Type)
	records := make([]sdnIPAMsDataSourceRecord, 0, len(ipams))
	for _, ipam := range ipams {
		if filter != "" && ipam.Type != filter {
			continue
		}
		staged := sdnPendingMap(ipam.Pending)
		record := sdnIPAMsDataSourceRecord{
			IPAM:    types.StringValue(ipam.IPAM),
			Type:    types.StringValue(ipam.Type),
			URL:     sdnStagedString(ipam.URL, staged["url"]),
			Section: sdnStagedInt64(ipam.Section.Ptr(), staged["section"]),
			State:   stringOrNull(ipam.State),
			Pending: types.MapNull(types.StringType),
		}
		if len(staged) > 0 {
			pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
			resp.Diagnostics.Append(pendingDiags...)
			record.Pending = pending
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].IPAM.ValueString() < records[j].IPAM.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnIPAMsDataSourceModel{ID: types.StringValue("sdn_ipams"), Type: config.Type, IPAMs: records})...)
}
