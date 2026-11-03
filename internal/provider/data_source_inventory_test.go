// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"
)

func TestStorageContentsDataSourceProjects(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/storage/local/content?content=iso": []map[string]any{
			{"volid": "local:iso/b.iso", "format": "iso", "size": 10, "ctime": 20, "protected": 1},
			{"volid": "local:iso/a.iso", "format": "iso", "size": 5, "notes": "seed"},
		},
	}
	readResp := runSDNDataSource(t, NewStorageContentsDataSource(), responses, map[string]any{"node": "pve-1", "storage": "local", "content": "iso"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state storageContentsModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if len(state.Volumes) != 2 || state.Volumes[0].VolID.ValueString() != "local:iso/a.iso" || state.Volumes[0].Content.ValueString() != "iso" {
		t.Fatalf("unexpected projection: %#v", state.Volumes)
	}
	if state.ID.ValueString() != "pve-1/local/iso" || state.Volumes[1].Size.ValueInt64() != 10 || !state.Volumes[1].Protected.ValueBool() {
		t.Fatalf("unexpected metadata: %#v", state)
	}
}

func TestGuestSnapshotsDataSources(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/qemu/101/snapshot": []map[string]any{
			{"name": "s2", "description": "second", "snaptime": 20, "vmstate": 1},
			{"name": "s1", "snaptime": 10},
			{"name": "current"},
		},
	}
	readResp := runSDNDataSource(t, NewQemuSnapshotsDataSource(), responses, map[string]any{"node": "pve-1", "vmid": int64(101)})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state guestSnapshotsModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if len(state.Snapshots) != 3 || state.Snapshots[0].Name.ValueString() != "current" || state.Snapshots[2].Name.ValueString() != "s2" || !state.Snapshots[2].VMState.ValueBool() {
		t.Fatalf("unexpected projection: %#v", state.Snapshots)
	}

	lxc := map[string]any{"/api2/json/nodes/pve-1/lxc/102/snapshot": []map[string]any{{"name": "s1"}}}
	lxcResp := runSDNDataSource(t, NewLXCSnapshotsDataSource(), lxc, map[string]any{"node": "pve-1", "vmid": int64(102)})
	if lxcResp.Diagnostics.HasError() {
		t.Fatalf("lxc read diagnostics: %v", lxcResp.Diagnostics)
	}
}

func TestUserInventoryDataSources(t *testing.T) {
	tokens := map[string]any{
		"/api2/json/access/users/root@pam/token": []map[string]any{
			{"tokenid": "b", "comment": "second", "expire": 0, "privsep": 0},
			{"tokenid": "a", "privsep": 1},
		},
	}
	readResp := runSDNDataSource(t, NewUserTokensDataSource(), tokens, map[string]any{"user_id": "root@pam"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("token read diagnostics: %v", readResp.Diagnostics)
	}
	var tokenState userTokensModel
	if diags := readResp.State.Get(context.Background(), &tokenState); diags.HasError() || len(tokenState.Tokens) != 2 || tokenState.Tokens[0].TokenID.ValueString() != "a" {
		t.Fatalf("unexpected token projection: %#v (%v)", tokenState.Tokens, diags)
	}

	entries := map[string]any{
		"/api2/json/access/tfa/root@pam": []map[string]any{
			{"id": "2", "type": "totp", "created": 20, "enable": 1},
			{"id": "1", "type": "recovery", "created": 10, "description": "codes"},
		},
	}
	entryResp := runSDNDataSource(t, NewUserTFAEntriesDataSource(), entries, map[string]any{"user_id": "root@pam"})
	if entryResp.Diagnostics.HasError() {
		t.Fatalf("tfa read diagnostics: %v", entryResp.Diagnostics)
	}
	var entryState userTFAEntriesModel
	if diags := entryResp.State.Get(context.Background(), &entryState); diags.HasError() || len(entryState.Entries) != 2 || entryState.Entries[0].EntryID.ValueString() != "1" {
		t.Fatalf("unexpected tfa projection: %#v (%v)", entryState.Entries, diags)
	}
}

func TestFirewallOptionsDataSources(t *testing.T) {
	cluster := map[string]any{"/api2/json/cluster/firewall/options": map[string]any{"enable": 1, "policy_in": "DROP", "policy_out": "ACCEPT"}}
	readResp := runSDNDataSource(t, NewClusterFirewallOptionsDataSource(), cluster, map[string]any{})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("cluster read diagnostics: %v", readResp.Diagnostics)
	}
	var clusterState clusterFirewallOptionsModel
	if diags := readResp.State.Get(context.Background(), &clusterState); diags.HasError() || !clusterState.Enable.ValueBool() || clusterState.PolicyIn.ValueString() != "DROP" {
		t.Fatalf("unexpected cluster options: %#v (%v)", clusterState, diags)
	}

	node := map[string]any{"/api2/json/nodes/pve-1/firewall/options": map[string]any{"enable": 1, "ndp": 1, "log_level_in": "info"}}
	nodeResp := runSDNDataSource(t, NewNodeFirewallOptionsDataSource(), node, map[string]any{"node": "pve-1"})
	if nodeResp.Diagnostics.HasError() {
		t.Fatalf("node read diagnostics: %v", nodeResp.Diagnostics)
	}
	var nodeState nodeFirewallOptionsDSModel
	if diags := nodeResp.State.Get(context.Background(), &nodeState); diags.HasError() || !nodeState.NDP.ValueBool() || nodeState.LogLevelIn.ValueString() != "info" {
		t.Fatalf("unexpected node options: %#v (%v)", nodeState, diags)
	}

	guest := map[string]any{"/api2/json/nodes/pve-1/qemu/101/firewall/options": map[string]any{"enable": 1, "macfilter": 1, "policy_in": "DROP"}}
	guestResp := runSDNDataSource(t, NewGuestFirewallOptionsDataSource(), guest, map[string]any{"node": "pve-1", "guest_type": "qemu", "vm_id": int64(101)})
	if guestResp.Diagnostics.HasError() {
		t.Fatalf("guest read diagnostics: %v", guestResp.Diagnostics)
	}
	var guestState guestFirewallOptionsModel
	if diags := guestResp.State.Get(context.Background(), &guestState); diags.HasError() || !guestState.MACFilter.ValueBool() {
		t.Fatalf("unexpected guest options: %#v (%v)", guestState, diags)
	}
}

func TestNodeNetworkInterfacesDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/network": []map[string]any{
			{"iface": "vmbr1", "type": "bridge", "active": 1, "exists": 1, "autostart": 1, "bridge_ports": "eth1", "cidr": "10.0.0.1/24"},
			{"iface": "eth0", "type": "eth", "active": 0, "exists": 1},
		},
	}
	readResp := runSDNDataSource(t, NewNodeNetworkInterfacesDataSource(), responses, map[string]any{"node": "pve-1"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state nodeNetworkInterfacesModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() || len(state.Interfaces) != 2 || state.Interfaces[0].Iface.ValueString() != "eth0" || state.Interfaces[1].BridgePorts.ValueString() != "eth1" {
		t.Fatalf("unexpected projection: %#v (%v)", state.Interfaces, diags)
	}
}
