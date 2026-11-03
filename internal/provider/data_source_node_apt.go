// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/hex"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &NodeAPTRepositoriesDataSource{}
var _ datasource.DataSourceWithValidateConfig = &NodeAPTRepositoriesDataSource{}

type NodeAPTRepositoriesDataSource struct{ client *Client }

type nodeAPTRepositoriesModel struct {
	Node  types.String              `tfsdk:"node"`
	Files []nodeAPTRepositoriesFile `tfsdk:"files"`
}

type nodeAPTRepositoriesFile struct {
	Path         types.String                `tfsdk:"path"`
	Digest       types.String                `tfsdk:"digest"`
	Product      types.String                `tfsdk:"product"`
	Repositories []nodeAPTRepositoriesRecord `tfsdk:"repositories"`
}

type nodeAPTRepositoriesRecord struct {
	Types      types.List   `tfsdk:"types"`
	URIs       types.List   `tfsdk:"uris"`
	Suites     types.List   `tfsdk:"suites"`
	Components types.List   `tfsdk:"components"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	Comment    types.String `tfsdk:"comment"`
}

func NewNodeAPTRepositoriesDataSource() datasource.DataSource {
	return &NodeAPTRepositoriesDataSource{}
}

func (d *NodeAPTRepositoriesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_apt_repositories"
}

func (d *NodeAPTRepositoriesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists the APT source files of one node from `/nodes/{node}/apt/repositories`, sorted by path, including the entry index position used by `proxmox_node_apt_repository`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"files": datasourceschema.ListNestedAttribute{
				Computed: true,
				NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
					"path":    datasourceschema.StringAttribute{Computed: true},
					"digest":  datasourceschema.StringAttribute{Computed: true},
					"product": datasourceschema.StringAttribute{Computed: true},
					"repositories": datasourceschema.ListNestedAttribute{
						Computed: true,
						NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
							"types":      datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
							"uris":       datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
							"suites":     datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
							"components": datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
							"enabled":    datasourceschema.BoolAttribute{Computed: true},
							"comment":    datasourceschema.StringAttribute{Computed: true},
						}},
					},
				}},
			},
		},
	}
}

func (d *NodeAPTRepositoriesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *NodeAPTRepositoriesDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config nodeAPTRepositoriesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if !resp.Diagnostics.HasError() {
		validateNodeName(config.Node, resp)
	}
}

func (d *NodeAPTRepositoriesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeAPTRepositoriesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	repositories, err := d.client.NodeAPTRepositories(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox APT Repositories", err.Error())
		return
	}
	files := make([]nodeAPTRepositoriesFile, 0, len(repositories.Files))
	for _, file := range repositories.Files {
		entries := make([]nodeAPTRepositoriesRecord, 0, len(file.Repositories))
		for _, entry := range file.Repositories {
			typesList, typesDiags := types.ListValueFrom(ctx, types.StringType, entry.Types)
			resp.Diagnostics.Append(typesDiags...)
			urisList, urisDiags := types.ListValueFrom(ctx, types.StringType, entry.URIs)
			resp.Diagnostics.Append(urisDiags...)
			suitesList, suitesDiags := types.ListValueFrom(ctx, types.StringType, entry.Suites)
			resp.Diagnostics.Append(suitesDiags...)
			componentsList, componentsDiags := types.ListValueFrom(ctx, types.StringType, entry.Components)
			resp.Diagnostics.Append(componentsDiags...)
			entries = append(entries, nodeAPTRepositoriesRecord{
				Types: typesList, URIs: urisList, Suites: suitesList,
				Components: componentsList, Enabled: types.BoolValue(entry.Enabled.Ptr() != nil && *entry.Enabled.Ptr()), Comment: stringOrNull(entry.Comment),
			})
		}
		files = append(files, nodeAPTRepositoriesFile{Path: types.StringValue(file.Path), Digest: stringOrNull(hexEncode(file.Digest)), Product: stringOrNull(file.Product), Repositories: entries})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path.ValueString() < files[j].Path.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeAPTRepositoriesModel{Node: config.Node, Files: files})...)
}

func hexEncode(value []byte) string {
	if len(value) == 0 {
		return ""
	}
	return hex.EncodeToString(value)
}
