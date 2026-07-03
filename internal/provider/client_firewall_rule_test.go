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
			assertFormValues(t, r, url.Values{
				"type":   {"in"},
				"action": {"ACCEPT"},
				"source": {"10.0.0.0/8"},
				"proto":  {"tcp"},
				"dport":  {"443"},
				"enable": {"1"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/firewall/rules/0" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{
				"type":    {"in"},
				"action":  {"ACCEPT"},
				"source":  {"10.0.0.0/8"},
				"proto":   {"tcp"},
				"dport":   {"443"},
				"enable":  {"1"},
				"comment": {"updated"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/firewall/rules/0" && r.Method == http.MethodDelete:
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewClient(ctx, ClientConfig{
		Endpoint:       server.URL,
		APITokenID:     "terraform@pve!provider",
		APITokenSecret: "token-secret",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}

	// GET list
	rules, err := client.GetFirewallRules(ctx)
	if err != nil {
		t.Fatalf("GetFirewallRules() unexpected error: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].Pos != 0 || rules[0].Type != "in" || rules[0].Action != "ACCEPT" || rules[0].Source != "10.0.0.0/8" || rules[0].DPort != "443" {
		t.Fatalf("unexpected rule 0: %#v", rules[0])
	}
	if rules[1].Pos != 1 || rules[1].Type != "out" || rules[1].Action != "DROP" {
		t.Fatalf("unexpected rule 1: %#v", rules[1])
	}

	// Create
	if err := client.CreateFirewallRule(ctx, FirewallRuleRequest{
		Type:   "in",
		Action: "ACCEPT",
		Source: stringPtr("10.0.0.0/8"),
		Proto:  stringPtr("tcp"),
		DPort:  stringPtr("443"),
		Enable: intPtr64(1),
	}); err != nil {
		t.Fatalf("CreateFirewallRule() unexpected error: %v", err)
	}

	// Update (full payload)
	if err := client.UpdateFirewallRule(ctx, 0, FirewallRuleRequest{
		Type:    "in",
		Action:  "ACCEPT",
		Source:  stringPtr("10.0.0.0/8"),
		Proto:   stringPtr("tcp"),
		DPort:   stringPtr("443"),
		Enable:  intPtr64(1),
		Comment: stringPtr("updated"),
	}); err != nil {
		t.Fatalf("UpdateFirewallRule() unexpected error: %v", err)
	}

	// Delete
	if err := client.DeleteFirewallRule(ctx, 0); err != nil {
		t.Fatalf("DeleteFirewallRule() unexpected error: %v", err)
	}
}
