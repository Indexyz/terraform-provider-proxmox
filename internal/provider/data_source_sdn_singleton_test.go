// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"testing"
)

func TestSDNZoneDataSourceReadsStagedZone(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/zones?pending=1": []map[string]any{
			{"zone": "other", "type": "simple"},
			{"zone": "vz1", "type": "vlan", "bridge": "vmbr0", "mtu": 1500, "nodes": "node1,node2", "dnszone": "example.com", "ipam": "pve", "reversedns": "1.1.1.1", "state": "changed", "pending": map[string]any{"mtu": "9000", "bridge": "vmbr1"}},
		},
	}
	readResp := runSDNDataSource(t, NewSDNZoneDataSource(), responses, map[string]any{"zone": "vz1"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state sdnZoneDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.ID.ValueString() != "vz1" || state.Type.ValueString() != "vlan" || state.Bridge.ValueString() != "vmbr1" || state.MTU.ValueInt64() != 9000 {
		t.Fatalf("staged overlay failed: %#v", state)
	}
	if state.DNSZone.ValueString() != "example.com" || state.State.ValueString() != "changed" || len(state.Pending.Elements()) != 2 {
		t.Fatalf("unexpected projection: %#v", state)
	}
	var nodes []string
	if diags := state.Nodes.ElementsAs(context.Background(), &nodes, false); diags.HasError() || fmt.Sprint(nodes) != fmt.Sprint([]string{"node1", "node2"}) {
		t.Fatalf("unexpected nodes: %#v (%v)", nodes, diags)
	}
}

func TestSDNVnetDataSourceReadsStagedVnet(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/vnets?pending=1": []map[string]any{
			{"vnet": "vn1", "zone": "vz1", "alias": "app", "tag": 100, "vlanaware": 0, "state": "new", "pending": map[string]any{"tag": "200", "vlanaware": "1", "isolate-ports": "1"}},
		},
	}
	readResp := runSDNDataSource(t, NewSDNVnetDataSource(), responses, map[string]any{"vnet": "vn1"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state sdnVnetDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.Zone.ValueString() != "vz1" || state.Tag.ValueInt64() != 200 || !state.VLANAware.ValueBool() || !state.IsolatePorts.ValueBool() {
		t.Fatalf("staged overlay failed: %#v", state)
	}
	if state.Alias.ValueString() != "app" || len(state.Pending.Elements()) != 3 {
		t.Fatalf("unexpected projection: %#v", state)
	}
}

func TestSDNSubnetDataSourceReadsStagedSubnet(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/vnets/vn1/subnets?pending=1": []map[string]any{
			{"subnet": "10.0.0.0/24", "cidr": "10.0.0.0/24", "vnet": "vn1", "gateway": "10.0.0.1", "snat": 0, "state": "changed", "pending": map[string]any{"gateway": "10.0.0.254", "snat": "1"}},
		},
	}
	readResp := runSDNDataSource(t, NewSDNSubnetDataSource(), responses, map[string]any{"vnet": "vn1", "subnet": "10.0.0.0/24"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state sdnSubnetDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.ID.ValueString() != "vn1/10.0.0.0/24" || state.Gateway.ValueString() != "10.0.0.254" || !state.SNAT.ValueBool() {
		t.Fatalf("staged overlay failed: %#v", state)
	}
	if len(state.Pending.Elements()) != 2 {
		t.Fatalf("unexpected projection: %#v", state)
	}
}

func TestSDNSingletonDataSourcesMissing(t *testing.T) {
	for name, tc := range map[string]struct {
		path   string
		config map[string]any
		read   func(map[string]any, map[string]any) bool
	}{
		"zone": {"/api2/json/cluster/sdn/zones?pending=1", map[string]any{"zone": "gone"}, func(responses, config map[string]any) bool {
			return runSDNDataSource(t, NewSDNZoneDataSource(), responses, config).Diagnostics.HasError()
		}},
		"vnet": {"/api2/json/cluster/sdn/vnets?pending=1", map[string]any{"vnet": "gone"}, func(responses, config map[string]any) bool {
			return runSDNDataSource(t, NewSDNVnetDataSource(), responses, config).Diagnostics.HasError()
		}},
		"subnet": {"/api2/json/cluster/sdn/vnets/vn1/subnets?pending=1", map[string]any{"vnet": "vn1", "subnet": "10.0.0.0/24"}, func(responses, config map[string]any) bool {
			return runSDNDataSource(t, NewSDNSubnetDataSource(), responses, config).Diagnostics.HasError()
		}},
		"ipam": {"/api2/json/cluster/sdn/ipams?pending=1", map[string]any{"ipam": "gone"}, func(responses, config map[string]any) bool {
			return runSDNDataSource(t, NewSDNIPAMDataSource(), responses, config).Diagnostics.HasError()
		}},
		"dns": {"/api2/json/cluster/sdn/dns?pending=1", map[string]any{"dns": "gone"}, func(responses, config map[string]any) bool {
			return runSDNDataSource(t, NewSDNDNSDataSource(), responses, config).Diagnostics.HasError()
		}},
		"controller": {"/api2/json/cluster/sdn/controllers?pending=1", map[string]any{"controller": "gone"}, func(responses, config map[string]any) bool {
			return runSDNDataSource(t, NewSDNControllerDataSource(), responses, config).Diagnostics.HasError()
		}},
		"fabric": {"/api2/json/cluster/sdn/fabrics/fabric?pending=1", map[string]any{"fabric_id": "gone"}, func(responses, config map[string]any) bool {
			return runSDNDataSource(t, NewSDNFabricDataSource(), responses, config).Diagnostics.HasError()
		}},
		"fabric_node": {"/api2/json/cluster/sdn/fabrics/node/f1?pending=1", map[string]any{"fabric_id": "f1", "node_id": "gone"}, func(responses, config map[string]any) bool {
			return runSDNDataSource(t, NewSDNFabricNodeDataSource(), responses, config).Diagnostics.HasError()
		}},
		"prefix_list": {"/api2/json/cluster/sdn/prefix-lists?pending=1", map[string]any{"prefix_list_id": "gone"}, func(responses, config map[string]any) bool {
			return runSDNDataSource(t, NewSDNPrefixListDataSource(), responses, config).Diagnostics.HasError()
		}},
		"route_map_entry": {"/api2/json/cluster/sdn/route-maps/entries?pending=1", map[string]any{"route_map_id": "rm1", "order": int64(10)}, func(responses, config map[string]any) bool {
			return runSDNDataSource(t, NewSDNRouteMapEntryDataSource(), responses, config).Diagnostics.HasError()
		}},
	} {
		t.Run(name, func(t *testing.T) {
			if !tc.read(map[string]any{tc.path: []any{}}, tc.config) {
				t.Fatal("missing entry must surface an error")
			}
		})
	}
}

func TestSDNIPAMDataSourceReadsStagedIPAM(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/ipams?pending=1": []map[string]any{
			{"ipam": "ip1", "type": "netbox", "url": "https://nb", "section": 1, "fingerprint": "aa", "state": "changed", "pending": map[string]any{"url": "https://nb2", "section": "2"}},
		},
	}
	readResp := runSDNDataSource(t, NewSDNIPAMDataSource(), responses, map[string]any{"ipam": "ip1"})
	var state sdnIPAMDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.URL.ValueString() != "https://nb2" || state.Section.ValueInt64() != 2 || state.Pending.Elements()["url"].(interface{}) == nil {
		t.Fatalf("staged overlay failed: %#v", state)
	}
}

