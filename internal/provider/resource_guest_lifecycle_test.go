// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// testResourceCreateResponse mirrors the framework server, which initializes
// CreateResponse.State.Raw with a typed null object so that partial
// SetAttribute writes (persistQemuVMIdentity) work like they do in production.
func testResourceCreateResponse(schema resource.SchemaResponse) resource.CreateResponse {
	return resource.CreateResponse{
		State: tfsdk.State{
			Schema: schema.Schema,
			Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(context.Background()), nil),
		},
	}
}

func minimalQemuVMModel(node string, vmID int64) qemuVMModel {
	return qemuVMModel{
		Node:      types.StringValue(node),
		VMID:      types.Int64Value(vmID),
		Common:    types.ObjectNull(qemuVMCommonAttrTypes()),
		CloudInit: types.ObjectNull(qemuVMCloudInitAttrTypes()),
		Network:   types.MapNull(types.ObjectType{AttrTypes: qemuVMNetworkAttrTypes()}),
		Disk:      types.MapNull(types.ObjectType{AttrTypes: qemuVMDiskAttrTypes()}),
		Serial:    types.MapNull(types.StringType),
		EFIDisk:   types.ObjectNull(qemuVMEFIDiskAttrTypes()),
		TPMState:  types.ObjectNull(qemuVMTPMStateAttrTypes()),
		VGA:       types.ObjectNull(qemuVMVGAAttrTypes()),
		Raw:       types.ObjectNull(qemuVMRawAttrTypes()),
		Clone:     types.ObjectNull(qemuVMCloneAttrTypes()),
	}
}

func minimalLXCContainerModel(node string, vmID int64) lxcContainerModel {
	return lxcContainerModel{
		Node:       types.StringValue(node),
		VMID:       types.Int64Value(vmID),
		Network:    types.MapNull(types.ObjectType{AttrTypes: lxcContainerNetworkAttrTypes()}),
		MountPoint: types.MapNull(types.ObjectType{AttrTypes: lxcContainerMountPointAttrTypes()}),
		Raw:        types.ObjectNull(lxcContainerRawAttrTypes()),
		Clone:      types.ObjectNull(lxcContainerCloneAttrTypes()),
	}
}

