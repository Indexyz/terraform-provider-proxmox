// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestClientGuestFirewallObjects(t *testing.T) {
	ctx := context.Background()
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/101/firewall/aliases" && r.Method == http.MethodGet:
			handler := &lifecycleHandler{}
			handler.envelope(w, []map[string]any{{"name": "mail", "cidr": "192.0.2.5", "digest": "d1", "comment": "mail"}})
		case r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/101/firewall/aliases" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{"name": {"web"}, "cidr": {"192.0.2.0/24"}})
			writeEnvelope(t, w, nil)
		case r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/101/firewall/aliases/web" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{"cidr": {"192.0.2.0/24"}, "comment": {"web"}, "digest": {"d1"}})
			writeEnvelope(t, w, nil)
		case r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/101/firewall/aliases/web" && r.Method == http.MethodDelete:
			assertFormValues(t, r, url.Values{"digest": {"d1"}})
			writeEnvelope(t, w, nil)
		case strings.HasSuffix(r.URL.EscapedPath(), "/firewall/ipset") && r.Method == http.MethodPost:
			if r.Form.Get("rename") != "" {
				assertFormValues(t, r, url.Values{"name": {"office"}, "rename": {"office"}, "comment": {"office2"}, "digest": {"d2"}})
			} else {
				assertFormValues(t, r, url.Values{"name": {"office"}, "comment": {"office"}})
			}
			writeEnvelope(t, w, nil)
		case strings.HasSuffix(r.URL.EscapedPath(), "/firewall/ipset/office") && r.Method == http.MethodDelete:
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)

	aliases, err := client.GuestFirewallAliases(ctx, "pve-1", "qemu", 101)
	if err != nil || len(aliases) != 1 || aliases[0].Name != "mail" {
		t.Fatalf("unexpected aliases: %#v (%v)", aliases, err)
	}
	alias, err := client.GetGuestFirewallAlias(ctx, "pve-1", "qemu", 101, "mail")
	if err != nil || alias.Digest != "d1" {
		t.Fatalf("unexpected alias: %#v (%v)", alias, err)
	}
	if _, err := client.GetGuestFirewallAlias(ctx, "pve-1", "qemu", 101, "missing"); err == nil {
		t.Fatal("missing alias must not be found")
	}
	if err := client.CreateGuestFirewallAlias(ctx, "pve-1", "qemu", 101, GuestFirewallAlias{Name: "web", CIDR: "192.0.2.0/24"}); err != nil {
		t.Fatalf("CreateGuestFirewallAlias() unexpected error: %v", err)
	}
	if err := client.UpdateGuestFirewallAlias(ctx, "pve-1", "qemu", 101, GuestFirewallAlias{Name: "web", CIDR: "192.0.2.0/24", Comment: "web", Digest: "d1"}); err != nil {
		t.Fatalf("UpdateGuestFirewallAlias() unexpected error: %v", err)
	}
	if err := client.DeleteGuestFirewallAlias(ctx, "pve-1", "qemu", 101, "web", "d1"); err != nil {
		t.Fatalf("DeleteGuestFirewallAlias() unexpected error: %v", err)
	}
	if err := client.CreateGuestFirewallIPSet(ctx, "pve-1", "qemu", 101, GuestFirewallIPSet{Name: "office", Comment: "office"}); err != nil {
		t.Fatalf("CreateGuestFirewallIPSet() unexpected error: %v", err)
	}
	if err := client.UpdateGuestFirewallIPSet(ctx, "pve-1", "qemu", 101, GuestFirewallIPSet{Name: "office", Comment: "office2", Digest: "d2"}); err != nil {
		t.Fatalf("UpdateGuestFirewallIPSet() unexpected error: %v", err)
	}
	if err := client.DeleteGuestFirewallIPSet(ctx, "pve-1", "qemu", 101, "office"); err != nil {
		t.Fatalf("DeleteGuestFirewallIPSet() unexpected error: %v", err)
	}
	for _, call := range calls {
		if !strings.Contains(call, "/nodes/pve-1/qemu/101/firewall/") {
			t.Fatalf("unexpected path: %s", call)
		}
	}
}