func TestSDNDNSDataSourceReadsStagedProvider(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/dns?pending=1": []map[string]any{
			{"dns": "pdns", "type": "powerdns", "url": "https://pd", "ttl": 300, "reversemaskv6": 16, "fingerprint": "bb", "pending": map[string]any{"ttl": "600"}},
		},
	}
	readResp := runSDNDataSource(t, NewSDNDNSDataSource(), responses, map[string]any{"dns": "pdns"})
	var state sdnDNSDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.TTL.ValueInt64() != 600 || state.ReverseMaskV6.ValueInt64() != 16 || state.Type.ValueString() != "powerdns" {
		t.Fatalf("staged overlay failed: %#v", state)
	}
}

func TestSDNControllerDataSourceReadsStagedController(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/controllers?pending=1": []map[string]any{
			{"controller": "evpn1", "type": "evpn", "asn": 65000, "node": "node1", "nodes": "node1,node2", "fabric": "f1", "loopback": "10.0.0.1", "route-map-in": "rm1", "state": "changed", "pending": map[string]any{"asn": "65001"}},
		},
	}
	readResp := runSDNDataSource(t, NewSDNControllerDataSource(), responses, map[string]any{"controller": "evpn1"})
	var state sdnControllerDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.ASN.ValueInt64() != 65001 || state.Fabric.ValueString() != "f1" || state.RouteMapIn.ValueString() != "rm1" {
		t.Fatalf("staged overlay failed: %#v", state)
	}
	var nodes []string
	if diags := state.Nodes.ElementsAs(context.Background(), &nodes, false); diags.HasError() || len(nodes) != 2 {
		t.Fatalf("unexpected nodes: %#v (%v)", nodes, diags)
	}
}