func TestQemuVMResourceFrameworkLifecycle(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	exists := true
	name := "wrapper-vm"
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.RawQuery != "" {
			handler.fail(w, "unexpected QEMU wrapper query: %s", r.URL.RawQuery)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu":
			if !handler.form(w, r, url.Values{"vmid": {"401"}, "name": {"wrapper-vm"}}) {
				return
			}
			handler.envelope(w, "UPID:pve one:qemu-create-wrapper")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-wrapper/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/401/config":
			if !exists {
				http.Error(w, "VM missing", http.StatusNotFound)
				return
			}
			handler.envelope(w, map[string]any{"name": name, "onboot": 0, "memory": 1024})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/401/status/current":
			handler.envelope(w, map[string]any{"status": "stopped", "uptime": 0})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/401/config":
			if !handler.form(w, r, url.Values{"name": {"wrapper-vm-updated"}}) {
				return
			}
			name = "wrapper-vm-updated"
			handler.envelope(w, nil)
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/401":
			if !handler.form(w, r, url.Values{}) {
				return
			}
			if !exists {
				http.Error(w, "VM missing", http.StatusNotFound)
				return
			}
			exists = false
			handler.envelope(w, "UPID:pve one:qemu-delete-wrapper")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-wrapper/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected QEMU wrapper request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := minimalQemuVMModel("pve one", 401)
	initial.Name = types.StringValue("wrapper-vm")
	createResp := testResourceCreateResponse(schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("QEMU wrapper create diagnostics: %v", createResp.Diagnostics)
	}
	var created qemuVMModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode QEMU wrapper create state: %v", diags)
	}
	if created.ID.ValueString() != "pve one/401" || created.Name.ValueString() != "wrapper-vm" || created.Memory.ValueInt64() != 1024 || created.Status.ValueString() != "stopped" || created.Uptime.ValueInt64() != 0 {
		t.Fatalf("unexpected QEMU wrapper typed state: %#v", created)
	}

	readResp := resource.ReadResponse{State: createResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: createResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("QEMU wrapper read diagnostics: %v", readResp.Diagnostics)
	}
	updated := minimalQemuVMModel("pve one", 401)
	updated.Name = types.StringValue("wrapper-vm-updated")
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, updated), State: readResp.State}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("QEMU wrapper update diagnostics: %v", updateResp.Diagnostics)
	}
	assertStateString(t, updateResp.State, path.Root("name"), "wrapper-vm-updated")

	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("QEMU wrapper delete diagnostics: %v", deleteResp.Diagnostics)
	}
	missingResp := resource.ReadResponse{State: updateResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: updateResp.State}, &missingResp)
	if missingResp.Diagnostics.HasError() || !missingResp.State.Raw.IsNull() {
		t.Fatalf("missing QEMU wrapper was not removed: diagnostics=%v raw=%v", missingResp.Diagnostics, missingResp.State.Raw)
	}
	var idempotentDelete resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: updateResp.State}, &idempotentDelete)
	if idempotentDelete.Diagnostics.HasError() {
		t.Fatalf("idempotent QEMU wrapper delete diagnostics: %v", idempotentDelete.Diagnostics)
	}
	handler.assert(t)
	wantCalls := []string{
		"POST /api2/json/nodes/pve%20one/qemu", "GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-wrapper/status", "GET /api2/json/nodes/pve%20one/qemu/401/config", "GET /api2/json/nodes/pve%20one/qemu/401/status/current",
		"GET /api2/json/nodes/pve%20one/qemu/401/config", "GET /api2/json/nodes/pve%20one/qemu/401/status/current",
		"GET /api2/json/nodes/pve%20one/qemu/401/config", "PUT /api2/json/nodes/pve%20one/qemu/401/config", "GET /api2/json/nodes/pve%20one/qemu/401/config", "GET /api2/json/nodes/pve%20one/qemu/401/status/current",
		"DELETE /api2/json/nodes/pve%20one/qemu/401", "GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-wrapper/status", "GET /api2/json/nodes/pve%20one/qemu/401/config", "DELETE /api2/json/nodes/pve%20one/qemu/401",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected QEMU wrapper call order: got %v want %v", calls, wantCalls)
	}

	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, minimalQemuVMModel("", 0))}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "pve one/401"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("QEMU wrapper import diagnostics: %v", importResp.Diagnostics)
	}
	assertStateString(t, importResp.State, path.Root("id"), "pve one/401")
	invalidImport := resource.ImportStateResponse{State: testResourceState(t, schema, minimalQemuVMModel("", 0))}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "bad"}, &invalidImport)
	if !invalidImport.Diagnostics.HasError() {
		t.Fatalf("expected invalid QEMU wrapper import diagnostics")
	}
}

