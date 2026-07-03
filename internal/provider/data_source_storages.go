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

var _ datasource.DataSource = &StoragesDataSource{}

type StoragesDataSource struct {
	client *Client
}

type StoragesDataSourceModel struct {
	ID       types.String               `tfsdk:"id"`
	Storages []StoragesDataSourceRecord `tfsdk:"storages"`
}

type StoragesDataSourceRecord struct {
	Storage types.String `tfsdk:"storage"`
	Type    types.String `tfsdk:"type"`
	Content types.String `tfsdk:"content"`
	Nodes   types.String `tfsdk:"nodes"`
	Disable types.Bool   `tfsdk:"disable"`
	Shared  types.Bool   `tfsdk:"shared"`
}

func NewStoragesDataSource() datasource.DataSource {
	return &StoragesDataSource{}
}

func (d *StoragesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storages"
}

func (d *StoragesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Fetches the list of all Proxmox VE storage pools from `/storage`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":       datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"storages": storagesDataSourceAttribute(),
		},
	}
}

func storagesDataSourceAttribute() datasourceschema.ListNestedAttribute {
	return datasourceschema.ListNestedAttribute{
		Computed: true,
		NestedObject: datasourceschema.NestedAttributeObject{
			Attributes: map[string]datasourceschema.Attribute{
				"storage": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Storage identifier."},
				"type":    datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Storage type."},
				"content": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Allowed content types."},
				"nodes":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Nodes this storage applies to."},
				"disable": datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the storage is disabled."},
				"shared":  datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the storage is shared."},
			},
		},
	}
}

func (d *StoragesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *StoragesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	storages, err := d.client.Storages(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Storages", err.Error())
		return
	}
	records := make([]StoragesDataSourceRecord, 0, len(storages))
	for _, s := range storages {
		records = append(records, StoragesDataSourceRecord{
			Storage: stringOrNull(s.Storage),
			Type:    stringOrNull(s.Type),
			Content: stringOrNull(s.Content),
			Nodes:   stringOrNull(s.Nodes),
			Disable: boolOrNull(s.Disable.Ptr()),
			Shared:  boolOrNull(s.Shared.Ptr()),
		})
	}
	state := StoragesDataSourceModel{
		ID:       types.StringValue(fmt.Sprintf("storages-%d", len(records))),
		Storages: records,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