func TestSDNFabricDataSourceReadsStagedFabric(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/fabrics/fabric?pending=1": []map[string]any{
			{"id": "f1", "protocol": "ospf", "ip_prefix": "10.0.0.0/16", "route_map_in": "rm1", "redistribute": []string{"connected"}, "state": "new", "pending": map[string]any{"ip_prefix": "10.9.0.0/16"}},
		},
	}
	readResp := runSDNDataSource(t, NewSDNFabricDataSource(), responses, map[string]any{"fabric_id": "f1"})
	var state sdnFabricDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.IPPrefix.ValueString() != "10.9.0.0/16" || state.RouteMapIn.ValueString() != "rm1" {
		t.Fatalf("staged overlay failed: %#v", state)
	}
	if got := sdnListValues(state.Redistribute); fmt.Sprint(got) != fmt.Sprint([]string{"connected"}) {
		t.Fatalf("unexpected redistribute: %v", got)
	}
}

func TestSDNFabricNodeDataSourceReadsStagedNode(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/fabrics/node/f1?pending=1": []map[string]any{
			{"fabric_id": "f1", "node_id": "n1", "protocol": "ospf", "ip": "10.0.0.1", "interfaces": []string{"eth1"}, "role": "spine", "endpoint": "10.0.0.1:51820", "allowed_ips": []string{"10.0.0.0/24"}, "peers": []string{"n2"}, "public_key": "pk", "state": "changed", "pending": map[string]any{"role": "leaf"}},
		},
	}
	readResp := runSDNDataSource(t, NewSDNFabricNodeDataSource(), responses, map[string]any{"fabric_id": "f1", "node_id": "n1"})
	var state sdnFabricNodeDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.Role.ValueString() != "leaf" || state.Endpoint.ValueString() != "10.0.0.1:51820" || state.PublicKey.ValueString() != "pk" {
		t.Fatalf("staged overlay failed: %#v", state)
	}
	if got := sdnListValues(state.AllowedIPs); fmt.Sprint(got) != fmt.Sprint([]string{"10.0.0.0/24"}) {
		t.Fatalf("unexpected allowed ips: %v", got)
	}
}

func TestSDNPrefixListDataSourceReadsStagedEntries(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/prefix-lists?pending=1": []map[string]any{
			{"id": "pl1", "entries": []string{"action=permit,prefix=10.0.0.0/24"}, "state": "changed", "pending": map[string]any{"entries": "action=deny,prefix=10.0.0.0/24"}},
		},
	}
	readResp := runSDNDataSource(t, NewSDNPrefixListDataSource(), responses, map[string]any{"prefix_list_id": "pl1"})
	var state sdnPrefixListDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if got := sdnListValues(state.Entries); fmt.Sprint(got) != fmt.Sprint([]string{"action=deny,prefix=10.0.0.0/24"}) {
		t.Fatalf("staged entries must win: %v", got)
	}
}

func TestSDNRouteMapEntryDataSourceReadsStagedEntry(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/sdn/route-maps/entries?pending=1": []map[string]any{
			{"route-map-id": "rm1", "order": 10, "action": "permit", "set": []string{"local-preference 200"}, "match": []string{"ip address prefix-list pl1"}, "exit-action": "on-match-next", "call": "rm2", "state": "new", "pending": map[string]any{"action": "deny"}},
		},
	}
	readResp := runSDNDataSource(t, NewSDNRouteMapEntryDataSource(), responses, map[string]any{"route_map_id": "rm1", "order": int64(10)})
	var state sdnRouteMapEntryDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.Action.ValueString() != "deny" || state.ExitAction.ValueString() != "on-match-next" || state.Call.ValueString() != "rm2" {
		t.Fatalf("staged overlay failed: %#v", state)
	}
	if got := sdnListValues(state.Set); fmt.Sprint(got) != fmt.Sprint([]string{"local-preference 200"}) {
		t.Fatalf("unexpected set: %v", got)
	}
}
