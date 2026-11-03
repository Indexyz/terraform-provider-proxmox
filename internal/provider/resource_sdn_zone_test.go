// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestSDNZoneSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewSDNZoneResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 30; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	for _, name := range []string{"zone", "type"} {
		attribute, ok := resp.Schema.Attributes[name].(resourceschema.StringAttribute)
		if !ok || !attribute.IsRequired() || len(attribute.PlanModifiers) == 0 {
			t.Fatalf("%s must be required with a replacement plan modifier", name)
		}
	}
	for _, name := range []string{"state", "pending", "id"} {
		if !resp.Schema.Attributes[name].IsComputed() {
			t.Fatalf("%s must be computed", name)
		}
	}
	for _, excluded := range []string{"digest", "lock_token", "lock-token", "fabric_nodes"} {
		if _, ok := resp.Schema.Attributes[excluded]; ok {
			t.Fatalf("all extra zones must not expose %q", excluded)
		}
	}
}

func validVLANZone(t *testing.T) sdnZoneModel {
	t.Helper()
	return sdnZoneModel{
		Zone:       types.StringValue("vz1"),
		Type:       types.StringValue(sdnZoneTypeVLAN),
		Bridge:     types.StringValue("vmbr0"),
		Nodes:      testStringSet(t, "node1", "node2"),
		MTU:        types.Int64Value(9000),
		DNS:        types.StringValue("dns1"),
		DNSZone:    types.StringValue("example.com"),
		IPAM:       types.StringValue("pve"),
		ReverseDNS: types.StringValue("rev1"),
	}
}

