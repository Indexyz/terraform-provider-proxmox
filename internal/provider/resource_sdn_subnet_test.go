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

func testDHCPRanges(t *testing.T, ranges ...sdnDHCPRangeModel) types.List {
	t.Helper()
	value, diags := types.ListValueFrom(context.Background(), types.ObjectType{AttrTypes: sdnDHCPRangeAttrTypes()}, ranges)
	if diags.HasError() {
		t.Fatalf("create dhcp ranges: %v", diags)
	}
	return value
}

func TestSDNSubnetSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewSDNSubnetResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 11; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	for _, name := range []string{"vnet", "cidr"} {
		attribute, ok := resp.Schema.Attributes[name].(resourceschema.StringAttribute)
		if !ok || !attribute.IsRequired() || len(attribute.PlanModifiers) == 0 {
			t.Fatalf("%s must be required with a replacement plan modifier", name)
		}
	}
	dhcpRange, ok := resp.Schema.Attributes["dhcp_range"].(resourceschema.ListNestedAttribute)
	if !ok || !dhcpRange.IsOptional() || !dhcpRange.IsComputed() {
		t.Fatal("dhcp_range must be an optional and computed nested list")
	}
	for _, name := range []string{"id", "zone", "state", "pending"} {
		if !resp.Schema.Attributes[name].IsComputed() {
			t.Fatalf("%s must be computed", name)
		}
	}
}

func TestValidateSDNSubnetConfig(t *testing.T) {
	valid := sdnSubnetModel{
		Vnet:          types.StringValue("vn1"),
		CIDR:          types.StringValue("10.1.0.0/24"),
		Gateway:       types.StringValue("10.1.0.1"),
		SNAT:          types.BoolValue(true),
		DNSZonePrefix: types.StringValue("adm"),
		DHCPDNSServer: types.StringValue("10.1.0.2"),
		DHCPRanges: testDHCPRanges(t,
			sdnDHCPRangeModel{StartAddress: types.StringValue("10.1.0.10"), EndAddress: types.StringValue("10.1.0.20")},
		),
	}
	if diags := validateSDNSubnetConfig(valid); diags.HasError() {
		t.Fatalf("valid config diagnostics: %v", diags)
	}
	if diags := validateSDNSubnetConfig(sdnSubnetModel{Vnet: types.StringUnknown(), CIDR: types.StringUnknown(), DHCPRanges: types.ListUnknown(types.ObjectType{AttrTypes: sdnDHCPRangeAttrTypes()})}); diags.HasError() {
		t.Fatalf("unknown values must defer: %v", diags)
	}

	invalid := map[string]func(*sdnSubnetModel){
		"bad vnet":       func(m *sdnSubnetModel) { m.Vnet = types.StringValue("vn-1") },
		"bad cidr":       func(m *sdnSubnetModel) { m.CIDR = types.StringValue("10.1.0.0") },
		"bad gateway":    func(m *sdnSubnetModel) { m.Gateway = types.StringValue("gateway") },
		"bad dns server": func(m *sdnSubnetModel) { m.DHCPDNSServer = types.StringValue("dns") },
		"bad dns prefix": func(m *sdnSubnetModel) { m.DNSZonePrefix = types.StringValue("a.b") },
		"bad range start": func(m *sdnSubnetModel) {
			m.DHCPRanges = testDHCPRanges(t, sdnDHCPRangeModel{StartAddress: types.StringValue("x"), EndAddress: types.StringValue("10.1.0.20")})
		},
	}
	for name, mutate := range invalid {
		config := valid
		mutate(&config)
		if diags := validateSDNSubnetConfig(config); !diags.HasError() {
			t.Fatalf("%s: expected invalid diagnostics: %#v", name, config)
		}
	}
}

func TestSDNSubnetRequestEncodesOnlyExplicitConfiguration(t *testing.T) {
	config := sdnSubnetModel{Vnet: types.StringValue("vn1"), CIDR: types.StringValue("10.1.0.0/24")}
	plan := config
	plan.Gateway = types.StringValue("10.1.0.1")
	plan.SNAT = types.BoolValue(true)
	plan.DHCPRanges = testDHCPRanges(t,
		sdnDHCPRangeModel{StartAddress: types.StringValue("10.1.0.10"), EndAddress: types.StringValue("10.1.0.20")},
	)

	request := sdnSubnetRequestFromModels(config, plan)
	if request.Gateway != nil || request.SNAT != nil || request.DHCPRanges != nil {
		t.Fatalf("request sent computed-only values: %#v", request)
	}

	config.Gateway = plan.Gateway
	config.SNAT = plan.SNAT
	config.DHCPRanges = plan.DHCPRanges
	request = sdnSubnetRequestFromModels(config, plan)
	if request.Gateway == nil || request.SNAT == nil || len(request.DHCPRanges) != 1 {
		t.Fatalf("request omitted explicit configuration: %#v", request)
	}
	form := sdnSubnetForm(request)
	if got := form["dhcp-range"]; len(got) != 1 || got[0] != "start-address=10.1.0.10,end-address=10.1.0.20" {
		t.Fatalf("dhcp range was not encoded: %#v", got)
	}
}

