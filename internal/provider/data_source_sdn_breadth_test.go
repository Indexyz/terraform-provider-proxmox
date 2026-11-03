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
)

func TestSDNIPAMsDataSourceProjectsAndFilters(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/ipams?pending=1": []map[string]any{
			{"ipam": "ip2", "type": "phpipam", "url": "https://p2", "section": 2, "state": "changed", "pending": map[string]any{"url": "https://staged"}},
			{"ipam": "ip1", "type": "netbox", "url": "https://p1", "section": 1, "state": "new"},
		},
	}
	readResp := runSDNDataSource(t, NewSDNIPAMsDataSource(), responses, map[string]any{})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state sdnIPAMsDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	got := make([]string, 0, len(state.IPAMs))
	for _, record := range state.IPAMs {
		got = append(got, fmt.Sprintf("%s|%s|%s|%d|%s|%d", record.IPAM.ValueString(), record.Type.ValueString(), record.URL.ValueString(), record.Section.ValueInt64(), record.State.ValueString(), len(record.Pending.Elements())))
	}
	want := []string{"ip1|netbox|https://p1|1|new|0", "ip2|phpipam|https://staged|2|changed|1"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected projection:\n got %v\nwant %v", got, want)
	}

	filtered := runSDNDataSource(t, NewSDNIPAMsDataSource(), responses, map[string]any{"type": "netbox"})
	var filteredState sdnIPAMsDataSourceModel
	if diags := filtered.State.Get(context.Background(), &filteredState); diags.HasError() || len(filteredState.IPAMs) != 1 || filteredState.IPAMs[0].IPAM.ValueString() != "ip1" {
		t.Fatalf("type filter failed: %#v (%v)", filteredState.IPAMs, diags)
	}
}

func TestSDNDNSsDataSourceProjectsAndFilters(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/dns?pending=1": []map[string]any{
			{"dns": "pdns", "type": "powerdns", "url": "https://pdns", "ttl": 300},
			{"dns": "cf", "type": "cloudflare", "ttl": 600, "pending": map[string]any{"ttl": "900"}},
		},
	}
	readResp := runSDNDataSource(t, NewSDNDNSsDataSource(), responses, map[string]any{})
	var state sdnDNSsDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if len(state.Servers) != 2 || state.Servers[0].DNS.ValueString() != "cf" || state.Servers[0].TTL.ValueInt64() != 900 || state.Servers[1].URL.ValueString() != "https://pdns" {
		t.Fatalf("unexpected projection: %#v", state.Servers)
	}

	filtered := runSDNDataSource(t, NewSDNDNSsDataSource(), responses, map[string]any{"type": "powerdns"})
	var filteredState sdnDNSsDataSourceModel
	if diags := filtered.State.Get(context.Background(), &filteredState); diags.HasError() || len(filteredState.Servers) != 1 || filteredState.Servers[0].DNS.ValueString() != "pdns" {
		t.Fatalf("type filter failed: %#v (%v)", filteredState.Servers, diags)
	}
}

func TestSDNControllersDataSourceProjectsAndFilters(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/controllers?pending=1": []map[string]any{
			{"controller": "c2", "type": "bgp", "asn": 65002, "node": "node2"},
			{"controller": "c1", "type": "evpn", "asn": 65001, "node": "node1", "pending": map[string]any{"asn": "65003"}},
		},
	}
	readResp := runSDNDataSource(t, NewSDNControllersDataSource(), responses, map[string]any{})
	var state sdnControllersDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if len(state.Controllers) != 2 || state.Controllers[0].Controller.ValueString() != "c1" || state.Controllers[0].ASN.ValueInt64() != 65003 || state.Controllers[1].Node.ValueString() != "node2" {
		t.Fatalf("unexpected projection: %#v", state.Controllers)
	}

	filtered := runSDNDataSource(t, NewSDNControllersDataSource(), responses, map[string]any{"type": "bgp"})
	var filteredState sdnControllersDataSourceModel
	if diags := filtered.State.Get(context.Background(), &filteredState); diags.HasError() || len(filteredState.Controllers) != 1 || filteredState.Controllers[0].Controller.ValueString() != "c2" {
		t.Fatalf("type filter failed: %#v (%v)", filteredState.Controllers, diags)
	}
}

func TestSDNFabricsDataSourceProjectsAndFilters(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/fabrics/fabric?pending=1": []map[string]any{
			{"id": "f2", "protocol": "ospf", "ip_prefix": "10.2.0.0/16"},
			{"id": "f1", "protocol": "openfabric", "ip_prefix": "10.1.0.0/16", "state": "new"},
		},
	}
	readResp := runSDNDataSource(t, NewSDNFabricsDataSource(), responses, map[string]any{})
	var state sdnFabricsDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if len(state.Fabrics) != 2 || state.Fabrics[0].FabricID.ValueString() != "f1" || state.Fabrics[0].State.ValueString() != "new" || state.Fabrics[1].IPPrefix.ValueString() != "10.2.0.0/16" {
		t.Fatalf("unexpected projection: %#v", state.Fabrics)
	}

	filtered := runSDNDataSource(t, NewSDNFabricsDataSource(), responses, map[string]any{"protocol": "ospf"})
	var filteredState sdnFabricsDataSourceModel
	if diags := filtered.State.Get(context.Background(), &filteredState); diags.HasError() || len(filteredState.Fabrics) != 1 || filteredState.Fabrics[0].FabricID.ValueString() != "f2" {
		t.Fatalf("protocol filter failed: %#v (%v)", filteredState.Fabrics, diags)
	}
}