func TestValidateSDNZoneConfig(t *testing.T) {
	if diags := validateSDNZoneConfig(validVLANZone(t)); diags.HasError() {
		t.Fatalf("valid vlan config diagnostics: %v", diags)
	}
	simple := sdnZoneModel{Zone: types.StringValue("sz"), Type: types.StringValue(sdnZoneTypeSimple), Nodes: testStringSet(t, "node1")}
	if diags := validateSDNZoneConfig(simple); diags.HasError() {
		t.Fatalf("valid simple config diagnostics: %v", diags)
	}
	qinq := sdnZoneModel{
		Zone: types.StringValue("qz"), Type: types.StringValue(sdnZoneTypeQinQ), Bridge: types.StringValue("vmbr0"),
		Tag: types.Int64Value(100), VLANProtocol: types.StringValue("802.1ad"),
	}
	if diags := validateSDNZoneConfig(qinq); diags.HasError() {
		t.Fatalf("valid qinq config diagnostics: %v", diags)
	}
	vxlan := sdnZoneModel{
		Zone: types.StringValue("xz"), Type: types.StringValue(sdnZoneTypeVXLAN),
		Peers: testStringSet(t, "10.0.0.1", "2001:db8::1"), VXLANPort: types.Int64Value(4789),
	}
	if diags := validateSDNZoneConfig(vxlan); diags.HasError() {
		t.Fatalf("valid vxlan config diagnostics: %v", diags)
	}
	evpn := sdnZoneModel{
		Zone: types.StringValue("ez"), Type: types.StringValue(sdnZoneTypeEVPN), VRFVXLAN: types.Int64Value(1000),
		Controller: types.StringValue("evpnctl"), SecondaryControllers: testStringSet(t, "ctl2"), ExitNodes: testStringSet(t, "node2"),
		MAC: types.StringValue("AA:BB:CC:DD:EE:FF"), ExitNodesPrimary: types.StringValue("node2"),
		AdvertiseSubnets: types.BoolValue(true), DisableARPNDSuppression: types.BoolValue(false),
		RTImport: types.StringValue("65000:100"), ExitNodesLocalRouting: types.BoolValue(true), VXLANPort: types.Int64Value(4789),
	}
	if diags := validateSDNZoneConfig(evpn); diags.HasError() {
		t.Fatalf("valid evpn config diagnostics: %v", diags)
	}
	faucet := sdnZoneModel{Zone: types.StringValue("fz"), Type: types.StringValue(sdnZoneTypeFaucet), Controller: types.StringValue("faucetctl"), DPID: types.Int64Value(1)}
	if diags := validateSDNZoneConfig(faucet); diags.HasError() {
		t.Fatalf("valid faucet config diagnostics: %v", diags)
	}
	unknownType := sdnZoneModel{Zone: types.StringValue("uz"), Type: types.StringUnknown(), Bridge: types.StringValue("vmbr0")}
	if diags := validateSDNZoneConfig(unknownType); diags.HasError() {
		t.Fatalf("unknown type must defer the variant matrix: %v", diags)
	}

	invalid := map[string]func(*sdnZoneModel){
		"short zone":          func(m *sdnZoneModel) { m.Zone = types.StringValue("a") },
		"long zone":           func(m *sdnZoneModel) { m.Zone = types.StringValue("abcdefghi") },
		"hyphen in zone":      func(m *sdnZoneModel) { m.Zone = types.StringValue("a-b") },
		"leading digit":       func(m *sdnZoneModel) { m.Zone = types.StringValue("1ab") },
		"unknown type":        func(m *sdnZoneModel) { m.Type = types.StringValue("foobar") },
		"vlan without bridge": func(m *sdnZoneModel) { m.Bridge = types.StringNull() },
		"mtu zero":            func(m *sdnZoneModel) { m.MTU = types.Int64Value(0) },
		"mtu too large":       func(m *sdnZoneModel) { m.MTU = types.Int64Value(70000) },
		"bad dhcp":            func(m *sdnZoneModel) { m.DHCP = types.StringValue("kea") },
		"dnszone without dns": func(m *sdnZoneModel) { m.DNS = types.StringNull() },
		"bad node":            func(m *sdnZoneModel) { m.Nodes = testStringSet(t, "node_1") },
		"bridge on simple": func(m *sdnZoneModel) {
			m.Type = types.StringValue(sdnZoneTypeSimple)
		},
		"mac learning on simple": func(m *sdnZoneModel) {
			m.Type = types.StringValue(sdnZoneTypeSimple)
			m.Bridge = types.StringNull()
			m.BridgeDisableMACLearning = types.BoolValue(true)
		},
		"dhcp on vlan": func(m *sdnZoneModel) { m.DHCP = types.StringValue("dnsmasq") },
		"tag on vlan":  func(m *sdnZoneModel) { m.Tag = types.Int64Value(100) },
		"peers on vlan": func(m *sdnZoneModel) {
			m.Peers = testStringSet(t, "10.0.0.1")
		},
		"qinq without tag": func(m *sdnZoneModel) {
			m.Type = types.StringValue(sdnZoneTypeQinQ)
		},
		"qinq tag out of range": func(m *sdnZoneModel) {
			m.Type = types.StringValue(sdnZoneTypeQinQ)
			m.Tag = types.Int64Value(1)
		},
		"bad vlan protocol": func(m *sdnZoneModel) {
			m.Type = types.StringValue(sdnZoneTypeQinQ)
			m.Tag = types.Int64Value(100)
			m.VLANProtocol = types.StringValue("802.1x")
		},
		"bad vxlan peer": func(m *sdnZoneModel) {
			m.Type = types.StringValue(sdnZoneTypeVXLAN)
			m.Peers = testStringSet(t, "peer")
		},
		"vxlan port out of range": func(m *sdnZoneModel) {
			m.Type = types.StringValue(sdnZoneTypeVXLAN)
			m.VXLANPort = types.Int64Value(70000)
		},
		"evpn without vrf": func(m *sdnZoneModel) {
			m.Type = types.StringValue(sdnZoneTypeEVPN)
			m.Controller = types.StringValue("ctl")
		},
		"evpn without controller": func(m *sdnZoneModel) {
			m.Type = types.StringValue(sdnZoneTypeEVPN)
			m.VRFVXLAN = types.Int64Value(1000)
		},
		"evpn vrf out of range": func(m *sdnZoneModel) {
			m.Type = types.StringValue(sdnZoneTypeEVPN)
			m.VRFVXLAN = types.Int64Value(0)
			m.Controller = types.StringValue("ctl")
		},
		"bad mac": func(m *sdnZoneModel) {
			m.Type = types.StringValue(sdnZoneTypeEVPN)
			m.VRFVXLAN = types.Int64Value(1000)
			m.Controller = types.StringValue("ctl")
			m.MAC = types.StringValue("not-a-mac")
		},
		"bad secondary controller": func(m *sdnZoneModel) {
			m.Type = types.StringValue(sdnZoneTypeEVPN)
			m.VRFVXLAN = types.Int64Value(1000)
			m.Controller = types.StringValue("ctl")
			m.SecondaryControllers = testStringSet(t, "-ctl")
		},
		"bad exit node": func(m *sdnZoneModel) {
			m.Type = types.StringValue(sdnZoneTypeEVPN)
			m.VRFVXLAN = types.Int64Value(1000)
			m.Controller = types.StringValue("ctl")
			m.ExitNodes = testStringSet(t, "node_2")
		},
		"faucet without dp id": func(m *sdnZoneModel) {
			m.Type = types.StringValue(sdnZoneTypeFaucet)
			m.Controller = types.StringValue("ctl")
			m.Bridge = types.StringNull()
		},
		"port on faucet": func(m *sdnZoneModel) {
			m.Type = types.StringValue(sdnZoneTypeFaucet)
			m.Controller = types.StringValue("ctl")
			m.DPID = types.Int64Value(1)
			m.Bridge = types.StringNull()
			m.VXLANPort = types.Int64Value(4789)
		},
	}
	for name, mutate := range invalid {
		config := validVLANZone(t)
		mutate(&config)
		if diags := validateSDNZoneConfig(config); !diags.HasError() {
			t.Fatalf("%s: expected invalid diagnostics: %#v", name, config)
		}
	}
}

