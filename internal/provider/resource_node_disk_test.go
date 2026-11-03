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

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const nodeDiskTaskUPID = "UPID:pve-1:0000:0000:0000:zfscreate:::"

func TestClientNodeDiskForms(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/disks/zfs" && r.Method == http.MethodPost:
			if !handler.form(w, r, url.Values{"name": {"tank"}, "devices": {"/dev/sdb,/dev/sdc"}, "raidlevel": {"mirror"}, "ashift": {"12"}, "add_storage": {"1"}}) {
				return
			}
			handler.envelope(w, nodeDiskTaskUPID)
		case strings.HasPrefix(r.URL.Path, "/api2/json/nodes/pve-1/tasks/"):
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.URL.Path == "/api2/json/nodes/pve-1/disks/zfs/tank" && r.Method == http.MethodDelete:
			if !handler.form(w, r, url.Values{"cleanup-config": {"1"}, "cleanup-disks": {"1"}}) {
				return
			}
			handler.envelope(w, nodeDiskTaskUPID)
		case r.URL.Path == "/api2/json/nodes/pve-1/disks/lvm" && r.Method == http.MethodPost:
			if !handler.form(w, r, url.Values{"name": {"vg"}, "device": {"/dev/sdb"}, "add_storage": {"0"}}) {
				return
			}
			handler.envelope(w, nodeDiskTaskUPID)
		case r.URL.Path == "/api2/json/nodes/pve-1/disks/initgpt" && r.Method == http.MethodPost:
			if !handler.form(w, r, url.Values{"disk": {"/dev/sdb"}, "uuid": {"11111111-2222-3333-4444-555555555555"}}) {
				return
			}
			handler.envelope(w, nodeDiskTaskUPID)
		case r.URL.Path == "/api2/json/nodes/pve-1/disks/wipedisk" && r.Method == http.MethodPut:
			if !handler.form(w, r, url.Values{"disk": {"/dev/sdb"}}) {
				return
			}
			handler.envelope(w, nodeDiskTaskUPID)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)

	upid, err := client.CreateNodeZFS(ctx, "pve-1", "tank", []string{"/dev/sdb", "/dev/sdc"}, "mirror", map[string]string{"ashift": "12", "add_storage": "1"})
	if err != nil || upid == "" {
		t.Fatalf("CreateNodeZFS() = %q, %v", upid, err)
	}
	if _, err := client.DeleteNodeDisk(ctx, "pve-1", "zfs", "tank", "", true); err != nil {
		t.Fatalf("DeleteNodeDisk() unexpected error: %v", err)
	}
	if _, err := client.CreateNodeLVM(ctx, "pve-1", "vg", "/dev/sdb", false); err != nil {
		t.Fatalf("CreateNodeLVM() unexpected error: %v", err)
	}
	if _, err := client.InitNodeGPT(ctx, "pve-1", "/dev/sdb", "11111111-2222-3333-4444-555555555555"); err != nil {
		t.Fatalf("InitNodeGPT() unexpected error: %v", err)
	}
	if _, err := client.WipeNodeDisk(ctx, "pve-1", "/dev/sdb"); err != nil {
		t.Fatalf("WipeNodeDisk() unexpected error: %v", err)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

func TestNodeDiskValidation(t *testing.T) {
	zfs := nodeDiskZFSPoolModel{
		Node: types.StringValue("pve-1"), Name: types.StringValue("tank"), RaidLevel: types.StringValue("mirror"),
		Devices: types.SetValueMust(types.StringType, []attr.Value{types.StringValue("/dev/sdb"), types.StringValue("/dev/sdc")}),
	}
	var resp resource.ValidateConfigResponse
	(&NodeDiskZFSResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &NodeDiskZFSResource{}), zfs)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("valid zfs diagnostics: %v", resp.Diagnostics)
	}
	invalid := map[string]nodeDiskZFSPoolModel{
		"bad raid":        {Node: types.StringValue("pve-1"), Name: types.StringValue("tank"), RaidLevel: types.StringValue("raid"), Devices: zfs.Devices},
		"bad compression": {Node: types.StringValue("pve-1"), Name: types.StringValue("tank"), RaidLevel: types.StringValue("single"), Compression: types.StringValue("brotli"), Devices: zfs.Devices},
	}
	invalid["single with two devices"] = nodeDiskZFSPoolModel{Node: types.StringValue("pve-1"), Name: types.StringValue("tank"), RaidLevel: types.StringValue("single"), Devices: zfs.Devices}
	invalid["mirror with one device"] = nodeDiskZFSPoolModel{Node: types.StringValue("pve-1"), Name: types.StringValue("tank"), RaidLevel: types.StringValue("mirror"), Devices: types.SetValueMust(types.StringType, []attr.Value{types.StringValue("/dev/sdb")})}
	for name, config := range invalid {
		var diags resource.ValidateConfigResponse
		(&NodeDiskZFSResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &NodeDiskZFSResource{}), config)}, &diags)
		if !diags.Diagnostics.HasError() {
			t.Fatalf("%s: expected diagnostics", name)
		}
	}
	lvm := nodeDiskLVMModel{Node: types.StringValue("pve-1"), Name: types.StringValue("vg"), Device: types.StringValue("/dev/sdb")}
	resp = resource.ValidateConfigResponse{}
	(&NodeDiskLVMResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &NodeDiskLVMResource{}), lvm)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("valid lvm diagnostics: %v", resp.Diagnostics)
	}
	lvm.Device = types.StringValue("sdb")
	resp = resource.ValidateConfigResponse{}
	(&NodeDiskLVMResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &NodeDiskLVMResource{}), lvm)}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("relative device accepted")
	}
}

func TestNodeGPTInitLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.Path == "/api2/json/nodes/pve-1/disks/initgpt" {
			handler.envelope(w, nodeDiskTaskUPID)
			return
		}
		handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
	}))
	defer server.Close()

	ctx := context.Background()
	res := &NodeGPTInitResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := nodeDiskCommandModel{ID: types.StringNull(), Node: types.StringValue("pve-1"), Disk: types.StringValue("/dev/sdb"), UUID: types.StringNull(), Triggers: testStringMap(t, map[string]string{"x": "1"}), UPID: types.StringNull()}
	createResp := resource.CreateResponse{State: testResourceState(t, schema, config)}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("init diagnostics: %v", createResp.Diagnostics)
	}
	var state nodeDiskCommandModel
	if diags := createResp.State.Get(ctx, &state); diags.HasError() || state.UPID.ValueString() != nodeDiskTaskUPID || state.ID.ValueString() != "pve-1//dev/sdb" {
		t.Fatalf("unexpected state: %#v (%v)", state, diags)
	}
}

func TestNodeDisksDataSourceProjectsAndSorts(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/disks/list": []map[string]any{
			{"devpath": "/dev/sdc", "size": 100, "used": "unused", "gpt": 1},
			{"devpath": "/dev/sdb", "size": 200, "used": "LVM", "model": "disk"},
		},
	}
	readResp := runSDNDataSource(t, NewNodeDisksDataSource(), responses, map[string]any{"node": "pve-1"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state nodeDisksModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() || len(state.Disks) != 2 || state.Disks[0].DevPath.ValueString() != "/dev/sdb" || !state.Disks[1].GPT.ValueBool() {
		t.Fatalf("unexpected projection: %#v (%v)", state.Disks, diags)
	}
}

func TestNodeDiskSmartDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/disks/smart?disk=%2Fdev%2Fsdb": map[string]any{
			"health": "PASSED", "type": "ata", "text": "raw", "attributes": map[string]any{"1": map[string]any{"value": 0}},
		},
	}
	readResp := runSDNDataSource(t, NewNodeDiskSmartDataSource(), responses, map[string]any{"node": "pve-1", "disk": "/dev/sdb"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state nodeDiskSmartModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() || state.Health.ValueString() != "PASSED" || len(state.Attributes.Elements()) != 1 {
		t.Fatalf("unexpected projection: %#v (%v)", state, diags)
	}
}