func TestSDNSubnetManagedFieldDeletion(t *testing.T) {
	config := sdnSubnetModel{Vnet: types.StringValue("vn1"), CIDR: types.StringValue("10.1.0.0/24"), Gateway: types.StringValue("10.1.0.1")}
	got := sdnSubnetDeleteKeys(config, []string{"dhcp-range", "dhcp-dns-server", "gateway", "snat"})
	if want := "dhcp-dns-server,dhcp-range,snat"; strings.Join(got, ",") != want {
		t.Fatalf("unexpected delete keys: got %v want %s", got, want)
	}
	if got := sdnSubnetDeleteKeys(config, nil); len(got) != 0 {
		t.Fatalf("imported unmanaged fields must not be deleted: %v", got)
	}
}

func TestSDNSubnetStateOverlaysStagedRanges(t *testing.T) {
	state, diags := sdnSubnetStateFromAPI(context.Background(), SDNSubnet{
		Subnet:     "vz1-10.1.0.0-24",
		CIDR:       "10.1.0.0/24",
		Vnet:       "vn1",
		Zone:       "vz1",
		Gateway:    "10.1.0.1",
		SNAT:       proxmoxOptionalBool{value: boolPtr(false)},
		State:      "changed",
		DHCPRanges: []SDNDHCPRange{{StartAddress: "10.1.0.10", EndAddress: "10.1.0.20"}},
		Pending: map[string]json.RawMessage{
			"gateway":    json.RawMessage(`"10.1.0.254"`),
			"snat":       json.RawMessage(`"1"`),
			"dhcp-range": json.RawMessage(`[{"start-address":"10.1.0.30","end-address":"10.1.0.40"}]`),
		},
	})
	if diags.HasError() {
		t.Fatalf("state mapping diagnostics: %v", diags)
	}
	if state.Gateway.ValueString() != "10.1.0.254" || !state.SNAT.ValueBool() {
		t.Fatalf("staged scalars were not applied: %#v", state)
	}
	var ranges []sdnDHCPRangeModel
	if diags := state.DHCPRanges.ElementsAs(context.Background(), &ranges, false); diags.HasError() || len(ranges) != 1 || ranges[0].StartAddress.ValueString() != "10.1.0.30" {
		t.Fatalf("staged ranges were not applied: %#v (%v)", ranges, diags)
	}
	if state.ID.ValueString() != "vz1-10.1.0.0-24" || state.CIDR.ValueString() != "10.1.0.0/24" {
		t.Fatalf("identity was not mapped: %#v", state)
	}

	running, diags := sdnSubnetStateFromAPI(context.Background(), SDNSubnet{Subnet: "vz1-10.1.0.0-24", CIDR: "10.1.0.0/24", Vnet: "vn1", State: "new"})
	if diags.HasError() {
		t.Fatalf("running-only diagnostics: %v", diags)
	}
	if !running.DHCPRanges.IsNull() || !running.Gateway.IsNull() || !running.Pending.IsNull() {
		t.Fatalf("absent values must stay null: %#v", running)
	}

	deleted, diags := sdnSubnetStateFromAPI(context.Background(), SDNSubnet{
		Subnet: "vz1-10.1.0.0-24", CIDR: "10.1.0.0/24", Vnet: "vn1",
		DHCPRanges: []SDNDHCPRange{{StartAddress: "10.1.0.10", EndAddress: "10.1.0.20"}},
		Pending:    map[string]json.RawMessage{"dhcp-range": json.RawMessage(`"deleted"`)},
	})
	if diags.HasError() || !deleted.DHCPRanges.IsNull() {
		t.Fatalf("staged range removal must clear the list: %#v (%v)", deleted.DHCPRanges, diags)
	}
}