func TestSDNZoneRequestUsesExplicitConfiguration(t *testing.T) {
	config := sdnZoneModel{
		Zone:   types.StringValue("vz1"),
		Type:   types.StringValue(sdnZoneTypeVLAN),
		Bridge: types.StringValue("vmbr0"),
		Nodes:  testStringSet(t, "node2", "node1"),
	}
	plan := config
	plan.MTU = types.Int64Value(9000)
	plan.State = types.StringValue("new")

	request, diags := sdnZoneRequestFromModels(context.Background(), config, plan)
	if diags.HasError() {
		t.Fatalf("sdnZoneRequestFromModels() unexpected diagnostics: %v", diags)
	}
	if request.MTU != nil || request.Type != sdnZoneTypeVLAN || request.Bridge == nil {
		t.Fatalf("unexpected request: %#v", request)
	}
	if got, want := request.Nodes, []string{"node1", "node2"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("nodes were not sorted: got %v want %v", got, want)
	}
	if got := sdnZoneForm(request).Get("nodes"); got != "node1,node2" {
		t.Fatalf("unexpected nodes wire value: %q", got)
	}
}

func TestSDNZoneManagedFieldDeletion(t *testing.T) {
	config := sdnZoneModel{
		Zone:   types.StringValue("vz1"),
		Type:   types.StringValue(sdnZoneTypeVLAN),
		Bridge: types.StringValue("vmbr0"),
		DNS:    types.StringValue("dns1"),
	}
	got := sdnZoneDeleteKeys(config, []string{"bridge", "bridge-disable-mac-learning", "dns", "ipam", "nodes"})
	if want := "bridge-disable-mac-learning,ipam,nodes"; strings.Join(got, ",") != want {
		t.Fatalf("unexpected delete keys: got %v want %s", got, want)
	}
	if got := sdnZoneDeleteKeys(config, nil); len(got) != 0 {
		t.Fatalf("imported unmanaged fields must not be deleted: %v", got)
	}
}