func TestQemuVMResourceCloneCreateSelection(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/qemu/9000/clone":
			if !handler.form(w, r, url.Values{"newid": {"402"}, "node": {"target node"}, "full": {"1"}}) {
				return
			}
			handler.envelope(w, "UPID:source node:qemu-clone-wrapper")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-wrapper/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/402/config":
			handler.envelope(w, map[string]any{"name": "cloned-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/402/status/current":
			handler.envelope(w, map[string]any{"status": "stopped"})
		default:
			handler.fail(w, "unexpected QEMU clone request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("target node", 402)
	model.Clone = mustQemuVMCloneValue(t, qemuVMCloneModel{SourceNode: types.StringValue("source node"), SourceVMID: types.Int64Value(9000), Full: types.BoolValue(true)})
	resp := testResourceCreateResponse(schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("QEMU clone wrapper diagnostics: %v", resp.Diagnostics)
	}
	if want := []string{"POST /api2/json/nodes/source%20node/qemu/9000/clone", "GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-wrapper/status", "GET /api2/json/nodes/target%20node/qemu/402/config", "GET /api2/json/nodes/target%20node/qemu/402/status/current"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected QEMU clone call order: got %v want %v", calls, want)
	}
	handler.assert(t)
}

func TestLXCContainerResourceFrameworkLifecycle(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	exists := true
	hostname := "wrapper-ct"
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.RawQuery != "" {
			handler.fail(w, "unexpected LXC wrapper query: %s", r.URL.RawQuery)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20two/lxc":
			if !handler.form(w, r, url.Values{"vmid": {"501"}, "ostemplate": {"local:vztmpl/debian.tar.zst"}, "rootfs": {"local-lvm:8"}, "hostname": {"wrapper-ct"}}) {
				return
			}
			handler.envelope(w, "UPID:pve two:lxc-create-wrapper")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20two/tasks/UPID:pve%20two:lxc-create-wrapper/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20two/lxc/501/config":
			if !exists {
				http.Error(w, "container missing", http.StatusNotFound)
				return
			}
			handler.envelope(w, map[string]any{"hostname": hostname, "rootfs": "local-lvm:vm-501-disk-0,size=8G", "memory": 512})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20two/lxc/501/status/current":
			handler.envelope(w, map[string]any{"status": "running", "uptime": 10})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/pve%20two/lxc/501/config":
			if !handler.form(w, r, url.Values{"hostname": {"wrapper-ct-updated"}, "memory": {"512"}, "onboot": {"0"}, "protection": {"0"}}) {
				return
			}
			hostname = "wrapper-ct-updated"
			handler.envelope(w, "UPID:pve two:lxc-update-wrapper")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20two/tasks/UPID:pve%20two:lxc-update-wrapper/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve%20two/lxc/501":
			if !handler.form(w, r, url.Values{}) {
				return
			}
			if !exists {
				http.Error(w, "container missing", http.StatusNotFound)
				return
			}
			exists = false
			handler.envelope(w, "UPID:pve two:lxc-delete-wrapper")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20two/tasks/UPID:pve%20two:lxc-delete-wrapper/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected LXC wrapper request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &LXCContainerResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := minimalLXCContainerModel("pve two", 501)
	initial.OSTemplate = types.StringValue("local:vztmpl/debian.tar.zst")
	initial.RootFS = types.StringValue("local-lvm:8")
	initial.Hostname = types.StringValue("wrapper-ct")
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("LXC wrapper create diagnostics: %v", createResp.Diagnostics)
	}
	var created lxcContainerModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode LXC wrapper create state: %v", diags)
	}
	if created.ID.ValueString() != "pve two/501" || created.Hostname.ValueString() != "wrapper-ct" || created.OSTemplate.ValueString() != "local:vztmpl/debian.tar.zst" || created.RootFS.ValueString() != "local-lvm:8" || created.Status.ValueString() != "running" {
		t.Fatalf("unexpected LXC wrapper typed state: %#v", created)
	}

	readResp := resource.ReadResponse{State: createResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: createResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("LXC wrapper read diagnostics: %v", readResp.Diagnostics)
	}
	var updated lxcContainerModel
	if diags := readResp.State.Get(context.Background(), &updated); diags.HasError() {
		t.Fatalf("decode LXC wrapper read state: %v", diags)
	}
	updated.Hostname = types.StringValue("wrapper-ct-updated")
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, updated), State: readResp.State}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("LXC wrapper update diagnostics: %v", updateResp.Diagnostics)
	}
	assertStateString(t, updateResp.State, path.Root("hostname"), "wrapper-ct-updated")

	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("LXC wrapper delete diagnostics: %v", deleteResp.Diagnostics)
	}
	missingResp := resource.ReadResponse{State: updateResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: updateResp.State}, &missingResp)
	if missingResp.Diagnostics.HasError() || !missingResp.State.Raw.IsNull() {
		t.Fatalf("missing LXC wrapper was not removed: diagnostics=%v raw=%v", missingResp.Diagnostics, missingResp.State.Raw)
	}
	var idempotentDelete resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: updateResp.State}, &idempotentDelete)
	if idempotentDelete.Diagnostics.HasError() {
		t.Fatalf("idempotent LXC wrapper delete diagnostics: %v", idempotentDelete.Diagnostics)
	}
	handler.assert(t)
	wantCalls := []string{
		"POST /api2/json/nodes/pve%20two/lxc", "GET /api2/json/nodes/pve%20two/tasks/UPID:pve%20two:lxc-create-wrapper/status", "GET /api2/json/nodes/pve%20two/lxc/501/config", "GET /api2/json/nodes/pve%20two/lxc/501/status/current",
		"GET /api2/json/nodes/pve%20two/lxc/501/config", "GET /api2/json/nodes/pve%20two/lxc/501/status/current",
		"GET /api2/json/nodes/pve%20two/lxc/501/config", "PUT /api2/json/nodes/pve%20two/lxc/501/config", "GET /api2/json/nodes/pve%20two/tasks/UPID:pve%20two:lxc-update-wrapper/status", "GET /api2/json/nodes/pve%20two/lxc/501/config", "GET /api2/json/nodes/pve%20two/lxc/501/status/current",
		"DELETE /api2/json/nodes/pve%20two/lxc/501", "GET /api2/json/nodes/pve%20two/tasks/UPID:pve%20two:lxc-delete-wrapper/status", "GET /api2/json/nodes/pve%20two/lxc/501/config", "DELETE /api2/json/nodes/pve%20two/lxc/501",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected LXC wrapper call order: got %v want %v", calls, wantCalls)
	}

	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, minimalLXCContainerModel("", 0))}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "pve two/501"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("LXC wrapper import diagnostics: %v", importResp.Diagnostics)
	}
	assertStateString(t, importResp.State, path.Root("id"), "pve two/501")
	invalidImport := resource.ImportStateResponse{State: testResourceState(t, schema, minimalLXCContainerModel("", 0))}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "pve two/nope"}, &invalidImport)
	if !invalidImport.Diagnostics.HasError() {
		t.Fatalf("expected invalid LXC wrapper import diagnostics")
	}
}

