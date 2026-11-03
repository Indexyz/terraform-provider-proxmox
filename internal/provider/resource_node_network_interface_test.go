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
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestNodeNetworkInterfaceSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewNodeNetworkInterfaceResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 28; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	for _, name := range []string{"node", "iface", "type"} {
		attribute, ok := resp.Schema.Attributes[name].(resourceschema.StringAttribute)
		if !ok || !attribute.IsRequired() || len(attribute.PlanModifiers) == 0 {
			t.Fatalf("%s must be required with a replacement plan modifier", name)
		}
	}
	if !resp.Schema.Attributes["id"].IsComputed() {
		t.Fatal("id must be computed")
	}
	if !resp.Schema.Attributes["ovs_ports"].IsComputed() || resp.Schema.Attributes["ovs_ports"].IsOptional() {
		t.Fatal("ovs_ports must be computed-only; the provider maintains the bridge port list")
	}
	for _, name := range []string{"ovs_tag", "ovs_options", "ovs_bridge", "ovs_bonds"} {
		attribute := resp.Schema.Attributes[name]
		if !attribute.IsOptional() || !attribute.IsComputed() {
			t.Fatalf("%s must be optional and computed", name)
		}
	}
	for _, excluded := range []string{"digest", "cidr", "cidr6", "families", "method", "active", "exists"} {
		if _, ok := resp.Schema.Attributes[excluded]; ok {
			t.Fatalf("node network interface schema must not expose %s", excluded)
		}
	}
}

func TestValidateNodeNetworkConfig(t *testing.T) {
	bridge := func() nodeNetworkModel {
		return nodeNetworkModel{
			Node:        types.StringValue("node1"),
			Iface:       types.StringValue("vmbr1"),
			Type:        types.StringValue(nodeNetworkTypeBridge),
			BridgePorts: testStringSet(t, "eth1", "eth2"),
			Address:     types.StringValue("10.0.0.2"),
			Netmask:     types.StringValue("255.255.255.0"),
			MTU:         types.Int64Value(9000),
			Autostart:   types.BoolValue(true),
		}
	}
	valid := []nodeNetworkModel{
		bridge(),
		{
			Node:        types.StringValue("node1"),
			Iface:       types.StringValue("bond1"),
			Type:        types.StringValue(nodeNetworkTypeBond),
			Slaves:      testStringSet(t, "eth1"),
			BondMode:    types.StringValue("active-backup"),
			BondPrimary: types.StringValue("eth1"),
			Address6:    types.StringValue("fe80::1"),
			Netmask6:    types.Int64Value(64),
		},
		{
			Node:          types.StringValue("node1"),
			Iface:         types.StringValue("bond0.100"),
			Type:          types.StringValue(nodeNetworkTypeVLAN),
			VLANID:        types.Int64Value(100),
			VLANRawDevice: types.StringValue("bond0"),
		},
		{
			Node:  types.StringValue("node1"),
			Iface: types.StringValue("eth5"),
			Type:  types.StringValue(nodeNetworkTypeEth),
		},
		{
			Node:       types.StringValue("node1"),
			Iface:      types.StringValue("vmbr2"),
			Type:       types.StringUnknown(),
			BridgeVIDs: types.StringValue("2 4"),
		},
	}
	for _, config := range valid {
		if diags := validateNodeNetworkConfig(config); diags.HasError() {
			t.Fatalf("valid config diagnostics: %v (%#v)", diags, config)
		}
	}

	invalid := map[string]nodeNetworkModel{}
	add := func(name string, mutate func(*nodeNetworkModel)) {
		config := bridge()
		mutate(&config)
		invalid[name] = config
	}
	add("bad node", func(m *nodeNetworkModel) { m.Node = types.StringValue("node_1") })
	add("single char iface", func(m *nodeNetworkModel) { m.Iface = types.StringValue("v") })
	add("bad iface", func(m *nodeNetworkModel) { m.Iface = types.StringValue("-vmbr") })
	add("unknown type", func(m *nodeNetworkModel) { m.Type = types.StringValue("OVSBridge") })
	add("address without netmask", func(m *nodeNetworkModel) { m.Netmask = types.StringNull() })
	add("netmask without address", func(m *nodeNetworkModel) { m.Address = types.StringNull() })
	add("address6 without netmask6", func(m *nodeNetworkModel) { m.Address6 = types.StringValue("fe80::1") })
	add("mtu too small", func(m *nodeNetworkModel) { m.MTU = types.Int64Value(1000) })
	add("mtu too large", func(m *nodeNetworkModel) { m.MTU = types.Int64Value(70000) })
	add("netmask6 out of range", func(m *nodeNetworkModel) {
		m.Address6 = types.StringValue("fe80::1")
		m.Netmask6 = types.Int64Value(129)
	})
	add("bad port", func(m *nodeNetworkModel) { m.BridgePorts = testStringSet(t, "eth 1") })
	add("slaves on bridge", func(m *nodeNetworkModel) { m.Slaves = testStringSet(t, "eth1") })
	add("vlan fields on bridge", func(m *nodeNetworkModel) { m.VLANID = types.Int64Value(100) })
	add("bridge ports on bond", func(m *nodeNetworkModel) {
		m.Type = types.StringValue(nodeNetworkTypeBond)
	})
	add("vlan tag out of range", func(m *nodeNetworkModel) {
		m.Type = types.StringValue(nodeNetworkTypeVLAN)
		m.BridgePorts = types.SetNull(types.StringType)
		m.VLANID = types.Int64Value(5000)
	})
	for name, config := range invalid {
		if diags := validateNodeNetworkConfig(config); !diags.HasError() {
			t.Fatalf("%s: expected invalid config diagnostics: %#v", name, config)
		}
	}
}

