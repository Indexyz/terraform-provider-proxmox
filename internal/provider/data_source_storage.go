// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &StorageDataSource{}

type StorageDataSource struct {
	client *Client
}

type storageDataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	Storage types.String `tfsdk:"storage"`
	Type    types.String `tfsdk:"type"`
	Content types.String `tfsdk:"content"`
	Nodes   types.String `tfsdk:"nodes"`
	Disable types.Bool   `tfsdk:"disable"`
	Shared  types.Bool   `tfsdk:"shared"`
	Path    types.String `tfsdk:"path"`
}

func NewStorageDataSource() datasource.DataSource {
	return &StorageDataSource{}
}

func (d *StorageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage"
}

func (d *StorageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Fetches details of a single Proxmox VE storage pool from `/storage/{storage}`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Storage identifier."},
			"storage": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "The storage identifier to look up."},
			"type":    datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Storage type."},
			"content": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Allowed content types."},
			"nodes":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Nodes this storage applies to."},
			"disable": datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the storage is disabled."},
			"shared":  datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the storage is shared."},
			"path":    datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "File system path (for dir type)."},
		},
	}
}

func (d *StorageDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *StorageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state storageDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	config, err := d.client.GetStorage(ctx, state.Storage.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Storage", err.Error())
		return
	}
	state.ID = types.StringValue(config.Storage)
	state.Type = stringOrNull(config.Type)
	state.Content = stringOrNull(config.Content)
	state.Nodes = stringOrNull(config.Nodes)
	state.Disable = boolOrNull(config.Disable.Ptr())
	state.Shared = boolOrNull(config.Shared.Ptr())
	state.Path = stringOrNull(config.Path)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
