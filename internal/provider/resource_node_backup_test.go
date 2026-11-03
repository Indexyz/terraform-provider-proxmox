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
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	nodeBackupTaskUPID = "UPID:pve-1:0000:0000:0000:vzdump:::"
	nodePruneTaskUPID  = "UPID:pve-1:0000:0000:0000:prunebackups:::"
)

func TestClientRunVzdump(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/pve-1/vzdump" || r.Method != http.MethodPost {
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
			return
		}
		if !handler.form(w, r, url.Values{
			"vmid": {"300"}, "storage": {"backup"}, "mode": {"stop"}, "compress": {"zstd"},
			"notes-template": {"nightly"}, "protected": {"1"}, "quiet": {"1"},
		}) {
			return
		}
		handler.envelope(w, nodeBackupTaskUPID)
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)
	upid, err := client.RunVzdump(context.Background(), "pve-1", 300, "backup", "stop", "zstd", "nightly", true, true)
	if err != nil || upid != nodeBackupTaskUPID {
		t.Fatalf("RunVzdump() = %q, %v", upid, err)
	}
	handler.assert(t)
}

func TestClientPruneBackups(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/pve-1/storage/backup/prunebackups" || r.Method != http.MethodDelete {
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
			return
		}
		if !handler.form(w, r, url.Values{"vmid": {"300"}, "type": {"qemu"}, "prune-backups": {"keep-last=1"}}) {
			return
		}
		handler.envelope(w, nodePruneTaskUPID)
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)
	upid, err := client.PruneBackups(context.Background(), "pve-1", "backup", "keep-last=1", "qemu", 300)
	if err != nil || upid != nodePruneTaskUPID {
		t.Fatalf("PruneBackups() = %q, %v", upid, err)
	}
	handler.assert(t)
}

func TestNodeVzdumpSchema(t *testing.T) {
	testResourceSchema(t, NewNodeVzdumpResource())
}

func TestNodeVzdumpLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/vzdump" && r.Method == http.MethodPost:
			handler.envelope(w, nodeBackupTaskUPID)
		case strings.HasPrefix(r.URL.Path, "/api2/json/nodes/pve-1/tasks/"):
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &NodeVzdumpResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := nodeVzdumpModel{
		Node: types.StringValue("pve-1"), VMID: types.Int64Value(300), Storage: types.StringValue("backup"), Mode: types.StringValue("snapshot"),
		Compress: types.StringValue("zstd"), NotesTemplate: types.StringValue("nightly"), Protected: types.BoolValue(false), Quiet: types.BoolValue(true),
		Triggers: testStringMap(t, map[string]string{"gen": "1"}),
	}
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("vzdump diagnostics: %v", resp.Diagnostics)
	}
	var state nodeVzdumpModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.ID.ValueString() != "pve-1/300" || state.UPID.ValueString() != nodeBackupTaskUPID {
		t.Fatalf("unexpected state: %#v", state)
	}
	handler.assert(t)
}

func TestNodeBackupPruneSchema(t *testing.T) {
	testResourceSchema(t, NewNodeBackupPruneResource())
}

func TestBackupNotBackedUpDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/backup-info/not-backed-up": []map[string]any{
			{"name": "ct-400", "type": "lxc", "vmid": 400},
			{"name": "vm-300", "type": "qemu", "vmid": 300},
		},
	}
	readResp := runSDNDataSource(t, NewBackupNotBackedUpDataSource(), responses, nil)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state backupNotBackedUpModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if len(state.Guests) != 2 || state.Guests[0].VMID.ValueInt64() != 300 || state.Guests[1].Type.ValueString() != "lxc" {
		t.Fatalf("unexpected projection: %#v", state.Guests)
	}
}

func TestBackupJobIncludedVolumesDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/backup/nightly/included_volumes": map[string]any{"children": []any{map[string]any{"type": "qemu", "leaf": 1, "name": "vm-300", "id": 300, "children": []any{map[string]any{"type": "disk", "id": 0}}}}},
	}
	readResp := runSDNDataSource(t, NewBackupJobIncludedVolumesDataSource(), responses, map[string]any{"id": "nightly"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state backupJobIncludedVolumesModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.ID.ValueString() != "nightly" || len(state.Volumes.Elements()) != 1 {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestClusterRealmSyncJobsDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/jobs/realm-sync": []map[string]any{
			{"id": "sync-1", "realm": "ad", "scope": "users", "schedule": "daily", "enabled": 1, "last-run": 10},
			{"id": "sync-2", "realm": "ldap", "scope": "groups", "schedule": "hourly", "enabled": 0},
		},
	}
	readResp := runSDNDataSource(t, NewClusterRealmSyncJobsDataSource(), responses, nil)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state clusterRealmSyncJobsModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if len(state.Jobs) != 2 || state.Jobs[0].Realm.ValueString() != "ad" || state.Jobs[0].LastRun.ValueInt64() != 10 || state.Jobs[1].Enabled.ValueBool() {
		t.Fatalf("unexpected projection: %#v", state.Jobs)
	}
}