func TestNodeNetworkRequestUsesExplicitConfigurationOnly(t *testing.T) {
	config := nodeNetworkModel{
		Node:        types.StringValue("node1"),
		Iface:       types.StringValue("vmbr1"),
		Type:        types.StringValue(nodeNetworkTypeBridge),
		BridgePorts: testStringSet(t, "eth2", "eth1"),
		Slaves:      types.SetNull(types.StringType),
		OVSBonds:    types.SetNull(types.StringType),
		OVSPorts:    types.SetNull(types.StringType),
		Comments:    types.StringValue("lab bridge"),
	}
	plan := config
	plan.Autostart = types.BoolValue(true)
	plan.MTU = types.Int64Value(9000)

	request, diags := nodeNetworkRequestFromModels(context.Background(), config, plan)
	if diags.HasError() {
		t.Fatalf("nodeNetworkRequestFromModels() unexpected diagnostics: %v", diags)
	}
	if request.Autostart != nil || request.MTU != nil {
		t.Fatalf("request sent computed-only plan values: %#v", request)
	}
	if request.Comments == nil || *request.Comments != "lab bridge" {
		t.Fatalf("request omitted explicit configuration: %#v", request)
	}
	form := nodeNetworkInterfaceForm(request)
	if got, want := form.Get("bridge_ports"), "eth1 eth2"; got != want {
		t.Fatalf("bridge ports were not sorted and space joined: got %q want %q", got, want)
	}
	if form.Has("slaves") {
		t.Fatal("null slaves must not be sent")
	}
}

func TestNodeNetworkExplicitEmptyPortListClearsSet(t *testing.T) {
	config := nodeNetworkModel{
		Node:        types.StringValue("node1"),
		Iface:       types.StringValue("vmbr1"),
		Type:        types.StringValue(nodeNetworkTypeBridge),
		BridgePorts: types.SetValueMust(types.StringType, nil),
	}
	request, _ := nodeNetworkRequestFromModels(context.Background(), config, config)
	form := nodeNetworkInterfaceForm(request)
	if value, ok := form["bridge_ports"]; !ok || value[0] != "" {
		t.Fatalf("explicitly empty bridge_ports must be sent as empty string: %#v", form)
	}
}

func TestNodeNetworkManagedFieldDeletion(t *testing.T) {
	config := nodeNetworkModel{
		Iface:       types.StringValue("vmbr1"),
		Type:        types.StringValue(nodeNetworkTypeBridge),
		BridgePorts: testStringSet(t, "eth1"),
		MTU:         types.Int64Value(9000),
	}
	got := nodeNetworkDeleteKeys(config, []string{"address", "bridge_ports", "gateway", "mtu", "netmask", "vlan-id"})
	want := []string{"address", "gateway", "netmask", "vlan-id"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected delete keys: got %v want %v", got, want)
	}
	if got := nodeNetworkDeleteKeys(config, nil); len(got) != 0 {
		t.Fatalf("imported unmanaged fields must not be deleted: %v", got)
	}
}

