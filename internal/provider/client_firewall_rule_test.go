// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestClientFirewallRuleMethods(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.URL.Path == "/api2/json/cluster/firewall/rules" && r.Method == http.MethodGet:
			writeEnvelope(t, w, []map[string]any{
				{"pos": 0, "type": "in", "action": "ACCEPT", "source": "10.0.0.0/8", "proto": "tcp", "dport": "443", "enable": 1, "comment": "https"},
				{"pos": 1, "type": "out", "action": "DROP", "dest": "192.168.0.0/16", "enable": 0},
			})
		case r.URL.Path == "/api2/json/cluster/firewall/rules" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{"type": {"in"}, "action": {"ACCEPT"}, "source": {"10.0.0.0/8"}, "proto": {"tcp"}, "dport": {"443"}, "enable": {"1"}})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/firewall/rules/0" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{"type": {"in"}, "action": {"ACCEPT"}, "source": {"10.0.0.0/8"}, "proto": {"tcp"}, "dport": {"443"}, "enable": {"1"}, "comment": {"updated"}})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/firewall/rules/0" && r.Method == http.MethodDelete:
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewClient(ctx, ClientConfig{Endpoint: server.URL, APITokenID: "terraform@pve!provider", APITokenSecret: "token-secret", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}
	rules, err := client.GetFirewallRules(ctx)
	if err != nil {
		t.Fatalf("GetFirewallRules() unexpected error: %v", err)
	}
	if len(rules) != 2 || rules[0].Pos != 0 || rules[0].Type != "in" || rules[0].Action != "ACCEPT" || rules[0].Source != "10.0.0.0/8" || rules[0].DPort != "443" {
		t.Fatalf("unexpected rules: %#v", rules)
	}
	if err := client.CreateFirewallRule(ctx, FirewallRuleRequest{Type: "in", Action: "ACCEPT", Source: stringPtr("10.0.0.0/8"), Proto: stringPtr("tcp"), DPort: stringPtr("443"), Enable: intPtr64(1)}); err != nil {
		t.Fatalf("CreateFirewallRule() unexpected error: %v", err)
	}
	if err := client.UpdateFirewallRule(ctx, 0, FirewallRuleRequest{Type: "in", Action: "ACCEPT", Source: stringPtr("10.0.0.0/8"), Proto: stringPtr("tcp"), DPort: stringPtr("443"), Enable: intPtr64(1), Comment: stringPtr("updated")}); err != nil {
		t.Fatalf("UpdateFirewallRule() unexpected error: %v", err)
	}
	if err := client.DeleteFirewallRule(ctx, 0); err != nil {
		t.Fatalf("DeleteFirewallRule() unexpected error: %v", err)
	}
}

func TestFirewallRulesPath(t *testing.T) {
	tests := []struct {
		scope FirewallRuleScope
		want  string
	}{
		{FirewallRuleScope{Kind: "cluster"}, "/cluster/firewall/rules"},
		{FirewallRuleScope{Kind: "node", Node: "pve-1"}, "/nodes/pve-1/firewall/rules"},
		{FirewallRuleScope{Kind: "guest", Node: "pve-1", GuestType: "qemu", VMID: 101}, "/nodes/pve-1/qemu/101/firewall/rules"},
		{FirewallRuleScope{Kind: "guest", Node: "pve-1", GuestType: "lxc", VMID: 102}, "/nodes/pve-1/lxc/102/firewall/rules"},
		{FirewallRuleScope{Kind: "security_group", SecurityGroup: "web-servers"}, "/cluster/firewall/groups/web-servers"},
	}
	for _, test := range tests {
		got, err := firewallRulesPath(test.scope)
		if err != nil || got != test.want {
			t.Fatalf("firewallRulesPath(%#v) = %q, %v; want %q", test.scope, got, err, test.want)
		}
	}
	if _, err := firewallRulesPath(FirewallRuleScope{Kind: "guest", GuestType: "openvz"}); err == nil {
		t.Fatal("expected unsupported guest type error")
	}
}

func TestClientScopedFirewallRuleMutation(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		if r.URL.Path != "/api2/json/cluster/firewall/groups/web-servers" && r.URL.Path != "/api2/json/cluster/firewall/groups/web-servers/0" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			writeEnvelope(t, w, []map[string]any{{"pos": 0, "type": "in", "action": "ACCEPT", "digest": "abc123"}})
		case http.MethodPost:
			assertFormValues(t, r, url.Values{"action": {"ACCEPT"}, "digest": {"abc123"}, "type": {"in"}})
			writeEnvelope(t, w, nil)
		case http.MethodPut:
			assertFormValues(t, r, url.Values{"action": {"ACCEPT"}, "delete": {"comment,log"}, "digest": {"abc123"}, "type": {"in"}})
			writeEnvelope(t, w, nil)
		case http.MethodDelete:
			assertFormValues(t, r, url.Values{"digest": {"abc123"}})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()
	client, err := NewClient(ctx, ClientConfig{Endpoint: server.URL, APITokenID: "terraform@pve!provider", APITokenSecret: "token-secret", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}
	scope := FirewallRuleScope{Kind: "security_group", SecurityGroup: "web-servers"}
	rules, err := client.GetScopedFirewallRules(ctx, scope)
	if err != nil || len(rules) != 1 || rules[0].Digest != "abc123" {
		t.Fatalf("GetScopedFirewallRules() = %#v, %v", rules, err)
	}
	req := FirewallRuleRequest{Type: "in", Action: "ACCEPT", Digest: stringPtr("abc123")}
	if err := client.CreateScopedFirewallRule(ctx, scope, req); err != nil {
		t.Fatalf("CreateScopedFirewallRule() unexpected error: %v", err)
	}
	req.Delete = []string{"log", "comment"}
	if err := client.UpdateScopedFirewallRule(ctx, scope, 0, req); err != nil {
		t.Fatalf("UpdateScopedFirewallRule() unexpected error: %v", err)
	}
	if err := client.DeleteScopedFirewallRule(ctx, scope, 0, "abc123"); err != nil {
		t.Fatalf("DeleteScopedFirewallRule() unexpected error: %v", err)
	}
}
