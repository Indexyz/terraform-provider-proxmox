// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &FirewallRuleResource{}

type FirewallRuleResource struct {
	client *Client
}

type firewallRuleModel struct {
	ID       types.String `tfsdk:"id"`
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
	Pos      types.Int64  `tfsdk:"pos"`
}

func NewFirewallRuleResource() resource.Resource {
	return &FirewallRuleResource{}
}

func (r *FirewallRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_rule"
}

func replaceIf() planmodifier.String {
	return stringplanmodifier.RequiresReplaceIfConfigured()
}

func (r *FirewallRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox VE cluster-level firewall rule through `/cluster/firewall/rules`. Rules are matched by content (identity fields); `pos` is computed and re-resolved on every operation. Import is not supported.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true, MarkdownDescription: "Content-based identifier (type/action + identity fields)."},
			"type":      schema.StringAttribute{Required: true, MarkdownDescription: "Rule direction: `in`, `out`, `forward`, or `group`.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"action":    schema.StringAttribute{Required: true, MarkdownDescription: "Rule action: `ACCEPT`, `DROP`, or `REJECT`.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"enable":    schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Whether the rule is active (1) or disabled (0). Defaults to 1."},
			"comment":   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Rule comment."},
			"source":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Source CIDR or address.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"dest":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Destination CIDR or address.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"proto":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Protocol (e.g. `tcp`, `udp`, `icmp`).", PlanModifiers: []planmodifier.String{replaceIf()}},
			"dport":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Destination port(s).", PlanModifiers: []planmodifier.String{replaceIf()}},
			"sport":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Source port(s).", PlanModifiers: []planmodifier.String{replaceIf()}},
			"icmp_type": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "ICMP type.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"iface":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Network interface.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"macro":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Predefined macro name.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"log":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Log level: `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, `debug`, or `nolog`.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"pos":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Position in the firewall ruleset (computed, re-resolved on every operation).", PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *FirewallRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", err.Error())
		return
	}
	r.client = client
}

// matchFirewallRule finds rules in the list that match the model's identity fields.
// Returns matched rules and diagnostics (error if ≥2 matches).
func matchFirewallRule(rules []FirewallRule, model firewallRuleModel) ([]FirewallRule, diag.Diagnostics) {
	var diags diag.Diagnostics
	var matches []FirewallRule
	for _, rule := range rules {
		if rule.Type == model.Type.ValueString() &&
			rule.Action == model.Action.ValueString() &&
			rule.Source == model.Source.ValueString() &&
			rule.Dest == model.Dest.ValueString() &&
			rule.Proto == model.Proto.ValueString() &&
			rule.DPort == model.DPort.ValueString() &&
			rule.SPort == model.SPort.ValueString() &&
			rule.ICMPType == model.ICMPType.ValueString() &&
			rule.Iface == model.Iface.ValueString() &&
			rule.Macro == model.Macro.ValueString() &&
			rule.Log == model.Log.ValueString() {
			matches = append(matches, rule)
		}
	}
	if len(matches) > 1 {
		diags.AddError("Ambiguous firewall rule match",
			fmt.Sprintf("%d rules found with identical content; cannot determine which pos to target. Remove duplicates or modify your config.", len(matches)))
		return nil, diags
	}
	return matches, diags
}

func firewallRuleStateFromAPI(model firewallRuleModel, rule FirewallRule) firewallRuleModel {
	enable := types.Int64Null()
	if rule.Enable.Ptr() != nil {
		enable = types.Int64Value(*rule.Enable.Ptr())
	}
	return firewallRuleModel{
		ID:       types.StringValue(fmt.Sprintf("%s/%s/%s", model.Type.ValueString(), model.Action.ValueString(), model.Source.ValueString())),
		Type:     model.Type,
		Action:   model.Action,
		Enable:   enable,
		Comment:  stringOrNull(rule.Comment),
		Source:   model.Source,
		Dest:     model.Dest,
		Proto:    model.Proto,
		DPort:    model.DPort,
		SPort:    model.SPort,
		ICMPType: model.ICMPType,
		Iface:    model.Iface,
		Macro:    model.Macro,
		Log:      model.Log,
		Pos:      types.Int64Value(int64(rule.Pos)),
	}
}

