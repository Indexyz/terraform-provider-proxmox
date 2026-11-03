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

var _ datasource.DataSource = &HAResourcesDataSource{}

type HAResourcesDataSource struct {
	client *Client
}

type haResourcesDataSourceModel struct {
	ID        types.String                  `tfsdk:"id"`
	Resources []haResourcesDataSourceRecord `tfsdk:"resources"`
}

type haResourcesDataSourceRecord struct {
	SID           types.String `tfsdk:"sid"`
	State         types.String `tfsdk:"state"`
	Comment       types.String `tfsdk:"comment"`
	Failback      types.Bool   `tfsdk:"failback"`
	AutoRebalance types.Bool   `tfsdk:"auto_rebalance"`
	MaxRestart    types.Int64  `tfsdk:"max_restart"`
	MaxRelocate   types.Int64  `tfsdk:"max_relocate"`
}

func NewHAResourcesDataSource() datasource.DataSource { return &HAResourcesDataSource{} }

func (d *HAResourcesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ha_resources"
}

func (d *HAResourcesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists the high-availability enrolled guests of the cluster from `/cluster/ha/resources`, sorted by HA id. No worker state is queried.",
		Attributes: map[string]datasourceschema.Attribute{
			"id": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"resources": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Enrolled HA resources, sorted by `sid`.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"sid":            datasourceschema.StringAttribute{Computed: true},
						"state":          datasourceschema.StringAttribute{Computed: true},
						"comment":        datasourceschema.StringAttribute{Computed: true},
						"failback":       datasourceschema.BoolAttribute{Computed: true},
						"auto_rebalance": datasourceschema.BoolAttribute{Computed: true},
						"max_restart":    datasourceschema.Int64Attribute{Computed: true},
						"max_relocate":   datasourceschema.Int64Attribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *HAResourcesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *HAResourcesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config haResourcesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resources, err := d.client.HAResources(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox HA Resources", fmt.Sprintf("Unable to call `/cluster/ha/resources`: %s", err))
		return
	}
	records := make([]haResourcesDataSourceRecord, 0, len(resources))
	for _, resource := range resources {
		records = append(records, haResourcesDataSourceRecord{
			SID:           types.StringValue(resource.SID),
			State:         stringOrNull(resource.State),
			Comment:       stringOrNull(resource.Comment),
			Failback:      nodesBoolOrNull(resource.Failback),
			AutoRebalance: nodesBoolOrNull(resource.AutoRebalance),
			MaxRestart:    int64OrNull(resource.MaxRestart.Ptr()),
			MaxRelocate:   int64OrNull(resource.MaxRelocate.Ptr()),
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].SID.ValueString() < records[j].SID.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &haResourcesDataSourceModel{ID: types.StringValue("ha_resources"), Resources: records})...)
}
