// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestQemuSnapshotResourceFrameworkLifecycle(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	exists := true
	description := ""
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.RawQuery != "" {
			handler.fail(w, "unexpected QEMU snapshot query: %s", r.URL.RawQuery)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/101/snapshot":
			if !handler.form(w, r, url.Values{"snapname": {"before deploy"}, "description": {"created description"}}) {
				return
			}
			handler.envelope(w, "UPID:pve one:qemu-create")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/101/snapshot":
			if !exists {
				handler.envelope(w, []any{})
				return
			}
			snapshot := map[string]any{"name": "before deploy", "parent": "base", "snaptime": 1700000101}
			if description != "" {
				snapshot["description"] = description
			}
			handler.envelope(w, []any{map[string]any{"name": "other"}, snapshot})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/101/snapshot/before%20deploy/config":
			if !handler.form(w, r, url.Values{"description": {"updated description"}}) {
				return
			}
			description = "updated description"
			handler.envelope(w, "UPID:pve one:qemu-update")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-update/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/101/snapshot/before%20deploy":
			if !handler.form(w, r, url.Values{}) {
				return
			}
			if !exists {
				http.Error(w, "snapshot missing", http.StatusNotFound)
				return
			}
			exists = false
			handler.envelope(w, "UPID:pve one:qemu-delete")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected QEMU snapshot request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuSnapshotResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := qemuSnapshotModel{Node: types.StringValue("pve one"), VMID: types.Int64Value(101), Name: types.StringValue("before deploy"), Description: types.StringValue("created description")}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("QEMU snapshot create diagnostics: %v", createResp.Diagnostics)
	}
	var created qemuSnapshotModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode QEMU snapshot create state: %v", diags)
	}
	if created.ID.ValueString() != "pve one/101/before deploy" || created.Description.ValueString() != "created description" || created.Parent.ValueString() != "base" || created.Snaptime.ValueInt64() != 1700000101 {
		t.Fatalf("unexpected typed QEMU snapshot create state: %#v", created)
	}

	readResp := resource.ReadResponse{State: createResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: createResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("QEMU snapshot read diagnostics: %v", readResp.Diagnostics)
	}
	assertStateString(t, readResp.State, path.Root("description"), "created description")

	updated := qemuSnapshotModel{Node: types.StringValue("pve one"), VMID: types.Int64Value(101), Name: types.StringValue("before deploy"), Description: types.StringValue("updated description")}
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, updated), State: readResp.State}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("QEMU snapshot changed update diagnostics: %v", updateResp.Diagnostics)
	}
	assertStateString(t, updateResp.State, path.Root("description"), "updated description")

	unchangedResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, updated), State: updateResp.State}, &unchangedResp)
	if unchangedResp.Diagnostics.HasError() {
		t.Fatalf("QEMU snapshot unchanged update diagnostics: %v", unchangedResp.Diagnostics)
	}
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: unchangedResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("QEMU snapshot delete diagnostics: %v", deleteResp.Diagnostics)
	}
	missingResp := resource.ReadResponse{State: unchangedResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: unchangedResp.State}, &missingResp)
	if missingResp.Diagnostics.HasError() || !missingResp.State.Raw.IsNull() {
		t.Fatalf("missing QEMU snapshot was not removed: diagnostics=%v raw=%v", missingResp.Diagnostics, missingResp.State.Raw)
	}
	var idempotentDelete resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: unchangedResp.State}, &idempotentDelete)
	if idempotentDelete.Diagnostics.HasError() {
		t.Fatalf("idempotent QEMU snapshot delete diagnostics: %v", idempotentDelete.Diagnostics)
	}
	handler.assert(t)
	wantCalls := []string{
		"POST /api2/json/nodes/pve%20one/qemu/101/snapshot", "GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create/status",
		"GET /api2/json/nodes/pve%20one/qemu/101/snapshot", "GET /api2/json/nodes/pve%20one/qemu/101/snapshot",
		"PUT /api2/json/nodes/pve%20one/qemu/101/snapshot/before%20deploy/config", "GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-update/status",
		"GET /api2/json/nodes/pve%20one/qemu/101/snapshot", "GET /api2/json/nodes/pve%20one/qemu/101/snapshot",
		"DELETE /api2/json/nodes/pve%20one/qemu/101/snapshot/before%20deploy", "GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete/status",
		"GET /api2/json/nodes/pve%20one/qemu/101/snapshot", "DELETE /api2/json/nodes/pve%20one/qemu/101/snapshot/before%20deploy",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected QEMU snapshot call order: got %v want %v", calls, wantCalls)
	}

	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, qemuSnapshotModel{})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "pve one/101/before deploy"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("QEMU snapshot import diagnostics: %v", importResp.Diagnostics)
	}
	assertStateString(t, importResp.State, path.Root("id"), "pve one/101/before deploy")
	var imported qemuSnapshotModel
	if diags := importResp.State.Get(context.Background(), &imported); diags.HasError() || imported.Node.ValueString() != "pve one" || imported.VMID.ValueInt64() != 101 || imported.Name.ValueString() != "before deploy" {
		t.Fatalf("unexpected QEMU snapshot import state: %#v diagnostics=%v", imported, diags)
	}
	invalidImport := resource.ImportStateResponse{State: testResourceState(t, schema, qemuSnapshotModel{})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "pve one/not-a-vmid/before deploy"}, &invalidImport)
	if !invalidImport.Diagnostics.HasError() || !containsDiagnostic(invalidImport.Diagnostics, "not-a-vmid") {
		t.Fatalf("expected invalid QEMU snapshot import diagnostics, got %v", invalidImport.Diagnostics)
	}
}