func TestLXCContainerResourceCloneSelectionAndRequiredCreateValidation(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/lxc/900/clone":
			if !handler.form(w, r, url.Values{"newid": {"502"}, "node": {"target node"}, "full": {"1"}}) {
				return
			}
			handler.envelope(w, "UPID:source node:lxc-clone-wrapper")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/tasks/UPID:source%20node:lxc-clone-wrapper/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/lxc/502/config":
			handler.envelope(w, map[string]any{"hostname": "cloned-ct", "rootfs": "local-lvm:vm-502-disk-0"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/lxc/502/status/current":
			handler.envelope(w, map[string]any{"status": "stopped"})
		default:
			handler.fail(w, "unexpected LXC clone request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	res := &LXCContainerResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalLXCContainerModel("target node", 502)
	model.Clone = mustLXCContainerCloneValue(t, lxcContainerCloneModel{SourceNode: types.StringValue("source node"), SourceVMID: types.Int64Value(900), Full: types.BoolValue(true)})
	resp := testResourceCreateResponse(schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("LXC clone wrapper diagnostics: %v", resp.Diagnostics)
	}
	if want := []string{"POST /api2/json/nodes/source%20node/lxc/900/clone", "GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:lxc-clone-wrapper/status", "GET /api2/json/nodes/target%20node/lxc/502/config", "GET /api2/json/nodes/target%20node/lxc/502/status/current"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected LXC clone call order: got %v want %v", calls, want)
	}

	missing := minimalLXCContainerModel("target node", 503)
	missingResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, missing)}, &missingResp)
	if !missingResp.Diagnostics.HasError() || !containsDiagnostic(missingResp.Diagnostics, "ostemplate") || !containsDiagnostic(missingResp.Diagnostics, "rootfs") || len(calls) != 4 {
		t.Fatalf("expected required LXC create validation without HTTP calls: calls=%v diagnostics=%v", calls, missingResp.Diagnostics)
	}
	handler.assert(t)
}

