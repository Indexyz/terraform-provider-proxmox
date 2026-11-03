// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestClientSDNVnetFirewallOptions(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path != "/api2/json/cluster/sdn/vnets/vn1/firewall/options" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"data":{"enable":1,"policy_forward":"DROP","log_level_forward":"info","digest":"fw-digest"}}`))
		case http.MethodPut:
			assertFormValues(t, r, url.Values{
				"delete":            {"policy_forward"},
				"digest":            {"fw-digest"},
				"enable":            {"0"},
				"log_level_forward": {"nolog"},
			})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	options, err := client.GetSDNVnetFirewallOptions(ctx, "vn1")
	if err != nil || options.Enable.Ptr() == nil || !*options.Enable.Ptr() || options.PolicyForward != "DROP" {
		t.Fatalf("GetSDNVnetFirewallOptions() = %#v, %v", options, err)
	}
	if err := client.SetSDNVnetFirewallOptions(ctx, "vn1", SDNVnetFirewallOptionsRequest{
		Enable: boolPtr(false), LogLevelForward: stringPtr("nolog"),
		Digest: stringPtr("fw-digest"), Delete: []string{"policy_forward"},
	}); err != nil {
		t.Fatalf("SetSDNVnetFirewallOptions() unexpected error: %v", err)
	}
}

func TestSDNVnetFirewallOptionsSchemaAndValidation(t *testing.T) {
	var resp resource.SchemaResponse
	NewSDNVnetFirewallOptionsResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 5; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}

	valid := sdnVnetFirewallOptionsModel{
		Vnet: types.StringValue("vn1"), Enable: types.BoolValue(true),
		PolicyForward: types.StringValue("ACCEPT"), LogLevelForward: types.StringValue("info"),
	}
	if diags := validateSDNVnetFirewallOptions(valid); diags.HasError() {
		t.Fatalf("valid diagnostics: %v", diags)
	}
	invalid := map[string]sdnVnetFirewallOptionsModel{
		"bad vnet":      {Vnet: types.StringValue("vn-1")},
		"bad policy":    {Vnet: types.StringValue("vn1"), PolicyForward: types.StringValue("REJECT")},
		"bad log level": {Vnet: types.StringValue("vn1"), LogLevelForward: types.StringValue("loud")},
	}
	for name, config := range invalid {
		if diags := validateSDNVnetFirewallOptions(config); !diags.HasError() {
			t.Fatalf("%s: expected invalid diagnostics: %#v", name, config)
		}
	}

	deleted := sdnVnetFirewallOptionsModel{Vnet: types.StringValue("vn1")}
	if got := sdnDeleteKeys(sdnVnetFirewallOptionsManagedFields(deleted), sdnVnetFirewallOptionKeys); len(got) != 3 {
		t.Fatalf("reset keys must delete every option: %v", got)
	}
}

func TestSDNVnetFirewallOptionsLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	options := map[string]any{"enable": 1, "policy_forward": "DROP", "digest": "d1"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			handler.envelope(w, options)
		case http.MethodPut:
			if err := r.ParseForm(); err != nil {
				handler.fail(w, "parse form: %v", err)
			}
			if r.Form.Get("delete") != "" {
				for _, key := range []string{"enable", "log_level_forward", "policy_forward"} {
					delete(options, key)
				}
			}
			for key, value := range r.Form {
				if key == "delete" || key == "digest" {
					continue
				}
				options[key] = value[0]
			}
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &SDNVnetFirewallOptionsResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := sdnVnetFirewallOptionsModel{
		Vnet:          types.StringValue("vn1"),
		PolicyForward: types.StringValue("DROP"),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created sdnVnetFirewallOptionsModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() || created.PolicyForward.ValueString() != "DROP" {
		t.Fatalf("unexpected created state: %#v (%v)", created, diags)
	}

	deleteResp := resource.DeleteResponse{State: createResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: createResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", deleteResp.Diagnostics)
	}
	if len(options) != 1 { // only the digest may remain
		t.Fatalf("delete must reset every option: %#v", options)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}
