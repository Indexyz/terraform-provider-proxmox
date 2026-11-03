// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestClientSDNPrefixListCRUD(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/prefix-lists":
			_, _ = w.Write([]byte(`{"data":[` + sdnPrefixListJSON + `]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/prefix-lists":
			assertFormValues(t, r, url.Values{
				"entries":    {"action=deny,prefix=10.0.0.0/8", "action=permit,prefix=10.1.0.0/16,le=32"},
				"id":         {"pl1"},
				"lock-token": {testSDNLockToken},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/sdn/prefix-lists/pl1":
			assertFormValues(t, r, url.Values{
				"delete":     {"entries"},
				"digest":     {"pl-digest"},
				"id":         {"pl1"},
				"lock-token": {testSDNLockToken},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/prefix-lists/pl1":
			assertFormValues(t, r, url.Values{"lock-token": {testSDNLockToken}})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	list, err := client.GetSDNPrefixList(ctx, "pl1")
	if err != nil || len(list.Entries) != 1 {
		t.Fatalf("GetSDNPrefixList() = %#v, %v", list, err)
	}
	if err := client.CreateSDNPrefixList(ctx, "pl1", SDNPrefixListRequest{
		Entries:   []string{"action=permit,prefix=10.1.0.0/16,le=32", "action=deny,prefix=10.0.0.0/8"},
		LockToken: testSDNLockToken,
	}); err != nil {
		t.Fatalf("CreateSDNPrefixList() unexpected error: %v", err)
	}
	if err := client.UpdateSDNPrefixList(ctx, "pl1", SDNPrefixListRequest{
		LockToken: testSDNLockToken, Digest: stringPtr("pl-digest"), Delete: []string{"entries"},
	}); err != nil {
		t.Fatalf("UpdateSDNPrefixList() unexpected error: %v", err)
	}
	if err := client.DeleteSDNPrefixList(ctx, "pl1", testSDNLockToken); err != nil {
		t.Fatalf("DeleteSDNPrefixList() unexpected error: %v", err)
	}
}

func TestSDNPrefixListValidationAndState(t *testing.T) {
	valid := sdnPrefixListModel{
		ListID:  types.StringValue("pl1"),
		Entries: testStringList(t, "action=permit,prefix=10.0.0.0/24"),
	}
	if diags := validateSDNPrefixListConfig(valid); diags.HasError() {
		t.Fatalf("valid diagnostics: %v", diags)
	}
	invalid := map[string]sdnPrefixListModel{
		"reserved id": {ListID: types.StringValue("only_default")},
		"bad id":      {ListID: types.StringValue("pl.1")},
		"no action":   {ListID: types.StringValue("pl1"), Entries: testStringList(t, "prefix=10.0.0.0/24")},
		"no prefix":   {ListID: types.StringValue("pl1"), Entries: testStringList(t, "action=permit")},
	}
	for name, config := range invalid {
		if diags := validateSDNPrefixListConfig(config); !diags.HasError() {
			t.Fatalf("%s: expected invalid diagnostics: %#v", name, config)
		}
	}

	state, diags := sdnPrefixListStateFromAPI(context.Background(), SDNPrefixList{
		ID: "pl1", Entries: []string{"action=permit,prefix=10.0.0.0/24"}, State: "changed",
		Pending: map[string]json.RawMessage{"entries": json.RawMessage(`["action=deny,prefix=10.9.0.0/16"]`)},
	})
	if diags.HasError() {
		t.Fatalf("state mapping diagnostics: %v", diags)
	}
	var entries []string
	if diags := state.Entries.ElementsAs(context.Background(), &entries, false); diags.HasError() || len(entries) != 1 || entries[0] != "action=deny,prefix=10.9.0.0/16" {
		t.Fatalf("staged entries must be applied verbatim: %#v (%v)", entries, diags)
	}

	deleted, diags := sdnPrefixListStateFromAPI(context.Background(), SDNPrefixList{
		ID: "pl1", Entries: []string{"action=permit,prefix=10.0.0.0/24"},
		Pending: map[string]json.RawMessage{"entries": json.RawMessage(`"deleted"`)},
	})
	if diags.HasError() || !deleted.Entries.IsNull() {
		t.Fatalf("staged entry removal must clear the list: %#v (%v)", deleted.Entries, diags)
	}
}

func TestSDNPrefixListLifecycle(t *testing.T) {
	lists := map[string]map[string]any{}
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/lock":
			handler.envelope(w, testSDNLockToken)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/lock":
			handler.envelope(w, nil)
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/prefix-lists":
			lists["pl1"] = map[string]any{"id": "pl1", "entries": []string{"action=permit,prefix=10.0.0.0/24"}, "state": "new"}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/prefix-lists":
			list := make([]map[string]any, 0, len(lists))
			for _, entry := range lists {
				list = append(list, entry)
			}
			handler.envelope(w, list)
		default:
			handler.fail(w, "unexpected prefix list request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &SDNPrefixListResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := sdnPrefixListModel{
		ListID:  types.StringValue("pl1"),
		Entries: testStringList(t, "action=permit,prefix=10.0.0.0/24"),
		Pending: types.MapNull(types.StringType),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created sdnPrefixListModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() || created.State.ValueString() != "new" {
		t.Fatalf("unexpected created state: %#v (%v)", created, diags)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

func TestClientSDNRouteMapEntryCRUD(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/route-maps/entries":
			_, _ = w.Write([]byte(`{"data":[` + sdnRouteMapEntryJSON + `]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/route-maps/entries":
			assertFormValues(t, r, url.Values{
				"action":       {"permit"},
				"call":         {"rm2"},
				"exit-action":  {"on-match-next"},
				"lock-token":   {testSDNLockToken},
				"match":        {"ip-address=10.0.0.0/8"},
				"order":        {"10"},
				"route-map-id": {"rm1"},
				"set":          {"local-preference=100"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/sdn/route-maps/entries/rm1/10":
			assertFormValues(t, r, url.Values{
				"action":       {"deny"},
				"delete":       {"call", "set"},
				"digest":       {"rm-digest"},
				"lock-token":   {testSDNLockToken},
				"order":        {"10"},
				"route-map-id": {"rm1"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/route-maps/entries/rm1/10":
			assertFormValues(t, r, url.Values{"lock-token": {testSDNLockToken}})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	entry, err := client.GetSDNRouteMapEntry(ctx, "rm1", 10)
	if err != nil || entry.Action != "permit" {
		t.Fatalf("GetSDNRouteMapEntry() = %#v, %v", entry, err)
	}
	if err := client.CreateSDNRouteMapEntry(ctx, SDNRouteMapEntryRequest{
		RouteMapID: "rm1", Order: 10, Action: "permit", Set: []string{"local-preference=100"},
		Match: []string{"ip-address=10.0.0.0/8"}, ExitAction: stringPtr("on-match-next"), Call: stringPtr("rm2"),
		LockToken: testSDNLockToken,
	}); err != nil {
		t.Fatalf("CreateSDNRouteMapEntry() unexpected error: %v", err)
	}
	if err := client.UpdateSDNRouteMapEntry(ctx, "rm1", 10, SDNRouteMapEntryRequest{
		RouteMapID: "rm1", Order: 10, Action: "deny", LockToken: testSDNLockToken,
		Digest: stringPtr("rm-digest"), Delete: []string{"set", "call"},
	}); err != nil {
		t.Fatalf("UpdateSDNRouteMapEntry() unexpected error: %v", err)
	}
	if err := client.DeleteSDNRouteMapEntry(ctx, "rm1", 10, testSDNLockToken); err != nil {
		t.Fatalf("DeleteSDNRouteMapEntry() unexpected error: %v", err)
	}
}

func TestSDNRouteMapEntryValidation(t *testing.T) {
	valid := sdnRouteMapEntryModel{
		RouteMapID: types.StringValue("rm1"), Order: types.Int64Value(10), Action: types.StringValue("permit"),
	}
	if diags := validateSDNRouteMapEntryConfig(valid); diags.HasError() {
		t.Fatalf("valid diagnostics: %v", diags)
	}
	invalid := map[string]sdnRouteMapEntryModel{
		"reserved id":    {RouteMapID: types.StringValue("pve_generated"), Order: types.Int64Value(1), Action: types.StringValue("permit")},
		"reserved name":  {RouteMapID: types.StringValue("MAP_VTEP_IN"), Order: types.Int64Value(1), Action: types.StringValue("permit")},
		"order too high": {RouteMapID: types.StringValue("rm1"), Order: types.Int64Value(70000), Action: types.StringValue("permit")},
		"bad action":     {RouteMapID: types.StringValue("rm1"), Order: types.Int64Value(1), Action: types.StringValue("allow")},
	}
	for name, config := range invalid {
		if diags := validateSDNRouteMapEntryConfig(config); !diags.HasError() {
			t.Fatalf("%s: expected invalid diagnostics: %#v", name, config)
		}
	}
}

const sdnPrefixListJSON = `{"id":"pl1","entries":["action=permit,prefix=10.0.0.0/24"],"state":"changed","digest":"pl-digest"}`

const sdnRouteMapEntryJSON = `{"route-map-id":"rm1","order":10,"action":"permit","set":["local-preference=100"],"state":"changed","digest":"rm-digest"}`
