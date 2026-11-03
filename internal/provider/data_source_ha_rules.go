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

var _ datasource.DataSource = &HARulesDataSource{}

type HARulesDataSource struct {
	client *Client
}

type haRulesDataSourceModel struct {
	ID    types.String              `tfsdk:"id"`
	Type  types.String              `tfsdk:"type"`
	Rules []haRulesDataSourceRecord `tfsdk:"rules"`
}

type haRulesDataSourceRecord struct {
	Rule      types.String `tfsdk:"rule"`
	Type      types.String `tfsdk:"type"`
	Resources types.List   `tfsdk:"resources"`
	Nodes     types.Map    `tfsdk:"nodes"`
	Affinity  types.String `tfsdk:"affinity"`
	Strict    types.Bool   `tfsdk:"strict"`
	Disable   types.Bool   `tfsdk:"disable"`
	Comment   types.String `tfsdk:"comment"`
	Errors    types.Map    `tfsdk:"errors"`
}

func NewHARulesDataSource() datasource.DataSource { return &HARulesDataSource{} }

func (d *HARulesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ha_rules"
}

func (d *HARulesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists the high-availability placement rules of the cluster from `/cluster/ha/rules`, sorted by rule name. `errors` carries the per-rule validation messages the HA manager reports.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"type": datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Restrict the result to `node-affinity` or `resource-affinity`. An empty value disables this filter."},
			"rules": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching rules, sorted by `rule`.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"rule":      datasourceschema.StringAttribute{Computed: true},
						"type":      datasourceschema.StringAttribute{Computed: true},
						"resources": datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
						"nodes":     datasourceschema.MapAttribute{Computed: true, ElementType: types.Int64Type},
						"affinity":  datasourceschema.StringAttribute{Computed: true},
						"strict":    datasourceschema.BoolAttribute{Computed: true},
						"disable":   datasourceschema.BoolAttribute{Computed: true},
						"comment":   datasourceschema.StringAttribute{Computed: true},
						"errors":    datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
					},
				},
			},
		},
	}
}

func (d *HARulesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *HARulesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config haRulesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rules, err := d.client.HARules(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox HA Rules", fmt.Sprintf("Unable to call `/cluster/ha/rules`: %s", err))
		return
	}
	filter := stringValue(config.Type)
	records := make([]haRulesDataSourceRecord, 0, len(rules))
	for _, rule := range rules {
		if filter != "" && rule.Type != filter {
			continue
		}
		resources, resourceDiags := types.ListValueFrom(ctx, types.StringType, splitProxmoxList(rule.Resources))
		resp.Diagnostics.Append(resourceDiags...)
		nodes, nodeDiags := haRuleNodeValues(ctx, rule.Nodes)
		resp.Diagnostics.Append(nodeDiags...)
		errors, errorDiags := types.MapValueFrom(ctx, types.StringType, rule.Errors)
		resp.Diagnostics.Append(errorDiags...)
		records = append(records, haRulesDataSourceRecord{
			Rule:      types.StringValue(rule.Rule),
			Type:      types.StringValue(rule.Type),
			Resources: resources,
			Nodes:     nodes,
			Affinity:  stringOrNull(rule.Affinity),
			Strict:    nodesBoolOrNull(rule.Strict),
			Disable:   nodesBoolOrNull(rule.Disable),
			Comment:   stringOrNull(rule.Comment),
			Errors:    errors,
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Rule.ValueString() < records[j].Rule.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &haRulesDataSourceModel{ID: types.StringValue("ha_rules"), Type: config.Type, Rules: records})...)
}
