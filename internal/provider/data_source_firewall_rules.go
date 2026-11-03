// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &FirewallRulesDataSource{}
var _ datasource.DataSourceWithValidateConfig = &FirewallRulesDataSource{}

type FirewallRulesDataSource struct {
	client *Client
}

type firewallRulesDataSourceModel struct {
	ID            types.String                    `tfsdk:"id"`
	Scope         types.String                    `tfsdk:"scope"`
	Node          types.String                    `tfsdk:"node"`
	GuestType     types.String                    `tfsdk:"guest_type"`
	VMID          types.Int64                     `tfsdk:"vmid"`
	SecurityGroup types.String                    `tfsdk:"security_group"`
	Vnet          types.String                    `tfsdk:"vnet"`
	Rules         []firewallRulesDataSourceRecord `tfsdk:"rules"`
}

type firewallRulesDataSourceRecord struct {
	Pos      types.Int64  `tfsdk:"pos"`
	Type     types.String `tfsdk:"type"`
	Action   types.String `tfsdk:"action"`
	Enable   types.Int64  `tfsdk:"enable"`
	Comment  types.String `tfsdk:"comment"`
	Source   types.String `tfsdk:"source"`
	Dest     types.String `tfsdk:"dest"`
	Proto    types.String `tfsdk:"proto"`
	DPort    types.String `tfsdk:"dport"`
	SPort    types.String `tfsdk:"sport"`
	ICMPType types.String `tfsdk:"icmp_type"`
	Iface    types.String `tfsdk:"iface"`
	Macro    types.String `tfsdk:"macro"`
	Log      types.String `tfsdk:"log"`
	Digest   types.String `tfsdk:"digest"`
}

var firewallRulesScopes = []string{"cluster", "node", "guest", "security_group", "vnet"}

func NewFirewallRulesDataSource() datasource.DataSource { return &FirewallRulesDataSource{} }

func (d *FirewallRulesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_rules"
}

func (d *FirewallRulesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists the firewall rules of one ruleset in their effective order, the same scopes `proxmox_firewall_rule` writes. `pos` and `digest` identify a rule, `pending` does not exist because Proxmox keeps firewall rules un-staged.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":             datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"scope":          datasourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Ruleset scope: `cluster` (default), `node`, `guest`, `security_group`, or `vnet`."},
			"node":           datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Node name for the `node` and `guest` scopes."},
			"guest_type":     datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "`qemu` or `lxc` for the `guest` scope."},
			"vmid":           datasourceschema.Int64Attribute{Optional: true, MarkdownDescription: "Guest VMID for the `guest` scope."},
			"security_group": datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Security group name for the `security_group` scope."},
			"vnet":           datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "SDN VNet for the `vnet` scope."},
			"rules": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Rules in their effective order.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"pos":       datasourceschema.Int64Attribute{Computed: true},
						"type":      datasourceschema.StringAttribute{Computed: true},
						"action":    datasourceschema.StringAttribute{Computed: true},
						"enable":    datasourceschema.Int64Attribute{Computed: true},
						"comment":   datasourceschema.StringAttribute{Computed: true},
						"source":    datasourceschema.StringAttribute{Computed: true},
						"dest":      datasourceschema.StringAttribute{Computed: true},
						"proto":     datasourceschema.StringAttribute{Computed: true},
						"dport":     datasourceschema.StringAttribute{Computed: true},
						"sport":     datasourceschema.StringAttribute{Computed: true},
						"icmp_type": datasourceschema.StringAttribute{Computed: true},
						"iface":     datasourceschema.StringAttribute{Computed: true},
						"macro":     datasourceschema.StringAttribute{Computed: true},
						"log":       datasourceschema.StringAttribute{Computed: true},
						"digest":    datasourceschema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *FirewallRulesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *FirewallRulesDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config firewallRulesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateFirewallRulesScope(config)...)
}

func validateFirewallRulesScope(config firewallRulesDataSourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	scope := "cluster"
	if isConfiguredValue(config.Scope) {
		scope = config.Scope.ValueString()
	}
	if !config.Scope.IsUnknown() && !slices.Contains(firewallRulesScopes, scope) {
		diags.AddAttributeError(path.Root("scope"), "Invalid firewall rules scope", "scope must be cluster, node, guest, security_group, or vnet")
		return diags
	}
	required := func(name string, value types.String) {
		if !isConfiguredValue(value) && !value.IsUnknown() {
			diags.AddAttributeError(path.Root(name), "Missing firewall rules scope reference", fmt.Sprintf("%s is required for scope %s", name, scope))
		}
	}
	switch scope {
	case "node":
		required("node", config.Node)
	case "guest":
		required("node", config.Node)
		required("guest_type", config.GuestType)
		if !isConfiguredValue(config.VMID) && !config.VMID.IsUnknown() {
			diags.AddAttributeError(path.Root("vmid"), "Missing firewall rules scope reference", "vmid is required for scope guest")
		}
	case "security_group":
		required("security_group", config.SecurityGroup)
	case "vnet":
		required("vnet", config.Vnet)
	}
	return diags
}

func (d *FirewallRulesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config firewallRulesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	scope := firewallScopeFromModel(firewallRuleModel{
		Scope:         config.Scope,
		Node:          config.Node,
		GuestType:     config.GuestType,
		VMID:          config.VMID,
		SecurityGroup: config.SecurityGroup,
		Vnet:          config.Vnet,
	})
	rules, err := d.client.GetScopedFirewallRules(ctx, scope)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Firewall Rules", fmt.Sprintf("Unable to read firewall rules of scope %s: %s", scope.Kind, err))
		return
	}
	records := make([]firewallRulesDataSourceRecord, 0, len(rules))
	for _, rule := range rules {
		enable := types.Int64Null()
		if rule.Enable.Ptr() != nil {
			enable = types.Int64Value(*rule.Enable.Ptr())
		}
		record := firewallRulesDataSourceRecord{
			Pos:      types.Int64Value(int64(rule.Pos)),
			Type:     stringOrNull(rule.Type),
			Action:   stringOrNull(rule.Action),
			Enable:   enable,
			Comment:  stringOrNull(rule.Comment),
			Source:   stringOrNull(rule.Source),
			Dest:     stringOrNull(rule.Dest),
			Proto:    stringOrNull(rule.Proto),
			DPort:    stringOrNull(rule.DPort),
			SPort:    stringOrNull(rule.SPort),
			ICMPType: stringOrNull(rule.ICMPType),
			Iface:    stringOrNull(rule.Iface),
			Macro:    stringOrNull(rule.Macro),
			Log:      stringOrNull(rule.Log),
			Digest:   stringOrNull(rule.Digest),
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Pos.ValueInt64() < records[j].Pos.ValueInt64() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &firewallRulesDataSourceModel{
		ID:            types.StringValue("firewall_rules/" + scope.Kind),
		Scope:         types.StringValue(scope.Kind),
		Node:          config.Node,
		GuestType:     config.GuestType,
		VMID:          config.VMID,
		SecurityGroup: config.SecurityGroup,
		Vnet:          config.Vnet,
		Rules:         records,
	})...)
}
