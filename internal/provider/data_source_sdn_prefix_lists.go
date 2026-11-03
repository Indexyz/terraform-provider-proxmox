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

var _ datasource.DataSource = &SDNPrefixListsDataSource{}

type SDNPrefixListsDataSource struct {
	client *Client
}

type sdnPrefixListsDataSourceModel struct {
	ID    types.String                     `tfsdk:"id"`
	Lists []sdnPrefixListsDataSourceRecord `tfsdk:"prefix_lists"`
}

type sdnPrefixListsDataSourceRecord struct {
	PrefixListID types.String `tfsdk:"prefix_list_id"`
	Entries      types.List   `tfsdk:"entries"`
	State        types.String `tfsdk:"state"`
	Pending      types.Map    `tfsdk:"pending"`
}

func NewSDNPrefixListsDataSource() datasource.DataSource { return &SDNPrefixListsDataSource{} }

func (d *SDNPrefixListsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_prefix_lists"
}

func (d *SDNPrefixListsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists staged SDN prefix lists from `/cluster/sdn/prefix-lists`, sorted by `prefix_list_id`. Entries are Proxmox property strings.",
		Attributes: map[string]datasourceschema.Attribute{
			"id": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"prefix_lists": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "All prefix lists, sorted by `prefix_list_id`.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"prefix_list_id": datasourceschema.StringAttribute{Computed: true},
						"entries":        datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
						"state":          datasourceschema.StringAttribute{Computed: true},
						"pending":        datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
					},
				},
			},
		},
	}
}

func (d *SDNPrefixListsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNPrefixListsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnPrefixListsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	lists, err := d.client.SDNPrefixLists(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN Prefix Lists", fmt.Sprintf("Unable to call `/cluster/sdn/prefix-lists`: %s", err))
		return
	}
	records := make([]sdnPrefixListsDataSourceRecord, 0, len(lists))
	for _, list := range lists {
		staged := sdnPendingMap(list.Pending)
		entries, entryDiags := types.ListValueFrom(ctx, types.StringType, sdnStagedRawList(list.Entries, staged["entries"]))
		resp.Diagnostics.Append(entryDiags...)
		record := sdnPrefixListsDataSourceRecord{
			PrefixListID: types.StringValue(list.ID),
			Entries:      entries,
			State:        stringOrNull(list.State),
			Pending:      types.MapNull(types.StringType),
		}
		if len(staged) > 0 {
			pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
			resp.Diagnostics.Append(pendingDiags...)
			record.Pending = pending
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].PrefixListID.ValueString() < records[j].PrefixListID.ValueString()
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnPrefixListsDataSourceModel{ID: types.StringValue("sdn_prefix_lists"), Lists: records})...)
}
