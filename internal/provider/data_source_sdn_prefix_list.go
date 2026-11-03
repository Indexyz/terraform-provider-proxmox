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

var _ datasource.DataSource = &SDNPrefixListDataSource{}

type SDNPrefixListDataSource struct {
	client *Client
}

type sdnPrefixListDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	PrefixListID types.String `tfsdk:"prefix_list_id"`
	Entries      types.List   `tfsdk:"entries"`
	State        types.String `tfsdk:"state"`
	Pending      types.Map    `tfsdk:"pending"`
}

func NewSDNPrefixListDataSource() datasource.DataSource { return &SDNPrefixListDataSource{} }

func (d *SDNPrefixListDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_prefix_list"
}

func (d *SDNPrefixListDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads one staged SDN prefix list from `/cluster/sdn/prefix-lists`. Entries stay Proxmox property strings.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":             datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Prefix list identifier."},
			"prefix_list_id": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Prefix list to read."},
			"entries":        datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
			"state":          datasourceschema.StringAttribute{Computed: true},
			"pending":        datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *SDNPrefixListDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNPrefixListDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnPrefixListDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := config.PrefixListID.ValueString()
	entry, err := d.client.GetSDNPrefixList(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN Prefix List", fmt.Sprintf("Unable to call `/cluster/sdn/prefix-lists`: %s", err))
		return
	}
	staged := sdnPendingMap(entry.Pending)
	entries, entryDiags := types.ListValueFrom(ctx, types.StringType, sdnStagedRawList(entry.Entries, staged["entries"]))
	resp.Diagnostics.Append(entryDiags...)
	pending := types.MapNull(types.StringType)
	if len(staged) > 0 {
		value, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		resp.Diagnostics.Append(pendingDiags...)
		pending = value
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnPrefixListDataSourceModel{
		ID:           types.StringValue(id),
		PrefixListID: types.StringValue(entry.ID),
		Entries:      entries,
		State:        stringOrNull(entry.State),
		Pending:      pending,
	})...)
}
