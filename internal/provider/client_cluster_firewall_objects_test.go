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

func TestClientClusterFirewallObjects(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.URL.Path == "/api2/json/cluster/firewall/aliases" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{"cidr": {"10.20.0.10"}, "comment": {"monitor"}, "name": {"monitoring"}})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/firewall/aliases/monitoring" && r.Method == http.MethodGet:
			writeEnvelope(t, w, map[string]any{"cidr": "10.20.0.10", "comment": "monitor", "digest": "alias-digest", "name": "monitoring"})
		case r.URL.Path == "/api2/json/cluster/firewall/aliases/monitoring" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{"cidr": {"10.20.0.11"}, "comment": {""}, "digest": {"alias-digest"}})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/firewall/aliases/monitoring" && r.Method == http.MethodDelete:
			assertFormValues(t, r, url.Values{"digest": {"alias-digest"}})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/firewall/ipset" && r.Method == http.MethodGet:
			writeEnvelope(t, w, []map[string]any{{"comment": "trusted", "digest": "set-digest", "name": "trusted"}})
		case r.URL.Path == "/api2/json/cluster/firewall/ipset" && r.Method == http.MethodPost:
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() unexpected error: %v", err)
			}
			if r.Form.Get("rename") == "" {
				assertFormValues(t, r, url.Values{"comment": {"trusted"}, "name": {"trusted"}})
			} else {
				assertFormValues(t, r, url.Values{"comment": {"updated"}, "digest": {"set-digest"}, "name": {"trusted"}, "rename": {"trusted"}})
			}
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/firewall/ipset/trusted" && r.Method == http.MethodDelete:
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/firewall/ipset/trusted" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{"cidr": {"10.10.0.0/24"}, "comment": {"management"}, "nomatch": {"0"}})
			writeEnvelope(t, w, nil)
		case r.URL.EscapedPath() == "/api2/json/cluster/firewall/ipset/trusted/10.10.0.0%2F24" && r.Method == http.MethodGet:
			writeEnvelope(t, w, map[string]any{"cidr": "10.10.0.0/24", "comment": "management", "digest": "entry-digest", "nomatch": 0})
		case r.URL.EscapedPath() == "/api2/json/cluster/firewall/ipset/trusted/10.10.0.0%2F24" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{"comment": {"updated"}, "digest": {"entry-digest"}, "nomatch": {"1"}})
			writeEnvelope(t, w, nil)
		case r.URL.EscapedPath() == "/api2/json/cluster/firewall/ipset/trusted/10.10.0.0%2F24" && r.Method == http.MethodDelete:
			assertFormValues(t, r, url.Values{"digest": {"entry-digest"}})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/firewall/groups" && r.Method == http.MethodGet:
			writeEnvelope(t, w, []map[string]any{{"comment": "web", "digest": "group-digest", "group": "web-servers"}})
		case r.URL.Path == "/api2/json/cluster/firewall/groups" && r.Method == http.MethodPost:
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() unexpected error: %v", err)
			}
			if r.Form.Get("rename") == "" {
				assertFormValues(t, r, url.Values{"comment": {"web"}, "group": {"web-servers"}})
			} else {
				assertFormValues(t, r, url.Values{"comment": {"updated"}, "digest": {"group-digest"}, "group": {"web-servers"}, "rename": {"web-servers"}})
			}
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/firewall/groups/web-servers" && r.Method == http.MethodDelete:
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s (%s)", r.Method, r.URL.String(), r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	client, err := NewClient(ctx, ClientConfig{Endpoint: server.URL, APITokenID: "terraform@pve!provider", APITokenSecret: "token-secret", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}
	alias := ClusterFirewallAlias{Name: "monitoring", CIDR: "10.20.0.10", Comment: "monitor"}
	if err := client.CreateClusterFirewallAlias(ctx, alias); err != nil {
		t.Fatalf("CreateClusterFirewallAlias() unexpected error: %v", err)
	}
	alias, err = client.GetClusterFirewallAlias(ctx, alias.Name)
	if err != nil || alias.Digest != "alias-digest" {
		t.Fatalf("GetClusterFirewallAlias() = %#v, %v", alias, err)
	}
	alias.CIDR, alias.Comment = "10.20.0.11", ""
	if err := client.UpdateClusterFirewallAlias(ctx, alias); err != nil {
		t.Fatalf("UpdateClusterFirewallAlias() unexpected error: %v", err)
	}
	if err := client.DeleteClusterFirewallAlias(ctx, alias.Name, alias.Digest); err != nil {
		t.Fatalf("DeleteClusterFirewallAlias() unexpected error: %v", err)
	}

	set := ClusterFirewallIPSet{Name: "trusted", Comment: "trusted"}
	if err := client.CreateClusterFirewallIPSet(ctx, set); err != nil {
		t.Fatalf("CreateClusterFirewallIPSet() unexpected error: %v", err)
	}
	set, err = client.GetClusterFirewallIPSet(ctx, set.Name)
	if err != nil || set.Digest != "set-digest" {
		t.Fatalf("GetClusterFirewallIPSet() = %#v, %v", set, err)
	}
	set.Comment = "updated"
	if err := client.UpdateClusterFirewallIPSet(ctx, set); err != nil {
		t.Fatalf("UpdateClusterFirewallIPSet() unexpected error: %v", err)
	}

	entry := ClusterFirewallIPSetEntry{CIDR: "10.10.0.0/24", Comment: "management", NoMatch: proxmoxOptionalBool{value: boolPtr(false)}}
	if err := client.CreateClusterFirewallIPSetEntry(ctx, set.Name, entry); err != nil {
		t.Fatalf("CreateClusterFirewallIPSetEntry() unexpected error: %v", err)
	}
	entry, err = client.GetClusterFirewallIPSetEntry(ctx, set.Name, entry.CIDR)
	if err != nil || entry.NoMatch.Ptr() == nil || *entry.NoMatch.Ptr() {
		t.Fatalf("GetClusterFirewallIPSetEntry() = %#v, %v", entry, err)
	}
	entry.Comment, entry.NoMatch = "updated", proxmoxOptionalBool{value: boolPtr(true)}
	if err := client.UpdateClusterFirewallIPSetEntry(ctx, set.Name, entry); err != nil {
		t.Fatalf("UpdateClusterFirewallIPSetEntry() unexpected error: %v", err)
	}
	if err := client.DeleteClusterFirewallIPSetEntry(ctx, set.Name, entry.CIDR, entry.Digest); err != nil {
		t.Fatalf("DeleteClusterFirewallIPSetEntry() unexpected error: %v", err)
	}
	if err := client.DeleteClusterFirewallIPSet(ctx, set.Name); err != nil {
		t.Fatalf("DeleteClusterFirewallIPSet() unexpected error: %v", err)
	}

	group := ClusterFirewallSecurityGroup{Name: "web-servers", Comment: "web"}
	if err := client.CreateClusterFirewallSecurityGroup(ctx, group); err != nil {
		t.Fatalf("CreateClusterFirewallSecurityGroup() unexpected error: %v", err)
	}
	group, err = client.GetClusterFirewallSecurityGroup(ctx, group.Name)
	if err != nil || group.Digest != "group-digest" {
		t.Fatalf("GetClusterFirewallSecurityGroup() = %#v, %v", group, err)
	}
	group.Comment = "updated"
	if err := client.UpdateClusterFirewallSecurityGroup(ctx, group); err != nil {
		t.Fatalf("UpdateClusterFirewallSecurityGroup() unexpected error: %v", err)
	}
	if err := client.DeleteClusterFirewallSecurityGroup(ctx, group.Name); err != nil {
		t.Fatalf("DeleteClusterFirewallSecurityGroup() unexpected error: %v", err)
	}
}
