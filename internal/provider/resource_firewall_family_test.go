// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestClusterFirewallObjectResourceSchemas(t *testing.T) {
	tests := []struct {
		resource resource.Resource
		count    int
	}{
		{NewClusterFirewallAliasResource(), 4},
		{NewClusterFirewallIPSetResource(), 3},
		{NewClusterFirewallIPSetEntryResource(), 5},
		{NewClusterFirewallSecurityGroupResource(), 3},
		{NewFirewallRuleResource(), 20},
	}
	for _, test := range tests {
		var resp resource.SchemaResponse
		test.resource.Schema(context.Background(), resource.SchemaRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
		}
		if got := len(resp.Schema.Attributes); got != test.count {
			t.Fatalf("unexpected attribute count: got %d want %d", got, test.count)
		}
	}
}

func TestValidateFirewallRuleConfig(t *testing.T) {
	valid := []firewallRuleModel{
		{Type: types.StringValue("in"), Enable: types.Int64Value(1)},
		{Scope: types.StringValue("node"), Node: types.StringValue("pve-1"), Type: types.StringValue("out")},
		{Scope: types.StringValue("guest"), Node: types.StringValue("pve-1"), GuestType: types.StringValue("qemu"), VMID: types.Int64Value(101), Type: types.StringValue("in")},
		{Scope: types.StringValue("security_group"), SecurityGroup: types.StringValue("web-servers"), Type: types.StringValue("forward")},
	}
	for _, config := range valid {
		if diags := validateFirewallRuleConfig(config); diags.HasError() {
			t.Fatalf("valid config diagnostics: %v", diags)
		}
	}
	invalid := []firewallRuleModel{
		{Scope: types.StringValue("node"), Type: types.StringValue("in")},
		{Scope: types.StringValue("guest"), Node: types.StringValue("pve-1"), GuestType: types.StringValue("openvz"), VMID: types.Int64Value(101), Type: types.StringValue("in")},
		{Scope: types.StringValue("guest"), Node: types.StringValue("pve-1"), GuestType: types.StringValue("lxc"), VMID: types.Int64Value(99), Type: types.StringValue("in")},
		{Scope: types.StringValue("security_group"), Node: types.StringValue("pve-1"), SecurityGroup: types.StringValue("web"), Type: types.StringValue("in")},
		{Scope: types.StringValue("cluster"), SecurityGroup: types.StringValue("web"), Type: types.StringValue("in")},
		{Scope: types.StringValue("sdn"), Type: types.StringValue("in")},
		{Type: types.StringValue("invalid")},
		{Type: types.StringValue("in"), Enable: types.Int64Value(2)},
	}
	for _, config := range invalid {
		if diags := validateFirewallRuleConfig(config); !diags.HasError() {
			t.Fatalf("expected invalid config diagnostics: %#v", config)
		}
	}
}

func TestClusterFirewallObjectFieldOwnership(t *testing.T) {
	if got := clusterFirewallManagedString("server-owned", types.StringNull(), false); got != "server-owned" {
		t.Fatalf("unmanaged comment was overwritten: %q", got)
	}
	if got := clusterFirewallManagedString("managed", types.StringNull(), true); got != "" {
		t.Fatalf("removed managed comment was not cleared: %q", got)
	}
	if got := clusterFirewallManagedString("old", types.StringValue("new"), false); got != "new" {
		t.Fatalf("configured comment was not applied: %q", got)
	}
	current := proxmoxOptionalBool{value: boolPointerValue(types.BoolValue(true))}
	if got := clusterFirewallManagedBool(current, types.BoolNull(), false); got.Ptr() == nil || !*got.Ptr() {
		t.Fatalf("unmanaged nomatch was overwritten: %#v", got)
	}
	if got := clusterFirewallManagedBool(current, types.BoolNull(), true); got.Ptr() == nil || *got.Ptr() {
		t.Fatalf("removed managed nomatch was not reset: %#v", got)
	}
}

func TestFirewallRuleManagedFields(t *testing.T) {
	config := firewallRuleModel{Comment: types.StringValue("updated"), Enable: types.Int64Value(1)}
	got := firewallRuleDeleteKeys(config, []string{"comment", "enable", "log", "source"})
	want := []string{"log", "source"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("unexpected delete keys: got %v want %v", got, want)
	}
	if got := firewallRuleDeleteKeys(config, nil); len(got) != 0 {
		t.Fatalf("unmanaged imported/default fields must not be deleted: %v", got)
	}
}

func TestFirewallRuleStateUsesAPIValues(t *testing.T) {
	model := firewallRuleModel{
		Type:   types.StringValue("in"),
		Action: types.StringValue("ACCEPT"),
		Source: types.StringUnknown(),
		Dest:   types.StringUnknown(),
		Proto:  types.StringUnknown(),
		DPort:  types.StringUnknown(),
		SPort:  types.StringUnknown(),
		Log:    types.StringUnknown(),
	}
	rule := FirewallRule{Pos: 2, Type: "in", Action: "ACCEPT", Source: "10.0.0.0/8", Proto: "tcp", DPort: "443"}
	state := firewallRuleStateFromAPI(model, rule)
	if state.Source.IsUnknown() || state.Dest.IsUnknown() || state.Proto.IsUnknown() || state.DPort.IsUnknown() || state.SPort.IsUnknown() || state.Log.IsUnknown() {
		t.Fatalf("state contains unknown API-backed fields: %#v", state)
	}
	if state.Source.ValueString() != "10.0.0.0/8" || state.Proto.ValueString() != "tcp" || state.DPort.ValueString() != "443" || !state.Dest.IsNull() {
		t.Fatalf("unexpected API-backed state: %#v", state)
	}
	other := state
	other.DPort = types.StringValue("8443")
	if firewallRuleID(firewallScopeFromModel(state), state) == firewallRuleID(firewallScopeFromModel(other), other) {
		t.Fatal("different rule identity fields produced the same ID")
	}
}

func TestMatchFirewallRuleRejectsDuplicates(t *testing.T) {
	model := firewallRuleModel{Type: types.StringValue("in"), Action: types.StringValue("ACCEPT")}
	rules := []FirewallRule{{Pos: 0, Type: "in", Action: "ACCEPT"}, {Pos: 1, Type: "in", Action: "ACCEPT"}}
	if matches, diags := matchFirewallRule(rules, model); matches != nil || !diags.HasError() {
		t.Fatalf("expected ambiguous match diagnostics: matches=%v diagnostics=%v", matches, diags)
	}
}
