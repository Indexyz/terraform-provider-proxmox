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

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestSDNVnetSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewSDNVnetResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 9; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	for _, name := range []string{"vnet", "zone"} {
		attribute, ok := resp.Schema.Attributes[name].(resourceschema.StringAttribute)
		if !ok || !attribute.IsRequired() {
			t.Fatalf("%s must be required", name)
		}
	}
	vnetAttribute, _ := resp.Schema.Attributes["vnet"].(resourceschema.StringAttribute)
	zoneAttribute, _ := resp.Schema.Attributes["zone"].(resourceschema.StringAttribute)
	if len(vnetAttribute.PlanModifiers) == 0 {
		t.Fatal("vnet must require replacement")
	}
	if len(zoneAttribute.PlanModifiers) != 0 {
		t.Fatal("zone must stay mutable so a VNet can move between zones")
	}
	for _, name := range []string{"alias", "tag", "vlanaware", "isolate_ports"} {
		if !resp.Schema.Attributes[name].IsOptional() || !resp.Schema.Attributes[name].IsComputed() {
			t.Fatalf("%s must be optional and computed", name)
		}
	}
	for _, name := range []string{"state", "pending", "id"} {
		if !resp.Schema.Attributes[name].IsComputed() {
			t.Fatalf("%s must be computed", name)
		}
	}
}

func TestValidateSDNVnetConfig(t *testing.T) {
	valid := sdnVnetModel{
		Vnet:         types.StringValue("vn1"),
		Zone:         types.StringValue("vz1"),
		Alias:        types.StringValue("app network"),
		Tag:          types.Int64Value(100),
		VLANAware:    types.BoolValue(true),
		IsolatePorts: types.BoolValue(false),
	}
	if diags := validateSDNVnetConfig(valid); diags.HasError() {
		t.Fatalf("valid config diagnostics: %v", diags)
	}
	if diags := validateSDNVnetConfig(sdnVnetModel{Vnet: types.StringUnknown(), Zone: types.StringUnknown(), Tag: types.Int64Unknown()}); diags.HasError() {
		t.Fatalf("unknown values must defer: %v", diags)
	}

	invalid := map[string]func(*sdnVnetModel){
		"short vnet":     func(m *sdnVnetModel) { m.Vnet = types.StringValue("v") },
		"long vnet":      func(m *sdnVnetModel) { m.Vnet = types.StringValue("vn1234567") },
		"hyphen in vnet": func(m *sdnVnetModel) { m.Vnet = types.StringValue("vn-1") },
		"bad zone":       func(m *sdnVnetModel) { m.Zone = types.StringValue("vz_1") },
		"tag zero":       func(m *sdnVnetModel) { m.Tag = types.Int64Value(0) },
		"tag too large":  func(m *sdnVnetModel) { m.Tag = types.Int64Value(16777216) },
		"alias too long": func(m *sdnVnetModel) { m.Alias = types.StringValue(strings.Repeat("a", 257)) },
	}
	for name, mutate := range invalid {
		config := valid
		mutate(&config)
		if diags := validateSDNVnetConfig(config); !diags.HasError() {
			t.Fatalf("%s: expected invalid diagnostics: %#v", name, config)
		}
	}
}

func TestSDNVnetRequestUsesExplicitConfiguration(t *testing.T) {
	config := sdnVnetModel{Vnet: types.StringValue("vn1"), Zone: types.StringValue("vz1"), Alias: types.StringValue("alias")}
	plan := config
	plan.Tag = types.Int64Value(100)
	plan.State = types.StringValue("new")

	request := sdnVnetRequestFromModels(config, plan)
	if request.Zone != "vz1" || request.Alias == nil || *request.Alias != "alias" {
		t.Fatalf("request omitted explicit configuration: %#v", request)
	}
	if request.Tag != nil || request.VLANAware != nil || request.IsolatePorts != nil {
		t.Fatalf("request sent computed-only values: %#v", request)
	}
}

func TestSDNVnetManagedFieldDeletion(t *testing.T) {
	config := sdnVnetModel{Vnet: types.StringValue("vn1"), Zone: types.StringValue("vz1"), Alias: types.StringValue("a")}
	got := sdnVnetDeleteKeys(config, []string{"alias", "isolate-ports", "tag", "vlanaware"})
	if want := "isolate-ports,tag,vlanaware"; strings.Join(got, ",") != want {
		t.Fatalf("unexpected delete keys: got %v want %s", got, want)
	}
	if got := sdnVnetDeleteKeys(config, nil); len(got) != 0 {
		t.Fatalf("imported unmanaged fields must not be deleted: %v", got)
	}
}