func TestSDNSubnetReadStateSkipsDeletedAndPreservesErrors(t *testing.T) {
	t.Run("deleted", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"data":[{"subnet":"vz1-10.1.0.0-24","cidr":"10.1.0.0/24","vnet":"vn1","state":"deleted"}]}`))
		}))
		defer server.Close()
		state, diags := (&SDNSubnetResource{client: newHAResourceTestClient(t, server.URL)}).readState(context.Background(), "vn1", "10.1.0.0/24")
		if diags.HasError() || !state.ID.IsNull() {
			t.Fatalf("deleted subnet must read as absent: %#v (%v)", state, diags)
		}
	})

	t.Run("api error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeAPIError(t, w, http.StatusForbidden, "missing SDN.Allocate")
		}))
		defer server.Close()
		_, diags := (&SDNSubnetResource{client: newHAResourceTestClient(t, server.URL)}).readState(context.Background(), "vn1", "10.1.0.0/24")
		if !diags.HasError() || !strings.Contains(diags[0].Detail(), "missing SDN.Allocate") {
			t.Fatalf("API error was not preserved: %v", diags)
		}
	})
}

func TestSDNSubnetImportValidation(t *testing.T) {
	for _, id := range []string{"vn1", "vn-1/10.1.0.0/24", "vn1/10.1.0.0"} {
		var resp resource.ImportStateResponse
		(&SDNSubnetResource{}).ImportState(context.Background(), resource.ImportStateRequest{ID: id}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected import diagnostic for %q", id)
		}
	}
	ctx := context.Background()
	schemaResp := testResourceSchema(t, &SDNSubnetResource{})
	var resp resource.ImportStateResponse
	resp.State = tfsdk.State{Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil), Schema: schemaResp.Schema}
	(&SDNSubnetResource{}).ImportState(ctx, resource.ImportStateRequest{ID: "vn1/10.1.0.0/24"}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}
	var state sdnSubnetModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() || state.Vnet.ValueString() != "vn1" || state.CIDR.ValueString() != "10.1.0.0/24" {
		t.Fatalf("unexpected imported state: %#v (%v)", state, diags)
	}
}

func TestSDNSubnetLifecycle(t *testing.T) {
	subnet := map[string]any{}
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
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/vnets/vn1/subnets":
			if !handler.form(w, r, url.Values{
				"gateway":    {"10.1.0.1"},
				"lock-token": {testSDNLockToken},
				"subnet":     {"10.1.0.0/24"},
				"type":       {"subnet"},
				"vnet":       {"vn1"},
			}) {
				return
			}
			subnet = map[string]any{"subnet": "vz1-10.1.0.0-24", "cidr": "10.1.0.0/24", "vnet": "vn1", "zone": "vz1", "gateway": "10.1.0.1", "state": "new", "digest": "digest-1"}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/vnets/vn1/subnets":
			if len(subnet) == 0 {
				handler.envelope(w, []any{})
				return
			}
			handler.envelope(w, []any{subnet})
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/sdn/vnets/vn1/subnets/vz1-10.1.0.0-24":
			if !handler.form(w, r, url.Values{
				"delete":     {"gateway"},
				"digest":     {"digest-1"},
				"lock-token": {testSDNLockToken},
				"snat":       {"1"},
				"subnet":     {"vz1-10.1.0.0-24"},
				"type":       {"subnet"},
				"vnet":       {"vn1"},
			}) {
				return
			}
			subnet = map[string]any{"subnet": "vz1-10.1.0.0-24", "cidr": "10.1.0.0/24", "vnet": "vn1", "zone": "vz1", "snat": 1, "state": "changed"}
			handler.envelope(w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/vnets/vn1/subnets/vz1-10.1.0.0-24":
			subnet = map[string]any{"subnet": "vz1-10.1.0.0-24", "cidr": "10.1.0.0/24", "vnet": "vn1", "state": "deleted"}
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected SDN subnet request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &SDNSubnetResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := sdnSubnetModel{
		Vnet:       types.StringValue("vn1"),
		CIDR:       types.StringValue("10.1.0.0/24"),
		Gateway:    types.StringValue("10.1.0.1"),
		DHCPRanges: types.ListNull(types.ObjectType{AttrTypes: sdnDHCPRangeAttrTypes()}),
		Pending:    types.MapNull(types.StringType),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created sdnSubnetModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() {
		t.Fatalf("decode created state: %v", diags)
	}
	if created.ID.ValueString() != "vz1-10.1.0.0-24" || created.Zone.ValueString() != "vz1" {
		t.Fatalf("unexpected created state: %#v", created)
	}

	updated := config
	updated.Gateway = types.StringNull()
	updated.SNAT = types.BoolValue(true)
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
	var refreshed sdnSubnetModel
	if diags := updateResp.State.Get(ctx, &refreshed); diags.HasError() {
		t.Fatalf("decode updated state: %v", diags)
	}
	if !refreshed.Gateway.IsNull() || !refreshed.SNAT.ValueBool() {
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
		t.Fatalf("deleted subnet was not removed: diagnostics=%v raw=%v", readResp.Diagnostics, readResp.State.Raw)
	}

	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}
