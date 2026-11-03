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

var _ datasource.DataSource = &SDNFabricsDataSource{}

type SDNFabricsDataSource struct {
	client *Client
}

type sdnFabricsDataSourceModel struct {
	ID       types.String                 `tfsdk:"id"`
	Protocol types.String                 `tfsdk:"protocol"`
	Fabrics  []sdnFabricsDataSourceRecord `tfsdk:"fabrics"`
}

type sdnFabricsDataSourceRecord struct {
	FabricID types.String `tfsdk:"fabric_id"`
	Protocol types.String `tfsdk:"protocol"`
	IPPrefix types.String `tfsdk:"ip_prefix"`
	State    types.String `tfsdk:"state"`
	Pending  types.Map    `tfsdk:"pending"`
}

func NewSDNFabricsDataSource() datasource.DataSource { return &SDNFabricsDataSource{} }

func (d *SDNFabricsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_fabrics"
}

func (d *SDNFabricsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists staged SDN fabrics from `/cluster/sdn/fabrics/fabric`, sorted by `fabric_id`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":       datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"protocol": datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Restrict the result to one fabric protocol. An empty value disables this filter."},
			"fabrics": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching fabrics, sorted by `fabric_id`.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"fabric_id": datasourceschema.StringAttribute{Computed: true},
						"protocol":  datasourceschema.StringAttribute{Computed: true},
						"ip_prefix": datasourceschema.StringAttribute{Computed: true},
						"state":     datasourceschema.StringAttribute{Computed: true},
						"pending":   datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
					},
				},
			},
		},
	}
}

func (d *SDNFabricsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNFabricsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnFabricsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fabrics, err := d.client.SDNFabrics(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN Fabrics", fmt.Sprintf("Unable to call `/cluster/sdn/fabrics/fabric`: %s", err))
		return
	}
	filter := stringValue(config.Protocol)
	records := make([]sdnFabricsDataSourceRecord, 0, len(fabrics))
	for _, fabric := range fabrics {
		if filter != "" && fabric.Protocol != filter {
			continue
		}
		staged := sdnPendingMap(fabric.Pending)
		record := sdnFabricsDataSourceRecord{
			FabricID: types.StringValue(fabric.ID),
			Protocol: types.StringValue(fabric.Protocol),
			IPPrefix: sdnStagedString(fabric.IPPrefix, staged["ip_prefix"]),
			State:    stringOrNull(fabric.State),
			Pending:  types.MapNull(types.StringType),
		}
		if len(staged) > 0 {
			pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
			resp.Diagnostics.Append(pendingDiags...)
			record.Pending = pending
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].FabricID.ValueString() < records[j].FabricID.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnFabricsDataSourceModel{ID: types.StringValue("sdn_fabrics"), Protocol: config.Protocol, Fabrics: records})...)
}
