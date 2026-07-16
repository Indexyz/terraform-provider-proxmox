// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &FirewallRuleResource{}
var _ resource.ResourceWithValidateConfig = &FirewallRuleResource{}

const firewallRuleManagedFieldsKey = "firewall-rule-managed-fields"

type FirewallRuleResource struct {
	client *Client
}

type firewallRuleModel struct {
	ID            types.String `tfsdk:"id"`
	Scope         types.String `tfsdk:"scope"`
	Node          types.String `tfsdk:"node"`
	GuestType     types.String `tfsdk:"guest_type"`
	VMID          types.Int64  `tfsdk:"vm_id"`
	SecurityGroup types.String `tfsdk:"security_group"`
	Type          types.String `tfsdk:"type"`
	Action        types.String `tfsdk:"action"`
	Enable        types.Int64  `tfsdk:"enable"`
	Comment       types.String `tfsdk:"comment"`
	Source        types.String `tfsdk:"source"`
	Dest          types.String `tfsdk:"dest"`
	Proto         types.String `tfsdk:"proto"`
	DPort         types.String `tfsdk:"dport"`
	SPort         types.String `tfsdk:"sport"`
	ICMPType      types.String `tfsdk:"icmp_type"`
	Iface         types.String `tfsdk:"iface"`
	Macro         types.String `tfsdk:"macro"`
	Log           types.String `tfsdk:"log"`
	Pos           types.Int64  `tfsdk:"pos"`
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
		MarkdownDescription: "Manages a Proxmox VE cluster, node, QEMU/LXC guest, or security-group firewall rule. Rules are matched by content; `pos` is computed and re-resolved immediately before every mutation. Import is not supported.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Content-based rule identifier."},
			"scope":          schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("cluster"), MarkdownDescription: "Ruleset scope: `cluster`, `node`, `guest`, or `security_group`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"node":           schema.StringAttribute{Optional: true, MarkdownDescription: "Node name for node and guest scopes. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"guest_type":     schema.StringAttribute{Optional: true, MarkdownDescription: "Guest type `qemu` or `lxc` for guest scope. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"vm_id":          schema.Int64Attribute{Optional: true, MarkdownDescription: "Guest VM/container ID for guest scope. Changes require replacement.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"security_group": schema.StringAttribute{Optional: true, MarkdownDescription: "Security group name for security_group scope. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"type":           schema.StringAttribute{Required: true, MarkdownDescription: "Rule direction: `in`, `out`, `forward`, or `group`.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"action":         schema.StringAttribute{Required: true, MarkdownDescription: "Rule action (`ACCEPT`, `DROP`, `REJECT`) or security group name.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"enable":         schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Whether the rule is active (1) or disabled (0). Defaults to 1."},
			"comment":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Rule comment."},
			"source":         schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Source CIDR, address, alias, or IP set.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"dest":           schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Destination CIDR, address, alias, or IP set.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"proto":          schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Protocol (for example `tcp`, `udp`, or `icmp`).", PlanModifiers: []planmodifier.String{replaceIf()}},
			"dport":          schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Destination ports.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"sport":          schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Source ports.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"icmp_type":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "ICMP type.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"iface":          schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Network interface.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"macro":          schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Predefined macro name.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"log":            schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Log level: `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, `debug`, or `nolog`.", PlanModifiers: []planmodifier.String{replaceIf()}},
			"pos":            schema.Int64Attribute{Computed: true, MarkdownDescription: "Current position in the ruleset.", PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
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

func (r *FirewallRuleResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config firewallRuleModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateFirewallRuleConfig(config)...)
}

func validateFirewallRuleConfig(config firewallRuleModel) diag.Diagnostics {
	var diags diag.Diagnostics
	scope := "cluster"
	if !config.Scope.IsNull() && !config.Scope.IsUnknown() {
		scope = config.Scope.ValueString()
	}
	configured := func(value types.String) bool { return !value.IsNull() && !value.IsUnknown() }
	configuredVMID := !config.VMID.IsNull() && !config.VMID.IsUnknown()
	unexpected := func(names ...string) {
		diags.AddError("Invalid firewall rule scope fields", fmt.Sprintf("scope %q does not support: %s", scope, strings.Join(names, ", ")))
	}
	switch scope {
	case "cluster":
		var fields []string
		if configured(config.Node) {
			fields = append(fields, "node")
		}
		if configured(config.GuestType) {
			fields = append(fields, "guest_type")
		}
		if configuredVMID {
			fields = append(fields, "vm_id")
		}
		if configured(config.SecurityGroup) {
			fields = append(fields, "security_group")
		}
		if len(fields) > 0 {
			unexpected(fields...)
		}
	case "node":
		if !configured(config.Node) {
			diags.AddAttributeError(path.Root("node"), "Missing firewall rule node", "node is required when scope is node")
		}
		var fields []string
		if configured(config.GuestType) {
			fields = append(fields, "guest_type")
		}
		if configuredVMID {
			fields = append(fields, "vm_id")
		}
		if configured(config.SecurityGroup) {
			fields = append(fields, "security_group")
		}
		if len(fields) > 0 {
			unexpected(fields...)
		}
	case "guest":
		if !configured(config.Node) {
			diags.AddAttributeError(path.Root("node"), "Missing firewall rule node", "node is required when scope is guest")
		}
		if !configured(config.GuestType) {
			diags.AddAttributeError(path.Root("guest_type"), "Missing firewall guest type", "guest_type is required when scope is guest")
		} else if !slices.Contains([]string{"qemu", "lxc"}, config.GuestType.ValueString()) {
			diags.AddAttributeError(path.Root("guest_type"), "Invalid firewall guest type", "guest_type must be qemu or lxc")
		}
		if !configuredVMID {
			diags.AddAttributeError(path.Root("vm_id"), "Missing firewall guest ID", "vm_id is required when scope is guest")
		} else if config.VMID.ValueInt64() < 100 || config.VMID.ValueInt64() > 999999999 {
			diags.AddAttributeError(path.Root("vm_id"), "Invalid firewall guest ID", "vm_id must be between 100 and 999999999")
		}
		if configured(config.SecurityGroup) {
			unexpected("security_group")
		}
	case "security_group":
		if !configured(config.SecurityGroup) {
			diags.AddAttributeError(path.Root("security_group"), "Missing firewall security group", "security_group is required when scope is security_group")
		}
		var fields []string
		if configured(config.Node) {
			fields = append(fields, "node")
		}
		if configured(config.GuestType) {
			fields = append(fields, "guest_type")
		}
		if configuredVMID {
			fields = append(fields, "vm_id")
		}
		if len(fields) > 0 {
			unexpected(fields...)
		}
	default:
		diags.AddAttributeError(path.Root("scope"), "Invalid firewall rule scope", "scope must be cluster, node, guest, or security_group")
	}
	if !config.Type.IsNull() && !config.Type.IsUnknown() && !slices.Contains([]string{"in", "out", "forward", "group"}, config.Type.ValueString()) {
		diags.AddAttributeError(path.Root("type"), "Invalid firewall rule type", "type must be in, out, forward, or group")
	}
	if !config.Enable.IsNull() && !config.Enable.IsUnknown() && config.Enable.ValueInt64() != 0 && config.Enable.ValueInt64() != 1 {
		diags.AddAttributeError(path.Root("enable"), "Invalid firewall rule enable value", "enable must be 0 or 1")
	}
	return diags
}

func firewallScopeFromModel(model firewallRuleModel) FirewallRuleScope {
	kind := model.Scope.ValueString()
	if kind == "" {
		kind = "cluster"
	}
	return FirewallRuleScope{
		Kind:          kind,
		Node:          model.Node.ValueString(),
		GuestType:     model.GuestType.ValueString(),
		VMID:          model.VMID.ValueInt64(),
		SecurityGroup: model.SecurityGroup.ValueString(),
	}
}

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
		diags.AddError("Ambiguous firewall rule match", fmt.Sprintf("%d rules found with identical content; cannot determine which pos to target. Remove duplicates or modify your config.", len(matches)))
		return nil, diags
	}
	return matches, diags
}

