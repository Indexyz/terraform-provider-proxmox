// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestFirewallRulesDataSourceReadsGuestScope(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/qemu/101/firewall/rules": []map[string]any{
			{"pos": 1, "type": "out", "action": "ACCEPT", "enable": 1, "comment": "dns", "dest": "1.1.1.1", "proto": "udp", "dport": "53", "digest": "d2"},
			{"pos": 0, "type": "in", "action": "DROP", "enable": 0, "source": "10.0.0.0/8", "log": "nolog", "iface": "net0", "macro": "SSH", "icmp-type": "echo-request", "sport": "1024", "digest": "d1"},
		},
	}
	config := map[string]any{"scope": "guest", "node": "pve-1", "guest_type": "qemu", "vmid": int64(101)}
	readResp := runSDNDataSource(t, NewFirewallRulesDataSource(), responses, config)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state firewallRulesDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if len(state.Rules) != 2 || state.Rules[0].Pos.ValueInt64() != 0 || state.Rules[1].Pos.ValueInt64() != 1 {
		t.Fatalf("rules were not ordered by pos: %#v", state.Rules)
	}
	first := state.Rules[0]
	if first.Action.ValueString() != "DROP" || first.Enable.ValueInt64() != 0 || first.ICMPType.ValueString() != "echo-request" || first.Macro.ValueString() != "SSH" || first.Digest.ValueString() != "d1" {
		t.Fatalf("unexpected rule projection: %#v", first)
	}
	if state.Scope.ValueString() != "guest" || state.Node.ValueString() != "pve-1" || state.GuestType.ValueString() != "qemu" || state.VMID.ValueInt64() != 101 {
		t.Fatalf("scope refs were not kept: %#v", state)
	}
}

func TestFirewallRulesDataSourceScopes(t *testing.T) {
	cases := map[string]struct {
		path   string
		config map[string]any
	}{
		"cluster":        {"/api2/json/cluster/firewall/rules", map[string]any{}},
		"node":           {"/api2/json/nodes/pve-1/firewall/rules", map[string]any{"scope": "node", "node": "pve-1"}},
		"security group": {"/api2/json/cluster/firewall/groups/web", map[string]any{"scope": "security_group", "security_group": "web"}},
		"vnet":           {"/api2/json/cluster/sdn/vnets/vn1/firewall/rules", map[string]any{"scope": "vnet", "vnet": "vn1"}},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			readResp := runSDNDataSource(t, NewFirewallRulesDataSource(), map[string]any{testCase.path: []any{}}, testCase.config)
			if readResp.Diagnostics.HasError() {
				t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
			}
			var state firewallRulesDataSourceModel
			if diags := readResp.State.Get(context.Background(), &state); diags.HasError() || len(state.Rules) != 0 {
				t.Fatalf("empty read must stay empty: %#v (%v)", state.Rules, diags)
			}
		})
	}
}

func TestFirewallRulesDataSourceValidation(t *testing.T) {
	valid := []firewallRulesDataSourceModel{
		{},
		{Scope: types.StringValue("node"), Node: types.StringValue("pve-1")},
		{Scope: types.StringValue("guest"), Node: types.StringValue("pve-1"), GuestType: types.StringValue("lxc"), VMID: types.Int64Value(102)},
		{Scope: types.StringValue("security_group"), SecurityGroup: types.StringValue("web")},
		{Scope: types.StringValue("vnet"), Vnet: types.StringValue("vn1")},
		{Scope: types.StringUnknown(), Node: types.StringUnknown()},
	}
	for _, config := range valid {
		if diags := validateFirewallRulesScope(config); diags.HasError() {
			t.Fatalf("valid config diagnostics: %v (%#v)", diags, config)
		}
	}
	invalid := []firewallRulesDataSourceModel{
		{Scope: types.StringValue("world")},
		{Scope: types.StringValue("node")},
		{Scope: types.StringValue("guest"), Node: types.StringValue("pve-1"), GuestType: types.StringValue("qemu")},
		{Scope: types.StringValue("security_group")},
		{Scope: types.StringValue("vnet")},
	}
	for _, config := range invalid {
		if diags := validateFirewallRulesScope(config); !diags.HasError() {
			t.Fatalf("invalid config accepted: %#v", config)
		}
	}
}

func TestFirewallRulesDataSourceEmpty(t *testing.T) {
	readResp := runSDNDataSource(t, NewFirewallRulesDataSource(), map[string]any{
		"/api2/json/cluster/firewall/rules": []any{},
	}, map[string]any{})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("empty read failed: %v", readResp.Diagnostics)
	}
}
