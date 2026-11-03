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
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestClientSDNDNSCRUD(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/dns":
			_, _ = w.Write([]byte(`{"data":[` + sdnDNSJSON + `]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/dns":
			assertFormValues(t, r, url.Values{
				"dns":           {"dns1"},
				"key":           {"secret"},
				"lock-token":    {testSDNLockToken},
				"reversemaskv6": {"64"},
				"ttl":           {"300"},
				"type":          {"powerdns"},
				"url":           {"https://pdns"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/sdn/dns/dns1":
			assertFormValues(t, r, url.Values{
				"delete":     {"ttl"},
				"digest":     {"dns-digest"},
				"dns":        {"dns1"},
				"lock-token": {testSDNLockToken},
				"type":       {"powerdns"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/dns/dns1":
			assertFormValues(t, r, url.Values{"lock-token": {testSDNLockToken}})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	server2, err := client.GetSDNDNS(ctx, "dns1")
	if err != nil || server2.TTL.Ptr() == nil || *server2.TTL.Ptr() != 300 {
		t.Fatalf("GetSDNDNS() = %#v, %v", server2, err)
	}
	if err := client.CreateSDNDNS(ctx, "dns1", SDNDNSRequest{
		Type: "powerdns", URL: stringPtr("https://pdns"), Key: stringPtr("secret"),
		TTL: intPtr64(300), ReverseMaskV6: intPtr64(64), LockToken: testSDNLockToken,
	}); err != nil {
		t.Fatalf("CreateSDNDNS() unexpected error: %v", err)
	}
	if err := client.UpdateSDNDNS(ctx, "dns1", SDNDNSRequest{
		Type: "powerdns", LockToken: testSDNLockToken, Digest: stringPtr("dns-digest"), Delete: []string{"ttl"},
	}); err != nil {
		t.Fatalf("UpdateSDNDNS() unexpected error: %v", err)
	}
	if err := client.DeleteSDNDNS(ctx, "dns1", testSDNLockToken); err != nil {
		t.Fatalf("DeleteSDNDNS() unexpected error: %v", err)
	}
}

func TestSDNDNSSchemaAndValidation(t *testing.T) {
	var resp resource.SchemaResponse
	NewSDNDNSResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 10; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	for _, name := range []string{"dns", "type", "url", "key"} {
		attribute, ok := resp.Schema.Attributes[name].(resourceschema.StringAttribute)
		if !ok || !attribute.IsRequired() {
			t.Fatalf("%s must be required", name)
		}
	}
	dnsAttribute, _ := resp.Schema.Attributes["dns"].(resourceschema.StringAttribute)
	if len(dnsAttribute.PlanModifiers) == 0 {
		t.Fatal("dns must require replacement")
	}

	valid := sdnDNSModel{
		DNS: types.StringValue("dns1"), Type: types.StringValue("powerdns"),
		URL: types.StringValue("https://pdns"), Key: types.StringValue("k"), TTL: types.Int64Value(300),
	}
	if diags := validateSDNDNSConfig(valid); diags.HasError() {
		t.Fatalf("valid diagnostics: %v", diags)
	}
	invalid := map[string]sdnDNSModel{
		"short id":     {DNS: types.StringValue("d"), Type: types.StringValue("powerdns")},
		"bad type":     {DNS: types.StringValue("dns1"), Type: types.StringValue("bind")},
		"negative ttl": {DNS: types.StringValue("dns1"), Type: types.StringValue("powerdns"), TTL: types.Int64Value(-1)},
		"mask out of range": {
			DNS: types.StringValue("dns1"), Type: types.StringValue("powerdns"), ReverseMaskV6: types.Int64Value(129),
		},
	}
	for name, config := range invalid {
		if diags := validateSDNDNSConfig(config); !diags.HasError() {
			t.Fatalf("%s: expected invalid diagnostics: %#v", name, config)
		}
	}
}

func TestSDNDNSStateStagedOverlay(t *testing.T) {
	state, diags := sdnDNSStateFromAPI(context.Background(), SDNDNS{
		DNS: "dns1", Type: "powerdns", URL: "https://old", TTL: proxmoxOptionalInt64{value: intPtr64(300)}, State: "changed",
		Pending: map[string]json.RawMessage{
			"url": json.RawMessage(`"https://new"`),
			"ttl": json.RawMessage(`"deleted"`),
		},
	})
	if diags.HasError() {
		t.Fatalf("state mapping diagnostics: %v", diags)
	}
	if state.URL.ValueString() != "https://new" || !state.TTL.IsNull() || state.State.ValueString() != "changed" {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestSDNDNSLifecycle(t *testing.T) {
	servers := map[string]map[string]any{}
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
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/dns":
			servers["dns1"] = map[string]any{"dns": "dns1", "type": "powerdns", "url": "https://pdns", "key": "k", "state": "new"}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/dns":
			list := make([]map[string]any, 0, len(servers))
			for _, entry := range servers {
				list = append(list, entry)
			}
			handler.envelope(w, list)
		default:
			handler.fail(w, "unexpected SDN dns request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &SDNDNSResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := sdnDNSModel{
		DNS: types.StringValue("dns1"), Type: types.StringValue("powerdns"),
		URL: types.StringValue("https://pdns"), Key: types.StringValue("k"),
		Pending: types.MapNull(types.StringType),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created sdnDNSModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() || created.State.ValueString() != "new" {
		t.Fatalf("unexpected created state: %#v (%v)", created, diags)
	}

	var importResp resource.ImportStateResponse
	importResp.State = tfsdk.State{Raw: tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil), Schema: schema.Schema}
	res.ImportState(ctx, resource.ImportStateRequest{ID: "dns1"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", importResp.Diagnostics)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

const sdnDNSJSON = `{"dns":"dns1","type":"powerdns","url":"https://pdns","key":"k","ttl":300,"state":"changed","digest":"dns-digest","pending":{"url":"https://new"}}`