func firewallRuleStateFromAPI(model firewallRuleModel, rule FirewallRule) firewallRuleModel {
	enable := types.Int64Null()
	if rule.Enable.Ptr() != nil {
		enable = types.Int64Value(*rule.Enable.Ptr())
	}
	scope := firewallScopeFromModel(model)
	state := firewallRuleModel{
		Scope:         types.StringValue(scope.Kind),
		Node:          model.Node,
		GuestType:     model.GuestType,
		VMID:          model.VMID,
		SecurityGroup: model.SecurityGroup,
		Type:          types.StringValue(rule.Type),
		Action:        types.StringValue(rule.Action),
		Enable:        enable,
		Comment:       stringOrNull(rule.Comment),
		Source:        stringOrNull(rule.Source),
		Dest:          stringOrNull(rule.Dest),
		Proto:         stringOrNull(rule.Proto),
		DPort:         stringOrNull(rule.DPort),
		SPort:         stringOrNull(rule.SPort),
		ICMPType:      stringOrNull(rule.ICMPType),
		Iface:         stringOrNull(rule.Iface),
		Macro:         stringOrNull(rule.Macro),
		Log:           stringOrNull(rule.Log),
		Pos:           types.Int64Value(int64(rule.Pos)),
	}
	state.ID = types.StringValue(firewallRuleID(scope, state))
	return state
}

