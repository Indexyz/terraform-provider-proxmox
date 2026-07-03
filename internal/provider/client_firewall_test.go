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

func TestClientNodeFirewallOptions(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/firewall/options" && r.Method == http.MethodGet:
			writeEnvelope(t, w, map[string]any{
				"enable":              1,
				"log_level_in":        "info",
				"protection_synflood": 1,
				"nosmurfs":            1,
			})
		case r.URL.Path == "/api2/json/nodes/pve-1/firewall/options" && r.Method == http.MethodPut:
			r.ParseForm()
			if r.Form.Get("delete") == "" {
				// Update call
				assertFormValues(t, r, url.Values{
					"enable":              {"1"},
					"log_level_in":        {"info"},
					"protection_synflood": {"1"},
					"nosmurfs":            {"1"},
				})
			} else {
				// Reset call (delete all)
				if r.Form.Get("delete") == "" {
					t.Fatal("expected delete keys on reset")
				}
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

	// Read
	opts, err := client.GetNodeFirewallOptions(ctx, "pve-1")
	if err != nil {
		t.Fatalf("GetNodeFirewallOptions() unexpected error: %v", err)
	}
	if opts.Enable.Ptr() == nil || !*opts.Enable.Ptr() {
		t.Fatalf("expected enable=true, got %#v", opts.Enable)
	}
	if opts.LogLevelIn != "info" {
		t.Fatalf("expected log_level_in=info, got %q", opts.LogLevelIn)
	}

	// Update
	if err := client.UpdateNodeFirewallOptions(ctx, "pve-1", NodeFirewallOptionsRequest{
		Enable:             boolPtr(true),
		LogLevelIn:         stringPtr("info"),
		ProtectionSynflood: boolPtr(true),
		Nosmurfs:           boolPtr(true),
	}); err != nil {
		t.Fatalf("UpdateNodeFirewallOptions() unexpected error: %v", err)
	}

	// Reset
	if err := client.DeleteNodeFirewallOptions(ctx, "pve-1"); err != nil {
		t.Fatalf("DeleteNodeFirewallOptions() unexpected error: %v", err)
	}
}
