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

func TestClientClusterFirewallOptions(t *testing.T) {
	ctx := context.Background()
	putCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		if r.URL.Path != "/api2/json/cluster/firewall/options" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			writeEnvelope(t, w, map[string]any{
				"enable":         1,
				"ebtables":       0,
				"log_ratelimit":  "enable=1,burst=5,rate=1/second",
				"policy_forward": "DROP",
				"policy_in":      "DROP",
				"policy_out":     "ACCEPT",
			})
		case http.MethodPut:
			putCount++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() unexpected error: %v", err)
			}
			if putCount == 1 {
				assertFormValues(t, r, url.Values{
					"enable":         {"1"},
					"ebtables":       {"0"},
					"log_ratelimit":  {"enable=1,burst=5,rate=1/second"},
					"policy_forward": {"DROP"},
					"policy_in":      {"DROP"},
					"policy_out":     {"ACCEPT"},
				})
			} else if got, want := r.Form.Get("delete"), "ebtables,enable,log_ratelimit,policy_forward,policy_in,policy_out"; got != want {
				t.Fatalf("unexpected reset delete list: got %q want %q", got, want)
			}
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

	options, err := client.GetClusterFirewallOptions(ctx)
	if err != nil {
		t.Fatalf("GetClusterFirewallOptions() unexpected error: %v", err)
	}
	if options.Enable.Ptr() == nil || !*options.Enable.Ptr() {
		t.Fatalf("expected enable=true, got %#v", options.Enable)
	}
	if options.Ebtables.Ptr() == nil || *options.Ebtables.Ptr() {
		t.Fatalf("expected ebtables=false, got %#v", options.Ebtables)
	}
	if options.PolicyIn != "DROP" {
		t.Fatalf("unexpected policy_in: %q", options.PolicyIn)
	}

	if err := client.UpdateClusterFirewallOptions(ctx, ClusterFirewallOptionsRequest{
		Enable:        boolPtr(true),
		Ebtables:      boolPtr(false),
		LogRateLimit:  stringPtr("enable=1,burst=5,rate=1/second"),
		PolicyForward: stringPtr("DROP"),
		PolicyIn:      stringPtr("DROP"),
		PolicyOut:     stringPtr("ACCEPT"),
	}); err != nil {
		t.Fatalf("UpdateClusterFirewallOptions() unexpected error: %v", err)
	}
	if err := client.ResetClusterFirewallOptions(ctx); err != nil {
		t.Fatalf("ResetClusterFirewallOptions() unexpected error: %v", err)
	}
}
