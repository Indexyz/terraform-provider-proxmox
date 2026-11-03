// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestClientSDNIPAMCRUD(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/ipams":
			if r.URL.Query().Get("pending") != "1" {
				t.Fatalf("expected pending=1, got %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"data":[` + sdnIPAMJSON + `]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/ipams":
			assertFormValues(t, r, url.Values{
				"ipam":       {"ip1"},
				"lock-token": {testSDNLockToken},
				"section":    {"3"},
				"token":      {"tok"},
				"type":       {"phpipam"},
				"url":        {"https://ipam.example.com"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/sdn/ipams/ip1":
			assertFormValues(t, r, url.Values{
				"delete":     {"section"},
				"digest":     {"ipam-digest"},
				"ipam":       {"ip1"},
				"lock-token": {testSDNLockToken},
				"type":       {"phpipam"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/ipams/ip1":
			assertFormValues(t, r, url.Values{"lock-token": {testSDNLockToken}})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	ipam, err := client.GetSDNIPAM(ctx, "ip1")
	if err != nil || ipam.Section.Ptr() == nil || *ipam.Section.Ptr() != 3 {
		t.Fatalf("GetSDNIPAM() = %#v, %v", ipam, err)
	}
	if err := client.CreateSDNIPAM(ctx, "ip1", SDNIPAMRequest{
		Type:      sdnIPAMTypePHPIPAM,
		URL:       stringPtr("https://ipam.example.com"),
		Token:     stringPtr("tok"),
		Section:   intPtr64(3),
		LockToken: testSDNLockToken,
	}); err != nil {
		t.Fatalf("CreateSDNIPAM() unexpected error: %v", err)
	}
	if err := client.UpdateSDNIPAM(ctx, "ip1", SDNIPAMRequest{
		Type:      sdnIPAMTypePHPIPAM,
		LockToken: testSDNLockToken,
		Digest:    stringPtr("ipam-digest"),
		Delete:    []string{"section"},
	}); err != nil {
		t.Fatalf("UpdateSDNIPAM() unexpected error: %v", err)
	}
	if err := client.DeleteSDNIPAM(ctx, "ip1", testSDNLockToken); err != nil {
		t.Fatalf("DeleteSDNIPAM() unexpected error: %v", err)
	}
	if _, err := newHAResourceTestClient(t, server.URL).GetSDNIPAM(ctx, "missing"); !errors.Is(err, errNotFound) && err == nil {
		t.Fatal("missing IPAM must not resolve")
	}
}

func TestSDNIPAMSchemaAndValidation(t *testing.T) {
	var resp resource.SchemaResponse
	NewSDNIPAMResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 9; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	for _, name := range []string{"ipam", "type"} {
		attribute, ok := resp.Schema.Attributes[name].(resourceschema.StringAttribute)
		if !ok || !attribute.IsRequired() || len(attribute.PlanModifiers) == 0 {
			t.Fatalf("%s must be required with replacement", name)
		}
	}

	valid := sdnIPAMModel{
		IPAM: types.StringValue("pn1"), Type: types.StringValue(sdnIPAMTypePHPIPAM),
		URL: types.StringValue("https://ipam.example.com"), Token: types.StringValue("tok"), Section: types.Int64Value(3),
	}
	if diags := validateSDNIPAMConfig(valid); diags.HasError() {
		t.Fatalf("valid phpipam diagnostics: %v", diags)
	}
	pve := sdnIPAMModel{IPAM: types.StringValue("pv1"), Type: types.StringValue(sdnIPAMTypePVE)}
	if diags := validateSDNIPAMConfig(pve); diags.HasError() {
		t.Fatalf("valid pve diagnostics: %v", diags)
	}
	if diags := validateSDNIPAMConfig(sdnIPAMModel{IPAM: types.StringUnknown(), Type: types.StringUnknown(), Section: types.Int64Unknown()}); diags.HasError() {
		t.Fatalf("unknown values must defer: %v", diags)
	}

	invalid := map[string]sdnIPAMModel{
		"short ipam":      {IPAM: types.StringValue("p"), Type: types.StringValue(sdnIPAMTypePVE)},
		"hyphen in ipam":  {IPAM: types.StringValue("p-1"), Type: types.StringValue(sdnIPAMTypePVE)},
		"unknown type":    {IPAM: types.StringValue("pn1"), Type: types.StringValue("kea")},
		"netbox no url":   {IPAM: types.StringValue("pn1"), Type: types.StringValue(sdnIPAMTypeNetbox), Token: types.StringValue("t")},
		"netbox no token": {IPAM: types.StringValue("pn1"), Type: types.StringValue(sdnIPAMTypeNetbox), URL: types.StringValue("u")},
		"phpipam no section": {
			IPAM: types.StringValue("pn1"), Type: types.StringValue(sdnIPAMTypePHPIPAM),
			URL: types.StringValue("u"), Token: types.StringValue("t"),
		},
		"url on pve": {IPAM: types.StringValue("pv1"), Type: types.StringValue(sdnIPAMTypePVE), URL: types.StringValue("u")},
		"negative section": {
			IPAM: types.StringValue("pn1"), Type: types.StringValue(sdnIPAMTypePHPIPAM),
			URL: types.StringValue("u"), Token: types.StringValue("t"), Section: types.Int64Value(-1),
		},
	}
	for name, config := range invalid {
		if diags := validateSDNIPAMConfig(config); !diags.HasError() {
			t.Fatalf("%s: expected invalid diagnostics: %#v", name, config)
		}
	}
}

func TestSDNIPAMStateAndDeletion(t *testing.T) {
	state, diags := sdnIPAMStateFromAPI(context.Background(), SDNIPAM{
		IPAM: "ip1", Type: sdnIPAMTypePHPIPAM, URL: "https://old", Section: proxmoxOptionalInt64{value: intPtr64(3)},
		State: "changed",
		Pending: map[string]json.RawMessage{
			"url":     json.RawMessage(`"https://new"`),
			"section": json.RawMessage(`"deleted"`),
		},
	})
	if diags.HasError() {
		t.Fatalf("state mapping diagnostics: %v", diags)
	}
	if state.URL.ValueString() != "https://new" || !state.Section.IsNull() {
		t.Fatalf("staged values were not applied: %#v", state)
	}

	deleted := sdnIPAMModel{IPAM: types.StringValue("ip1")}
	got := sdnDeleteKeys(sdnIPAMManagedFields(deleted), []string{"token", "url"})
	if strings.Join(got, ",") != "token,url" {
		t.Fatalf("unexpected delete keys: %v", got)
	}
}

func TestSDNIPAMImportAndLifecycle(t *testing.T) {
	ipams := map[string]map[string]any{}
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
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/ipams":
			if !handler.form(w, r, url.Values{
				"ipam":       {"ip1"},
				"lock-token": {testSDNLockToken},
				"token":      {"tok"},
				"type":       {"netbox"},
				"url":        {"https://ipam"},
			}) {
				return
			}
			ipams["ip1"] = map[string]any{"ipam": "ip1", "type": "netbox", "url": "https://ipam", "token": "tok", "state": "new"}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/ipams":
			list := make([]map[string]any, 0, len(ipams))
			for _, entry := range ipams {
				list = append(list, entry)
			}
			handler.envelope(w, list)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/sdn/ipams/ip1":
			ipams["ip1"]["url"] = "https://ipam2"
			ipams["ip1"]["state"] = "changed"
			handler.envelope(w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/ipams/ip1":
			ipams["ip1"] = map[string]any{"ipam": "ip1", "type": "netbox", "state": "deleted"}
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected SDN ipam request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &SDNIPAMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := sdnIPAMModel{
		IPAM: types.StringValue("ip1"), Type: types.StringValue(sdnIPAMTypeNetbox),
		URL: types.StringValue("https://ipam"), Token: types.StringValue("tok"),
		Pending: types.MapNull(types.StringType),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created sdnIPAMModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() || created.State.ValueString() != "new" {
		t.Fatalf("unexpected created state: %#v (%v)", created, diags)
	}

	updated := config
	updated.URL = types.StringValue("https://ipam2")
	ipams["ip1"]["digest"] = "digest-1"
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}, Private: createResp.Private}
	res.Update(ctx, resource.UpdateRequest{
		Config:  testResourceConfig(t, schema, updated),
		Plan:    testResourcePlan(t, schema, updated),
		State:   createResp.State,
		Private: createResp.Private,
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", updateResp.Diagnostics)
	}

	deleteResp := resource.DeleteResponse{State: updateResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", deleteResp.Diagnostics)
	}
	readResp := resource.ReadResponse{State: updateResp.State}
	res.Read(ctx, resource.ReadRequest{State: updateResp.State}, &readResp)
	if readResp.Diagnostics.HasError() || !readResp.State.Raw.IsNull() {
		t.Fatalf("deleted IPAM was not removed: %v / %v", readResp.Diagnostics, readResp.State.Raw)
	}

	var importResp resource.ImportStateResponse
	importResp.State = tfsdk.State{Raw: tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil), Schema: schema.Schema}
	res.ImportState(ctx, resource.ImportStateRequest{ID: "ip1"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", importResp.Diagnostics)
	}
	var imported sdnIPAMModel
	if diags := importResp.State.Get(ctx, &imported); diags.HasError() || imported.IPAM.ValueString() != "ip1" {
		t.Fatalf("unexpected imported state: %#v (%v)", imported, diags)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

const sdnIPAMJSON = `{"ipam":"ip1","type":"phpipam","url":"https://ipam","token":"tok","section":3,"state":"changed","digest":"ipam-digest","pending":{"url":"https://new"}}`