func TestNodeNetworkStateFromAPIMapsHyphenatedKeys(t *testing.T) {
	state, diags := nodeNetworkStateFromAPI(context.Background(), "node1", NodeNetworkInterface{
		Iface:           "bond0.100",
		Type:            nodeNetworkTypeVLAN,
		Autostart:       proxmoxOptionalBool{value: boolPtr(true)},
		MTU:             proxmoxOptionalInt64{value: intPtr64(1400)},
		VLANID:          proxmoxOptionalInt64{value: intPtr64(100)},
		VLANRawDevice:   "bond0",
		BondPrimary:     "eth1",
		Netmask6:        proxmoxOptionalInt64{value: intPtr64(64)},
		Address6:        "fe80::1",
		BridgePorts:     "eth1 eth2",
		BridgeVLANAware: proxmoxOptionalBool{value: boolPtr(false)},
	})
	if diags.HasError() {
		t.Fatalf("nodeNetworkStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "node1/bond0.100" || state.Node.ValueString() != "node1" {
		t.Fatalf("unexpected identity: %#v", state)
	}
	if !state.Autostart.ValueBool() || state.MTU.ValueInt64() != 1400 || state.VLANID.ValueInt64() != 100 {
		t.Fatalf("unexpected scalars: %#v", state)
	}
	if state.VLANRawDevice.ValueString() != "bond0" || state.BondPrimary.ValueString() != "eth1" || state.Address6.ValueString() != "fe80::1" || state.Netmask6.ValueInt64() != 64 {
		t.Fatalf("hyphenated keys were not mapped: %#v", state)
	}
	if !state.BridgeVLANAware.IsNull() && state.BridgeVLANAware.ValueBool() {
		t.Fatalf("bridge_vlan_aware must be false: %#v", state.BridgeVLANAware)
	}
	var ports []string
	if diags := state.BridgePorts.ElementsAs(context.Background(), &ports, false); diags.HasError() || len(ports) != 2 {
		t.Fatalf("unexpected bridge ports: %#v (%v)", ports, diags)
	}
	if !state.Comments.IsNull() || !state.Address.IsNull() || state.ID.IsNull() {
		t.Fatalf("missing values must stay null: %#v", state)
	}

	empty, diags := nodeNetworkStateFromAPI(context.Background(), "node1", NodeNetworkInterface{Iface: "vmbr2", Type: nodeNetworkTypeBridge, BridgePorts: "none"})
	if diags.HasError() || len(empty.BridgePorts.Elements()) != 0 {
		t.Fatalf("empty bridge ports must normalize to an empty set: %#v (%v)", empty.BridgePorts, diags)
	}
}

func TestNodeNetworkReadStateUsesCollectionAndPreservesErrors(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeEnvelope(t, w, []any{})
		}))
		defer server.Close()
		state, diags := (&NodeNetworkInterfaceResource{client: newHAResourceTestClient(t, server.URL)}).readState(context.Background(), "node1", "vmbr9")
		if diags.HasError() || !state.ID.IsNull() {
			t.Fatalf("missing readState() = %#v, %v", state, diags)
		}
	})

	t.Run("api error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"iface":"permission denied"},"data":null}`))
		}))
		defer server.Close()
		_, diags := (&NodeNetworkInterfaceResource{client: newHAResourceTestClient(t, server.URL)}).readState(context.Background(), "node1", "vmbr1")
		if !diags.HasError() || !strings.Contains(diags[0].Detail(), "status 500") || !strings.Contains(diags[0].Detail(), "permission denied") {
			t.Fatalf("readState() did not preserve API error: %v", diags)
		}
	})
}

func TestNodeNetworkImportValidation(t *testing.T) {
	for _, id := range []string{"node1", "/vmbr1", "node_1/vmbr1", "node1/v", "node1/vmbr 1"} {
		var resp resource.ImportStateResponse
		(&NodeNetworkInterfaceResource{}).ImportState(context.Background(), resource.ImportStateRequest{ID: id}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected import diagnostic for %q", id)
		}
	}
	ctx := context.Background()
	schemaResp := testResourceSchema(t, &NodeNetworkInterfaceResource{})
	var resp resource.ImportStateResponse
	resp.State = tfsdk.State{Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil), Schema: schemaResp.Schema}
	(&NodeNetworkInterfaceResource{}).ImportState(ctx, resource.ImportStateRequest{ID: "node1/vmbr7"}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}
	var state nodeNetworkModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("unable to read imported state: %v", diags)
	}
	if state.ID.ValueString() != "node1/vmbr7" || state.Node.ValueString() != "node1" || state.Iface.ValueString() != "vmbr7" {
		t.Fatalf("unexpected imported state: %#v", state)
	}
}