func TestSDNVnetReadStateOverlaysStagedValues(t *testing.T) {
	state, diags := sdnVnetStateFromAPI(context.Background(), SDNVnet{
		Vnet:      "vn1",
		Zone:      "vz1",
		Alias:     "old",
		Tag:       proxmoxOptionalInt64{value: intPtr64(100)},
		VLANAware: proxmoxOptionalBool{value: boolPtr(false)},
		State:     "changed",
		Digest:    "d",
		Pending: map[string]json.RawMessage{
			"zone":      json.RawMessage(`"vz2"`),
			"alias":     json.RawMessage(`"deleted"`),
			"vlanaware": json.RawMessage(`"1"`),
		},
	})
	if diags.HasError() {
		t.Fatalf("state mapping diagnostics: %v", diags)
	}
	if state.Zone.ValueString() != "vz2" || !state.Alias.IsNull() || !state.VLANAware.ValueBool() {
		t.Fatalf("staged values were not applied: %#v", state)
	}
	if state.Tag.ValueInt64() != 100 || state.State.ValueString() != "changed" {
		t.Fatalf("running values were not preserved: %#v", state)
	}
}

func TestSDNVnetReadStateSkipsDeletedAndPreservesErrors(t *testing.T) {
	t.Run("deleted", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"data":[{"vnet":"vn1","zone":"vz1","state":"deleted"}]}`))
		}))
		defer server.Close()
		state, diags := (&SDNVnetResource{client: newHAResourceTestClient(t, server.URL)}).readState(context.Background(), "vn1")
		if diags.HasError() || !state.ID.IsNull() {
			t.Fatalf("deleted vnet must read as absent: %#v (%v)", state, diags)
		}
	})

	t.Run("api error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeAPIError(t, w, http.StatusForbidden, "missing SDN.Allocate")
		}))
		defer server.Close()
		_, diags := (&SDNVnetResource{client: newHAResourceTestClient(t, server.URL)}).readState(context.Background(), "vn1")
		if !diags.HasError() || !strings.Contains(diags[0].Detail(), "missing SDN.Allocate") {
			t.Fatalf("API error was not preserved: %v", diags)
		}
	})
}

func TestSDNVnetImportValidation(t *testing.T) {
	for _, id := range []string{"v", "vn1234567", "vn-1"} {
		var resp resource.ImportStateResponse
		(&SDNVnetResource{}).ImportState(context.Background(), resource.ImportStateRequest{ID: id}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected import diagnostic for %q", id)
		}
	}
	ctx := context.Background()
	schemaResp := testResourceSchema(t, &SDNVnetResource{})
	var resp resource.ImportStateResponse
	resp.State = tfsdk.State{Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil), Schema: schemaResp.Schema}
	(&SDNVnetResource{}).ImportState(ctx, resource.ImportStateRequest{ID: "vn1"}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}
	var state sdnVnetModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() || state.Vnet.ValueString() != "vn1" {
		t.Fatalf("unexpected imported state: %#v (%v)", state, diags)
	}
}

func TestSDNVnetLifecycle(t *testing.T) {
	vnets := map[string]map[string]any{}
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
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/vnets":
			if !handler.form(w, r, url.Values{
				"lock-token": {testSDNLockToken},
				"tag":        {"100"},
				"type":       {"vnet"},
				"vnet":       {"vn1"},
				"zone":       {"vz1"},
			}) {
				return
			}
			vnets["vn1"] = map[string]any{"vnet": "vn1", "zone": "vz1", "tag": 100, "state": "new"}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/vnets":
			list := make([]map[string]any, 0, len(vnets))
			for _, entry := range vnets {
				list = append(list, entry)
			}
			handler.envelope(w, list)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/sdn/vnets/vn1":
			if !handler.form(w, r, url.Values{
				"delete":     {"tag"},
				"digest":     {"digest-1"},
				"lock-token": {testSDNLockToken},
				"type":       {"vnet"},
				"vnet":       {"vn1"},
				"zone":       {"vz2"},
			}) {
				return
			}
			vnets["vn1"] = map[string]any{"vnet": "vn1", "zone": "vz2", "state": "changed"}
			handler.envelope(w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/vnets/vn1":
			vnets["vn1"] = map[string]any{"vnet": "vn1", "zone": "vz2", "state": "deleted"}
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected SDN vnet request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &SDNVnetResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := sdnVnetModel{
		Vnet:    types.StringValue("vn1"),
		Zone:    types.StringValue("vz1"),
		Tag:     types.Int64Value(100),
		Pending: types.MapNull(types.StringType),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created sdnVnetModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() {
		t.Fatalf("decode created state: %v", diags)
	}
	if created.State.ValueString() != "new" || created.Zone.ValueString() != "vz1" || created.Tag.ValueInt64() != 100 {
		t.Fatalf("unexpected created state: %#v", created)
	}

	updated := config
	updated.Zone = types.StringValue("vz2")
	updated.Tag = types.Int64Null()
	vnets["vn1"]["digest"] = "digest-1"
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
	var refreshed sdnVnetModel
	if diags := updateResp.State.Get(ctx, &refreshed); diags.HasError() {
		t.Fatalf("decode updated state: %v", diags)
	}
	if refreshed.Zone.ValueString() != "vz2" || !refreshed.Tag.IsNull() {
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
		t.Fatalf("deleted vnet was not removed: diagnostics=%v raw=%v", readResp.Diagnostics, readResp.State.Raw)
	}

	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}
