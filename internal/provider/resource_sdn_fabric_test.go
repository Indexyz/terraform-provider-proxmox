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

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestClientSDNFabricCRUD(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/fabrics/fabric":
			_, _ = w.Write([]byte(`{"data":[` + sdnFabricJSON + `]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/fabrics/fabric":
			assertFormValues(t, r, url.Values{
				"area":         {"0.0.0.1"},
				"id":           {"fab1"},
				"ip_prefix":    {"10.10.0.0/24"},
				"lock-token":   {testSDNLockToken},
				"protocol":     {"ospf"},
				"redistribute": {"source=connected", "source=static,route-map=rm1"},
				"route_filter": {"pl1"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/sdn/fabrics/fabric/fab1":
			assertFormValues(t, r, url.Values{
				"area":       {"0.0.0.2"},
				"delete":     {"ip_prefix", "redistribute"},
				"digest":     {"fabric-digest"},
				"id":         {"fab1"},
				"lock-token": {testSDNLockToken},
				"protocol":   {"ospf"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/fabrics/fabric/fab1":
			assertFormValues(t, r, url.Values{"lock-token": {testSDNLockToken}})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	fabric, err := client.GetSDNFabric(ctx, "fab1")
	if err != nil || fabric.Area != "0.0.0.1" || len(fabric.Redistribute) != 1 {
		t.Fatalf("GetSDNFabric() = %#v, %v", fabric, err)
	}
	if err := client.CreateSDNFabric(ctx, "fab1", SDNFabricRequest{
		Protocol: "ospf", IPPrefix: stringPtr("10.10.0.0/24"), Area: stringPtr("0.0.0.1"),
		RouteFilter: stringPtr("pl1"), Redistribute: []string{"source=connected", "source=static,route-map=rm1"},
		LockToken: testSDNLockToken,
	}); err != nil {
		t.Fatalf("CreateSDNFabric() unexpected error: %v", err)
	}
	if err := client.UpdateSDNFabric(ctx, "fab1", SDNFabricRequest{
		Protocol: "ospf", Area: stringPtr("0.0.0.2"), LockToken: testSDNLockToken,
		Digest: stringPtr("fabric-digest"), Delete: []string{"redistribute", "ip_prefix"},
	}); err != nil {
		t.Fatalf("UpdateSDNFabric() unexpected error: %v", err)
	}
	if err := client.DeleteSDNFabric(ctx, "fab1", testSDNLockToken); err != nil {
		t.Fatalf("DeleteSDNFabric() unexpected error: %v", err)
	}
}

func TestSDNFabricValidationAndState(t *testing.T) {
	openfabric := sdnFabricModel{
		FabricID: types.StringValue("fab1"), Protocol: types.StringValue("openfabric"),
		IPPrefix: types.StringValue("10.10.0.0/24"), HelloInterval: types.Float64Value(3),
	}
	if diags := validateSDNFabricConfig(openfabric); diags.HasError() {
		t.Fatalf("valid openfabric diagnostics: %v", diags)
	}
	bgp := sdnFabricModel{
		FabricID: types.StringValue("fab2"), Protocol: types.StringValue("bgp"),
		Redistribute: types.ListValueMust(types.StringType, []attr.Value{types.StringValue("source=connected")}),
		RouteMapIn:   types.StringValue("rm1"),
	}
	if diags := validateSDNFabricConfig(bgp); diags.HasError() {
		t.Fatalf("valid bgp diagnostics: %v", diags)
	}
	invalid := map[string]sdnFabricModel{
		"bad id":       {FabricID: types.StringValue("fabric-too-long"), Protocol: types.StringValue("ospf")},
		"bad protocol": {FabricID: types.StringValue("fab1"), Protocol: types.StringValue("rip")},
		"area on openfabric": {
			FabricID: types.StringValue("fab1"), Protocol: types.StringValue("openfabric"), Area: types.StringValue("1"),
		},
		"keepalive on ospf": {
			FabricID: types.StringValue("fab1"), Protocol: types.StringValue("ospf"), PersistentKeepalive: types.Int64Value(25),
		},
		"bad prefix": {FabricID: types.StringValue("fab1"), Protocol: types.StringValue("ospf"), IPPrefix: types.StringValue("x")},
		"hello out of range": {
			FabricID: types.StringValue("fab1"), Protocol: types.StringValue("openfabric"), HelloInterval: types.Float64Value(601),
		},
	}
	for name, config := range invalid {
		if diags := validateSDNFabricConfig(config); !diags.HasError() {
			t.Fatalf("%s: expected invalid diagnostics: %#v", name, config)
		}
	}

	state, diags := sdnFabricStateFromAPI(context.Background(), SDNFabric{
		ID: "fab1", Protocol: "ospf", Area: "0.0.0.1", State: "changed",
		Redistribute: []string{"source=connected"},
		Pending:      map[string]json.RawMessage{"area": json.RawMessage(`"0.0.0.2"`), "redistribute": json.RawMessage(`["source=static"]`)},
	})
	if diags.HasError() {
		t.Fatalf("state mapping diagnostics: %v", diags)
	}
	if state.Area.ValueString() != "0.0.0.2" {
		t.Fatalf("staged area was not applied: %#v", state)
	}
	var redistribute []string
	if diags := state.Redistribute.ElementsAs(context.Background(), &redistribute, false); diags.HasError() || len(redistribute) != 1 || redistribute[0] != "source=static" {
		t.Fatalf("staged redistribute was not applied: %#v (%v)", redistribute, diags)
	}
}

func TestClientSDNFabricNodeCRUD(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/fabrics/node/fab1":
			_, _ = w.Write([]byte(`{"data":[` + sdnFabricNodeJSON + `]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/fabrics/node/fab1":
			assertFormValues(t, r, url.Values{
				"interfaces": {"name=eth1"},
				"ip":         {"10.10.0.1"},
				"lock-token": {testSDNLockToken},
				"node_id":    {"node1"},
				"protocol":   {"ospf"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/sdn/fabrics/node/fab1/node1":
			assertFormValues(t, r, url.Values{
				"delete":     {"ip"},
				"digest":     {"node-digest"},
				"interfaces": {"name=eth2"},
				"lock-token": {testSDNLockToken},
				"node_id":    {"node1"},
				"protocol":   {"ospf"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/fabrics/node/fab1/node1":
			assertFormValues(t, r, url.Values{"lock-token": {testSDNLockToken}})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	node, err := client.GetSDNFabricNode(ctx, "fab1", "node1")
	if err != nil || node.IP != "10.10.0.1" || len(node.Interfaces) != 1 {
		t.Fatalf("GetSDNFabricNode() = %#v, %v", node, err)
	}
	if err := client.CreateSDNFabricNode(ctx, "fab1", "node1", SDNFabricNodeRequest{
		Protocol: "ospf", IP: stringPtr("10.10.0.1"), Interfaces: []string{"name=eth1"}, LockToken: testSDNLockToken,
	}); err != nil {
		t.Fatalf("CreateSDNFabricNode() unexpected error: %v", err)
	}
	if err := client.UpdateSDNFabricNode(ctx, "fab1", "node1", SDNFabricNodeRequest{
		Protocol: "ospf", Interfaces: []string{"name=eth2"}, LockToken: testSDNLockToken,
		Digest: stringPtr("node-digest"), Delete: []string{"ip"},
	}); err != nil {
		t.Fatalf("UpdateSDNFabricNode() unexpected error: %v", err)
	}
	if err := client.DeleteSDNFabricNode(ctx, "fab1", "node1", testSDNLockToken); err != nil {
		t.Fatalf("DeleteSDNFabricNode() unexpected error: %v", err)
	}
}

func TestSDNFabricNodeValidationAndLifecycle(t *testing.T) {
	wireguard := sdnFabricNodeModel{
		FabricID: types.StringValue("fab1"), NodeID: types.StringValue("node1"),
		Protocol: types.StringValue("wireguard"), PublicKey: types.StringValue("key"), Role: types.StringValue("external"),
		AllowedIPs: types.ListValueMust(types.StringType, []attr.Value{types.StringValue("10.10.0.0/24")}),
	}
	if diags := validateSDNFabricNodeConfig(wireguard); diags.HasError() {
		t.Fatalf("valid wireguard diagnostics: %v", diags)
	}
	invalid := map[string]sdnFabricNodeModel{
		"keepalive field on ospf": {
			FabricID: types.StringValue("fab1"), NodeID: types.StringValue("node1"),
			Protocol: types.StringValue("ospf"), PublicKey: types.StringValue("key"),
		},
		"bad role": {
			FabricID: types.StringValue("fab1"), NodeID: types.StringValue("node1"),
			Protocol: types.StringValue("wireguard"), Role: types.StringValue("peer"),
		},
		"bad allowed ip": {
			FabricID: types.StringValue("fab1"), NodeID: types.StringValue("node1"),
			Protocol:   types.StringValue("wireguard"),
			AllowedIPs: types.ListValueMust(types.StringType, []attr.Value{types.StringValue("nope")}),
		},
		"bad node": {FabricID: types.StringValue("fab1"), NodeID: types.StringValue("node_1"), Protocol: types.StringValue("ospf")},
	}
	for name, config := range invalid {
		if diags := validateSDNFabricNodeConfig(config); !diags.HasError() {
			t.Fatalf("%s: expected invalid diagnostics: %#v", name, config)
		}
	}

	handler := &lifecycleHandler{}
	nodes := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/lock":
			handler.envelope(w, testSDNLockToken)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/lock":
			handler.envelope(w, nil)
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/fabrics/node/fab1":
			nodes["node1"] = map[string]any{"fabric_id": "fab1", "node_id": "node1", "protocol": "ospf", "state": "new"}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/fabrics/node/fab1":
			list := make([]map[string]any, 0, len(nodes))
			for _, entry := range nodes {
				list = append(list, entry)
			}
			handler.envelope(w, list)
		default:
			handler.fail(w, "unexpected fabric node request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &SDNFabricNodeResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := sdnFabricNodeModel{
		FabricID: types.StringValue("fab1"), NodeID: types.StringValue("node1"), Protocol: types.StringValue("ospf"),
		Interfaces: types.ListNull(types.StringType), AllowedIPs: types.ListNull(types.StringType), Peers: types.ListNull(types.StringType),
		Pending: types.MapNull(types.StringType),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created sdnFabricNodeModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() || created.ID.ValueString() != "fab1/node1" || created.State.ValueString() != "new" {
		t.Fatalf("unexpected created state: %#v (%v)", created, diags)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

const sdnFabricJSON = `{"id":"fab1","protocol":"ospf","ip_prefix":"10.10.0.0/24","area":"0.0.0.1","state":"changed","digest":"fabric-digest","redistribute":["source=connected"]}`

const sdnFabricNodeJSON = `{"fabric_id":"fab1","node_id":"node1","protocol":"ospf","ip":"10.10.0.1","interfaces":["name=eth1"],"state":"changed","digest":"node-digest"}`