func TestNodeNetworkInterfaceLifecycle(t *testing.T) {
	exists := true
	handler := &lifecycleHandler{}
	var ifaceState map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/node1/network":
			if !handler.form(w, r, url.Values{
				"address":      {"10.0.0.2"},
				"bridge_ports": {"eth1 eth2"},
				"iface":        {"vmbr7"},
				"netmask":      {"255.255.255.0"},
				"type":         {"bridge"},
			}) {
				return
			}
			ifaceState = map[string]string{"iface": "vmbr7", "type": "bridge", "address": "10.0.0.2", "netmask": "255.255.255.0", "bridge_ports": "eth1 eth2"}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/node1/network":
			if !exists {
				handler.envelope(w, []any{})
				return
			}
			handler.envelope(w, []any{ifaceState})
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/nodes/node1/network/vmbr7":
			if !handler.form(w, r, url.Values{
				"delete":       {"address,netmask"},
				"mtu":          {"9000"},
				"type":         {"bridge"},
				"bridge_ports": {"eth1 eth3"},
			}) {
				return
			}
			ifaceState = map[string]string{"iface": "vmbr7", "type": "bridge", "mtu": "9000", "bridge_ports": "eth1 eth3"}
			handler.envelope(w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/nodes/node1/network/vmbr7":
			exists = false
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected node network request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &NodeNetworkInterfaceResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := nodeNetworkModel{
		Node:        types.StringValue("node1"),
		Iface:       types.StringValue("vmbr7"),
		Type:        types.StringValue(nodeNetworkTypeBridge),
		BridgePorts: testStringSet(t, "eth2", "eth1"),
		Slaves:      types.SetNull(types.StringType),
		OVSBonds:    types.SetNull(types.StringType),
		OVSPorts:    types.SetNull(types.StringType),
		Address:     types.StringValue("10.0.0.2"),
		Netmask:     types.StringValue("255.255.255.0"),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created nodeNetworkModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() {
		t.Fatalf("decode created state: %v", diags)
	}
	if created.ID.ValueString() != "node1/vmbr7" || created.Node.ValueString() != "node1" {
		t.Fatalf("unexpected created state: %#v", created)
	}

	updated := config
	updated.Address = types.StringNull()
	updated.Netmask = types.StringNull()
	updated.MTU = types.Int64Value(9000)
	updated.BridgePorts = testStringSet(t, "eth1", "eth3")
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
	var refreshed nodeNetworkModel
	if diags := updateResp.State.Get(ctx, &refreshed); diags.HasError() {
		t.Fatalf("decode updated state: %v", diags)
	}
	if !refreshed.Address.IsNull() || refreshed.MTU.ValueInt64() != 9000 {
		t.Fatalf("unexpected updated state: %#v", refreshed)
	}

	deleteResp := resource.DeleteResponse{State: updateResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", deleteResp.Diagnostics)
	}
	readResp := resource.ReadResponse{State: updateResp.State}
	res.Read(ctx, resource.ReadRequest{State: updateResp.State}, &readResp)
	if readResp.Diagnostics.HasError() || !readResp.State.Raw.IsNull() {
		t.Fatalf("missing interface was not removed: diagnostics=%v raw=%v", readResp.Diagnostics, readResp.State.Raw)
	}
	idempotentDelete := resource.DeleteResponse{State: updateResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, &idempotentDelete)
	if idempotentDelete.Diagnostics.HasError() {
		t.Fatalf("idempotent delete diagnostics: %v", idempotentDelete.Diagnostics)
	}

	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

func TestValidateNodeNetworkOVSConfig(t *testing.T) {
	base := func() nodeNetworkModel {
		return nodeNetworkModel{
			Node:       types.StringValue("node1"),
			Iface:      types.StringValue("ovs1"),
			Type:       types.StringValue(nodeNetworkTypeOVSIntPort),
			OVSBridge:  types.StringValue("ovsbr0"),
			OVSTag:     types.Int64Value(100),
			OVSOptions: types.StringValue("trunks=100,200"),
		}
	}
	valid := []nodeNetworkModel{
		base(),
		{
			Node: types.StringValue("node1"), Iface: types.StringValue("ovsp1"),
			Type: types.StringValue(nodeNetworkTypeOVSPort), OVSBridge: types.StringValue("ovsbr0"),
		},
		{
			Node: types.StringValue("node1"), Iface: types.StringValue("ovsb0"),
			Type: types.StringValue(nodeNetworkTypeOVSBond), OVSBridge: types.StringValue("ovsbr0"),
			OVSBonds: testStringSet(t, "eth1", "eth2"), BondMode: types.StringValue("lacp-balance-slb"),
		},
		{
			Node: types.StringValue("node1"), Iface: types.StringValue("ovsbr0"),
			Type: types.StringValue(nodeNetworkTypeOVSBridge), MTU: types.Int64Value(9000), Autostart: types.BoolValue(true),
		},
	}
	for _, config := range valid {
		if diags := validateNodeNetworkConfig(config); diags.HasError() {
			t.Fatalf("valid OVS config diagnostics: %v (%#v)", diags, config)
		}
	}

	invalid := map[string]func(*nodeNetworkModel){
		"ovs_tag out of range": func(m *nodeNetworkModel) { m.OVSTag = types.Int64Value(4095) },
		"ovs_bridge missing":   func(m *nodeNetworkModel) { m.OVSBridge = types.StringNull() },
		"bad ovs bond slave":   func(m *nodeNetworkModel) { m.OVSBonds = testStringSet(t, "eth 1") },
		"ovs field on bridge": func(m *nodeNetworkModel) {
			m.Type = types.StringValue(nodeNetworkTypeOVSBridge)
			m.OVSTag = types.Int64Value(5)
		},
		"autostart on ovs port": func(m *nodeNetworkModel) { m.Autostart = types.BoolValue(true) },
		"linux mode on ovs bond": func(m *nodeNetworkModel) {
			m.Type = types.StringValue(nodeNetworkTypeOVSBond)
			m.BondMode = types.StringValue("802.3ad")
			m.OVSBonds = testStringSet(t, "eth1")
		},
		"ovs mode on linux bond": func(m *nodeNetworkModel) {
			m.Type = types.StringValue(nodeNetworkTypeBond)
			m.BondMode = types.StringValue("lacp-balance-tcp")
			m.OVSBridge = types.StringNull()
		},
	}
	for name, mutate := range invalid {
		config := base()
		mutate(&config)
		if diags := validateNodeNetworkConfig(config); !diags.HasError() {
			t.Fatalf("%s: expected invalid diagnostics: %#v", name, config)
		}
	}
}

func TestNodeNetworkOVSStateAndForms(t *testing.T) {
	state, diags := nodeNetworkStateFromAPI(context.Background(), "node1", NodeNetworkInterface{
		Iface: "ovsbr0", Type: nodeNetworkTypeOVSBridge, OVSPorts: "ovsp1 ovsb0",
	})
	if diags.HasError() {
		t.Fatalf("state mapping diagnostics: %v", diags)
	}
	var ports []string
	if diags := state.OVSPorts.ElementsAs(context.Background(), &ports, false); diags.HasError() || len(ports) != 2 {
		t.Fatalf("ovs_ports were not mapped: %#v (%v)", ports, diags)
	}
	if !state.OVSTag.IsNull() || !state.OVSBridge.IsNull() || !state.OVSOptions.IsNull() {
		t.Fatalf("unset OVS fields must stay null: %#v", state)
	}

	config := nodeNetworkModel{
		Node: types.StringValue("node1"), Iface: types.StringValue("ovsb0"),
		Type: types.StringValue(nodeNetworkTypeOVSBond), OVSBridge: types.StringValue("ovsbr0"),
		OVSBonds: testStringSet(t, "eth2", "eth1"), OVSTag: types.Int64Value(10),
	}
	request, requestDiags := nodeNetworkRequestFromModels(context.Background(), config, config)
	if requestDiags.HasError() {
		t.Fatalf("request diagnostics: %v", requestDiags)
	}
	form := nodeNetworkInterfaceForm(request)
	if got, want := form.Get("ovs_bonds"), "eth1 eth2"; got != want {
		t.Fatalf("ovs bonds were not encoded: got %q want %q", got, want)
	}
	if form.Get("ovs_bridge") != "ovsbr0" || form.Get("ovs_tag") != "10" {
		t.Fatalf("ovs fields were not encoded: %#v", form)
	}
}

// ovsBridgeMock serves a node with one OVS bridge and records bridge updates so
// the port membership cross-write can be asserted.
func ovsBridgeMock(t *testing.T, handler *lifecycleHandler, calls *[]string, bridgeUpdates *[]url.Values) *httptest.Server {
	t.Helper()
	interfaces := map[string]map[string]any{
		"ovsbr0": {"iface": "ovsbr0", "type": "OVSBridge", "ovs_ports": "eth9"},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		*calls = append(*calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/node1/network":
			list := make([]map[string]any, 0, len(interfaces))
			for _, entry := range interfaces {
				list = append(list, entry)
			}
			handler.envelope(w, list)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/node1/network":
			if err := r.ParseForm(); err != nil {
				handler.fail(w, "parse create form: %v", err)
				return
			}
			iface := r.Form.Get("iface")
			interfaces[iface] = map[string]any{"iface": iface, "type": r.Form.Get("type"), "ovs_bridge": r.Form.Get("ovs_bridge")}
			handler.envelope(w, nil)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.EscapedPath(), "/api2/json/nodes/node1/network/"):
			if err := r.ParseForm(); err != nil {
				handler.fail(w, "parse bridge form: %v", err)
				return
			}
			*bridgeUpdates = append(*bridgeUpdates, r.Form)
			bridge := strings.TrimPrefix(r.URL.EscapedPath(), "/api2/json/nodes/node1/network/")
			interfaces[bridge]["ovs_ports"] = r.Form.Get("ovs_ports")
			handler.envelope(w, nil)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/api2/json/nodes/node1/network/"):
			delete(interfaces, strings.TrimPrefix(r.URL.EscapedPath(), "/api2/json/nodes/node1/network/"))
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected OVS request: %s %s", r.Method, r.URL.String())
		}
	}))
}