func TestLXCSnapshotResourceFrameworkLifecycle(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	exists := true
	description := ""
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.RawQuery != "" {
			handler.fail(w, "unexpected LXC snapshot query: %s", r.URL.RawQuery)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20two/lxc/202/snapshot":
			if !handler.form(w, r, url.Values{"snapname": {"before patch"}, "description": {"created LXC description"}}) {
				return
			}
			handler.envelope(w, "UPID:pve two:lxc-create")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20two/tasks/UPID:pve%20two:lxc-create/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20two/lxc/202/snapshot":
			if !exists {
				handler.envelope(w, []any{})
				return
			}
			snapshot := map[string]any{"name": "before patch", "parent": "base-lxc", "snaptime": 1700000202}
			if description != "" {
				snapshot["description"] = description
			}
			handler.envelope(w, []any{snapshot})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/pve%20two/lxc/202/snapshot/before%20patch/config":
			if !handler.form(w, r, url.Values{"description": {"updated LXC description"}}) {
				return
			}
			description = "updated LXC description"
			handler.envelope(w, "UPID:pve two:lxc-update")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20two/tasks/UPID:pve%20two:lxc-update/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve%20two/lxc/202/snapshot/before%20patch":
			if !handler.form(w, r, url.Values{}) {
				return
			}
			if !exists {
				http.Error(w, "snapshot missing", http.StatusNotFound)
				return
			}
			exists = false
			handler.envelope(w, "UPID:pve two:lxc-delete")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20two/tasks/UPID:pve%20two:lxc-delete/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected LXC snapshot request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &LXCSnapshotResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := lxcSnapshotModel{Node: types.StringValue("pve two"), VMID: types.Int64Value(202), Name: types.StringValue("before patch"), Description: types.StringValue("created LXC description")}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("LXC snapshot create diagnostics: %v", createResp.Diagnostics)
	}
	var created lxcSnapshotModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode LXC snapshot create state: %v", diags)
	}
	if created.ID.ValueString() != "pve two/202/before patch" || created.Description.ValueString() != "created LXC description" || created.Parent.ValueString() != "base-lxc" || created.Snaptime.ValueInt64() != 1700000202 {
		t.Fatalf("unexpected typed LXC snapshot create state: %#v", created)
	}

	readResp := resource.ReadResponse{State: createResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: createResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("LXC snapshot read diagnostics: %v", readResp.Diagnostics)
	}
	assertStateString(t, readResp.State, path.Root("description"), "created LXC description")

	updated := lxcSnapshotModel{Node: types.StringValue("pve two"), VMID: types.Int64Value(202), Name: types.StringValue("before patch"), Description: types.StringValue("updated LXC description")}
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, updated), State: readResp.State}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("LXC snapshot changed update diagnostics: %v", updateResp.Diagnostics)
	}
	assertStateString(t, updateResp.State, path.Root("description"), "updated LXC description")

	unchangedResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, updated), State: updateResp.State}, &unchangedResp)
	if unchangedResp.Diagnostics.HasError() {
		t.Fatalf("LXC snapshot unchanged update diagnostics: %v", unchangedResp.Diagnostics)
	}
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: unchangedResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("LXC snapshot delete diagnostics: %v", deleteResp.Diagnostics)
	}
	missingResp := resource.ReadResponse{State: unchangedResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: unchangedResp.State}, &missingResp)
	if missingResp.Diagnostics.HasError() || !missingResp.State.Raw.IsNull() {
		t.Fatalf("missing LXC snapshot was not removed: diagnostics=%v raw=%v", missingResp.Diagnostics, missingResp.State.Raw)
	}
	var idempotentDelete resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: unchangedResp.State}, &idempotentDelete)
	if idempotentDelete.Diagnostics.HasError() {
		t.Fatalf("idempotent LXC snapshot delete diagnostics: %v", idempotentDelete.Diagnostics)
	}
	handler.assert(t)
	wantCalls := []string{
		"POST /api2/json/nodes/pve%20two/lxc/202/snapshot", "GET /api2/json/nodes/pve%20two/tasks/UPID:pve%20two:lxc-create/status",
		"GET /api2/json/nodes/pve%20two/lxc/202/snapshot", "GET /api2/json/nodes/pve%20two/lxc/202/snapshot",
		"PUT /api2/json/nodes/pve%20two/lxc/202/snapshot/before%20patch/config", "GET /api2/json/nodes/pve%20two/tasks/UPID:pve%20two:lxc-update/status",
		"GET /api2/json/nodes/pve%20two/lxc/202/snapshot", "GET /api2/json/nodes/pve%20two/lxc/202/snapshot",
		"DELETE /api2/json/nodes/pve%20two/lxc/202/snapshot/before%20patch", "GET /api2/json/nodes/pve%20two/tasks/UPID:pve%20two:lxc-delete/status",
		"GET /api2/json/nodes/pve%20two/lxc/202/snapshot", "DELETE /api2/json/nodes/pve%20two/lxc/202/snapshot/before%20patch",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected LXC snapshot call order: got %v want %v", calls, wantCalls)
	}

	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, lxcSnapshotModel{})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "pve two/202/before patch"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("LXC snapshot import diagnostics: %v", importResp.Diagnostics)
	}
	assertStateString(t, importResp.State, path.Root("id"), "pve two/202/before patch")
	var imported lxcSnapshotModel
	if diags := importResp.State.Get(context.Background(), &imported); diags.HasError() || imported.Node.ValueString() != "pve two" || imported.VMID.ValueInt64() != 202 || imported.Name.ValueString() != "before patch" {
		t.Fatalf("unexpected LXC snapshot import state: %#v diagnostics=%v", imported, diags)
	}
	invalidImport := resource.ImportStateResponse{State: testResourceState(t, schema, lxcSnapshotModel{})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "pve two/202"}, &invalidImport)
	if !invalidImport.Diagnostics.HasError() || !containsDiagnostic(invalidImport.Diagnostics, "node/vm_id/name form") {
		t.Fatalf("expected invalid LXC snapshot import diagnostics, got %v", invalidImport.Diagnostics)
	}
}

