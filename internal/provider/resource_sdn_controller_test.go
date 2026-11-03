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

func TestClientSDNControllerCRUD(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/controllers":
			_, _ = w.Write([]byte(`{"data":[` + sdnControllerJSON + `]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/controllers":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error: %v", err)
			}
			switch r.Form.Get("type") {
			case sdnControllerTypeBGP:
				assertFormValues(t, r, url.Values{
					"asn":        {"65000"},
					"controller": {"bgp1"},
					"ebgp":       {"1"},
					"lock-token": {testSDNLockToken},
					"node":       {"node1"},
					"peers":      {"10.0.0.1,10.0.0.2"},
					"type":       {sdnControllerTypeBGP},
				})
			case sdnControllerTypeISIS:
				assertFormValues(t, r, url.Values{
					"controller":  {"isis1"},
					"isis-domain": {"lab"},
					"isis-ifaces": {"eth1,eth2"},
					"isis-net":    {"49.0001.1111.1111.1111.00"},
					"lock-token":  {testSDNLockToken},
					"loopback":    {"lo"},
					"node":        {"node1"},
					"type":        {sdnControllerTypeISIS},
				})
			default:
				t.Fatalf("unexpected controller type %q", r.Form.Get("type"))
			}
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/sdn/controllers/bgp1":
			assertFormValues(t, r, url.Values{
				"bgp-mode":   {"external"},
				"controller": {"bgp1"},
				"delete":     {"ebgp"},
				"digest":     {"controller-digest"},
				"lock-token": {testSDNLockToken},
				"type":       {sdnControllerTypeEVPN},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/controllers/bgp1":
			assertFormValues(t, r, url.Values{"lock-token": {testSDNLockToken}})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	controller, err := client.GetSDNController(ctx, "bgp1")
	if err != nil || controller.ASN.Ptr() == nil || *controller.ASN.Ptr() != 65000 {
		t.Fatalf("GetSDNController() = %#v, %v", controller, err)
	}
	if err := client.CreateSDNController(ctx, "bgp1", SDNControllerRequest{
		Type: sdnControllerTypeBGP, Node: stringPtr("node1"), ASN: intPtr64(65000),
		Peers: []string{"10.0.0.2", "10.0.0.1"}, EBGP: boolPtr(true), LockToken: testSDNLockToken,
	}); err != nil {
		t.Fatalf("CreateSDNController(bgp) unexpected error: %v", err)
	}
	if err := client.CreateSDNController(ctx, "isis1", SDNControllerRequest{
		Type: sdnControllerTypeISIS, Node: stringPtr("node1"), ISISDomain: stringPtr("lab"),
		ISISNet: stringPtr("49.0001.1111.1111.1111.00"), ISISIfaces: []string{"eth2", "eth1"},
		Loopback: stringPtr("lo"), LockToken: testSDNLockToken,
	}); err != nil {
		t.Fatalf("CreateSDNController(isis) unexpected error: %v", err)
	}
	if err := client.UpdateSDNController(ctx, "bgp1", SDNControllerRequest{
		Type: sdnControllerTypeEVPN, BGPMode: stringPtr("external"),
		LockToken: testSDNLockToken, Digest: stringPtr("controller-digest"), Delete: []string{"ebgp"},
	}); err != nil {
		t.Fatalf("UpdateSDNController() unexpected error: %v", err)
	}
	if err := client.DeleteSDNController(ctx, "bgp1", testSDNLockToken); err != nil {
		t.Fatalf("DeleteSDNController() unexpected error: %v", err)
	}
}

func TestSDNControllerSchemaAndValidation(t *testing.T) {
	var resp resource.SchemaResponse
	NewSDNControllerResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 21; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	if _, ok := resp.Schema.Attributes["controller"].(resourceschema.StringAttribute); !ok {
		t.Fatal("controller must be a string attribute")
	}
	controllerAttribute, _ := resp.Schema.Attributes["controller"].(resourceschema.StringAttribute)
	if len(controllerAttribute.PlanModifiers) == 0 {
		t.Fatal("controller must require replacement")
	}

	bgp := sdnControllerModel{
		Controller: types.StringValue("bgp1"), Type: types.StringValue(sdnControllerTypeBGP),
		Node: types.StringValue("node1"), ASN: types.Int64Value(65000), Peers: testStringSet(t, "10.0.0.1"),
	}
	if diags := validateSDNControllerConfig(bgp); diags.HasError() {
		t.Fatalf("valid bgp diagnostics: %v", diags)
	}
	evpn := sdnControllerModel{Controller: types.StringValue("evpn1"), Type: types.StringValue(sdnControllerTypeEVPN), ASN: types.Int64Value(65000)}
	if diags := validateSDNControllerConfig(evpn); diags.HasError() {
		t.Fatalf("valid evpn diagnostics: %v", diags)
	}
	faucet := sdnControllerModel{Controller: types.StringValue("fct1"), Type: types.StringValue(sdnControllerTypeFaucet)}
	if diags := validateSDNControllerConfig(faucet); diags.HasError() {
		t.Fatalf("valid faucet diagnostics: %v", diags)
	}
	isis := sdnControllerModel{
		Controller: types.StringValue("isis1"), Type: types.StringValue(sdnControllerTypeISIS), Node: types.StringValue("node1"),
		ISISDomain: types.StringValue("lab"), ISISNet: types.StringValue("49.0001.1111.1111.1111.00"),
		ISISIfaces: testStringSet(t, "eth1"),
	}
	if diags := validateSDNControllerConfig(isis); diags.HasError() {
		t.Fatalf("valid isis diagnostics: %v", diags)
	}
	if diags := validateSDNControllerConfig(sdnControllerModel{Controller: types.StringUnknown(), Type: types.StringUnknown(), ASN: types.Int64Unknown()}); diags.HasError() {
		t.Fatalf("unknown values must defer: %v", diags)
	}

	invalid := map[string]sdnControllerModel{
		"short id":        {Controller: types.StringValue("b"), Type: types.StringValue(sdnControllerTypeFaucet)},
		"unknown type":    {Controller: types.StringValue("ctl"), Type: types.StringValue("ospf")},
		"bgp without asn": {Controller: types.StringValue("ctl"), Type: types.StringValue(sdnControllerTypeBGP), Node: types.StringValue("n"), Peers: testStringSet(t, "10.0.0.1")},
		"bgp without peers": {
			Controller: types.StringValue("ctl"), Type: types.StringValue(sdnControllerTypeBGP),
			Node: types.StringValue("n"), ASN: types.Int64Value(1),
		},
		"evpn without asn": {Controller: types.StringValue("ctl"), Type: types.StringValue(sdnControllerTypeEVPN)},
		"isis without net": {
			Controller: types.StringValue("ctl"), Type: types.StringValue(sdnControllerTypeISIS),
			Node: types.StringValue("n"), ISISDomain: types.StringValue("d"), ISISIfaces: testStringSet(t, "eth1"),
		},
		"asn out of range": {
			Controller: types.StringValue("ctl"), Type: types.StringValue(sdnControllerTypeEVPN), ASN: types.Int64Value(4294967296),
		},
		"bad bgp mode": {
			Controller: types.StringValue("ctl"), Type: types.StringValue(sdnControllerTypeEVPN), ASN: types.Int64Value(1), BGPMode: types.StringValue("hybrid"),
		},
		"bgp-mode on bgp": {
			Controller: types.StringValue("ctl"), Type: types.StringValue(sdnControllerTypeBGP),
			Node: types.StringValue("n"), ASN: types.Int64Value(1), Peers: testStringSet(t, "10.0.0.1"), BGPMode: types.StringValue("auto"),
		},
		"bad peer": {
			Controller: types.StringValue("ctl"), Type: types.StringValue(sdnControllerTypeBGP),
			Node: types.StringValue("n"), ASN: types.Int64Value(1), Peers: testStringSet(t, "peer"),
		},
		"bad isis net": {
			Controller: types.StringValue("ctl"), Type: types.StringValue(sdnControllerTypeISIS),
			Node: types.StringValue("n"), ISISDomain: types.StringValue("d"),
			ISISIfaces: testStringSet(t, "eth1"), ISISNet: types.StringValue("nope"),
		},
		"ebgp multihop out of range": {
			Controller: types.StringValue("ctl"), Type: types.StringValue(sdnControllerTypeBGP),
			Node: types.StringValue("n"), ASN: types.Int64Value(1), Peers: testStringSet(t, "10.0.0.1"), EBGPMultihop: types.Int64Value(256),
		},
		"asn on faucet": {
			Controller: types.StringValue("ctl"), Type: types.StringValue(sdnControllerTypeFaucet), ASN: types.Int64Value(1),
		},
	}
	for name, config := range invalid {
		if diags := validateSDNControllerConfig(config); !diags.HasError() {
			t.Fatalf("%s: expected invalid diagnostics: %#v", name, config)
		}
	}
}

func TestSDNControllerStateStagedLists(t *testing.T) {
	state, diags := sdnControllerStateFromAPI(context.Background(), SDNController{
		Controller: "evpn1", Type: sdnControllerTypeEVPN, ASN: proxmoxOptionalInt64{value: intPtr64(65000)},
		Peers: "10.0.0.1", Nodes: "node1", State: "changed",
		Pending: map[string]json.RawMessage{
			"peers": json.RawMessage(`"10.0.0.2,10.0.0.3"`),
			"nodes": json.RawMessage(`["node2"]`),
			"asn":   json.RawMessage(`"65001"`),
		},
	})
	if diags.HasError() {
		t.Fatalf("state mapping diagnostics: %v", diags)
	}
	var peers []string
	if diags := state.Peers.ElementsAs(context.Background(), &peers, false); diags.HasError() || len(peers) != 2 {
		t.Fatalf("staged peers were not applied: %#v (%v)", peers, diags)
	}
	var nodes []string
	if diags := state.Nodes.ElementsAs(context.Background(), &nodes, false); diags.HasError() || len(nodes) != 1 || nodes[0] != "node2" {
		t.Fatalf("staged node array was not applied: %#v (%v)", nodes, diags)
	}
	if state.ASN.ValueInt64() != 65001 {
		t.Fatalf("staged asn was not applied: %#v", state.ASN)
	}
}

func TestSDNControllerLifecycle(t *testing.T) {
	controllers := map[string]map[string]any{}
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
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/controllers":
			if !handler.form(w, r, url.Values{
				"controller": {"evpn1"},
				"asn":        {"65000"},
				"lock-token": {testSDNLockToken},
				"type":       {"evpn"},
			}) {
				return
			}
			controllers["evpn1"] = map[string]any{"controller": "evpn1", "type": "evpn", "asn": 65000, "state": "new"}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/controllers":
			list := make([]map[string]any, 0, len(controllers))
			for _, entry := range controllers {
				list = append(list, entry)
			}
			handler.envelope(w, list)
		default:
			handler.fail(w, "unexpected SDN controller request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &SDNControllerResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := sdnControllerModel{
		Controller: types.StringValue("evpn1"), Type: types.StringValue(sdnControllerTypeEVPN), ASN: types.Int64Value(65000),
		Peers: types.SetNull(types.StringType), Nodes: types.SetNull(types.StringType), ISISIfaces: types.SetNull(types.StringType),
		Pending: types.MapNull(types.StringType),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created sdnControllerModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() || created.State.ValueString() != "new" || created.ASN.ValueInt64() != 65000 {
		t.Fatalf("unexpected created state: %#v (%v)", created, diags)
	}

	var importResp resource.ImportStateResponse
	importResp.State = tfsdk.State{Raw: tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil), Schema: schema.Schema}
	res.ImportState(ctx, resource.ImportStateRequest{ID: "evpn1"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", importResp.Diagnostics)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

const sdnControllerJSON = `{"controller":"bgp1","type":"bgp","node":"node1","asn":65000,"peers":"10.0.0.1","state":"changed","digest":"controller-digest","pending":{"asn":"65001"}}`
