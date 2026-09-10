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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// testResourceCreateResponse mirrors the framework server, which initializes
// CreateResponse.State.Raw with a typed null object so that partial
// SetAttribute writes (persistQemuVMIdentity) work like they do in production,
// and which always provides initialized private state for SetKey writes
// (retained pending task UPIDs).
func testResourceCreateResponse(t *testing.T, schema resource.SchemaResponse) resource.CreateResponse {
	resp := resource.CreateResponse{
		State: tfsdk.State{
			Schema: schema.Schema,
			Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(context.Background()), nil),
		},
	}
	initializeResourcePrivate(t, &resp)
	return resp
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
	createResp := testResourceCreateResponse(t, schema)
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
		"GET /api2/json/nodes/pve%20one/qemu/401/config", "DELETE /api2/json/nodes/pve%20one/qemu/401", "GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-wrapper/status", "GET /api2/json/nodes/pve%20one/qemu/401/config", "GET /api2/json/nodes/pve%20one/qemu/401/config",
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

	tests := []struct {
		name     string
		full     types.Bool
		wantForm url.Values
	}{
		{
			name:     "full clone sends full=1",
			full:     types.BoolValue(true),
			wantForm: url.Values{"newid": {"402"}, "target": {"target node"}, "full": {"1"}},
		},
		{
			name:     "linked clone sends full=0",
			full:     types.BoolValue(false),
			wantForm: url.Values{"newid": {"402"}, "target": {"target node"}, "full": {"0"}},
		},
		{
			// Omitting `full` keeps Proxmox's own default: templates are
			// linked-cloned, normal VMs are fully copied.
			name:     "omitted full is not sent",
			full:     types.BoolNull(),
			wantForm: url.Values{"newid": {"402"}, "target": {"target node"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := &lifecycleHandler{}
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !handler.auth(w, r) {
					return
				}
				calls = append(calls, r.Method+" "+r.URL.EscapedPath())
				switch {
				case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/qemu/9000/clone":
					if !handler.form(w, r, test.wantForm) {
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
			model.Clone = mustQemuVMCloneValue(t, qemuVMCloneModel{SourceNode: types.StringValue("source node"), SourceVMID: types.Int64Value(9000), Full: test.full})
			resp := testResourceCreateResponse(t, schema)
			res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("QEMU clone wrapper diagnostics: %v", resp.Diagnostics)
			}
			if want := []string{"POST /api2/json/nodes/source%20node/qemu/9000/clone", "GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-wrapper/status", "GET /api2/json/nodes/target%20node/qemu/402/config", "GET /api2/json/nodes/target%20node/qemu/402/status/current"}; !reflect.DeepEqual(calls, want) {
				t.Fatalf("unexpected QEMU clone call order: got %v want %v", calls, want)
			}

			var stored qemuVMModel
			if diags := resp.State.Get(context.Background(), &stored); diags.HasError() {
				t.Fatalf("decode clone state: %v", diags)
			}
			if got := decodeQemuVMClone(t, stored.Clone); !got.Full.Equal(test.full) {
				t.Fatalf("expected clone.full %v in state, got %v", test.full, got.Full)
			}
			handler.assert(t)
		})
	}
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
	createResp := testResourceCreateResponse(t, schema)
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
	resp := testResourceCreateResponse(t, schema)
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
	resp := testResourceCreateResponse(t, schema)
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
	resp := testResourceCreateResponse(t, schema)
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
	resp := testResourceCreateResponse(t, schema)
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
	resp := testResourceCreateResponse(t, schema)
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

func TestQemuVMResourceCreateStartsGuestWhenConfigured(t *testing.T) {
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
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu":
			if !handler.form(w, r, url.Values{"vmid": {"410"}, "name": {"hooked-vm"}}) {
				return
			}
			handler.envelope(w, "UPID:pve one:qemu-create-hooked")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-hooked/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/410/status/start":
			handler.envelope(w, "UPID:pve one:qemu-start-hooked")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-start-hooked/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/410/config":
			handler.envelope(w, map[string]any{"name": "hooked-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/410/status/current":
			handler.envelope(w, map[string]any{"status": "running", "uptime": 5})
		default:
			handler.fail(w, "unexpected hooked create request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("pve one", 410)
	model.Name = types.StringValue("hooked-vm")
	model.StartOnCreate = types.BoolValue(true)
	model.StopOnDestroy = types.BoolValue(true)
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("hooked create diagnostics: %v", resp.Diagnostics)
	}
	var created qemuVMModel
	if diags := resp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode hooked create state: %v", diags)
	}
	if created.Status.ValueString() != "running" || !created.StartOnCreate.ValueBool() || !created.StopOnDestroy.ValueBool() {
		t.Fatalf("unexpected hooked create state: %#v", created)
	}
	wantCalls := []string{
		"POST /api2/json/nodes/pve%20one/qemu",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-hooked/status",
		"POST /api2/json/nodes/pve%20one/qemu/410/status/start",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-start-hooked/status",
		"GET /api2/json/nodes/pve%20one/qemu/410/config",
		"GET /api2/json/nodes/pve%20one/qemu/410/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected hooked create call order: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceCloneStartsGuestAfterConfigUpdate(t *testing.T) {
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
			if !handler.form(w, r, url.Values{"newid": {"411"}, "target": {"target node"}, "name": {"hooked-clone"}}) {
				return
			}
			handler.envelope(w, "UPID:source node:qemu-clone-hooked")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-hooked/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/411/config":
			handler.envelope(w, nil)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/411/status/start":
			handler.envelope(w, "UPID:target node:qemu-start-hooked")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/tasks/UPID:target%20node:qemu-start-hooked/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/411/config":
			handler.envelope(w, map[string]any{"name": "hooked-clone"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/411/status/current":
			handler.envelope(w, map[string]any{"status": "running", "uptime": 3})
		default:
			handler.fail(w, "unexpected hooked clone request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("target node", 411)
	model.Name = types.StringValue("hooked-clone")
	model.StartOnCreate = types.BoolValue(true)
	model.Clone = mustQemuVMCloneValue(t, qemuVMCloneModel{SourceNode: types.StringValue("source node"), SourceVMID: types.Int64Value(9000)})
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("hooked clone diagnostics: %v", resp.Diagnostics)
	}
	wantCalls := []string{
		"POST /api2/json/nodes/source%20node/qemu/9000/clone",
		"GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-hooked/status",
		"PUT /api2/json/nodes/target%20node/qemu/411/config",
		"POST /api2/json/nodes/target%20node/qemu/411/status/start",
		"GET /api2/json/nodes/target%20node/tasks/UPID:target%20node:qemu-start-hooked/status",
		"GET /api2/json/nodes/target%20node/qemu/411/config",
		"GET /api2/json/nodes/target%20node/qemu/411/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected hooked clone call order: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceCreateKeepsTrackedGuestWhenStartFails(t *testing.T) {
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
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu":
			handler.envelope(w, "UPID:pve one:qemu-create-startfail")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-startfail/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/412/status/start":
			handler.envelope(w, "UPID:pve one:qemu-start-startfail")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-start-startfail/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "start failed: KVM unavailable"})
		default:
			handler.fail(w, "unexpected start failure request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("pve one", 412)
	model.StartOnCreate = types.BoolValue(true)
	model.StopOnDestroy = types.BoolValue(true)
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "start failed: KVM unavailable") {
		t.Fatalf("expected start failure diagnostics: %v", resp.Diagnostics)
	}
	var partial qemuVMModel
	if diags := resp.State.Get(context.Background(), &partial); diags.HasError() {
		t.Fatalf("decode partial start-failure state: %v", diags)
	}
	if partial.ID.ValueString() != "pve one/412" || partial.VMID.ValueInt64() != 412 || !partial.StopOnDestroy.ValueBool() || !partial.StartOnCreate.ValueBool() {
		t.Fatalf("expected tracked identity and stop policy after failed start, got: %#v", partial)
	}
	wantCalls := []string{
		"POST /api2/json/nodes/pve%20one/qemu",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-startfail/status",
		"POST /api2/json/nodes/pve%20one/qemu/412/status/start",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-start-startfail/status",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("failed start must not trigger further actions: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceReadAndUpdateNeverStartOrStop(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/413/config":
			handler.envelope(w, map[string]any{"name": "observed-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/413/status/current":
			handler.envelope(w, map[string]any{"status": "stopped", "uptime": 0})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/413/config":
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected observed request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("pve one", 413)
	stateModel.Name = types.StringValue("observed-vm")
	stateModel.StartOnCreate = types.BoolValue(true)
	stateModel.StopOnDestroy = types.BoolValue(false)
	state := testResourceState(t, schema, stateModel)

	readResp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("observed read diagnostics: %v", readResp.Diagnostics)
	}
	// Refreshing a stopped guest with start_on_create set must not start it.
	wantReadCalls := []string{
		"GET /api2/json/nodes/pve%20one/qemu/413/config",
		"GET /api2/json/nodes/pve%20one/qemu/413/status/current",
	}
	if !reflect.DeepEqual(calls, wantReadCalls) {
		t.Fatalf("read must remain observed-only: got %v want %v", calls, wantReadCalls)
	}

	var refreshed qemuVMModel
	if diags := readResp.State.Get(context.Background(), &refreshed); diags.HasError() {
		t.Fatalf("decode refreshed state: %v", diags)
	}
	refreshed.Name = types.StringValue("observed-vm-renamed")
	refreshed.StopOnDestroy = types.BoolValue(true)
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, refreshed), State: readResp.State}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("observed update diagnostics: %v", updateResp.Diagnostics)
	}
	wantUpdateCalls := append(wantReadCalls[:len(wantReadCalls):len(wantReadCalls)],
		"GET /api2/json/nodes/pve%20one/qemu/413/config",
		"PUT /api2/json/nodes/pve%20one/qemu/413/config",
		"GET /api2/json/nodes/pve%20one/qemu/413/config",
		"GET /api2/json/nodes/pve%20one/qemu/413/status/current",
	)
	if !reflect.DeepEqual(calls, wantUpdateCalls) {
		t.Fatalf("update must not start or stop the guest: got %v want %v", calls, wantUpdateCalls)
	}
	var updated qemuVMModel
	if diags := updateResp.State.Get(context.Background(), &updated); diags.HasError() {
		t.Fatalf("decode updated state: %v", diags)
	}
	if !updated.StopOnDestroy.ValueBool() || !updated.StartOnCreate.ValueBool() {
		t.Fatalf("update must persist lifecycle policy changes: %#v", updated)
	}
	handler.assert(t)
}

func TestQemuVMResourceDeleteStopsRunningGuestWhenConfigured(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/414/config":
			handler.envelope(w, map[string]any{"name": "stoppable-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/414/status/current":
			handler.envelope(w, map[string]any{"status": "running", "uptime": 30})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/414/status/stop":
			handler.envelope(w, "UPID:pve one:qemu-stop-414")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-stop-414/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/414":
			handler.envelope(w, "UPID:pve one:qemu-delete-414")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-414/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected stop-on-destroy request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("pve one", 414)
	stateModel.StopOnDestroy = types.BoolValue(true)
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: testResourceState(t, schema, stateModel)}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("stop-on-destroy delete diagnostics: %v", deleteResp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/pve%20one/qemu/414/config",
		"GET /api2/json/nodes/pve%20one/qemu/414/status/current",
		"POST /api2/json/nodes/pve%20one/qemu/414/status/stop",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-stop-414/status",
		"DELETE /api2/json/nodes/pve%20one/qemu/414",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-414/status",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected stop-on-destroy call order: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceDeleteSkipsStopForStoppedGuest(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/415/config":
			handler.envelope(w, map[string]any{"name": "stopped-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/415/status/current":
			handler.envelope(w, map[string]any{"status": "stopped", "uptime": 0})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/415":
			handler.envelope(w, "UPID:pve one:qemu-delete-415")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-415/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected stopped-guest delete request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("pve one", 415)
	stateModel.StopOnDestroy = types.BoolValue(true)
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: testResourceState(t, schema, stateModel)}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("stopped-guest delete diagnostics: %v", deleteResp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/pve%20one/qemu/415/config",
		"GET /api2/json/nodes/pve%20one/qemu/415/status/current",
		"DELETE /api2/json/nodes/pve%20one/qemu/415",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-415/status",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("stopped guest must be deleted without a stop task: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceDeleteMissingGuestSucceeds(t *testing.T) {
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
		switch r.URL.EscapedPath() {
		case "/api2/json/nodes/pve%20one/qemu/416/config", "/api2/json/nodes/pve%20one/qemu/416/status/current", "/api2/json/nodes/pve%20one/qemu/416":
			http.Error(w, "VM missing", http.StatusNotFound)
		default:
			handler.fail(w, "unexpected missing-guest request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("pve one", 416)
	stateModel.StopOnDestroy = types.BoolValue(true)
	for _, wantCall := range []string{
		"GET /api2/json/nodes/pve%20one/qemu/416/config",
		// A repeated destroy after completed remote deletion succeeds without
		// any stop attempt or delete call.
		"GET /api2/json/nodes/pve%20one/qemu/416/config",
	} {
		before := len(calls)
		var deleteResp resource.DeleteResponse
		res.Delete(context.Background(), resource.DeleteRequest{State: testResourceState(t, schema, stateModel)}, &deleteResp)
		if deleteResp.Diagnostics.HasError() {
			t.Fatalf("missing-guest delete diagnostics: %v", deleteResp.Diagnostics)
		}
		if got := calls[before:]; !reflect.DeepEqual(got, []string{wantCall}) {
			t.Fatalf("missing guest must not be stopped or deleted again: got %v want %v", got, []string{wantCall})
		}
	}
	handler.assert(t)
}

func TestQemuVMResourceDeleteMissingConfig500Succeeds(t *testing.T) {
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
		if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/417/config" {
			// PVE 9 reports missing QEMU configs as HTTP 500 with this exact
			// message shape; the client classifies it as not found.
			http.Error(w, "configuration file 'nodes/pve one/qemu-server/417.conf' does not exist\n", http.StatusInternalServerError)
			return
		}
		handler.fail(w, "missing-config 500 destroy must not act further: %s %s", r.Method, r.URL.String())
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("pve one", 417)
	stateModel.StopOnDestroy = types.BoolValue(true)
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: testResourceState(t, schema, stateModel)}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("missing-config 500 delete diagnostics: %v", deleteResp.Diagnostics)
	}
	wantCalls := []string{"GET /api2/json/nodes/pve%20one/qemu/417/config"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("missing-config 500 destroy must only discover: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceDeleteAbortsWhenStopFails(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/418/config":
			handler.envelope(w, map[string]any{"name": "unstopable-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/418/status/current":
			handler.envelope(w, map[string]any{"status": "running", "uptime": 60})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/418/status/stop":
			handler.envelope(w, "UPID:pve one:qemu-stop-418")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-stop-418/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "ERROR: guest shutdown failed"})
		default:
			handler.fail(w, "unexpected stop-failure request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("pve one", 418)
	stateModel.StopOnDestroy = types.BoolValue(true)
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: testResourceState(t, schema, stateModel)}, &deleteResp)
	if !deleteResp.Diagnostics.HasError() || !containsDiagnostic(deleteResp.Diagnostics, "ERROR: guest shutdown failed") {
		t.Fatalf("expected stop failure diagnostics: %v", deleteResp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/pve%20one/qemu/418/config",
		"GET /api2/json/nodes/pve%20one/qemu/418/status/current",
		"POST /api2/json/nodes/pve%20one/qemu/418/status/stop",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-stop-418/status",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("failed stop must abort before delete: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceDeleteAbortsWhenStopTimesOut(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/419/config":
			handler.envelope(w, map[string]any{"name": "slow-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/419/status/current":
			handler.envelope(w, map[string]any{"status": "running", "uptime": 120})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/419/status/stop":
			handler.envelope(w, "UPID:pve one:qemu-stop-419")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-stop-419/status":
			handler.envelope(w, map[string]any{"status": "running", "exitstatus": ""})
		default:
			handler.fail(w, "unexpected stop-timeout request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("pve one", 419)
	stateModel.StopOnDestroy = types.BoolValue(true)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var deleteResp resource.DeleteResponse
	res.Delete(ctx, resource.DeleteRequest{State: testResourceState(t, schema, stateModel)}, &deleteResp)
	if !deleteResp.Diagnostics.HasError() || !containsDiagnostic(deleteResp.Diagnostics, "qemu-stop-419") {
		t.Fatalf("expected stop timeout diagnostics: %v", deleteResp.Diagnostics)
	}
	handler.assert(t)
}

func TestQemuVMResourceDeleteAbortsWhenStopTaskStatus404(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/423/config":
			handler.envelope(w, map[string]any{"name": "vanishing-stop-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/423/status/current":
			handler.envelope(w, map[string]any{"status": "running", "uptime": 90})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/423/status/stop":
			handler.envelope(w, "UPID:pve one:qemu-stop-423")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-stop-423/status":
			// A 404 while polling the stop task does not prove the VM
			// disappeared or that the stop succeeded, so the destroy must
			// abort instead of treating it as already-stopped.
			http.Error(w, "no such task", http.StatusNotFound)
		default:
			handler.fail(w, "unexpected stop-task-404 request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("pve one", 423)
	stateModel.StopOnDestroy = types.BoolValue(true)
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: testResourceState(t, schema, stateModel)}, &deleteResp)
	if !deleteResp.Diagnostics.HasError() || !containsDiagnostic(deleteResp.Diagnostics, "qemu-stop-423") {
		t.Fatalf("expected stop task 404 diagnostics: %v", deleteResp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/pve%20one/qemu/423/config",
		"GET /api2/json/nodes/pve%20one/qemu/423/status/current",
		"POST /api2/json/nodes/pve%20one/qemu/423/status/stop",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-stop-423/status",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("stop task 404 must abort before the VM DELETE: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceDeleteRetriesAfterInterruptedDelete(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	deleteAttempts := 0
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/420/config":
			handler.envelope(w, map[string]any{"name": "retried-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/420/status/current":
			handler.envelope(w, map[string]any{"status": "stopped", "uptime": 0})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/420":
			deleteAttempts++
			handler.envelope(w, "UPID:pve one:qemu-delete-420")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-420/status":
			if deleteAttempts == 1 {
				handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "ERROR: storage lock held"})
				return
			}
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected interrupted-delete request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("pve one", 420)
	stateModel.StopOnDestroy = types.BoolValue(true)

	interrupted := resource.DeleteResponse{}
	res.Delete(context.Background(), resource.DeleteRequest{State: testResourceState(t, schema, stateModel)}, &interrupted)
	if !interrupted.Diagnostics.HasError() || !containsDiagnostic(interrupted.Diagnostics, "ERROR: storage lock held") {
		t.Fatalf("expected interrupted delete diagnostics: %v", interrupted.Diagnostics)
	}

	retry := resource.DeleteResponse{}
	res.Delete(context.Background(), resource.DeleteRequest{State: testResourceState(t, schema, stateModel)}, &retry)
	if retry.Diagnostics.HasError() {
		t.Fatalf("retry delete diagnostics: %v", retry.Diagnostics)
	}
	handler.assert(t)
}

func TestQemuVMResourceCreateVMIDRaceDoesNotTouchWinner(t *testing.T) {
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		if r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu" {
			// The winning unrelated guest was created after the nextid probe;
			// the create POST must fail without adopting or acting on it.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			if err := json.NewEncoder(w).Encode(map[string]any{"errors": map[string]string{"vmid": "VM 421 already exists"}, "data": nil}); err != nil {
				handler.fail(w, "encode response: %v", err)
			}
			return
		}
		handler.fail(w, "VMID race must not touch the winning VM: %s %s", r.Method, r.URL.String())
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("pve one", 421)
	model.Name = types.StringValue("raced-vm")
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "already exists") {
		t.Fatalf("expected VMID collision diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatalf("collided create must not persist state: %v", resp.State.Raw)
	}
	wantCalls := []string{"POST /api2/json/nodes/pve%20one/qemu"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("VMID race must only attempt the create POST: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceCloneVMIDRaceDoesNotTouchWinner(t *testing.T) {
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		if r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/qemu/9000/clone" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			if err := json.NewEncoder(w).Encode(map[string]any{"errors": map[string]string{"newid": "VM 422 already exists"}, "data": nil}); err != nil {
				handler.fail(w, "encode response: %v", err)
			}
			return
		}
		handler.fail(w, "clone VMID race must not touch the winning VM: %s %s", r.Method, r.URL.String())
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("target node", 422)
	model.Clone = mustQemuVMCloneValue(t, qemuVMCloneModel{SourceNode: types.StringValue("source node"), SourceVMID: types.Int64Value(9000)})
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "already exists") {
		t.Fatalf("expected clone VMID collision diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatalf("collided clone must not persist state: %v", resp.State.Raw)
	}
	wantCalls := []string{"POST /api2/json/nodes/source%20node/qemu/9000/clone"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("clone VMID race must only attempt the clone POST: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

// TestQemuVMResourceCreatePollTimeoutRetainsAcceptedTask proves the accepted
// create is not lost across a polling timeout: identity and the pending UPID
// stay in state, a refresh while the task runs neither drops the resource nor
// polls anywhere but the task owner, and a settled task lets the ordinary
// read and destroy flows proceed.
func TestQemuVMResourceCreatePollTimeoutRetainsAcceptedTask(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	const upid = "UPID:pve one:qemu-create-timeout"
	var callsMu sync.Mutex
	var recorded []string
	recordCall := func(method, path string) {
		callsMu.Lock()
		defer callsMu.Unlock()
		recorded = append(recorded, method+" "+path)
	}
	takeCalls := func() []string {
		callsMu.Lock()
		defer callsMu.Unlock()
		taken := recorded
		recorded = nil
		return taken
	}
	taskRunning := atomic.Bool{}
	taskRunning.Store(true)
	exists := atomic.Bool{}
	exists.Store(false)
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		recordCall(r.Method, r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu":
			handler.envelope(w, upid)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-timeout/status":
			if taskRunning.Load() {
				handler.envelope(w, map[string]any{"status": "running"})
				return
			}
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/430/config":
			if !exists.Load() {
				http.Error(w, "VM missing", http.StatusNotFound)
				return
			}
			handler.envelope(w, map[string]any{"name": "timeout-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/430/status/current":
			handler.envelope(w, map[string]any{"status": "stopped", "uptime": 0})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/430":
			exists.Store(false)
			handler.envelope(w, "UPID:pve one:qemu-delete-timeout")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-timeout/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected poll timeout request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("pve one", 430)
	model.Name = types.StringValue("timeout-vm")

	// The create wait times out while the accepted task still runs: the
	// failure keeps the guest identity and the pending UPID for recovery.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	createResp := testResourceCreateResponse(t, schema)
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &createResp)
	if !createResp.Diagnostics.HasError() || !containsDiagnostic(createResp.Diagnostics, "qemu-create-timeout") {
		t.Fatalf("expected create poll timeout diagnostics, got %v", createResp.Diagnostics)
	}
	var partial qemuVMModel
	if diags := createResp.State.Get(context.Background(), &partial); diags.HasError() {
		t.Fatalf("decode retained create state: %v", diags)
	}
	if partial.ID.ValueString() != "pve one/430" || partial.VMID.ValueInt64() != 430 {
		t.Fatalf("expected tracked identity after poll timeout, got: %#v", partial)
	}
	retained, privateDiags := createResp.Private.GetKey(context.Background(), qemuVMPendingTaskKey)
	if privateDiags.HasError() || string(retained) != `{"upid":"UPID:pve one:qemu-create-timeout"}` {
		t.Fatalf("expected retained pending create task, got %q diags %v", retained, privateDiags)
	}
	takeCalls()

	// Refresh while the accepted task runs: state is kept unchanged and only
	// the task owner is polled, no absence decision, no guest access.
	readResp := resource.ReadResponse{State: createResp.State, Private: createResp.Private}
	res.Read(context.Background(), resource.ReadRequest{State: createResp.State, Private: createResp.Private}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("refresh during pending create diagnostics: %v", readResp.Diagnostics)
	}
	if !readResp.State.Raw.Equal(createResp.State.Raw) {
		t.Fatal("refresh during a pending create mutated state")
	}
	if calls := takeCalls(); !reflect.DeepEqual(calls, []string{"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-timeout/status"}) {
		t.Fatalf("running pending create refresh must only poll the accepted task: %v", calls)
	}

	// The task settles successfully; the refresh clears the pending task and
	// reads the guest normally.
	taskRunning.Store(false)
	exists.Store(true)
	res.Read(context.Background(), resource.ReadRequest{State: readResp.State, Private: readResp.Private}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("refresh after settled create diagnostics: %v", readResp.Diagnostics)
	}
	cleared, privateDiags := readResp.Private.GetKey(context.Background(), qemuVMPendingTaskKey)
	if privateDiags.HasError() || string(cleared) != `{}` {
		t.Fatalf("expected settled pending task to be cleared, got %q diags %v", cleared, privateDiags)
	}
	if calls := takeCalls(); !reflect.DeepEqual(calls, []string{
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-timeout/status",
		"GET /api2/json/nodes/pve%20one/qemu/430/config",
		"GET /api2/json/nodes/pve%20one/qemu/430/status/current",
	}) {
		t.Fatalf("unexpected post-settle refresh calls: %v", calls)
	}

	// Destroy after reconciliation proceeds through the ordinary delete flow.
	deleteResp := resource.DeleteResponse{State: readResp.State, Private: readResp.Private}
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State, Private: readResp.Private}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("reconciled delete diagnostics: %v", deleteResp.Diagnostics)
	}
	if calls := takeCalls(); !reflect.DeepEqual(calls, []string{
		"GET /api2/json/nodes/pve%20one/qemu/430/config",
		"DELETE /api2/json/nodes/pve%20one/qemu/430",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-timeout/status",
	}) {
		t.Fatalf("unexpected reconciled delete calls: %v", calls)
	}
	handler.assert(t)
}

// TestQemuVMResourceCloneFailedTaskRecoveredByRead proves a terminal failed
// clone task does not wedge the resource: the failure keeps the accepted task
// for reconciliation, and the refresh after the task settled clears it and
// lets the ordinary flow decide from the (absent) guest.
func TestQemuVMResourceCloneFailedTaskRecoveredByRead(t *testing.T) {
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
			handler.envelope(w, "UPID:source node:qmclone-failed")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/tasks/UPID:source%20node:qmclone-failed/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "ERROR: clone failed: storage full"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/431/config":
			http.Error(w, "VM missing", http.StatusNotFound)
		default:
			handler.fail(w, "unexpected clone failure request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("target node", 431)
	model.Clone = mustQemuVMCloneValue(t, qemuVMCloneModel{SourceNode: types.StringValue("source node"), SourceVMID: types.Int64Value(9000), Full: types.BoolValue(true)})
	createResp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &createResp)
	if !createResp.Diagnostics.HasError() || !containsDiagnostic(createResp.Diagnostics, "clone failed: storage full") {
		t.Fatalf("expected clone task failure diagnostics, got %v", createResp.Diagnostics)
	}
	var partial qemuVMModel
	if diags := createResp.State.Get(context.Background(), &partial); diags.HasError() {
		t.Fatalf("decode retained clone state: %v", diags)
	}
	if partial.ID.ValueString() != "target node/431" {
		t.Fatalf("expected tracked clone identity after task failure, got: %#v", partial)
	}
	retained, privateDiags := createResp.Private.GetKey(context.Background(), qemuVMPendingTaskKey)
	if privateDiags.HasError() || string(retained) != `{"upid":"UPID:source node:qmclone-failed"}` {
		t.Fatalf("expected retained pending clone task, got %q diags %v", retained, privateDiags)
	}

	// The refresh polls the failed task on the source node, clears it, and
	// removes the absent guest so a later apply can reallocate.
	readResp := resource.ReadResponse{State: createResp.State, Private: createResp.Private}
	res.Read(context.Background(), resource.ReadRequest{State: createResp.State, Private: createResp.Private}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("refresh after failed clone diagnostics: %v", readResp.Diagnostics)
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("absent guest after failed clone was not removed: %v", readResp.State.Raw)
	}
	cleared, privateDiags := readResp.Private.GetKey(context.Background(), qemuVMPendingTaskKey)
	if privateDiags.HasError() || string(cleared) != `{}` {
		t.Fatalf("expected settled pending task to be cleared, got %q diags %v", cleared, privateDiags)
	}
	wantCalls := []string{
		"POST /api2/json/nodes/source%20node/qemu/9000/clone",
		"GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:qmclone-failed/status",
		"GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:qmclone-failed/status",
		"GET /api2/json/nodes/target%20node/qemu/431/config",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected clone failure call order: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

// TestQemuVMResourceDeleteAwaitsPendingCreateTask proves destroy does not
// race an accepted create task: while the task runs the destroy polls only
// the task owner and aborts on wait failure without touching the guest; once
// the task settles the ordinary delete flow runs.
func TestQemuVMResourceDeleteAwaitsPendingCreateTask(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	var callsMu sync.Mutex
	var recorded []string
	recordCall := func(method, path string) {
		callsMu.Lock()
		defer callsMu.Unlock()
		recorded = append(recorded, method+" "+path)
	}
	takeCalls := func() []string {
		callsMu.Lock()
		defer callsMu.Unlock()
		taken := recorded
		recorded = nil
		return taken
	}
	taskRunning := atomic.Bool{}
	taskRunning.Store(true)
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		recordCall(r.Method, r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-432/status":
			if taskRunning.Load() {
				handler.envelope(w, map[string]any{"status": "running"})
				return
			}
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/432/config":
			handler.envelope(w, map[string]any{"name": "pending-vm"})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/432":
			handler.envelope(w, "UPID:pve one:qemu-delete-432")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-432/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected pending delete request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("pve one", 432)
	state := testResourceState(t, schema, stateModel)
	reqPrivate := resource.DeleteResponse{}
	initializeResourcePrivate(t, &reqPrivate)
	if diags := reqPrivate.Private.SetKey(context.Background(), qemuVMPendingTaskKey, []byte(`{"upid":"UPID:pve one:qemu-create-432"}`)); diags.HasError() {
		t.Fatalf("seed pending task private state: %v", diags)
	}

	// First attempt: the pending create still runs and the wait is
	// interrupted, so the destroy must abort without any guest access.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	interrupted := resource.DeleteResponse{}
	initializeResourcePrivate(t, &interrupted)
	res.Delete(ctx, resource.DeleteRequest{State: state, Private: reqPrivate.Private}, &interrupted)
	if !interrupted.Diagnostics.HasError() || !containsDiagnostic(interrupted.Diagnostics, "qemu-create-432") {
		t.Fatalf("expected pending create wait diagnostics, got %v", interrupted.Diagnostics)
	}
	calls := takeCalls()
	if len(calls) == 0 {
		t.Fatal("expected pending task polls")
	}
	for _, call := range calls {
		if call != "GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-432/status" {
			t.Fatalf("interrupted destroy must only poll the accepted task: %v", calls)
		}
	}

	// Retry after the task settled: cleanup proceeds through the ordinary flow.
	taskRunning.Store(false)
	retry := resource.DeleteResponse{}
	initializeResourcePrivate(t, &retry)
	res.Delete(context.Background(), resource.DeleteRequest{State: state, Private: reqPrivate.Private}, &retry)
	if retry.Diagnostics.HasError() {
		t.Fatalf("retry delete diagnostics: %v", retry.Diagnostics)
	}
	handler.assert(t)
	wantCalls := []string{
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-432/status",
		"GET /api2/json/nodes/pve%20one/qemu/432/config",
		"DELETE /api2/json/nodes/pve%20one/qemu/432",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-432/status",
	}
	if got := takeCalls(); !reflect.DeepEqual(got, wantCalls) {
		t.Fatalf("unexpected retry delete call order: got %v want %v", got, wantCalls)
	}
}

// TestQemuVMResourceDeleteSettledFailedTaskPermitsDirectDestroy proves an
// already settled failed retained create/clone task no longer blocks destroy:
// one owner-node poll classifies the task, then the ordinary cleanup flow
// decides from the guest itself, without an intervening Read.
func TestQemuVMResourceDeleteSettledFailedTaskPermitsDirectDestroy(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	t.Run("create task failed with guest remaining", func(t *testing.T) {
		handler := &lifecycleHandler{}
		var calls []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !handler.auth(w, r) {
				return
			}
			calls = append(calls, r.Method+" "+r.URL.EscapedPath())
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-459/status":
				handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "storage task failed"})
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/459/config":
				handler.envelope(w, map[string]any{"name": "partial-vm"})
			case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/459":
				handler.envelope(w, "UPID:pve one:qemu-delete-459")
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-459/status":
				handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
			default:
				handler.fail(w, "unexpected settled create delete request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		res := &QemuVMResource{client: testLifecycleClient(t, server)}
		schema := testResourceSchema(t, res)
		state := testResourceState(t, schema, minimalQemuVMModel("pve one", 459))
		reqPrivate := resource.DeleteResponse{}
		initializeResourcePrivate(t, &reqPrivate)
		if diags := reqPrivate.Private.SetKey(context.Background(), qemuVMPendingTaskKey, []byte(`{"upid":"UPID:pve one:qemu-create-459"}`)); diags.HasError() {
			t.Fatalf("seed pending task private state: %v", diags)
		}
		deleteResp := resource.DeleteResponse{}
		initializeResourcePrivate(t, &deleteResp)
		res.Delete(context.Background(), resource.DeleteRequest{State: state, Private: reqPrivate.Private}, &deleteResp)
		if deleteResp.Diagnostics.HasError() {
			t.Fatalf("settled failed create task blocked direct destroy: %v", deleteResp.Diagnostics)
		}
		wantCalls := []string{
			"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-459/status",
			"GET /api2/json/nodes/pve%20one/qemu/459/config",
			"DELETE /api2/json/nodes/pve%20one/qemu/459",
			"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-459/status",
		}
		if !reflect.DeepEqual(calls, wantCalls) {
			t.Fatalf("unexpected settled create delete calls: got %v want %v", calls, wantCalls)
		}
		cleared, privateDiags := deleteResp.Private.GetKey(context.Background(), qemuVMPendingTaskKey)
		if privateDiags.HasError() || string(cleared) != `{}` {
			t.Fatalf("expected settled pending task to be cleared, got %q diags %v", cleared, privateDiags)
		}
		handler.assert(t)
	})

	t.Run("clone task failed with guest absent", func(t *testing.T) {
		handler := &lifecycleHandler{}
		var calls []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !handler.auth(w, r) {
				return
			}
			calls = append(calls, r.Method+" "+r.URL.EscapedPath())
			switch {
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/tasks/UPID:source%20node:qmclone-failed-460/status":
				// The failed clone task is polled on its owner, the source node.
				handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "ERROR: clone failed: storage full"})
			case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/460/config":
				http.Error(w, "VM missing", http.StatusNotFound)
			default:
				handler.fail(w, "unexpected settled clone delete request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()

		res := &QemuVMResource{client: testLifecycleClient(t, server)}
		schema := testResourceSchema(t, res)
		state := testResourceState(t, schema, minimalQemuVMModel("target node", 460))
		reqPrivate := resource.DeleteResponse{}
		initializeResourcePrivate(t, &reqPrivate)
		if diags := reqPrivate.Private.SetKey(context.Background(), qemuVMPendingTaskKey, []byte(`{"upid":"UPID:source node:qmclone-failed-460"}`)); diags.HasError() {
			t.Fatalf("seed pending task private state: %v", diags)
		}
		deleteResp := resource.DeleteResponse{}
		initializeResourcePrivate(t, &deleteResp)
		res.Delete(context.Background(), resource.DeleteRequest{State: state, Private: reqPrivate.Private}, &deleteResp)
		if deleteResp.Diagnostics.HasError() {
			t.Fatalf("settled failed clone task blocked direct destroy: %v", deleteResp.Diagnostics)
		}
		wantCalls := []string{
			"GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:qmclone-failed-460/status",
			"GET /api2/json/nodes/target%20node/qemu/460/config",
		}
		if !reflect.DeepEqual(calls, wantCalls) {
			t.Fatalf("absent guest must end destroy idempotently without a DELETE: got %v want %v", calls, wantCalls)
		}
		cleared, privateDiags := deleteResp.Private.GetKey(context.Background(), qemuVMPendingTaskKey)
		if privateDiags.HasError() || string(cleared) != `{}` {
			t.Fatalf("expected settled pending task to be cleared, got %q diags %v", cleared, privateDiags)
		}
		handler.assert(t)
	})
}

// TestQemuVMResourceDeleteFailedWaitPreventsPrematureDeletion proves the
// classify-then-await contract stays conservative for active work: while the
// retained create task still runs at classification, a task that fails during
// the await aborts the destroy without touching the guest; the retry then
// classifies the settled failed task and cleanup proceeds.
func TestQemuVMResourceDeleteFailedWaitPreventsPrematureDeletion(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	var callsMu sync.Mutex
	var recorded []string
	recordCall := func(method, path string) {
		callsMu.Lock()
		defer callsMu.Unlock()
		recorded = append(recorded, method+" "+path)
	}
	takeCalls := func() []string {
		callsMu.Lock()
		defer callsMu.Unlock()
		taken := recorded
		recorded = nil
		return taken
	}
	statusPolls := atomic.Int64{}
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		recordCall(r.Method, r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-461/status":
			if statusPolls.Add(1) == 1 {
				handler.envelope(w, map[string]any{"status": "running"})
				return
			}
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "ERROR: create failed"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/461/config":
			handler.envelope(w, map[string]any{"name": "failed-create-vm"})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/461":
			handler.envelope(w, "UPID:pve one:qemu-delete-461")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-461/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected failed-wait delete request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := testResourceState(t, schema, minimalQemuVMModel("pve one", 461))
	reqPrivate := resource.DeleteResponse{}
	initializeResourcePrivate(t, &reqPrivate)
	if diags := reqPrivate.Private.SetKey(context.Background(), qemuVMPendingTaskKey, []byte(`{"upid":"UPID:pve one:qemu-create-461"}`)); diags.HasError() {
		t.Fatalf("seed pending task private state: %v", diags)
	}

	// First attempt: classification sees the task running, the await then
	// observes it fail, and the destroy must abort without guest access.
	first := resource.DeleteResponse{}
	initializeResourcePrivate(t, &first)
	res.Delete(context.Background(), resource.DeleteRequest{State: state, Private: reqPrivate.Private}, &first)
	if !first.Diagnostics.HasError() || !containsDiagnostic(first.Diagnostics, "qemu-create-461") || !containsDiagnostic(first.Diagnostics, "create failed") {
		t.Fatalf("expected failed pending create wait diagnostics, got %v", first.Diagnostics)
	}
	if calls := takeCalls(); !reflect.DeepEqual(calls, []string{
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-461/status",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-461/status",
	}) {
		t.Fatalf("failed wait must abort before any guest access: %v", calls)
	}

	// Retry: the task is now already settled, so the direct destroy proceeds.
	retry := resource.DeleteResponse{}
	initializeResourcePrivate(t, &retry)
	res.Delete(context.Background(), resource.DeleteRequest{State: state, Private: reqPrivate.Private}, &retry)
	if retry.Diagnostics.HasError() {
		t.Fatalf("retry after settled failed create diagnostics: %v", retry.Diagnostics)
	}
	if calls := takeCalls(); !reflect.DeepEqual(calls, []string{
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-461/status",
		"GET /api2/json/nodes/pve%20one/qemu/461/config",
		"DELETE /api2/json/nodes/pve%20one/qemu/461",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-461/status",
	}) {
		t.Fatalf("unexpected retry delete calls: %v", calls)
	}
	handler.assert(t)
}

// TestQemuVMResourceDeleteTaskStatus404KeepsState proves a 404 while polling
// the accepted delete task is not a missing VM: the destroy must fail with
// the task identity, keep the state retryable, and never re-issue the DELETE
// on the false success path.
func TestQemuVMResourceDeleteTaskStatus404KeepsState(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/433/config":
			handler.envelope(w, map[string]any{"name": "vanishing-task-vm"})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/433":
			handler.envelope(w, "UPID:pve one:qemu-delete-433")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-433/status":
			// A missing task status proves nothing about the guest deletion.
			http.Error(w, "no such task", http.StatusNotFound)
		default:
			handler.fail(w, "unexpected delete-task-404 request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("pve one", 433)
	state := testResourceState(t, schema, stateModel)
	deleteResp := resource.DeleteResponse{State: state}
	res.Delete(context.Background(), resource.DeleteRequest{State: state}, &deleteResp)
	if !deleteResp.Diagnostics.HasError() || !containsDiagnostic(deleteResp.Diagnostics, "qemu-delete-433") {
		t.Fatalf("expected delete task 404 diagnostics, got %v", deleteResp.Diagnostics)
	}
	if deleteResp.State.Raw.IsNull() {
		t.Fatal("unverified VM delete must keep the state for a retry")
	}
	wantCalls := []string{
		"GET /api2/json/nodes/pve%20one/qemu/433/config",
		"DELETE /api2/json/nodes/pve%20one/qemu/433",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-delete-433/status",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("delete task 404 must abort after one DELETE attempt: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

// TestQemuVMResourceDeleteRejectsUnverifiedAcknowledgement proves a destroy
// whose DELETE request is acknowledged with HTTP 200 but no usable UPID fails
// and keeps the state retryable: the async endpoint must name its task, so an
// empty or malformed acknowledgement is never a completed deletion and no
// task poll may run against it.
func TestQemuVMResourceDeleteRejectsUnverifiedAcknowledgement(t *testing.T) {
	tests := []struct {
		name string
		ack  any
	}{
		{"null data", nil},
		{"empty string", ""},
		{"malformed upid", "not-a-upid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := &lifecycleHandler{}
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !handler.auth(w, r) {
					return
				}
				calls = append(calls, r.Method+" "+r.URL.EscapedPath())
				switch {
				case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/434/config":
					handler.envelope(w, map[string]any{"name": "unverified-delete-vm"})
				case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/434":
					handler.envelope(w, test.ack)
				default:
					handler.fail(w, "unverified acknowledgement must not poll: %s %s", r.Method, r.URL.String())
				}
			}))
			defer server.Close()

			res := &QemuVMResource{client: testLifecycleClient(t, server)}
			schema := testResourceSchema(t, res)
			state := testResourceState(t, schema, minimalQemuVMModel("pve one", 434))
			deleteResp := resource.DeleteResponse{State: state}
			res.Delete(context.Background(), resource.DeleteRequest{State: state}, &deleteResp)
			if !deleteResp.Diagnostics.HasError() || !containsDiagnostic(deleteResp.Diagnostics, "UPID") {
				t.Fatalf("expected unverified delete acknowledgement diagnostics, got %v", deleteResp.Diagnostics)
			}
			if deleteResp.State.Raw.IsNull() {
				t.Fatal("unverified VM delete must keep the state for a retry")
			}
			wantCalls := []string{
				"GET /api2/json/nodes/pve%20one/qemu/434/config",
				"DELETE /api2/json/nodes/pve%20one/qemu/434",
			}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("invalid delete acknowledgement must not poll task status: got %v want %v", calls, wantCalls)
			}
			handler.assert(t)
		})
	}
}

// TestQemuVMUpdateKeepsPlanDiskKeysInState proves the post-update read uses
// the plan's intended disk key set: a newly declared managed slot added by
// the update survives in state, and the template's inherited system disk
// stays out of the managed disk map while its wire config remains untouched.
func TestQemuVMUpdateKeepsPlanDiskKeysInState(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	configUpdated := false
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/460/config":
			if configUpdated {
				handler.envelope(w, map[string]any{
					"ide2":  "local:iso/seed-gen2.iso,media=cdrom",
					"sata3": "local-lvm:vm-460-disk-3,size=32G,media=disk",
					"scsi0": "local-lvm:vm-460-disk-0,size=8G",
				})
				return
			}
			handler.envelope(w, map[string]any{
				"ide2":  "local:iso/seed-gen1.iso,media=cdrom",
				"scsi0": "local-lvm:vm-460-disk-0,size=8G",
			})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/460/config":
			if !handler.form(w, r, url.Values{
				"ide2":  {"local:iso/seed-gen2.iso,media=cdrom"},
				"sata3": {"local-lvm:32G,media=disk"},
			}) {
				return
			}
			// The exact-form assertion above already proves the PUT carries
			// only the plan's managed slots; the inherited scsi0 is absent.
			configUpdated = true
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/460/status/current":
			handler.envelope(w, map[string]any{"status": "stopped"})
		default:
			handler.fail(w, "unexpected disk plan update request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("target node", 460)
	stateModel.ID = types.StringValue("target node/460")
	stateModel.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{
		"ide2": cdromDiskEntry("local:iso/seed-gen1.iso"),
	})
	planModel := minimalQemuVMModel("target node", 460)
	planModel.ID = types.StringValue("target node/460")
	planModel.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{
		"ide2":  cdromDiskEntry("local:iso/seed-gen2.iso"),
		"sata3": {Storage: types.StringValue("local-lvm"), Size: types.StringValue("32G"), Media: types.StringValue("disk")},
	})

	updateState := tfsdk.State{Schema: schema.Schema}
	if diags := updateState.Set(context.Background(), stateModel); diags.HasError() {
		t.Fatalf("encode update state: %v", diags)
	}
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, planModel), State: updateState}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("disk plan update diagnostics: %v", updateResp.Diagnostics)
	}

	var updated qemuVMModel
	if diags := updateResp.State.Get(context.Background(), &updated); diags.HasError() {
		t.Fatalf("decode updated state: %v", diags)
	}
	disks := decodeQemuVMDiskMap(t, updated.Disk)
	if len(disks) != 2 {
		t.Fatalf("expected exactly the plan's managed slots in state, got %v", disks)
	}
	if got := disks["ide2"]; got.Volume.ValueString() != "local:iso/seed-gen2.iso" {
		t.Fatalf("updated managed slot must carry the observed wire value, got %#v", got)
	}
	if got := disks["sata3"]; got.Volume.ValueString() != "local-lvm:vm-460-disk-3" {
		t.Fatalf("slot added by the plan must survive the update in state, got %#v", got)
	}
	handler.assert(t)
}