func TestSnapshotResourcesPreserveAPIErrorDetail(t *testing.T) {
	for _, test := range []struct {
		name string
		kind string
	}{
		{name: "qemu", kind: "qemu"},
		{name: "lxc", kind: "lxc"},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := &lifecycleHandler{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !handler.auth(w, r) {
					return
				}
				wantPath := "/api2/json/nodes/pve/" + test.kind + "/303/snapshot"
				if r.Method != http.MethodGet || r.URL.EscapedPath() != wantPath || r.URL.RawQuery != "" {
					handler.fail(w, "unexpected snapshot error request: %s %s", r.Method, r.URL.String())
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"errors":{"snapshot":"missing VM.Snapshot permission"}}`))
			}))
			defer server.Close()

			if test.kind == "qemu" {
				res := &QemuSnapshotResource{client: testLifecycleClient(t, server)}
				schema := testResourceSchema(t, res)
				state := testResourceState(t, schema, qemuSnapshotModel{ID: types.StringValue("pve/303/snap"), Node: types.StringValue("pve"), VMID: types.Int64Value(303), Name: types.StringValue("snap")})
				resp := resource.ReadResponse{State: state}
				res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
				if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "status 403") || !containsDiagnostic(resp.Diagnostics, "missing VM.Snapshot permission") {
					t.Fatalf("expected preserved QEMU snapshot API detail, got %v", resp.Diagnostics)
				}
				if !resp.State.Raw.Equal(state.Raw) {
					t.Fatalf("QEMU snapshot API error unexpectedly mutated state: %v", resp.State.Raw)
				}
			} else {
				res := &LXCSnapshotResource{client: testLifecycleClient(t, server)}
				schema := testResourceSchema(t, res)
				state := testResourceState(t, schema, lxcSnapshotModel{ID: types.StringValue("pve/303/snap"), Node: types.StringValue("pve"), VMID: types.Int64Value(303), Name: types.StringValue("snap")})
				resp := resource.ReadResponse{State: state}
				res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
				if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "status 403") || !containsDiagnostic(resp.Diagnostics, "missing VM.Snapshot permission") {
					t.Fatalf("expected preserved LXC snapshot API detail, got %v", resp.Diagnostics)
				}
				if !resp.State.Raw.Equal(state.Raw) {
					t.Fatalf("LXC snapshot API error unexpectedly mutated state: %v", resp.State.Raw)
				}
			}
			handler.assert(t)
		})
	}
}