func firewallRuleID(scope FirewallRuleScope, model firewallRuleModel) string {
	location := scope.Kind
	switch scope.Kind {
	case "node":
		location += "/" + scope.Node
	case "guest":
		location += fmt.Sprintf("/%s/%s/%d", scope.Node, scope.GuestType, scope.VMID)
	case "security_group":
		location += "/" + scope.SecurityGroup
	}
	identity, _ := json.Marshal([]string{
		location,
		model.Type.ValueString(),
		model.Action.ValueString(),
		model.Source.ValueString(),
		model.Dest.ValueString(),
		model.Proto.ValueString(),
		model.DPort.ValueString(),
		model.SPort.ValueString(),
		model.ICMPType.ValueString(),
		model.Iface.ValueString(),
		model.Macro.ValueString(),
		model.Log.ValueString(),
	})
	return string(identity)
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

func firewallRuleManagedFields(model firewallRuleModel) []string {
	var fields []string
	addString := func(key string, value types.String) {
		if !value.IsNull() && !value.IsUnknown() {
			fields = append(fields, key)
		}
	}
	if !model.Enable.IsNull() && !model.Enable.IsUnknown() {
		fields = append(fields, "enable")
	}
	addString("comment", model.Comment)
	addString("source", model.Source)
	addString("dest", model.Dest)
	addString("proto", model.Proto)
	addString("dport", model.DPort)
	addString("sport", model.SPort)
	addString("icmp-type", model.ICMPType)
	addString("iface", model.Iface)
	addString("macro", model.Macro)
	addString("log", model.Log)
	slices.Sort(fields)
	return fields
}

func firewallRuleDeleteKeys(model firewallRuleModel, previouslyManaged []string) []string {
	current := firewallRuleManagedFields(model)
	var deleted []string
	for _, key := range previouslyManaged {
		if !slices.Contains(current, key) {
			deleted = append(deleted, key)
		}
	}
	slices.Sort(deleted)
	return deleted
}

func currentFirewallDigest(rules []FirewallRule) *string {
	for _, rule := range rules {
		if rule.Digest != "" {
			return &rule.Digest
		}
	}
	return nil
}

func (r *FirewallRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firewallRuleModel
	var config firewallRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	scope := firewallScopeFromModel(plan)
	existing, err := r.client.GetScopedFirewallRules(ctx, scope)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Firewall Rules", err.Error())
		return
	}
	matches, matchDiags := matchFirewallRule(existing, plan)
	resp.Diagnostics.Append(matchDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(matches) > 0 {
		resp.Diagnostics.AddError("Duplicate firewall rule", fmt.Sprintf("a firewall rule with identical content already exists at pos %d; remove the existing rule or modify your config", matches[0].Pos))
		return
	}
	createReq := firewallRuleRequestFromModel(plan)
	createReq.Digest = currentFirewallDigest(existing)
	if err := r.client.CreateScopedFirewallRule(ctx, scope, createReq); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Firewall Rule", err.Error())
		return
	}
	state, diags := r.readState(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *FirewallRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firewallRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if refreshed.ID.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *FirewallRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config firewallRuleModel
	var state firewallRuleModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManagedJSON, privateDiags := req.Private.GetKey(ctx, firewallRuleManagedFieldsKey)
	resp.Diagnostics.Append(privateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	var previouslyManaged []string
	if len(previouslyManagedJSON) > 0 {
		if err := json.Unmarshal(previouslyManagedJSON, &previouslyManaged); err != nil {
			resp.Diagnostics.AddError("Unable to Read Firewall Rule State", fmt.Sprintf("unable to decode managed fields: %v", err))
			return
		}
	}
	scope := firewallScopeFromModel(state)
	rules, err := r.client.GetScopedFirewallRules(ctx, scope)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Firewall Rules", err.Error())
		return
	}
	matches, matchDiags := matchFirewallRule(rules, state)
	resp.Diagnostics.Append(matchDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateReq := firewallRuleRequestFromModel(config)
	updateReq.Delete = firewallRuleDeleteKeys(config, previouslyManaged)
	updateReq.Digest = currentFirewallDigest(rules)
	if len(matches) == 0 {
		if err := r.client.CreateScopedFirewallRule(ctx, scope, updateReq); err != nil {
			resp.Diagnostics.AddError("Unable to Create Proxmox Firewall Rule", err.Error())
			return
		}
	} else if err := r.client.UpdateScopedFirewallRule(ctx, scope, matches[0].Pos, updateReq); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Firewall Rule", err.Error())
		return
	}
	refreshed, diags := r.readState(ctx, config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *FirewallRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state firewallRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	scope := firewallScopeFromModel(state)
	rules, err := r.client.GetScopedFirewallRules(ctx, scope)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Firewall Rules", err.Error())
		return
	}
	matches, matchDiags := matchFirewallRule(rules, state)
	resp.Diagnostics.Append(matchDiags...)
	if resp.Diagnostics.HasError() || len(matches) == 0 {
		return
	}
	if err := r.client.DeleteScopedFirewallRule(ctx, scope, matches[0].Pos, matches[0].Digest); err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Firewall Rule", err.Error())
	}
}

func (r *FirewallRuleResource) readState(ctx context.Context, model firewallRuleModel) (firewallRuleModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	rules, err := r.client.GetScopedFirewallRules(ctx, firewallScopeFromModel(model))
	if err != nil {
		if errors.Is(err, errNotFound) {
			return firewallRuleModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Firewall Rules", err.Error())
		return firewallRuleModel{}, diags
	}
	matches, matchDiags := matchFirewallRule(rules, model)
	diags.Append(matchDiags...)
	if diags.HasError() || len(matches) == 0 {
		return firewallRuleModel{ID: types.StringNull()}, diags
	}
	return firewallRuleStateFromAPI(model, matches[0]), diags
}

func (r *FirewallRuleResource) storeManagedFields(ctx context.Context, model firewallRuleModel, private interface {
	SetKey(context.Context, string, []byte) diag.Diagnostics
}, diags *diag.Diagnostics) {
	managedFields, err := json.Marshal(firewallRuleManagedFields(model))
	if err != nil {
		diags.AddError("Unable to Store Firewall Rule State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, firewallRuleManagedFieldsKey, managedFields)...)
}