func firewallRuleRequestFromModel(model firewallRuleModel) FirewallRuleRequest {
	return FirewallRuleRequest{
		Type:     model.Type.ValueString(),
		Action:   model.Action.ValueString(),
		Enable:   int64PointerValue(model.Enable),
		Comment:  stringPointerValue(model.Comment),
		Source:   stringPointerValue(model.Source),
		Dest:     stringPointerValue(model.Dest),
		Proto:    stringPointerValue(model.Proto),
		DPort:    stringPointerValue(model.DPort),
		SPort:    stringPointerValue(model.SPort),
		ICMPType: stringPointerValue(model.ICMPType),
		Iface:    stringPointerValue(model.Iface),
		Macro:    stringPointerValue(model.Macro),
		Log:      stringPointerValue(model.Log),
	}
}

func (r *FirewallRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firewallRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Pre-check: error if a duplicate already exists
	existing, err := r.client.GetFirewallRules(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Firewall Rules", err.Error())
		return
	}
	matches, matchDiags := matchFirewallRule(existing, plan)
	resp.Diagnostics.Append(matchDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(matches) >= 1 {
		resp.Diagnostics.AddError("Duplicate firewall rule",
			fmt.Sprintf("a firewall rule with identical content already exists at pos %d; remove the existing rule or modify your config", matches[0].Pos))
		return
	}

	if err := r.client.CreateFirewallRule(ctx, firewallRuleRequestFromModel(plan)); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Firewall Rule", err.Error())
		return
	}

	// Re-GET to resolve pos
	rules, err := r.client.GetFirewallRules(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Firewall Rules After Create", err.Error())
		return
	}
	matches, matchDiags = matchFirewallRule(rules, plan)
	resp.Diagnostics.Append(matchDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(matches) == 0 {
		resp.Diagnostics.AddError("Firewall Rule Not Found After Create", "the rule was created but could not be found on re-read")
		return
	}
	state := firewallRuleStateFromAPI(plan, matches[0])
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *FirewallRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firewallRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rules, err := r.client.GetFirewallRules(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Firewall Rules", err.Error())
		return
	}
	matches, matchDiags := matchFirewallRule(rules, state)
	resp.Diagnostics.Append(matchDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(matches) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}
	refreshed := firewallRuleStateFromAPI(state, matches[0])
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *FirewallRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan firewallRuleModel
	var state firewallRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rules, err := r.client.GetFirewallRules(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Firewall Rules", err.Error())
		return
	}
	// Match by OLD state identity to find the rule to update
	matches, matchDiags := matchFirewallRule(rules, state)
	resp.Diagnostics.Append(matchDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(matches) == 0 {
		// Rule gone; create new one
		if err := r.client.CreateFirewallRule(ctx, firewallRuleRequestFromModel(plan)); err != nil {
			resp.Diagnostics.AddError("Unable to Create Proxmox Firewall Rule", err.Error())
			return
		}
	} else {
		if err := r.client.UpdateFirewallRule(ctx, matches[0].Pos, firewallRuleRequestFromModel(plan)); err != nil {
			resp.Diagnostics.AddError("Unable to Update Proxmox Firewall Rule", err.Error())
			return
		}
	}
	// Re-read to get the updated pos
	rules, err = r.client.GetFirewallRules(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Firewall Rules After Update", err.Error())
		return
	}
	matches, matchDiags = matchFirewallRule(rules, plan)
	resp.Diagnostics.Append(matchDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(matches) == 0 {
		resp.Diagnostics.AddError("Firewall Rule Not Found After Update", "the rule was updated but could not be found on re-read")
		return
	}
	refreshed := firewallRuleStateFromAPI(plan, matches[0])
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *FirewallRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state firewallRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rules, err := r.client.GetFirewallRules(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Firewall Rules", err.Error())
		return
	}
	matches, matchDiags := matchFirewallRule(rules, state)
	resp.Diagnostics.Append(matchDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(matches) == 0 {
		return // already gone
	}
	if err := r.client.DeleteFirewallRule(ctx, matches[0].Pos); err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Firewall Rule", err.Error())
	}
}