func TestQemuVMResourceCreateAllocatesNextVMID(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/nextid":
			if r.URL.RawQuery != "" {
				handler.fail(w, "unexpected nextid query: %q", r.URL.RawQuery)
				return
			}
			handler.envelope(w, 105)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu":
			if !handler.form(w, r, url.Values{"vmid": {"105"}, "name": {"auto-vm"}}) {
				return
			}
			handler.envelope(w, "UPID:pve one:qemu-create-nextid")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-nextid/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/105/config":
			handler.envelope(w, map[string]any{"name": "auto-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/105/status/current":
			handler.envelope(w, map[string]any{"status": "stopped"})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("pve one", 0)
	model.VMID = types.Int64Unknown()
	model.Name = types.StringValue("auto-vm")
	resp := testResourceCreateResponse(schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("auto VMID create diagnostics: %v", resp.Diagnostics)
	}
	var created qemuVMModel
	if diags := resp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode auto VMID create state: %v", diags)
	}
	if created.VMID.ValueInt64() != 105 || created.ID.ValueString() != "pve one/105" || !created.VMIDStart.IsNull() {
		t.Fatalf("unexpected auto VMID state: %#v", created)
	}
	if len(calls) < 2 || calls[0] != "GET /api2/json/cluster/nextid" || calls[1] != "POST /api2/json/nodes/pve%20one/qemu" {
		t.Fatalf("expected nextid call before create: %v", calls)
	}
	handler.assert(t)
}

func TestQemuVMResourceCreateAllocatesFromVMIDStart(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/nextid" && r.URL.RawQuery == "vmid=200":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			if err := json.NewEncoder(w).Encode(map[string]any{"errors": map[string]string{"vmid": "VM 200 already exists"}, "data": nil}); err != nil {
				handler.fail(w, "encode response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/nextid" && r.URL.RawQuery == "vmid=201":
			handler.envelope(w, 201)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu":
			if !handler.form(w, r, url.Values{"vmid": {"201"}}) {
				return
			}
			handler.envelope(w, "UPID:pve one:qemu-create-nextid-start")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-nextid-start/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/201/config":
			handler.envelope(w, map[string]any{"name": "start-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/201/status/current":
			handler.envelope(w, map[string]any{"status": "stopped"})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("pve one", 0)
	model.VMID = types.Int64Unknown()
	model.VMIDStart = types.Int64Value(200)
	resp := testResourceCreateResponse(schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("vm_id_start create diagnostics: %v", resp.Diagnostics)
	}
	var created qemuVMModel
	if diags := resp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode vm_id_start create state: %v", diags)
	}
	if created.VMID.ValueInt64() != 201 || created.VMIDStart.ValueInt64() != 200 || created.ID.ValueString() != "pve one/201" {
		t.Fatalf("unexpected vm_id_start state: %#v", created)
	}
	handler.assert(t)
}

func TestQemuVMResourceCreateKeepsPartialStateWhenReadFails(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/cluster/nextid":
			handler.envelope(w, 105)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu":
			handler.envelope(w, "UPID:pve one:qemu-create-partial")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-partial/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/105/config":
			http.Error(w, "config read failed", http.StatusInternalServerError)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("pve one", 0)
	model.VMID = types.Int64Unknown()
	resp := testResourceCreateResponse(schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected read failure diagnostics: %v", resp.Diagnostics)
	}
	var partial qemuVMModel
	if diags := resp.State.Get(context.Background(), &partial); diags.HasError() {
		t.Fatalf("decode partial state: %v", diags)
	}
	if partial.ID.ValueString() != "pve one/105" || partial.Node.ValueString() != "pve one" || partial.VMID.ValueInt64() != 105 {
		t.Fatalf("expected tracked identity in partial state, got: %#v", partial)
	}
	if !containsDiagnostic(resp.Diagnostics, "config read failed") {
		t.Fatalf("expected config read failure diagnostic: %v", resp.Diagnostics)
	}
	handler.assert(t)
}

func TestQemuVMResourceCloneKeepsPartialStateWhenUpdateFails(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/cluster/nextid" && r.URL.RawQuery == "vmid=201":
			handler.envelope(w, 201)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/qemu/9000/clone":
			handler.envelope(w, "UPID:source node:qemu-clone-partial")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-partial/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/201/config":
			http.Error(w, "clone update failed", http.StatusInternalServerError)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("target node", 0)
	model.VMID = types.Int64Unknown()
	model.VMIDStart = types.Int64Value(201)
	model.Name = types.StringValue("cloned-vm")
	model.Clone = mustQemuVMCloneValue(t, qemuVMCloneModel{SourceNode: types.StringValue("source node"), SourceVMID: types.Int64Value(9000), Full: types.BoolValue(true)})
	resp := testResourceCreateResponse(schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected clone update failure diagnostics: %v", resp.Diagnostics)
	}
	t.Logf("DIAGS: %v", resp.Diagnostics)
	t.Logf("STATE RAW: %v", resp.State.Raw)
	var partial qemuVMModel
	if diags := resp.State.Get(context.Background(), &partial); diags.HasError() {
		t.Fatalf("decode partial state: %v", diags)
	}
	if partial.ID.ValueString() != "target node/201" || partial.VMID.ValueInt64() != 201 || partial.VMIDStart.ValueInt64() != 201 || partial.Node.ValueString() != "target node" {
		t.Fatalf("expected tracked identity in partial state, got: %#v", partial)
	}
	if !containsDiagnostic(resp.Diagnostics, "clone update failed") {
		t.Fatalf("expected clone update failure diagnostic: %v", resp.Diagnostics)
	}
	handler.assert(t)
}

func TestQemuVMResourceValidateConfigVMIDAllocation(t *testing.T) {
	res := &QemuVMResource{}
	schema := testResourceSchema(t, res)

	both := minimalQemuVMModel("pve one", 105)
	both.VMIDStart = types.Int64Value(200)
	bothResp := resource.ValidateConfigResponse{}
	res.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, schema, both)}, &bothResp)
	if !bothResp.Diagnostics.HasError() || !containsDiagnostic(bothResp.Diagnostics, "vm_id_start") {
		t.Fatalf("expected vm_id/vm_id_start conflict diagnostic: %v", bothResp.Diagnostics)
	}

	outOfRange := minimalQemuVMModel("pve one", 0)
	outOfRange.VMID = types.Int64Null()
	outOfRange.VMIDStart = types.Int64Value(99)
	rangeResp := resource.ValidateConfigResponse{}
	res.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, schema, outOfRange)}, &rangeResp)
	if !rangeResp.Diagnostics.HasError() || !containsDiagnostic(rangeResp.Diagnostics, "must be between") {
		t.Fatalf("expected vm_id_start range diagnostic: %v", rangeResp.Diagnostics)
	}

	valid := minimalQemuVMModel("pve one", 0)
	valid.VMID = types.Int64Null()
	valid.VMIDStart = types.Int64Value(200)
	validResp := resource.ValidateConfigResponse{}
	res.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, schema, valid)}, &validResp)
	if validResp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", validResp.Diagnostics)
	}

	// Unknown values may still resolve to null, so conflicts are deferred
	// instead of failing valid module configurations during validation.
	unknownVMID := minimalQemuVMModel("pve one", 0)
	unknownVMID.VMID = types.Int64Unknown()
	unknownVMID.VMIDStart = types.Int64Value(200)
	unknownResp := resource.ValidateConfigResponse{}
	res.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, schema, unknownVMID)}, &unknownResp)
	if unknownResp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics for unknown vm_id: %v", unknownResp.Diagnostics)
	}

	unknownStart := minimalQemuVMModel("pve one", 105)
	unknownStart.VMIDStart = types.Int64Unknown()
	unknownStartResp := resource.ValidateConfigResponse{}
	res.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, schema, unknownStart)}, &unknownStartResp)
	if unknownStartResp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics for unknown vm_id_start: %v", unknownStartResp.Diagnostics)
	}
}
