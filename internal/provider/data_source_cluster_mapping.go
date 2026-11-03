// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ClusterMappingDataSource{}
var _ datasource.DataSourceWithValidateConfig = &ClusterMappingDataSource{}

type ClusterMappingDataSource struct {
	client *Client
}

type clusterMappingDataSourceModel struct {
	Kind          types.String `tfsdk:"kind"`
	ID            types.String `tfsdk:"id"`
	Description   types.String `tfsdk:"description"`
	LiveMigration types.Bool   `tfsdk:"live_migration_capable"`
	Map           types.List   `tfsdk:"map"`
}

func NewClusterMappingDataSource() datasource.DataSource { return &ClusterMappingDataSource{} }

func (d *ClusterMappingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_mapping"
}

func (d *ClusterMappingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads one cluster hardware mapping of type `pci`, `usb` or `dir` from `/cluster/mapping/{kind}/{id}`.",
		Attributes: map[string]datasourceschema.Attribute{
			"kind":                   datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Mapping type: `pci`, `usb` or `dir`."},
			"id":                     datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Mapping identifier."},
			"description":            datasourceschema.StringAttribute{Computed: true},
			"live_migration_capable": datasourceschema.BoolAttribute{Computed: true},
			"map":                    datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Raw Proxmox property strings such as `node=pve-1,path=/srv/share`."},
		},
	}
}

func (d *ClusterMappingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ClusterMappingDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config clusterMappingDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(validateClusterMappingDSKind(config.Kind)...)
}

func validateClusterMappingDSKind(kind types.String) diag.Diagnostics {
	var diags diag.Diagnostics
	if !isConfiguredValue(kind) {
		return diags
	}
	if !slices.Contains([]string{clusterMappingKindPCI, clusterMappingKindUSB, clusterMappingKindDir}, kind.ValueString()) {
		diags.AddAttributeError(path.Root("kind"), "Invalid mapping kind", "kind must be pci, usb or dir.")
	}
	return diags
}

func (d *ClusterMappingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config clusterMappingDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	kind := config.Kind.ValueString()
	entry, err := d.client.GetClusterMapping(ctx, kind, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Cluster Mapping", fmt.Sprintf("Unable to call `/cluster/mapping/%s`: %s", kind, err))
		return
	}
	rawMap, mapDiags := types.ListValueFrom(ctx, types.StringType, entry.Map)
	resp.Diagnostics.Append(mapDiags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &clusterMappingDataSourceModel{
		Kind:          types.StringValue(kind),
		ID:            types.StringValue(entry.ID),
		Description:   stringOrNull(entry.Description),
		LiveMigration: boolOrNull(entry.LiveMigration.Ptr()),
		Map:           rawMap,
	})...)
}