func TestNodeNetworkOVSPortMaintainsBridgeMembership(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	var calls []string
	var bridgeUpdates []url.Values
	server := ovsBridgeMock(t, handler, &calls, &bridgeUpdates)
	defer server.Close()

	res := &NodeNetworkInterfaceResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := ovsPortTestModel("ovsbr0")
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	if len(bridgeUpdates) != 1 || bridgeUpdates[0].Get("ovs_ports") != "eth9 ovsp1" || bridgeUpdates[0].Get("type") != "nodeNetworkTypeOVSBridge" && bridgeUpdates[0].Get("type") != "OVSBridge" {
		t.Fatalf("port was not appended to the bridge ports: %#v", bridgeUpdates)
	}

	deleteResp := resource.DeleteResponse{State: createResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: createResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", deleteResp.Diagnostics)
	}
	if len(bridgeUpdates) != 2 || bridgeUpdates[1].Get("ovs_ports") != "eth9" {
		t.Fatalf("port was not removed from the bridge ports: %#v", bridgeUpdates)
	}
	// The detach must happen before the interface deletion so no dangling
	// bridge reference is left behind.
	putIndex, deleteIndex := -1, -1
	for index, call := range calls {
		if call == "PUT /api2/json/nodes/node1/network/ovsbr0" && putIndex < 0 && index > 0 {
			putIndex = index
		}
		if call == "DELETE /api2/json/nodes/node1/network/ovsp1" {
			deleteIndex = index
		}
	}
	if putIndex < 0 || deleteIndex < putIndex {
		t.Fatalf("unexpected call order: %v", calls)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

func TestNodeNetworkOVSPortWithoutBridgeSkipsMembership(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	var calls []string
	var bridgeUpdates []url.Values
	server := ovsBridgeMock(t, handler, &calls, &bridgeUpdates)
	defer server.Close()

	res := &NodeNetworkInterfaceResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := ovsPortTestModel("")
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	if len(bridgeUpdates) != 0 {
		t.Fatalf("unattached port must not touch the bridge: %#v", bridgeUpdates)
	}
}

func TestNodeNetworkOVSPortRelocateMovesMembership(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	var calls []string
	var bridgeUpdates []url.Values
	server := ovsBridgeMock(t, handler, &calls, &bridgeUpdates)
	defer server.Close()

	res := &NodeNetworkInterfaceResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	prior := ovsPortTestModel("ovsbr0")
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, prior), Plan: testResourcePlan(t, schema, prior)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	bridgeUpdates = nil

	// Re-point the port to a second bridge.
	handler2 := &lifecycleHandler{}
	interfaces := map[string]map[string]any{
		"ovsbr0": {"iface": "ovsbr0", "type": "OVSBridge", "ovs_ports": "ovsp1"},
		"ovsp1":  {"iface": "ovsp1", "type": "OVSPort", "ovs_bridge": "ovsbr0"},
		"ovsbr1": {"iface": "ovsbr1", "type": "OVSBridge", "ovs_ports": ""},
	}
	var calls2 []string
	var updates2 []url.Values
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler2.auth(w, r) {
			return
		}
		calls2 = append(calls2, r.Method+" "+r.URL.EscapedPath())
		switch r.Method {
		case http.MethodGet:
			list := make([]map[string]any, 0, len(interfaces))
			for _, entry := range interfaces {
				list = append(list, entry)
			}
			handler2.envelope(w, list)
		case http.MethodPut:
			if err := r.ParseForm(); err != nil {
				handler2.fail(w, "parse bridge form: %v", err)
				return
			}
			updates2 = append(updates2, r.Form)
			bridge := strings.TrimPrefix(r.URL.EscapedPath(), "/api2/json/nodes/node1/network/")
			if _, ok := interfaces[bridge]; !ok {
				// The port itself is updated too; keep its record.
				interfaces["ovsp1"]["ovs_bridge"] = r.Form.Get("ovs_bridge")
				handler2.envelope(w, nil)
				return
			}
			interfaces[bridge]["ovs_ports"] = r.Form.Get("ovs_ports")
			handler2.envelope(w, nil)
		default:
			handler2.fail(w, "unexpected relocate request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server2.Close()

	res2 := &NodeNetworkInterfaceResource{client: testLifecycleClient(t, server2)}
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}, Private: createResp.Private}
	res2.Update(ctx, resource.UpdateRequest{
		Config:  testResourceConfig(t, schema, func() nodeNetworkModel { next := prior; next.OVSBridge = types.StringValue("ovsbr1"); return next }()),
		Plan:    testResourcePlan(t, schema, func() nodeNetworkModel { next := prior; next.OVSBridge = types.StringValue("ovsbr1"); return next }()),
		State:   createResp.State,
		Private: createResp.Private,
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("relocate diagnostics: %v", updateResp.Diagnostics)
	}
	if got := interfaces["ovsbr0"]["ovs_ports"]; got != "" {
		t.Fatalf("old bridge still lists the port: %v", got)
	}
	if got := interfaces["ovsbr1"]["ovs_ports"]; got != "ovsp1" {
		t.Fatalf("new bridge does not list the port: %v", got)
	}
	for _, failure := range handler2.failures {
		t.Fatal(failure)
	}
}

func TestNodeNetworkOVSPortAttachFailureSurfaces(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			handler.envelope(w, []map[string]any{{"iface": "ovsbr0", "type": "OVSBridge"}})
		case http.MethodPost:
			handler.envelope(w, nil)
		case http.MethodPut:
			writeAPIError(t, w, http.StatusInternalServerError, "bridge locked")
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &NodeNetworkInterfaceResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := ovsPortTestModel("ovsbr0")
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if !createResp.Diagnostics.HasError() || !strings.Contains(createResp.Diagnostics[0].Detail(), "bridge locked") {
		t.Fatalf("attach failure was not surfaced: %v", createResp.Diagnostics)
	}
}

func TestNodeNetworkOVSPortDeleteToleratesMissingBridge(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	var deleted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			// The bridge is gone, the port record still points at it.
			handler.envelope(w, []map[string]any{{"iface": "ovsp1", "type": "OVSPort", "ovs_bridge": "ovsbr-gone"}})
		case http.MethodDelete:
			deleted = true
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &NodeNetworkInterfaceResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := ovsPortTestModel("ovsbr-gone")
	deleteResp := resource.DeleteResponse{State: testResourceState(t, schema, state)}
	res.Delete(ctx, resource.DeleteRequest{State: deleteResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", deleteResp.Diagnostics)
	}
	if !deleted {
		t.Fatal("port was not deleted after the detach failed with not-found")
	}
}

// ovsPortModel builds an OVS port model with every set attribute typed null so
// tfsdk values convert cleanly.
func ovsPortTestModel(bridge string) nodeNetworkModel {
	const iface = "ovsp1"
	model := nodeNetworkModel{
		ID:          types.StringValue("node1/" + iface),
		Node:        types.StringValue("node1"),
		Iface:       types.StringValue(iface),
		Type:        types.StringValue(nodeNetworkTypeOVSPort),
		BridgePorts: types.SetNull(types.StringType),
		Slaves:      types.SetNull(types.StringType),
		OVSPorts:    types.SetNull(types.StringType),
		OVSBonds:    types.SetNull(types.StringType),
	}
	if bridge != "" {
		model.OVSBridge = types.StringValue(bridge)
	}
	return model
}

func TestNodeNetworkOVSPortAdoptsExistingNIC(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	interfaces := map[string]map[string]any{
		"ovsbr0": {"iface": "ovsbr0", "type": "OVSBridge", "ovs_ports": ""},
		"eth1":   {"iface": "eth1", "type": "eth", "address": "10.0.0.5", "netmask": "255.255.255.0"},
	}
	var portUpdates []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			list := make([]map[string]any, 0, len(interfaces))
			for _, entry := range interfaces {
				list = append(list, entry)
			}
			handler.envelope(w, list)
		case http.MethodPost:
			writeAPIError(t, w, http.StatusBadRequest, "interface already exists")
		case http.MethodPut:
			if err := r.ParseForm(); err != nil {
				handler.fail(w, "parse form: %v", err)
				return
			}
			bridge := strings.TrimPrefix(r.URL.EscapedPath(), "/api2/json/nodes/node1/network/")
			if entry, ok := interfaces[bridge]; ok && entry["type"] == "OVSBridge" {
				interfaces[bridge]["ovs_ports"] = r.Form.Get("ovs_ports")
				handler.envelope(w, nil)
				return
			}
			portUpdates = append(portUpdates, r.Form)
			interfaces["eth1"]["type"] = "OVSPort"
			interfaces["eth1"]["ovs_bridge"] = r.Form.Get("ovs_bridge")
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &NodeNetworkInterfaceResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := nodeNetworkModel{
		ID: types.StringValue("node1/eth1"), Node: types.StringValue("node1"), Iface: types.StringValue("eth1"),
		Type: types.StringValue(nodeNetworkTypeOVSPort), OVSBridge: types.StringValue("ovsbr0"),
		BridgePorts: types.SetNull(types.StringType), Slaves: types.SetNull(types.StringType),
		OVSPorts: types.SetNull(types.StringType), OVSBonds: types.SetNull(types.StringType),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("adoption diagnostics: %v", createResp.Diagnostics)
	}
	if len(portUpdates) != 1 || portUpdates[0].Get("type") != "OVSPort" || portUpdates[0].Get("ovs_bridge") != "ovsbr0" {
		t.Fatalf("existing NIC was not converted to an OVS port: %#v", portUpdates)
	}
	if _, ok := portUpdates[0]["address"]; ok {
		t.Fatalf("adoption must not overwrite unconfigured settings: %#v", portUpdates[0])
	}
	if got := interfaces["ovsbr0"]["ovs_ports"]; got != "eth1" {
		t.Fatalf("adopted port was not attached to the bridge: %v", got)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

func TestNodeNetworkOVSPortAdoptionRejectsForeignType(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			handler.envelope(w, []map[string]any{{"iface": "vmbr0", "type": "bridge"}})
		case http.MethodPost:
			writeAPIError(t, w, http.StatusBadRequest, "interface already exists")
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &NodeNetworkInterfaceResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := ovsPortTestModel("vmbr0")
	config.Iface = types.StringValue("vmbr0")
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if !createResp.Diagnostics.HasError() || !strings.Contains(createResp.Diagnostics[0].Detail(), `"bridge"`) {
		t.Fatalf("foreign interface type was adopted: %v", createResp.Diagnostics)
	}
}

func TestNodeNetworkDuplicateNonOVSPortKeepsOriginalError(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.Method == http.MethodPost {
			writeAPIError(t, w, http.StatusBadRequest, "interface already exists")
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
	}))
	defer server.Close()

	res := &NodeNetworkInterfaceResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := ovsPortTestModel("")
	config.Type = types.StringValue(nodeNetworkTypeOVSBridge)
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if !createResp.Diagnostics.HasError() || !strings.Contains(createResp.Diagnostics[0].Detail(), "already exists") {
		t.Fatalf("duplicate OVSBridge was not reported: %v", createResp.Diagnostics)
	}
}