func TestSDNPrefixListsDataSourceProjectsEntries(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/prefix-lists?pending=1": []map[string]any{
			{"id": "pl2", "entries": []string{"action=deny,prefix=10.2.0.0/24"}},
			{"id": "pl1", "entries": []string{"action=permit,prefix=10.1.0.0/24"}, "pending": map[string]any{"entries": "action=permit,prefix=10.9.0.0/24"}},
		},
	}
	readResp := runSDNDataSource(t, NewSDNPrefixListsDataSource(), responses, map[string]any{})
	var state sdnPrefixListsDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if len(state.Lists) != 2 || state.Lists[0].PrefixListID.ValueString() != "pl1" {
		t.Fatalf("unexpected projection: %#v", state.Lists)
	}
	entries := sdnListValues(state.Lists[0].Entries)
	if fmt.Sprint(entries) != fmt.Sprint([]string{"action=permit,prefix=10.9.0.0/24"}) {
		t.Fatalf("staged entries must win: %v", entries)
	}
	if got := sdnListValues(state.Lists[1].Entries); fmt.Sprint(got) != fmt.Sprint([]string{"action=deny,prefix=10.2.0.0/24"}) {
		t.Fatalf("running entries must be kept: %v", got)
	}
}

func TestSDNRouteMapEntriesDataSourceFiltersByRouteMap(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/route-maps/entries?pending=1": []map[string]any{
			{"route-map-id": "rm1", "order": 20, "action": "deny"},
			{"route-map-id": "rm2", "order": 10, "action": "permit"},
			{"route-map-id": "rm1", "order": 10, "action": "permit"},
		},
	}
	readResp := runSDNDataSource(t, NewSDNRouteMapEntriesDataSource(), responses, map[string]any{"route_map_id": "rm1"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state sdnRouteMapEntriesDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	got := make([]string, 0, len(state.Entries))
	for _, record := range state.Entries {
		got = append(got, fmt.Sprintf("%s|%d|%s", record.RouteMapID.ValueString(), record.Order.ValueInt64(), record.Action.ValueString()))
	}
	want := []string{"rm1|10|permit", "rm1|20|deny"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected projection:\n got %v\nwant %v", got, want)
	}
}

func TestSDNSubnetsDataSourceProjectsSubnet(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/vnets/vnet1/subnets?pending=1": []map[string]any{
			{"subnet": "10.0.2.0/24", "gateway": "10.0.2.1"},
			{"subnet": "10.0.1.0/24", "state": "changed", "pending": map[string]any{"gateway": "10.0.1.254"}},
		},
	}
	readResp := runSDNDataSource(t, NewSDNSubnetsDataSource(), responses, map[string]any{"vnet": "vnet1"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state sdnSubnetsDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if len(state.Subnets) != 2 || state.Subnets[0].Subnet.ValueString() != "10.0.1.0/24" || state.Subnets[0].Gateway.ValueString() != "10.0.1.254" || state.Subnets[1].VNet.ValueString() != "vnet1" {
		t.Fatalf("unexpected projection: %#v", state.Subnets)
	}
}

func TestSDNBreadthDataSourcesEmptyAndError(t *testing.T) {
	cases := []struct {
		name       string
		dataSource datasource.DataSource
		path       string
		config     map[string]any
	}{
		{"ipams", NewSDNIPAMsDataSource(), "/api2/json/cluster/sdn/ipams?pending=1", map[string]any{}},
		{"dns", NewSDNDNSsDataSource(), "/api2/json/cluster/sdn/dns?pending=1", map[string]any{}},
		{"controllers", NewSDNControllersDataSource(), "/api2/json/cluster/sdn/controllers?pending=1", map[string]any{}},
		{"fabrics", NewSDNFabricsDataSource(), "/api2/json/cluster/sdn/fabrics/fabric?pending=1", map[string]any{}},
		{"prefix_lists", NewSDNPrefixListsDataSource(), "/api2/json/cluster/sdn/prefix-lists?pending=1", map[string]any{}},
		{"route_map_entries", NewSDNRouteMapEntriesDataSource(), "/api2/json/cluster/sdn/route-maps/entries?pending=1", map[string]any{"route_map_id": "rm1"}},
		{"subnets", NewSDNSubnetsDataSource(), "/api2/json/cluster/sdn/vnets/vnet1/subnets?pending=1", map[string]any{"vnet": "vnet1"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			empty := runSDNDataSource(t, testCase.dataSource, map[string]any{testCase.path: []any{}}, testCase.config)
			if empty.Diagnostics.HasError() {
				t.Fatalf("empty read failed: %v", empty.Diagnostics)
			}

			handler := &lifecycleHandler{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handler.auth(w, r)
				writeAPIError(t, w, http.StatusForbidden, "missing SDN.Audit")
			}))
			defer server.Close()
			var schemaResp datasource.SchemaResponse
			testCase.dataSource.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
			configurable, ok := testCase.dataSource.(datasource.DataSourceWithConfigure)
			if !ok {
				t.Fatalf("%T does not implement DataSourceWithConfigure", testCase.dataSource)
			}
			configurable.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: testLifecycleClient(t, server)}, &datasource.ConfigureResponse{})
			readResp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
			testCase.dataSource.Read(context.Background(), datasource.ReadRequest{Config: testDataSourceConfig(t, schemaResp, testCase.config)}, &readResp)
			if len(readResp.Diagnostics) == 0 {
				t.Fatal("expected API error diagnostics")
			}
		})
	}
}