func TestSDNZoneStateOverlaysStagedValues(t *testing.T) {
	state, diags := sdnZoneStateFromAPI(context.Background(), SDNZone{
		Zone:   "vz1",
		Type:   sdnZoneTypeVLAN,
		Nodes:  "node1",
		Bridge: "vmbr0",
		DNS:    "dns1",
		State:  "changed",
		Digest: "d",
		Pending: map[string]json.RawMessage{
			"bridge": json.RawMessage(`"vmbr1"`),
			"dns":    json.RawMessage(`"deleted"`),
			"mtu":    json.RawMessage(`"1500"`),
		},
	})
	if diags.HasError() {
		t.Fatalf("sdnZoneStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if state.Bridge.ValueString() != "vmbr1" || !state.DNS.IsNull() || state.MTU.ValueInt64() != 1500 {
		t.Fatalf("staged values were not applied: %#v", state)
	}
	if state.State.ValueString() != "changed" {
		t.Fatalf("state was not mapped: %#v", state.State)
	}
	var pending map[string]string
	if diags := state.Pending.ElementsAs(context.Background(), &pending, false); diags.HasError() || pending["dns"] != "deleted" || pending["bridge"] != "vmbr1" {
		t.Fatalf("pending diff was not mapped: %#v (%v)", pending, diags)
	}

	running, diags := sdnZoneStateFromAPI(context.Background(), SDNZone{Zone: "vz2", Type: sdnZoneTypeSimple, Nodes: "node1", State: "new"})
	if diags.HasError() {
		t.Fatalf("running-only state diagnostics: %v", diags)
	}
	if running.Bridge.ValueString() != "" || !running.MTU.IsNull() || !running.Pending.IsNull() || !running.Bridge.IsNull() {
		t.Fatalf("absent values must stay null: %#v", running)
	}
	if running.State.ValueString() != "new" {
		t.Fatalf("new state was not mapped: %#v", running.State)
	}
}

func TestSDNZoneReadStateUsesCollectionAndSkipsDeleted(t *testing.T) {
	t.Run("deleted", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"data":[{"zone":"vz1","type":"vlan","state":"deleted"}]}`))
		}))
		defer server.Close()
		state, diags := (&SDNZoneResource{client: newHAResourceTestClient(t, server.URL)}).readState(context.Background(), "vz1")
		if diags.HasError() || !state.ID.IsNull() {
			t.Fatalf("deleted zone must read as absent: %#v (%v)", state, diags)
		}
	})

	t.Run("api error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeAPIError(t, w, http.StatusForbidden, "missing SDN.Audit")
		}))
		defer server.Close()
		_, diags := (&SDNZoneResource{client: newHAResourceTestClient(t, server.URL)}).readState(context.Background(), "vz1")
		if !diags.HasError() || !strings.Contains(diags[0].Detail(), "missing SDN.Audit") {
			t.Fatalf("API error was not preserved: %v", diags)
		}
	})
}

func TestSDNZoneImportValidation(t *testing.T) {
	for _, id := range []string{"a", "abcdefghi", "a-b", "1ab"} {
		var resp resource.ImportStateResponse
		(&SDNZoneResource{}).ImportState(context.Background(), resource.ImportStateRequest{ID: id}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected import diagnostic for %q", id)
		}
	}
	ctx := context.Background()
	schemaResp := testResourceSchema(t, &SDNZoneResource{})
	var resp resource.ImportStateResponse
	resp.State = tfsdk.State{Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil), Schema: schemaResp.Schema}
	(&SDNZoneResource{}).ImportState(ctx, resource.ImportStateRequest{ID: "vz1"}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}
	var state sdnZoneModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() || state.Zone.ValueString() != "vz1" || state.ID.ValueString() != "vz1" {
		t.Fatalf("unexpected imported state: %#v (%v)", state, diags)
	}
}

func TestSDNZoneLifecycleLockUnlockAndWarning(t *testing.T) {
	zones := map[string]map[string]any{}
	var unlockFails bool
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/lock":
			handler.envelope(w, testSDNLockToken)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/lock":
			if unlockFails {
				writeAPIError(t, w, http.StatusInternalServerError, "lock vanished")
				return
			}
			handler.envelope(w, nil)
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/zones":
			if !handler.form(w, r, url.Values{
				"bridge":     {"vmbr0"},
				"lock-token": {testSDNLockToken},
				"nodes":      {"node1"},
				"type":       {"vlan"},
				"zone":       {"vz1"},
			}) {
				return
			}
			zones["vz1"] = map[string]any{"zone": "vz1", "type": "vlan", "bridge": "vmbr0", "nodes": "node1", "state": "new"}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/zones":
			handler.envelope(w, rawZoneList(zones))
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/sdn/zones/vz1":
			if !handler.form(w, r, url.Values{
				"bridge":     {"vmbr1"},
				"delete":     {"nodes"},
				"digest":     {"digest-1"},
				"lock-token": {testSDNLockToken},
				"type":       {"vlan"},
				"zone":       {"vz1"},
			}) {
				return
			}
			zones["vz1"] = map[string]any{"zone": "vz1", "type": "vlan", "bridge": "vmbr1", "state": "changed"}
			handler.envelope(w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/zones/vz1":
			zones["vz1"] = map[string]any{"zone": "vz1", "type": "vlan", "bridge": "vmbr1", "state": "deleted"}
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected SDN request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &SDNZoneResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := sdnZoneModel{
		Zone:   types.StringValue("vz1"),
		Type:   types.StringValue(sdnZoneTypeVLAN),
		Bridge: types.StringValue("vmbr0"),
		Nodes:  testStringSet(t, "node1"),
	}
	config.Pending = types.MapNull(types.StringType)
	config.Peers = types.SetNull(types.StringType)
	config.ExitNodes = types.SetNull(types.StringType)
	config.SecondaryControllers = types.SetNull(types.StringType)
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created sdnZoneModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() {
		t.Fatalf("decode created state: %v", diags)
	}
	if created.State.ValueString() != "new" || created.Bridge.ValueString() != "vmbr0" {
		t.Fatalf("unexpected created state: %#v", created)
	}

	updated := config
	updated.Bridge = types.StringValue("vmbr1")
	updated.Nodes = types.SetNull(types.StringType)
	zones["vz1"] = map[string]any{"zone": "vz1", "type": "vlan", "bridge": "vmbr0", "nodes": "node1", "state": "new", "digest": "digest-1"}
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
	var refreshed sdnZoneModel
	if diags := updateResp.State.Get(ctx, &refreshed); diags.HasError() {
		t.Fatalf("decode updated state: %v", diags)
	}
	if refreshed.Bridge.ValueString() != "vmbr1" || !refreshed.Nodes.IsNull() {
		t.Fatalf("unexpected updated state: %#v", refreshed)
	}

	unlockFails = true
	deleteResp := resource.DeleteResponse{State: updateResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", deleteResp.Diagnostics)
	}
	foundWarning := false
	for _, diagnostic := range deleteResp.Diagnostics {
		if diagnostic.Severity() == diag.SeverityWarning {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("stale lock must warn: %v", deleteResp.Diagnostics)
	}

	readResp := resource.ReadResponse{State: updateResp.State}
	res.Read(ctx, resource.ReadRequest{State: updateResp.State}, &readResp)
	if readResp.Diagnostics.HasError() || !readResp.State.Raw.IsNull() {
		t.Fatalf("deleted zone was not removed: diagnostics=%v raw=%v", readResp.Diagnostics, readResp.State.Raw)
	}

	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

func rawZoneList(zones map[string]map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(zones))
	for _, entry := range zones {
		result = append(result, entry)
	}
	return result
}

func TestSDNZoneStateMapsExtraTypes(t *testing.T) {
	state, diags := sdnZoneStateFromAPI(context.Background(), SDNZone{
		Zone:                 "ez1",
		Type:                 sdnZoneTypeEVPN,
		VRFVXLAN:             proxmoxOptionalInt64{value: intPtr64(1000)},
		Controller:           "ctl",
		SecondaryControllers: []string{"ctl2"},
		ExitNodes:            "node2",
		State:                "changed",
		Pending: map[string]json.RawMessage{
			"secondary-controllers": json.RawMessage(`["ctl2","ctl3"]`),
			"exitnodes":             json.RawMessage(`"node3,node2"`),
			"tag":                   json.RawMessage(`"100"`),
			"rt-import":             json.RawMessage(`"deleted"`),
		},
	})
	if diags.HasError() {
		t.Fatalf("state mapping diagnostics: %v", diags)
	}
	var controllers []string
	if diags := state.SecondaryControllers.ElementsAs(context.Background(), &controllers, false); diags.HasError() || len(controllers) != 2 {
		t.Fatalf("staged controller array was not applied: %#v (%v)", controllers, diags)
	}
	var exitNodes []string
	if diags := state.ExitNodes.ElementsAs(context.Background(), &exitNodes, false); diags.HasError() || len(exitNodes) != 2 {
		t.Fatalf("staged exit nodes were not applied: %#v (%v)", exitNodes, diags)
	}
	if state.VRFVXLAN.ValueInt64() != 1000 || state.Controller.ValueString() != "ctl" {
		t.Fatalf("running values were not preserved: %#v", state)
	}
	if !state.RTImport.IsNull() || state.Tag.ValueInt64() != 100 {
		t.Fatalf("staged scalar handling is wrong: rt=%#v tag=%#v", state.RTImport, state.Tag)
	}
}
