// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func runSDNDataSource(t *testing.T, ds datasource.DataSource, responses map[string]any, config map[string]any) *datasource.ReadResponse {
	t.Helper()
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		key := r.URL.EscapedPath()
		if r.URL.RawQuery != "" {
			key += "?" + r.URL.RawQuery
		}
		data, ok := responses[key]
		if !ok || (r.Method != http.MethodGet && r.Method != http.MethodPost) {
			handler.fail(w, "unexpected request: %s %s", r.Method, key)
			return
		}
		handler.envelope(w, data)
	}))
	t.Cleanup(server.Close)

	var schemaResp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
	configurable, ok := ds.(datasource.DataSourceWithConfigure)
	if !ok {
		t.Fatalf("%T does not implement DataSourceWithConfigure", ds)
	}
	var configureResp datasource.ConfigureResponse
	configurable.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: testLifecycleClient(t, server)}, &configureResp)
	if configureResp.Diagnostics.HasError() {
		t.Fatalf("configure diagnostics: %v", configureResp.Diagnostics)
	}
	readResp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	ds.Read(context.Background(), datasource.ReadRequest{Config: testDataSourceConfig(t, schemaResp, config)}, readResp)
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
	return readResp
}

func TestSDNZonesDataSourceReadsFiltersAndSorts(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/zones?pending=1": []map[string]any{
			{"zone": "vz2", "type": "vlan", "bridge": "vmbr0", "state": "changed", "pending": map[string]any{"bridge": "vmbr1"}},
			{"zone": "vz1", "type": "simple", "nodes": "node1", "state": "new"},
		},
	}

	readResp := runSDNDataSource(t, NewSDNZonesDataSource(), responses, map[string]any{})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state sdnZonesDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	got := make([]string, 0, len(state.Zones))
	for _, record := range state.Zones {
		got = append(got, fmt.Sprintf("%s|%s|%s|%s", record.Zone.ValueString(), record.Type.ValueString(), record.Bridge.ValueString(), record.State.ValueString()))
	}
	want := []string{"vz1|simple||new", "vz2|vlan|vmbr1|changed"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected projection:\n got %v\nwant %v", got, want)
	}

	filtered := runSDNDataSource(t, NewSDNZonesDataSource(), responses, map[string]any{"type": "vlan"})
	var filteredState sdnZonesDataSourceModel
	if diags := filtered.State.Get(context.Background(), &filteredState); diags.HasError() || len(filteredState.Zones) != 1 || filteredState.Zones[0].Zone.ValueString() != "vz2" {
		t.Fatalf("type filter failed: %#v (%v)", filteredState.Zones, diags)
	}
}

func TestSDNZonesDataSourceEmptyAndError(t *testing.T) {
	empty := runSDNDataSource(t, NewSDNZonesDataSource(), map[string]any{"/api2/json/cluster/sdn/zones?pending=1": []any{}}, map[string]any{})
	var state sdnZonesDataSourceModel
	if diags := empty.State.Get(context.Background(), &state); diags.HasError() || len(state.Zones) != 0 {
		t.Fatalf("empty result must stay empty: %#v (%v)", state.Zones, diags)
	}

	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !handler.auth(w, &http.Request{Header: http.Header{"Authorization": {"PVEAPIToken=terraform@pve!provider=token-secret"}}}) {
			return
		}
		writeAPIError(t, w, http.StatusForbidden, "missing SDN.Audit")
	}))
	defer server.Close()
	ds := NewSDNZonesDataSource()
	var schemaResp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
	configurable, ok := ds.(datasource.DataSourceWithConfigure)
	if !ok {
		t.Fatalf("%T does not implement DataSourceWithConfigure", ds)
	}
	configurable.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: testLifecycleClient(t, server)}, &datasource.ConfigureResponse{})
	readResp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	ds.Read(context.Background(), datasource.ReadRequest{Config: testDataSourceConfig(t, schemaResp, map[string]any{})}, &readResp)
	if len(readResp.Diagnostics) == 0 {
		t.Fatal("expected API error diagnostics")
	}
}

func TestSDNVnetsDataSourceFiltersByZone(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/vnets?pending=1": []map[string]any{
			{"vnet": "vn2", "zone": "vz2", "tag": 20, "state": "new"},
			{"vnet": "vn1", "zone": "vz1", "state": "changed", "pending": map[string]any{"zone": "vz2"}},
		},
	}

	readResp := runSDNDataSource(t, NewSDNVnetsDataSource(), responses, map[string]any{})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state sdnVnetsDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	got := make([]string, 0, len(state.Vnets))
	for _, record := range state.Vnets {
		got = append(got, fmt.Sprintf("%s|%s", record.Vnet.ValueString(), record.Zone.ValueString()))
	}
	// The staged zone wins, so both VNets belong to vz2 in the staged view.
	want := []string{"vn1|vz2", "vn2|vz2"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected projection:\n got %v\nwant %v", got, want)
	}

	filtered := runSDNDataSource(t, NewSDNVnetsDataSource(), responses, map[string]any{"zone": "vz1"})
	var filteredState sdnVnetsDataSourceModel
	if diags := filtered.State.Get(context.Background(), &filteredState); diags.HasError() || len(filteredState.Vnets) != 0 {
		t.Fatalf("vz1 has no staged VNets left: %#v (%v)", filteredState.Vnets, diags)
	}

	if !state.Vnets[0].Tag.IsNull() || state.Vnets[1].Tag.ValueInt64() != 20 {
		t.Fatalf("unexpected tags: %#v", state.Vnets)
	}
	_ = types.StringNull()
}