func TestClientGuestFirewallIPSetEntries(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeEnvelope(t, w, []map[string]any{{"cidr": "10.0.0.5", "digest": "d1", "nomatch": 1}})
		case http.MethodPost:
			assertFormValues(t, r, url.Values{"cidr": {"10.0.0.0/24"}, "comment": {"net"}, "nomatch": {"1"}})
			writeEnvelope(t, w, nil)
		case http.MethodPut:
			assertFormValues(t, r, url.Values{"comment": {"net2"}, "nomatch": {"0"}, "digest": {"d1"}})
			writeEnvelope(t, w, nil)
		case http.MethodDelete:
			assertFormValues(t, r, url.Values{"digest": {"d1"}})
			writeEnvelope(t, w, nil)
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)

	entries, err := client.GuestFirewallIPSetEntries(ctx, "pve-1", "lxc", 102, "set")
	if err != nil || len(entries) != 1 || entries[0].CIDR != "10.0.0.5" || entries[0].NoMatch.Ptr() == nil {
		t.Fatalf("unexpected entries: %#v (%v)", entries, err)
	}
	if err := client.CreateGuestFirewallIPSetEntry(ctx, "pve-1", "lxc", 102, "set", GuestFirewallIPSetEntry{CIDR: "10.0.0.0/24", Comment: "net", NoMatch: proxmoxOptionalBool{value: boolPtr(true)}}); err != nil {
		t.Fatalf("CreateGuestFirewallIPSetEntry() unexpected error: %v", err)
	}
	if err := client.UpdateGuestFirewallIPSetEntry(ctx, "pve-1", "lxc", 102, "set", GuestFirewallIPSetEntry{CIDR: "10.0.0.5", Comment: "net2", NoMatch: proxmoxOptionalBool{value: boolPtr(false)}, Digest: "d1"}); err != nil {
		t.Fatalf("UpdateGuestFirewallIPSetEntry() unexpected error: %v", err)
	}
	if err := client.DeleteGuestFirewallIPSetEntry(ctx, "pve-1", "lxc", 102, "set", "10.0.0.5", "d1"); err != nil {
		t.Fatalf("DeleteGuestFirewallIPSetEntry() unexpected error: %v", err)
	}
}

func TestGuestFirewallAliasLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	aliases := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if err := r.ParseForm(); err != nil {
			handler.fail(w, "parse form: %v", err)
			return
		}
		path := r.URL.EscapedPath()
		switch {
		case path == "/api2/json/nodes/pve-1/qemu/101/firewall/aliases" && r.Method == http.MethodPost:
			aliases[r.Form.Get("name")] = map[string]any{"name": r.Form.Get("name"), "cidr": r.Form.Get("cidr"), "digest": "d1"}
			handler.envelope(w, nil)
		case path == "/api2/json/nodes/pve-1/qemu/101/firewall/aliases" && r.Method == http.MethodGet:
			list := make([]map[string]any, 0, len(aliases))
			for _, alias := range aliases {
				list = append(list, alias)
			}
			handler.envelope(w, list)
		case strings.HasPrefix(path, "/api2/json/nodes/pve-1/qemu/101/firewall/aliases/") && r.Method == http.MethodGet:
			name := strings.TrimPrefix(path, "/api2/json/nodes/pve-1/qemu/101/firewall/aliases/")
			alias, ok := aliases[name]
			if !ok {
				handler.envelope(w, []any{})
				return
			}
			handler.envelope(w, []map[string]any{alias})
		case strings.HasPrefix(path, "/api2/json/nodes/pve-1/qemu/101/firewall/aliases/") && r.Method == http.MethodPut:
			name := strings.TrimPrefix(path, "/api2/json/nodes/pve-1/qemu/101/firewall/aliases/")
			aliases[name]["cidr"] = r.Form.Get("cidr")
			aliases[name]["comment"] = r.Form.Get("comment")
			handler.envelope(w, nil)
		case strings.HasPrefix(path, "/api2/json/nodes/pve-1/qemu/101/firewall/aliases/") && r.Method == http.MethodDelete:
			delete(aliases, strings.TrimPrefix(path, "/api2/json/nodes/pve-1/qemu/101/firewall/aliases/"))
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, path)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &GuestFirewallAliasResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := guestFirewallAliasModel{Node: types.StringValue("pve-1"), GuestType: types.StringValue("qemu"), VMID: types.Int64Value(101), Name: types.StringValue("web"), CIDR: types.StringValue("192.0.2.0/24"), Comment: types.StringValue("web")}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created guestFirewallAliasModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() || created.ID.ValueString() != "pve-1/qemu/101/web" {
		t.Fatalf("unexpected created state: %#v (%v)", created, diags)
	}
	updated := config
	updated.CIDR = types.StringValue("192.0.2.0/25")
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}, Private: createResp.Private}
	res.Update(ctx, resource.UpdateRequest{Config: testResourceConfig(t, schema, updated), Plan: testResourcePlan(t, schema, updated), State: createResp.State, Private: createResp.Private}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", updateResp.Diagnostics)
	}
	var refreshed guestFirewallAliasModel
	if diags := updateResp.State.Get(ctx, &refreshed); diags.HasError() || refreshed.CIDR.ValueString() != "192.0.2.0/25" {
		t.Fatalf("update did not apply: %#v (%v)", refreshed, diags)
	}
	deleteResp := resource.DeleteResponse{State: updateResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() || len(aliases) != 0 {
		t.Fatalf("delete must remove the alias: %v %#v", deleteResp.Diagnostics, aliases)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

func TestGuestFirewallValidateAndImport(t *testing.T) {
	if diags := validateGuestFirewallScope(types.StringValue("pve-1"), types.StringValue("qemu"), types.Int64Value(101)); diags.HasError() {
		t.Fatalf("valid scope diagnostics: %v", diags)
	}
	for _, test := range []struct {
		node, kind string
		vmid       int64
	}{
		{"node_1", "qemu", 101}, {"pve-1", "xen", 101}, {"pve-1", "lxc", 99},
	} {
		if diags := validateGuestFirewallScope(types.StringValue(test.node), types.StringValue(test.kind), types.Int64Value(test.vmid)); !diags.HasError() {
			t.Fatalf("invalid scope accepted: %#v", test)
		}
	}
	if diags := validateGuestFirewallScope(types.StringUnknown(), types.StringUnknown(), types.Int64Unknown()); diags.HasError() {
		t.Fatalf("unknown scope must defer: %v", diags)
	}

	scope, name, diags := parseGuestFirewallImportID("pve-1/lxc/102/office")
	if diags.HasError() || scope.Node != "pve-1" || scope.GuestType != "lxc" || scope.VMID != 102 || name != "office" {
		t.Fatalf("unexpected import parse: %#v %q (%v)", scope, name, diags)
	}
	if _, _, diags := parseGuestFirewallImportID("pve-1/lxc/office"); !diags.HasError() {
		t.Fatal("short import id must fail")
	}
}

func TestGuestFirewallAliasSchemaRequiresReplacement(t *testing.T) {
	res := NewGuestFirewallAliasResource()
	schemaResp := testResourceSchema(t, res)
	if got, want := len(schemaResp.Schema.Attributes), 7; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	for _, name := range []string{"node", "guest_type", "name"} {
		attribute, ok := schemaResp.Schema.Attributes[name].(resourceschema.StringAttribute)
		if !ok || !attribute.IsRequired() || len(attribute.PlanModifiers) == 0 {
			t.Fatalf("%s must be required with a replacement modifier", name)
		}
	}
	vmid, ok := schemaResp.Schema.Attributes["vmid"].(resourceschema.Int64Attribute)
	if !ok || !vmid.IsRequired() || len(vmid.PlanModifiers) == 0 {
		t.Fatal("vmid must be required with a replacement modifier")
	}
}
