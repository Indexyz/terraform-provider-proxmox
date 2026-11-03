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

var _ datasource.DataSource = &SDNControllersDataSource{}

type SDNControllersDataSource struct {
	client *Client
}

type sdnControllersDataSourceModel struct {
	ID          types.String                     `tfsdk:"id"`
	Type        types.String                     `tfsdk:"type"`
	Controllers []sdnControllersDataSourceRecord `tfsdk:"controllers"`
}

type sdnControllersDataSourceRecord struct {
	Controller types.String `tfsdk:"controller"`
	Type       types.String `tfsdk:"type"`
	ASN        types.Int64  `tfsdk:"asn"`
	Node       types.String `tfsdk:"node"`
	State      types.String `tfsdk:"state"`
	Pending    types.Map    `tfsdk:"pending"`
}

func NewSDNControllersDataSource() datasource.DataSource { return &SDNControllersDataSource{} }

func (d *SDNControllersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_controllers"
}

func (d *SDNControllersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists staged SDN controllers from `/cluster/sdn/controllers`, sorted by `controller`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"type": datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Restrict the result to one controller type. An empty value disables this filter."},
			"controllers": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching controllers, sorted by `controller`.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"controller": datasourceschema.StringAttribute{Computed: true},
						"type":       datasourceschema.StringAttribute{Computed: true},
						"asn":        datasourceschema.Int64Attribute{Computed: true},
						"node":       datasourceschema.StringAttribute{Computed: true},
						"state":      datasourceschema.StringAttribute{Computed: true},
						"pending":    datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
					},
				},
			},
		},
	}
}

func (d *SDNControllersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNControllersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnControllersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	controllers, err := d.client.SDNControllers(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN Controllers", fmt.Sprintf("Unable to call `/cluster/sdn/controllers`: %s", err))
		return
	}
	filter := stringValue(config.Type)
	records := make([]sdnControllersDataSourceRecord, 0, len(controllers))
	for _, controller := range controllers {
		if filter != "" && controller.Type != filter {
			continue
		}
		staged := sdnPendingMap(controller.Pending)
		record := sdnControllersDataSourceRecord{
			Controller: types.StringValue(controller.Controller),
			Type:       types.StringValue(controller.Type),
			ASN:        sdnStagedInt64(controller.ASN.Ptr(), staged["asn"]),
			Node:       sdnStagedString(controller.Node, staged["node"]),
			State:      stringOrNull(controller.State),
			Pending:    types.MapNull(types.StringType),
		}
		if len(staged) > 0 {
			pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
			resp.Diagnostics.Append(pendingDiags...)
			record.Pending = pending
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Controller.ValueString() < records[j].Controller.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnControllersDataSourceModel{ID: types.StringValue("sdn_controllers"), Type: config.Type, Controllers: records})...)
}
