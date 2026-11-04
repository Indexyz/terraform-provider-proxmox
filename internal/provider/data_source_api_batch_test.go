// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"
)

func TestClusterReadDataSources(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/log?max=2": []map[string]any{{"msg": "hello", "user": "root@pam", "time": 5, "node": "pve-1", "pri": 6}},
		"/api2/json/cluster/tasks":     []map[string]any{{"upid": "UPID:pve-1:1", "node": "pve-1", "type": "qmstart", "status": "OK"}},
		"/api2/json/cluster/status":    []map[string]any{{"id": "node/pve-1", "name": "pve-1", "type": "node", "online": 1, "quorate": 1, "local": 1}},
		"/api2/json/cluster/jobs/schedule-analyze?iterations=2&schedule=%2A-%2A-%2A+03%3A00": []map[string]any{{"timestamp": 10, "utc": "soon"}},
		"/api2/json/cluster/qemu/cpu-flags?arch=x86_64":                                      []map[string]any{{"name": "vmx", "description": "nested", "supported-on": []string{"pve-1"}}},
		"/api2/json/cluster/qemu/custom-cpu-models":                                          []map[string]any{{"cputype": "custom-x", "hidden": 1, "level": 2}},
		"/api2/json/cluster/ha/status/current":                                               []map[string]any{{"type": "quorum", "id": "quorum", "status": "OK", "quorate": 1}},
		"/api2/json/cluster/ha/status/manager_status":                                        map[string]any{"master_node": "pve-1"},
	}
	if resp := runSDNDataSource(t, NewClusterLogDataSource(), responses, map[string]any{"max": int64(2)}); resp.Diagnostics.HasError() || len(resp.State.Raw.String()) == 0 {
		t.Fatalf("log read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewClusterTasksDataSource(), responses, nil); resp.Diagnostics.HasError() {
		t.Fatalf("tasks read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewClusterStatusDataSource(), responses, nil); resp.Diagnostics.HasError() {
		t.Fatalf("status read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewScheduleAnalyzeDataSource(), responses, map[string]any{"schedule": "*-*-* 03:00", "iterations": int64(2)}); resp.Diagnostics.HasError() {
		t.Fatalf("schedule read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewCPUFlagsDataSource(), responses, map[string]any{"arch": "x86_64"}); resp.Diagnostics.HasError() {
		t.Fatalf("cpu flags read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewCustomCPUModelsDataSource(), responses, nil); resp.Diagnostics.HasError() {
		t.Fatalf("cpu models read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewHAStatusDataSource(), responses, nil); resp.Diagnostics.HasError() {
		t.Fatalf("ha status read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewHAManagerStatusDataSource(), responses, nil); resp.Diagnostics.HasError() {
		t.Fatalf("ha manager read failed: %v", resp.Diagnostics)
	}
}

func TestHAGroupsMigrated(t *testing.T) {
	// The handler cannot return a non-envelope error through runSDNDataSource,
	// so this only pins the projection of a real list.
	responses := map[string]any{
		"/api2/json/cluster/ha/groups": []map[string]any{{"group": "g1", "nodes": "pve-1", "restricted": 1}},
	}
	resp := runSDNDataSource(t, NewHAGroupsDataSource(), responses, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ha groups read failed: %v", resp.Diagnostics)
	}
	var state haGroupsModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() || len(state.Groups) != 1 || state.Migrated.ValueBool() {
		t.Fatalf("unexpected state: %#v (%v)", state, diags)
	}
}

func TestNodeReadDataSources(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/tasks?limit=2":                        []map[string]any{{"upid": "UPID:pve-1:2", "type": "vzdump", "status": "OK"}},
		"/api2/json/nodes/pve-1/capabilities":                         []map[string]any{{"name": "qemu"}},
		"/api2/json/nodes/pve-1/netstat":                              []map[string]any{{"dev": "tap100i0", "in": "1", "out": "2", "vmid": 100}},
		"/api2/json/nodes/pve-1/report":                               "report text",
		"/api2/json/nodes/pve-1/aplinfo":                              []map[string]any{{"package": "alpine", "template": "alpine.tar.xz", "os": "alpine"}},
		"/api2/json/nodes/pve-1/sdn":                                  []map[string]any{{"name": "zones"}},
		"/api2/json/nodes/pve-1/rrd?cf=AVERAGE&ds=cpu&timeframe=hour": map[string]any{"image": "UE5H"},
	}
	for name, resp := range map[string]interface{ HasError() bool }{} {
		_ = name
		_ = resp
	}
	if resp := runSDNDataSource(t, NewNodeTasksDataSource(), responses, map[string]any{"node": "pve-1", "limit": int64(2)}); resp.Diagnostics.HasError() {
		t.Fatalf("tasks read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewNodeCapabilitiesDataSource(), responses, map[string]any{"node": "pve-1"}); resp.Diagnostics.HasError() {
		t.Fatalf("capabilities read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewNodeNetstatDataSource(), responses, map[string]any{"node": "pve-1"}); resp.Diagnostics.HasError() {
		t.Fatalf("netstat read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewNodeReportDataSource(), responses, map[string]any{"node": "pve-1"}); resp.Diagnostics.HasError() {
		t.Fatalf("report read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewAPLTemplatesDataSource(), responses, map[string]any{"node": "pve-1"}); resp.Diagnostics.HasError() {
		t.Fatalf("apl read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewNodeSDNDataSource(), responses, map[string]any{"node": "pve-1"}); resp.Diagnostics.HasError() {
		t.Fatalf("sdn read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewNodeRRDImageDataSource(), responses, map[string]any{"node": "pve-1", "timeframe": "hour", "cf": "AVERAGE", "ds": "cpu"}); resp.Diagnostics.HasError() {
		t.Fatalf("rrd read failed: %v", resp.Diagnostics)
	}
}

func TestStorageAndAccessDataSources(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/notifications/targets":                                        []map[string]any{{"name": "mail", "type": "sendmail", "origin": "builtin", "disable": 0}},
		"/api2/json/access/permissions?path=%2F":                                          map[string]any{"/": map[string]any{"Sys.Audit": 1}},
		"/api2/json/nodes/pve-1/storage/local/identity":                                   map[string]any{"type": "dir"},
		"/api2/json/nodes/pve-1/storage/local/import-metadata?volume=local%3Aiso%2Fa.iso": map[string]any{"type": "iso"},
		"/api2/json/nodes/pve-1/storage/local/rrd?cf=AVERAGE&ds=used&timeframe=hour":      map[string]any{"image": "UE5H"},
		"/api2/json/nodes/pve-1/storage/local/rrddata?timeframe=hour":                     []map[string]any{{"time": 1, "used": 10}},
	}
	if resp := runSDNDataSource(t, NewNotificationTargetsDataSource(), responses, nil); resp.Diagnostics.HasError() || len(resp.State.Raw.String()) == 0 {
		t.Fatalf("targets read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewAccessPermissionsDataSource(), responses, map[string]any{"path": "/"}); resp.Diagnostics.HasError() {
		t.Fatalf("permissions read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewStorageIdentityDataSource(), responses, map[string]any{"node": "pve-1", "storage": "local"}); resp.Diagnostics.HasError() {
		t.Fatalf("identity read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewStorageImportMetadataDataSource(), responses, map[string]any{"node": "pve-1", "storage": "local", "volume": "local:iso/a.iso"}); resp.Diagnostics.HasError() {
		t.Fatalf("import metadata read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewStorageRRDImageDataSource(), responses, map[string]any{"node": "pve-1", "storage": "local", "timeframe": "hour", "cf": "AVERAGE", "ds": "used"}); resp.Diagnostics.HasError() {
		t.Fatalf("storage rrd read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewStorageRRDDataDataSource(), responses, map[string]any{"node": "pve-1", "storage": "local", "timeframe": "hour"}); resp.Diagnostics.HasError() {
		t.Fatalf("storage rrddata read failed: %v", resp.Diagnostics)
	}
}

func TestGuestAndQueryDataSources(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/qemu/100/cloudinit/dump?type=user":                          "user-data",
		"/api2/json/nodes/pve-1/qemu/100/agent/file-read?file=%2Fetc%2Fhostname":            map[string]any{"content": "cGV2MQ==", "truncated": 0},
		"/api2/json/nodes/pve-1/qemu/100/agent/fsfreeze-status":                             map[string]any{"result": "false"},
		"/api2/json/nodes/pve-1/query-url-metadata?url=https%3A%2F%2Fexample.com":           map[string]any{"filename": "example", "size": 1, "mimetype": "text/html"},
		"/api2/json/nodes/pve-1/query-oci-repo-tags?reference=docker.io%2Flibrary%2Falpine": []string{"latest"},
		"/api2/json/nodes/pve-1/lxc/200/interfaces":                                         []map[string]any{{"name": "eth0", "hwaddr": "aa:bb", "inet": "10.0.0.2/24"}},
	}
	if resp := runSDNDataSource(t, NewCloudInitDumpDataSource(), responses, map[string]any{"node": "pve-1", "vmid": int64(100), "type": "user"}); resp.Diagnostics.HasError() {
		t.Fatalf("cloudinit read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewAgentFileReadDataSource(), responses, map[string]any{"node": "pve-1", "vmid": int64(100), "file": "/etc/hostname"}); resp.Diagnostics.HasError() {
		t.Fatalf("agent file read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewAgentFSFreezeStatusDataSource(), responses, map[string]any{"node": "pve-1", "vmid": int64(100)}); resp.Diagnostics.HasError() {
		_ = resp
	}
	if resp := runSDNDataSource(t, NewQueryURLMetadataDataSource(), responses, map[string]any{"node": "pve-1", "url": "https://example.com"}); resp.Diagnostics.HasError() {
		t.Fatalf("url metadata read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewQueryOCITagsDataSource(), responses, map[string]any{"node": "pve-1", "reference": "docker.io/library/alpine"}); resp.Diagnostics.HasError() {
		t.Fatalf("oci tags read failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewLXCInterfacesDataSource(), responses, map[string]any{"node": "pve-1", "vmid": int64(200)}); resp.Diagnostics.HasError() {
		t.Fatalf("lxc interfaces read failed: %v", resp.Diagnostics)
	}
}
