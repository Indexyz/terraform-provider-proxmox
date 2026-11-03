// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/url"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ACMEChallengeSchemaDataSource{}

type ACMEChallengeSchemaDataSource struct{ client *Client }

type acmeChallengeSchemaModel struct {
	Type    types.String `tfsdk:"type"`
	Plugins types.List   `tfsdk:"plugins"`
	Fields  types.Map    `tfsdk:"fields"`
}

type acmeChallengeSchemaEntry struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Schema struct {
		Fields map[string]struct {
			Type string `json:"type"`
		} `json:"fields"`
	} `json:"schema"`
}

func NewACMEChallengeSchemaDataSource() datasource.DataSource {
	return &ACMEChallengeSchemaDataSource{}
}

func (d *ACMEChallengeSchemaDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acme_challenge_schema"
}

func (d *ACMEChallengeSchemaDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists the ACME plugin types Proxmox knows for one challenge type from `/cluster/acme/challenge-schema`, together with the credential fields each plugin expects. Use the ids as `proxmox_acme_plugin.api`.",
		Attributes: map[string]datasourceschema.Attribute{
			"type":    datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Challenge type, usually `dns`. An empty value disables the filter."},
			"plugins": datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Plugin ids sorted alphabetically."},
			"fields":  datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Comma separated credential field names per plugin id."},
		},
	}
}

func (d *ACMEChallengeSchemaDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ACMEChallengeSchemaDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config acmeChallengeSchemaModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var entries []acmeChallengeSchemaEntry
	if err := d.client.do(ctx, "GET", "/cluster/acme/challenge-schema", url.Values{}, nil, &entries); err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox ACME Challenge Schema", fmt.Sprintf("Unable to call `/cluster/acme/challenge-schema`: %s", err))
		return
	}
	filter := stringValue(config.Type)
	plugins := []string{}
	fields := map[string]string{}
	for _, entry := range entries {
		if filter != "" && entry.Type != filter {
			continue
		}
		plugins = append(plugins, entry.ID)
		names := make([]string, 0, len(entry.Schema.Fields))
		for name := range entry.Schema.Fields {
			names = append(names, name)
		}
		slices.Sort(names)
		if len(names) > 0 {
			fields[entry.ID] = joinComma(names)
		}
	}
	slices.Sort(plugins)
	pluginList, pluginDiags := types.ListValueFrom(ctx, types.StringType, plugins)
	resp.Diagnostics.Append(pluginDiags...)
	fieldMap, fieldDiags := types.MapValueFrom(ctx, types.StringType, fields)
	resp.Diagnostics.Append(fieldDiags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &acmeChallengeSchemaModel{Type: config.Type, Plugins: pluginList, Fields: fieldMap})...)
}
