// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func cdromDiskEntry(volume string) qemuVMDiskModel {
	return qemuVMDiskModel{Media: types.StringValue("cdrom"), Volume: types.StringValue(volume)}
}

func cloneModelWithCDROM(t *testing.T, vmID int64, volume string) qemuVMModel {
	model := minimalQemuVMModel("target node", vmID)
	model.Name = types.StringValue("chain-vm")
	model.Clone = mustQemuVMCloneValue(t, qemuVMCloneModel{SourceNode: types.StringValue("source node"), SourceVMID: types.Int64Value(9000)})
	model.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry(volume)})
	return model
}

// cloneModelWithMarkedCDROM declares ide2 as the NoCloud seed slot through the
// explicit marker, mirroring the documented seed attachment workflow.
func cloneModelWithMarkedCDROM(t *testing.T, vmID int64, volume string) qemuVMModel {
	model := cloneModelWithCDROM(t, vmID, volume)
	model.NoCloudCDROMSlot = types.StringValue("ide2")
	return model
}

// TestQemuVMAndNoCloudISOOrderedChain walks the full provisioning and destroy
// chain against one mock API: generation seed upload (on the ISO resource's
// node and task owner), clone of a cloud-init template on the VM node with
// the inherited Proxmox cloud-init drive replaced by the seed in the same
// slot, configured start, hard-power-off stop plus delete, and seed cleanup
// strictly after the VM is gone.
func TestQemuVMAndNoCloudISOOrderedChain(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	seedFileExists := false
	configUpdated := false
	handler := &lifecycleHandler{}
	var calls []string
	uploadPath := "/api2/json/nodes/pve%20one/storage/local%20iso/upload"
	contentPath := "/api2/json/nodes/pve%20one/storage/local%20iso/content"
	deletePath := contentPath + "/local%20iso:iso%2Fseed%20gen1.iso"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/storage":
			handler.envelope(w, noCloudStorageEntries(noCloudLifecycleStorage))
		case r.Method == http.MethodGet && (r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage"):
			handler.envelope(w, []map[string]any{
				{"storage": noCloudLifecycleStorage, "type": "dir", "content": "iso,vztmpl", "active": 1, "enabled": 1, "shared": 0},
				{"storage": "local-lvm", "type": "lvmpool", "content": "images,rootdir", "active": 1, "enabled": 1, "shared": 0},
			})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == contentPath:
			if r.URL.Query().Get("content") != "iso" {
				handler.fail(w, "unexpected content query: %s", r.URL.RawQuery)
				return
			}
			items := []any{}
			if seedFileExists {
				items = append(items, map[string]any{"volid": noCloudLifecycleVolumeID, "format": "iso", "size": 393216, "content": "iso"})
			}
			handler.envelope(w, items)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage/local%20iso/content":
			// The marked seed is verified against the VM node's authoritative
			// ISO collection before the clone starts.
			if r.URL.Query().Get("content") != "iso" {
				handler.fail(w, "unexpected VM node content query: %s", r.URL.RawQuery)
				return
			}
			items := []any{}
			if seedFileExists {
				items = append(items, map[string]any{"volid": noCloudLifecycleVolumeID, "format": "iso", "size": 393216, "content": "iso"})
			}
			handler.envelope(w, items)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == uploadPath:
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				handler.fail(w, "parse upload form: %v", err)
				return
			}
			seedFileExists = true
			handler.envelope(w, noCloudLifecycleUploadID)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-two/tasks/UPID:pve-two:imgcopy-1/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/qemu/9000/clone":
			if !handler.form(w, r, url.Values{"newid": {"430"}, "target": {"target node"}, "name": {"chain-vm"}}) {
				return
			}
			handler.envelope(w, "UPID:source node:qemu-clone-430")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-430/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/430/config":
			if configUpdated {
				handler.envelope(w, map[string]any{
					"name": "chain-vm",
					"ide2": noCloudLifecycleVolumeID + ",media=cdrom",
				})
				return
			}
			// The clone inherits the template's Proxmox cloud-init drive in
			// ide2 with extra grammar the typed disk parser does not know;
			// the wire-level check must still recognize and replace it.
			handler.envelope(w, map[string]any{
				"name":  "chain-vm",
				"ide2":  "local:vm-430-cloudinit,media=cdrom,backend=unexpected",
				"scsi0": "local-lvm:vm-430-disk-0,size=8G",
			})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/430/config":
			if !handler.form(w, r, url.Values{"name": {"chain-vm"}, "ide2": {noCloudLifecycleVolumeID + ",media=cdrom"}}) {
				return
			}
			configUpdated = true
			handler.envelope(w, nil)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/430/status/start":
			handler.envelope(w, "UPID:target node:qemu-start-430")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/tasks/UPID:target%20node:qemu-start-430/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/430/status/stop":
			handler.envelope(w, "UPID:target node:qemu-stop-430")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/tasks/UPID:target%20node:qemu-stop-430/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/430/status/current":
			handler.envelope(w, map[string]any{"status": "running", "uptime": 5})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/430":
			handler.envelope(w, "UPID:target node:qemu-delete-430")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/tasks/UPID:target%20node:qemu-delete-430/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == deletePath:
			seedFileExists = false
			handler.envelope(w, noCloudLifecycleDeleteID)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:delete-1/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected chain request: %s %s raw=%q", r.Method, r.URL.String(), r.URL.RawPath)
		}
	}))
	defer server.Close()

	// 1. The generation seed is uploaded before any VM exists.
	isoResource := &NoCloudISOResource{client: testLifecycleClient(t, server)}
	isoSchema := testResourceSchema(t, isoResource)
	isoCreate := resource.CreateResponse{State: tfsdk.State{Schema: isoSchema.Schema}}
	initializeResourcePrivate(t, &isoCreate)
	isoResource.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, isoSchema, noCloudLifecycleModel())}, &isoCreate)
	if isoCreate.Diagnostics.HasError() {
		t.Fatalf("nocloud create diagnostics: %v", isoCreate.Diagnostics)
	}

	// 2. The VM clones the template, safely replaces the inherited
	// Proxmox cloud-init drive with the marked seed, and starts once
	// configured. The marker makes the provider verify the exact seed volume
	// in the VM node's ISO collection before cloning.
	vmResource := &QemuVMResource{client: testLifecycleClient(t, server)}
	vmSchema := testResourceSchema(t, vmResource)
	vmModel := cloneModelWithMarkedCDROM(t, 430, noCloudLifecycleVolumeID)
	vmModel.StartOnCreate = types.BoolValue(true)
	vmModel.StopOnDestroy = types.BoolValue(true)
	vmCreate := testResourceCreateResponse(t, vmSchema)
	vmResource.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, vmSchema, vmModel)}, &vmCreate)
	if vmCreate.Diagnostics.HasError() {
		t.Fatalf("qemu vm create diagnostics: %v", vmCreate.Diagnostics)
	}
	var created qemuVMModel
	if diags := vmCreate.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode qemu vm create state: %v", diags)
	}
	if created.ID.ValueString() != "target node/430" || created.Status.ValueString() != "running" {
		t.Fatalf("unexpected chain create state: %#v", created)
	}
	var disks map[string]qemuVMDiskModel
	if diags := created.Disk.ElementsAs(context.Background(), &disks, false); diags.HasError() {
		t.Fatalf("decode chain disk state: %v", diags)
	}
	if disks["ide2"].Volume.ValueString() != noCloudLifecycleVolumeID || disks["ide2"].Media.ValueString() != "cdrom" {
		t.Fatalf("expected attached seed in ide2 state, got %#v", disks["ide2"])
	}

	// 3. Destroy stops (hard power-off) and deletes the VM before the seed.
	vmDelete := resource.DeleteResponse{}
	vmResource.Delete(context.Background(), resource.DeleteRequest{State: vmCreate.State}, &vmDelete)
	if vmDelete.Diagnostics.HasError() {
		t.Fatalf("qemu vm delete diagnostics: %v", vmDelete.Diagnostics)
	}

	// 4. Only then is the seed cleaned up.
	isoDelete := resource.DeleteResponse{State: tfsdk.State{Schema: isoSchema.Schema}}
	initializeResourcePrivate(t, &isoDelete)
	isoResource.Delete(context.Background(), resource.DeleteRequest{State: isoCreate.State, Private: isoDelete.Private}, &isoDelete)
	if isoDelete.Diagnostics.HasError() {
		t.Fatalf("nocloud delete diagnostics: %v", isoDelete.Diagnostics)
	}
	if seedFileExists {
		t.Fatal("seed media was not cleaned up after the VM destroy")
	}

	handler.assert(t)
	wantCalls := []string{
		"GET /api2/json/nodes/pve%20one/storage",
		"GET " + contentPath,
		"POST " + uploadPath,
		"GET /api2/json/nodes/pve-two/tasks/UPID:pve-two:imgcopy-1/status",
		"GET /api2/json/nodes/target%20node/storage",
		"GET /api2/json/nodes/target%20node/storage/local%20iso/content",
		"POST /api2/json/nodes/source%20node/qemu/9000/clone",
		"GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-430/status",
		"GET /api2/json/nodes/target%20node/qemu/430/config",
		"PUT /api2/json/nodes/target%20node/qemu/430/config",
		"POST /api2/json/nodes/target%20node/qemu/430/status/start",
		"GET /api2/json/nodes/target%20node/tasks/UPID:target%20node:qemu-start-430/status",
		"GET /api2/json/nodes/target%20node/qemu/430/config",
		"GET /api2/json/nodes/target%20node/qemu/430/status/current",
		"GET /api2/json/nodes/target%20node/qemu/430/config",
		"GET /api2/json/nodes/target%20node/qemu/430/status/current",
		"POST /api2/json/nodes/target%20node/qemu/430/status/stop",
		"GET /api2/json/nodes/target%20node/tasks/UPID:target%20node:qemu-stop-430/status",
		"DELETE /api2/json/nodes/target%20node/qemu/430",
		"GET /api2/json/nodes/target%20node/tasks/UPID:target%20node:qemu-delete-430/status",
		"GET " + contentPath,
		"DELETE " + deletePath,
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:delete-1/status",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected chain call order: got %v want %v", calls, wantCalls)
	}
}

func TestQemuVMCreateAttachesCDROMOnPlainCreate(t *testing.T) {
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
			if !handler.form(w, r, url.Values{"vmid": {"440"}, "name": {"plain-vm"}, "ide2": {"local:iso/seed.iso,media=cdrom"}}) {
				return
			}
			handler.envelope(w, "UPID:pve one:qemu-create-440")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-440/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/440/config":
			handler.envelope(w, map[string]any{"name": "plain-vm", "ide2": "local:iso/seed.iso,media=cdrom"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/440/status/current":
			handler.envelope(w, map[string]any{"status": "stopped"})
		default:
			handler.fail(w, "unexpected plain cdrom create request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("pve one", 440)
	model.Name = types.StringValue("plain-vm")
	model.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("local:iso/seed.iso")})
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("plain cdrom create diagnostics: %v", resp.Diagnostics)
	}
	wantCalls := []string{
		"POST /api2/json/nodes/pve%20one/qemu",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-440/status",
		"GET /api2/json/nodes/pve%20one/qemu/440/config",
		"GET /api2/json/nodes/pve%20one/qemu/440/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected plain cdrom create calls: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

// TestQemuVMCloneRefusesForeignMediumWhenMarked proves a marked clone cannot
// overwrite the foreign ISO inherited in the marked slot; the unmarked
// ordinary replacement of the same layout keeps its baseline behavior
// (TestQemuVMCloneReplacesUnmarkedInheritedISO).
func TestQemuVMCloneRefusesForeignMediumWhenMarked(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage":
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage/local/content":
			handler.envelope(w, []map[string]any{{"volid": "local:iso/runner-seed.iso", "format": "iso", "size": 393216, "content": "iso"}})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/qemu/9000/clone":
			handler.envelope(w, "UPID:source node:qemu-clone-431")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-431/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/431/config":
			handler.envelope(w, map[string]any{"ide2": "local:iso/template-seed.iso,media=cdrom"})
		default:
			handler.fail(w, "unexpected foreign-medium request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := cloneModelWithMarkedCDROM(t, 431, "local:iso/runner-seed.iso")
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "would replace an existing disk or unknown medium") {
		t.Fatalf("expected foreign-medium diagnostics: %v", resp.Diagnostics)
	}
	var partial qemuVMModel
	if diags := resp.State.Get(context.Background(), &partial); diags.HasError() {
		t.Fatalf("decode retained state: %v", diags)
	}
	if partial.ID.ValueString() != "target node/431" {
		t.Fatalf("clone identity must stay tracked after refusal: %#v", partial)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/target%20node/storage",
		"GET /api2/json/nodes/target%20node/storage/local/content",
		"POST /api2/json/nodes/source%20node/qemu/9000/clone",
		"GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-431/status",
		"GET /api2/json/nodes/target%20node/qemu/431/config",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("foreign medium must abort before update or start: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMCloneRefusesSecondSeedOnOtherSlotWhenMarked(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage":
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage/local/content":
			handler.envelope(w, []map[string]any{{"volid": "local:iso/runner-seed.iso", "format": "iso", "size": 393216, "content": "iso"}})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/qemu/9000/clone":
			handler.envelope(w, "UPID:source node:qemu-clone-432")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-432/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/432/config":
			handler.envelope(w, map[string]any{
				"ide2": "local:vm-432-cloudinit,media=cdrom",
				"ide3": "local:iso/other-seed.iso,media=cdrom",
			})
		default:
			handler.fail(w, "unexpected double-seed request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := cloneModelWithMarkedCDROM(t, 432, "local:iso/runner-seed.iso")
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "more than one ISO/cloud-init drive") {
		t.Fatalf("expected double-seed diagnostics: %v", resp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/target%20node/storage",
		"GET /api2/json/nodes/target%20node/storage/local/content",
		"POST /api2/json/nodes/source%20node/qemu/9000/clone",
		"GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-432/status",
		"GET /api2/json/nodes/target%20node/qemu/432/config",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("double seed must abort before update or start: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

// TestQemuVMCloneRefusesHardDiskOverwriteWhenMarked proves a marked clone
// cannot overwrite an inherited hard disk in the planned CD-ROM slot.
func TestQemuVMCloneRefusesHardDiskOverwriteWhenMarked(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage":
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage/local/content":
			handler.envelope(w, []map[string]any{{"volid": "local:iso/runner-seed.iso", "format": "iso", "size": 393216, "content": "iso"}})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/qemu/9000/clone":
			handler.envelope(w, "UPID:source node:qemu-clone-433")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-433/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/433/config":
			handler.envelope(w, map[string]any{"ide2": "local-lvm:vm-433-disk-0,size=8G"})
		default:
			handler.fail(w, "unexpected disk-overwrite request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := cloneModelWithMarkedCDROM(t, 433, "local:iso/runner-seed.iso")
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "would replace an existing disk or unknown medium") {
		t.Fatalf("expected disk-overwrite diagnostics: %v", resp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/target%20node/storage",
		"GET /api2/json/nodes/target%20node/storage/local/content",
		"POST /api2/json/nodes/source%20node/qemu/9000/clone",
		"GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-433/status",
		"GET /api2/json/nodes/target%20node/qemu/433/config",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("disk overwrite must abort before update or start: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMCreateRejectsUnsafeCDROMPlanWithoutHTTP(t *testing.T) {
	tests := []struct {
		name     string
		slot     string
		volume   string
		contains string
	}{
		{"virtio slot", "virtio0", "local:iso/seed.iso", "cannot hold CD-ROM media"},
		{"unknown slot", "foo0", "local:iso/seed.iso", "cannot hold CD-ROM media"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("unsafe CD-ROM plan must be rejected without any HTTP call")
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			res := &QemuVMResource{client: testLifecycleClient(t, server)}
			schema := testResourceSchema(t, res)
			model := minimalQemuVMModel("pve one", 441)
			model.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{test.slot: cdromDiskEntry(test.volume)})
			resp := testResourceCreateResponse(t, schema)
			res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
			if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, test.contains) {
				t.Fatalf("expected unsafe slot diagnostics (%s), got %v", test.contains, resp.Diagnostics)
			}
			if !resp.State.Raw.IsNull() {
				t.Fatalf("unsafe plan must not persist state: %v", resp.State.Raw)
			}
		})
	}
}

func TestQemuVMCreateRejectsSecondMarkedSeedWithoutHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("second marked seed must be rejected without any HTTP call")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("pve one", 442)
	model.NoCloudCDROMSlot = types.StringValue("ide2")
	model.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{
		"ide2": cdromDiskEntry("local:iso/seed-a.iso"),
		"ide3": cdromDiskEntry("local:iso/seed-b.iso"),
	})
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "besides the marked NoCloud slot") {
		t.Fatalf("expected second marked seed diagnostics: %v", resp.Diagnostics)
	}
}

// Without the marker, ordinary multi-ISO VMs are supported: no seed layout
// restrictions apply and no storage checks run on the VM node.
func TestQemuVMCreateAllowsTwoUnmarkedISOs(t *testing.T) {
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
			if !handler.form(w, r, url.Values{
				"vmid": {"442"},
				"ide2": {"local:iso/installer.iso,media=cdrom"},
				"ide3": {"local:iso/drivers.iso,media=cdrom"},
			}) {
				return
			}
			handler.envelope(w, "UPID:pve one:qemu-create-442")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-442/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/442/config":
			handler.envelope(w, map[string]any{
				"ide2": "local:iso/installer.iso,media=cdrom",
				"ide3": "local:iso/drivers.iso,media=cdrom",
			})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/442/status/current":
			handler.envelope(w, map[string]any{"status": "stopped"})
		default:
			handler.fail(w, "unexpected multi-iso request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("pve one", 442)
	model.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{
		"ide2": cdromDiskEntry("local:iso/installer.iso"),
		"ide3": cdromDiskEntry("local:iso/drivers.iso"),
	})
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unmarked multi-iso create diagnostics: %v", resp.Diagnostics)
	}
	wantCalls := []string{
		"POST /api2/json/nodes/pve%20one/qemu",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qemu-create-442/status",
		"GET /api2/json/nodes/pve%20one/qemu/442/config",
		"GET /api2/json/nodes/pve%20one/qemu/442/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unmarked multi-iso create must skip seed checks: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMCreateRejectsUnattachedAndPseudoMarkedSlotWithoutHTTP(t *testing.T) {
	tests := []struct {
		name     string
		disks    map[string]qemuVMDiskModel
		contains string
	}{
		{
			name:     "marker names a slot without planned media",
			disks:    map[string]qemuVMDiskModel{},
			contains: "attaches no CD-ROM media to it",
		},
		{
			name:     "marker slot plans an ISO volume without explicit media",
			disks:    map[string]qemuVMDiskModel{"ide2": {Volume: types.StringValue("local:iso/seed.iso")}},
			contains: "must set `media = \"cdrom\"` explicitly",
		},
		{
			name:     "marker slot plans an explicit disk medium",
			disks:    map[string]qemuVMDiskModel{"ide2": {Volume: types.StringValue("local-lvm:vm-444-disk-0"), Media: types.StringValue("disk")}},
			contains: "must set `media = \"cdrom\"` explicitly",
		},
		{
			name:     "marker slot plans an empty bay",
			disks:    map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("none")},
			contains: "must plan a real ISO volume",
		},
		{
			name:     "marker slot plans the physical drive",
			disks:    map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("cdrom")},
			contains: "must plan a real ISO volume",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("invalid marked slot must be rejected without any HTTP call")
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			res := &QemuVMResource{client: testLifecycleClient(t, server)}
			schema := testResourceSchema(t, res)
			model := minimalQemuVMModel("pve one", 444)
			model.NoCloudCDROMSlot = types.StringValue("ide2")
			model.Disk = mustQemuVMDiskMapValue(t, test.disks)
			resp := testResourceCreateResponse(t, schema)
			res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
			if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, test.contains) {
				t.Fatalf("expected invalid marked slot diagnostics (%s), got %v", test.contains, resp.Diagnostics)
			}
			if !resp.State.Raw.IsNull() {
				t.Fatalf("invalid marked slot must not persist state: %v", resp.State.Raw)
			}
		})
	}
}

func TestQemuVMNoCloudSlotConfigValidation(t *testing.T) {
	t.Parallel()

	valid := minimalQemuVMModel("pve-1", 101)
	valid.NoCloudCDROMSlot = types.StringValue("ide2")
	if diags := validateQemuVMNoCloudSlotConfig(valid); diags.HasError() {
		t.Fatalf("ide2 marker must validate: %v", diags)
	}

	unmarked := minimalQemuVMModel("pve-1", 101)
	if diags := validateQemuVMNoCloudSlotConfig(unmarked); diags.HasError() {
		t.Fatalf("unmarked model must validate: %v", diags)
	}

	invalid := minimalQemuVMModel("pve-1", 101)
	invalid.NoCloudCDROMSlot = types.StringValue("virtio0")
	diags := validateQemuVMNoCloudSlotConfig(invalid)
	if !diags.HasError() || !containsDiagnostic(diags, "cannot hold CD-ROM media") {
		t.Fatalf("expected invalid marker slot diagnostics, got %v", diags)
	}
}

func TestQemuVMCreateRejectsSeedStorageUnusableOnVMNode(t *testing.T) {
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
		if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage" {
			// The VM node can see the storage but it cannot serve ISO files,
			// unlike the VM disk storages that are listed alongside it.
			handler.envelope(w, []map[string]any{
				{"storage": "local", "type": "dir", "content": "vztmpl", "active": 1, "enabled": 1, "shared": 0},
				{"storage": "local-lvm", "type": "lvmpool", "content": "images,rootdir", "active": 1, "enabled": 1, "shared": 0},
			})
			return
		}
		handler.fail(w, "unexpected unusable-storage request: %s %s", r.Method, r.URL.String())
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := cloneModelWithMarkedCDROM(t, 443, "local:iso/runner-seed.iso")
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "does not support iso content") {
		t.Fatalf("expected unusable seed storage diagnostics: %v", resp.Diagnostics)
	}
	if want := []string{"GET /api2/json/nodes/target%20node/storage"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("unusable seed storage must abort before the clone: got %v want %v", calls, want)
	}
	handler.assert(t)
}

// TestQemuVMCreateRejectsSeedVolumeMissingOnVMNode models a cross-node
// fixture: the seed was uploaded to the same-named storage on another node,
// so the VM node's authoritative ISO collection lacks the exact volume and
// the create must fail before any clone or start.
func TestQemuVMCreateRejectsSeedVolumeMissingOnVMNode(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage":
			// The storage identifier is visible on the VM node, but it is
			// not shared: the same-named storage on the upload node holds the
			// file, this node's collection does not.
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage/local/content":
			if r.URL.Query().Get("content") != "iso" {
				handler.fail(w, "unexpected content query: %s", r.URL.RawQuery)
				return
			}
			handler.envelope(w, []map[string]any{})
		default:
			handler.fail(w, "unexpected missing-seed request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := cloneModelWithMarkedCDROM(t, 445, "local:iso/runner-seed.iso")
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "does not exist in storage \"local\" on node \"target node\"") {
		t.Fatalf("expected missing seed volume diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatalf("missing seed volume must not persist state: %v", resp.State.Raw)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/target%20node/storage",
		"GET /api2/json/nodes/target%20node/storage/local/content",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("missing seed volume must abort before the clone: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

// TestQemuVMCloneReplacesFileBackedCloudInitDrive covers the real dir/NFS
// file-backed Proxmox cloud-init grammar: a template whose ide2 holds
// `storage:<vmid>/vm-<vmid>-cloudinit.qcow2` is recognized and replaced by
// the marked seed in the same slot.
func TestQemuVMCloneReplacesFileBackedCloudInitDrive(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage":
			// Shared storage: the ISO is visible from the VM node even though
			// it was uploaded elsewhere.
			handler.envelope(w, []map[string]any{
				{"storage": "nfsvol", "type": "nfs", "content": "iso", "active": 1, "enabled": 1, "shared": 1},
			})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage/nfsvol/content":
			handler.envelope(w, []map[string]any{{"volid": "nfsvol:iso/runner-seed.iso", "format": "iso", "size": 393216, "content": "iso"}})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/qemu/9000/clone":
			handler.envelope(w, "UPID:source node:qemu-clone-446")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-446/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/446/config":
			handler.envelope(w, map[string]any{
				"ide2":  "nfsvol:446/vm-446-cloudinit.qcow2,media=cdrom",
				"scsi0": "local-lvm:vm-446-disk-0,size=8G",
			})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/446/config":
			if !handler.form(w, r, url.Values{"name": {"chain-vm"}, "ide2": {"nfsvol:iso/runner-seed.iso,media=cdrom"}}) {
				return
			}
			handler.envelope(w, nil)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/446/status/start":
			handler.envelope(w, "UPID:target node:qemu-start-446")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/tasks/UPID:target%20node:qemu-start-446/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/446/status/current":
			handler.envelope(w, map[string]any{"status": "running", "uptime": 3})
		default:
			handler.fail(w, "unexpected file-backed cloudinit request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := cloneModelWithMarkedCDROM(t, 446, "nfsvol:iso/runner-seed.iso")
	model.StartOnCreate = types.BoolValue(true)
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("file-backed cloudinit replacement diagnostics: %v", resp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/target%20node/storage",
		"GET /api2/json/nodes/target%20node/storage/nfsvol/content",
		"POST /api2/json/nodes/source%20node/qemu/9000/clone",
		"GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-446/status",
		"GET /api2/json/nodes/target%20node/qemu/446/config",
		"PUT /api2/json/nodes/target%20node/qemu/446/config",
		"POST /api2/json/nodes/target%20node/qemu/446/status/start",
		"GET /api2/json/nodes/target%20node/tasks/UPID:target%20node:qemu-start-446/status",
		"GET /api2/json/nodes/target%20node/qemu/446/config",
		"GET /api2/json/nodes/target%20node/qemu/446/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected file-backed cloudinit replacement calls: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

// TestQemuVMCloneRefusesRawSecondISOWhenMarked proves the seed count inspects
// the full effective wire configuration: an inherited second ISO on another
// slot whose options the typed parser does not fully understand still refuses
// a marked seed attachment.
func TestQemuVMCloneRefusesRawSecondISOWhenMarked(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage":
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage/local/content":
			handler.envelope(w, []map[string]any{{"volid": "local:iso/runner-seed.iso", "format": "iso", "size": 393216, "content": "iso"}})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/qemu/9000/clone":
			handler.envelope(w, "UPID:source node:qemu-clone-447")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-447/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/447/config":
			handler.envelope(w, map[string]any{
				"ide2":  "local:vm-447-cloudinit,media=cdrom",
				"sata0": "mirror:iso/second-seed.iso,backend=unexpected",
			})
		default:
			handler.fail(w, "unexpected raw second iso request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := cloneModelWithMarkedCDROM(t, 447, "local:iso/runner-seed.iso")
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "more than one ISO/cloud-init drive") || !containsDiagnostic(resp.Diagnostics, "sata0") {
		t.Fatalf("expected raw second iso diagnostics: %v", resp.Diagnostics)
	}
	var partial qemuVMModel
	if diags := resp.State.Get(context.Background(), &partial); diags.HasError() {
		t.Fatalf("decode retained state: %v", diags)
	}
	if partial.ID.ValueString() != "target node/447" {
		t.Fatalf("clone identity must stay tracked after refusal: %#v", partial)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/target%20node/storage",
		"GET /api2/json/nodes/target%20node/storage/local/content",
		"POST /api2/json/nodes/source%20node/qemu/9000/clone",
		"GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-447/status",
		"GET /api2/json/nodes/target%20node/qemu/447/config",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("raw second iso must abort before update or start: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

// TestQemuVMCloneRefusesPseudoClearOfInheritedDiskWhenMarked proves a planned
// pseudo volume (empty bay / physical drive) cannot clear a hard disk or
// foreign medium in a marked workflow.
func TestQemuVMCloneRefusesPseudoClearOfInheritedDiskWhenMarked(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage":
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage/local/content":
			handler.envelope(w, []map[string]any{{"volid": "local:iso/runner-seed.iso", "format": "iso", "size": 393216, "content": "iso"}})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/qemu/9000/clone":
			handler.envelope(w, "UPID:source node:qemu-clone-448")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-448/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/448/config":
			handler.envelope(w, map[string]any{
				"ide2":  "local:vm-448-cloudinit,media=cdrom",
				"sata0": "local-lvm:vm-448-disk-0,size=8G",
			})
		default:
			handler.fail(w, "unexpected pseudo-clear request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := cloneModelWithMarkedCDROM(t, 448, "local:iso/runner-seed.iso")
	model.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{
		"ide2":  cdromDiskEntry("local:iso/runner-seed.iso"),
		"sata0": {Media: types.StringValue("cdrom"), Volume: types.StringValue("cdrom")},
	})
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "would replace an existing disk or unknown medium") {
		t.Fatalf("expected pseudo-clear diagnostics: %v", resp.Diagnostics)
	}
	var partial qemuVMModel
	if diags := resp.State.Get(context.Background(), &partial); diags.HasError() {
		t.Fatalf("decode retained state: %v", diags)
	}
	if partial.ID.ValueString() != "target node/448" {
		t.Fatalf("clone identity must stay tracked after refusal: %#v", partial)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/target%20node/storage",
		"GET /api2/json/nodes/target%20node/storage/local/content",
		"POST /api2/json/nodes/source%20node/qemu/9000/clone",
		"GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-448/status",
		"GET /api2/json/nodes/target%20node/qemu/448/config",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("pseudo-clear must abort before update or start: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

// TestQemuVMCloneRefusesBarePseudoClearOfInheritedDiskWhenMarked proves the
// clone ownership guard inspects positional none/cdrom mutations even when
// they omit media=cdrom: a bare pseudo value in the typed disk map or planned
// raw.extra_config cannot clear the template's inherited hard disk, and the
// clone must abort before any config update or start while keeping its
// tracked identity for retry.
func TestQemuVMCloneRefusesBarePseudoClearOfInheritedDiskWhenMarked(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	for name, mutate := range map[string]func(m *qemuVMModel){
		"raw bare pseudo": func(m *qemuVMModel) {
			m.Raw = mustQemuVMRawValue(t, qemuVMRawModel{ExtraConfig: mustStringMapValue(t, map[string]string{"sata0": "none"})})
		},
		"typed bare pseudo": func(m *qemuVMModel) {
			m.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{
				"ide2":  cdromDiskEntry("local:iso/runner-seed.iso"),
				"sata0": {Volume: types.StringValue("none")},
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			handler := &lifecycleHandler{}
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !handler.auth(w, r) {
					return
				}
				calls = append(calls, r.Method+" "+r.URL.EscapedPath())
				switch {
				case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage":
					handler.envelope(w, noCloudStorageEntries("local"))
				case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage/local/content":
					handler.envelope(w, []map[string]any{{"volid": "local:iso/runner-seed.iso", "format": "iso", "size": 393216, "content": "iso"}})
				case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/qemu/9000/clone":
					handler.envelope(w, "UPID:source node:qemu-clone-448")
				case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-448/status":
					handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
				case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/448/config":
					handler.envelope(w, map[string]any{
						"ide2":  "local:vm-448-cloudinit,media=cdrom",
						"sata0": "local-lvm:vm-448-disk-0,size=8G",
					})
				default:
					handler.fail(w, "unexpected bare pseudo-clear request: %s %s", r.Method, r.URL.String())
				}
			}))
			defer server.Close()

			res := &QemuVMResource{client: testLifecycleClient(t, server)}
			schema := testResourceSchema(t, res)
			model := cloneModelWithMarkedCDROM(t, 448, "local:iso/runner-seed.iso")
			mutate(&model)
			resp := testResourceCreateResponse(t, schema)
			res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
			if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "would replace an existing disk or unknown medium") || !containsDiagnostic(resp.Diagnostics, "sata0") {
				t.Fatalf("expected bare pseudo-clear diagnostics: %v", resp.Diagnostics)
			}
			var partial qemuVMModel
			if diags := resp.State.Get(context.Background(), &partial); diags.HasError() {
				t.Fatalf("decode retained state: %v", diags)
			}
			if partial.ID.ValueString() != "target node/448" {
				t.Fatalf("clone identity must stay tracked after refusal: %#v", partial)
			}
			wantCalls := []string{
				"GET /api2/json/nodes/target%20node/storage",
				"GET /api2/json/nodes/target%20node/storage/local/content",
				"POST /api2/json/nodes/source%20node/qemu/9000/clone",
				"GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-448/status",
				"GET /api2/json/nodes/target%20node/qemu/448/config",
			}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("bare pseudo-clear must abort before update or start: got %v want %v", calls, wantCalls)
			}
			handler.assert(t)
		})
	}
}

// TestQemuVMCloneReplacesUnmarkedInheritedISO proves the owner-approved
// ordinary clone workflow keeps its baseline behavior: replacing an inherited
// installer ISO with the planned one in the same slot succeeds without any
// NoCloud ownership inspection (and without the intermediate config read that
// only the marked workflow performs).
func TestQemuVMCloneReplacesUnmarkedInheritedISO(t *testing.T) {
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
			handler.envelope(w, "UPID:source node:qemu-clone-454")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-454/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/454/config":
			if !handler.form(w, r, url.Values{"name": {"chain-vm"}, "ide2": {"local:iso/installer-new.iso,media=cdrom"}}) {
				return
			}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/454/config":
			handler.envelope(w, map[string]any{"name": "chain-vm", "ide2": "local:iso/installer-new.iso,media=cdrom"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/454/status/current":
			handler.envelope(w, map[string]any{"status": "stopped"})
		default:
			handler.fail(w, "unexpected unmarked replacement request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := cloneModelWithCDROM(t, 454, "local:iso/installer-new.iso")
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unmarked clone replacement diagnostics: %v", resp.Diagnostics)
	}
	wantCalls := []string{
		"POST /api2/json/nodes/source%20node/qemu/9000/clone",
		"GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-454/status",
		"PUT /api2/json/nodes/target%20node/qemu/454/config",
		"GET /api2/json/nodes/target%20node/qemu/454/config",
		"GET /api2/json/nodes/target%20node/qemu/454/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unmarked clone replacement must keep the ordinary flow: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

// TestQemuVMCreateRejectsPlannedRawSecondSeedWithoutHTTP proves planned raw
// extra_config disk slots join the marked seed safety: the request would
// carry both drives, but the raw second seed is rejected before any API
// request is sent.
func TestQemuVMCreateRejectsPlannedRawSecondSeedWithoutHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("planned raw second seed must be rejected without any HTTP call")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := cloneModelWithMarkedCDROM(t, 455, "local:iso/runner-seed.iso")
	model.Raw = mustQemuVMRawValue(t, qemuVMRawModel{ExtraConfig: mustStringMapValue(t, map[string]string{
		"ide3": "local:iso/second-seed.iso,media=cdrom",
	})})
	// The fixture must plan the second seed through the raw escape hatch that
	// the typed disk map alone does not inspect.
	request, diags := qemuVMUpdateRequestFromModel(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("fixture request encoding: %v", diags)
	}
	if request.ExtraConfig["ide3"] == "" {
		t.Fatal("fixture did not plan a raw second seed")
	}
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "besides the marked NoCloud slot") || !containsDiagnostic(resp.Diagnostics, "ide3") {
		t.Fatalf("expected planned raw second seed diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatalf("planned raw second seed must not persist state: %v", resp.State.Raw)
	}
}

// TestQemuVMUpdateRefusesPlannedRawSecondSeed proves a marked in-place update
// also counts planned raw extra_config seeds: the update is refused before
// any storage lookup or PUT.
func TestQemuVMUpdateRefusesPlannedRawSecondSeed(t *testing.T) {
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/456/config":
			handler.envelope(w, map[string]any{"ide2": "local:iso/seed-gen1.iso,media=cdrom"})
		default:
			handler.fail(w, "raw second seed update must not issue further requests: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("target node", 456)
	stateModel.ID = types.StringValue("target node/456")
	stateModel.NoCloudCDROMSlot = types.StringValue("ide2")
	stateModel.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("local:iso/seed-gen1.iso")})
	planModel := stateModel
	planModel.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("local:iso/seed-gen2.iso")})
	planModel.Raw = mustQemuVMRawValue(t, qemuVMRawModel{ExtraConfig: mustStringMapValue(t, map[string]string{
		"ide3": "local:iso/second-seed.iso,media=cdrom",
	})})

	updateState := tfsdk.State{Schema: schema.Schema}
	if diags := updateState.Set(context.Background(), stateModel); diags.HasError() {
		t.Fatalf("encode update state: %v", diags)
	}
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, planModel), State: updateState}, &updateResp)
	if !updateResp.Diagnostics.HasError() || !containsDiagnostic(updateResp.Diagnostics, "besides the marked NoCloud slot") || !containsDiagnostic(updateResp.Diagnostics, "ide3") {
		t.Fatalf("expected planned raw second seed diagnostics: %v", updateResp.Diagnostics)
	}
	if want := []string{"GET /api2/json/nodes/target%20node/qemu/456/config"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("raw second seed update must abort before storage or PUT: got %v want %v", calls, want)
	}
	handler.assert(t)
}

// TestQemuVMUpdateRefusesPlannedRawOverwriteOfInheritedDisk proves raw target
// ownership validation: a planned raw pseudo CD-ROM value cannot clear an
// inherited hard disk in a marked workflow.
func TestQemuVMUpdateRefusesPlannedRawOverwriteOfInheritedDisk(t *testing.T) {
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/457/config":
			handler.envelope(w, map[string]any{
				"ide2": "local:iso/seed-gen1.iso,media=cdrom",
				"ide3": "local-lvm:vm-457-disk-0,size=8G",
			})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage":
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage/local/content":
			handler.envelope(w, []map[string]any{{"volid": "local:iso/seed-gen1.iso", "format": "iso", "size": 393216, "content": "iso"}})
		default:
			handler.fail(w, "raw overwrite update must not issue a PUT: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("target node", 457)
	stateModel.ID = types.StringValue("target node/457")
	stateModel.NoCloudCDROMSlot = types.StringValue("ide2")
	stateModel.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("local:iso/seed-gen1.iso")})
	planModel := stateModel
	// The marked seed itself stays identical so the guard reaches the raw
	// target: a planned raw pseudo CD-ROM value cannot clear the inherited
	// hard disk on ide3.
	planModel.Raw = mustQemuVMRawValue(t, qemuVMRawModel{ExtraConfig: mustStringMapValue(t, map[string]string{
		"ide3": "none,media=cdrom",
	})})

	updateState := tfsdk.State{Schema: schema.Schema}
	if diags := updateState.Set(context.Background(), stateModel); diags.HasError() {
		t.Fatalf("encode update state: %v", diags)
	}
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, planModel), State: updateState}, &updateResp)
	if !updateResp.Diagnostics.HasError() || !containsDiagnostic(updateResp.Diagnostics, "would replace an existing disk or unknown medium") || !containsDiagnostic(updateResp.Diagnostics, "ide3") {
		t.Fatalf("expected raw overwrite diagnostics: %v", updateResp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/target%20node/qemu/457/config",
		"GET /api2/json/nodes/target%20node/storage",
		"GET /api2/json/nodes/target%20node/storage/local/content",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("raw overwrite update must abort before the PUT: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

// TestQemuVMUpdateSendsOnlyChangedDiskAndNetworkSlots proves the narrower
// update write contract: slots that are new or changed relative to prior
// state reach the wire, while the unchanged marked seed is no longer resent
// as mutation intent just because refreshed state observed it.
func TestQemuVMUpdateSendsOnlyChangedDiskAndNetworkSlots(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	configUpdated := false
	var putForm url.Values
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage":
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage/local/content":
			handler.envelope(w, []map[string]any{{"volid": "local:iso/seed-gen1.iso", "format": "iso", "size": 393216, "content": "iso"}})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/462/config":
			if configUpdated {
				handler.envelope(w, map[string]any{
					"ide2":  "local:iso/seed-gen1.iso,media=cdrom",
					"sata3": "local-lvm:vm-462-disk-3,size=32G,media=disk",
					"net0":  "virtio=BC:24:11:00:00:46,bridge=vmbr1",
				})
				return
			}
			handler.envelope(w, map[string]any{
				"ide2": "local:iso/seed-gen1.iso,media=cdrom",
				"net0": "virtio=BC:24:11:00:00:46,bridge=vmbr0",
			})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/462/config":
			if err := r.ParseForm(); err != nil {
				handler.fail(w, "parse update form: %v", err)
				return
			}
			putForm = r.Form
			configUpdated = true
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/462/status/current":
			handler.envelope(w, map[string]any{"status": "stopped"})
		default:
			handler.fail(w, "unexpected slot update request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("target node", 462)
	stateModel.ID = types.StringValue("target node/462")
	stateModel.NoCloudCDROMSlot = types.StringValue("ide2")
	stateModel.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("local:iso/seed-gen1.iso")})
	stateModel.Network = mustQemuVMNetworkMapValue(t, map[string]qemuVMNetworkModel{
		"net0": {Model: types.StringValue("virtio"), MACAddr: types.StringValue("BC:24:11:00:00:46"), Bridge: types.StringValue("vmbr0")},
	})
	planModel := stateModel
	planModel.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{
		"ide2":  cdromDiskEntry("local:iso/seed-gen1.iso"),
		"sata3": {Storage: types.StringValue("local-lvm"), Size: types.StringValue("32G"), Media: types.StringValue("disk")},
	})
	planModel.Network = mustQemuVMNetworkMapValue(t, map[string]qemuVMNetworkModel{
		"net0": {Model: types.StringValue("virtio"), MACAddr: types.StringValue("BC:24:11:00:00:46"), Bridge: types.StringValue("vmbr1")},
	})

	updateState := tfsdk.State{Schema: schema.Schema}
	if diags := updateState.Set(context.Background(), stateModel); diags.HasError() {
		t.Fatalf("encode update state: %v", diags)
	}
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, planModel), State: updateState}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("slot update diagnostics: %v", updateResp.Diagnostics)
	}

	if got := putForm["ide2"]; len(got) != 0 {
		t.Fatalf("unchanged marked seed must not be resent, got %v", got)
	}
	if got := putForm["sata3"]; len(got) != 1 || got[0] != "local-lvm:32G,media=disk" {
		t.Fatalf("added managed slot must be sent, got %v", got)
	}
	if got := putForm["net0"]; len(got) != 1 || got[0] != "virtio=BC:24:11:00:00:46,bridge=vmbr1" {
		t.Fatalf("changed network slot must be sent, got %v", got)
	}
	if len(putForm) != 2 {
		t.Fatalf("PUT must carry exactly the changed slots, got %v", putForm)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/target%20node/qemu/462/config",
		"GET /api2/json/nodes/target%20node/storage",
		"GET /api2/json/nodes/target%20node/storage/local/content",
		"PUT /api2/json/nodes/target%20node/qemu/462/config",
		"GET /api2/json/nodes/target%20node/qemu/462/config",
		"GET /api2/json/nodes/target%20node/qemu/462/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected slot update calls: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

// TestQemuVMUpdateRefusesBarePseudoClearOfInheritedDisk proves the marked
// workflow inspects positional none/cdrom slot mutations even when they omit
// media=cdrom: a bare pseudo value in the typed disk map or planned
// raw.extra_config cannot clear an inherited hard disk, and the update must
// abort before any PUT.
func TestQemuVMUpdateRefusesBarePseudoClearOfInheritedDisk(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	for name, mutate := range map[string]func(m *qemuVMModel){
		"raw bare pseudo": func(m *qemuVMModel) {
			m.Raw = mustQemuVMRawValue(t, qemuVMRawModel{ExtraConfig: mustStringMapValue(t, map[string]string{"ide3": "none"})})
		},
		"typed bare pseudo": func(m *qemuVMModel) {
			m.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{
				"ide2": cdromDiskEntry("local:iso/seed-gen1.iso"),
				"ide3": {Volume: types.StringValue("none")},
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			handler := &lifecycleHandler{}
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !handler.auth(w, r) {
					return
				}
				calls = append(calls, r.Method+" "+r.URL.EscapedPath())
				switch {
				case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/457/config":
					handler.envelope(w, map[string]any{
						"ide2": "local:iso/seed-gen1.iso,media=cdrom",
						"ide3": "local-lvm:vm-457-disk-0,size=8G",
					})
				case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage":
					handler.envelope(w, noCloudStorageEntries("local"))
				case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage/local/content":
					handler.envelope(w, []map[string]any{{"volid": "local:iso/seed-gen1.iso", "format": "iso", "size": 393216, "content": "iso"}})
				default:
					handler.fail(w, "bare pseudo update must not issue a PUT: %s %s", r.Method, r.URL.String())
				}
			}))
			defer server.Close()

			res := &QemuVMResource{client: testLifecycleClient(t, server)}
			schema := testResourceSchema(t, res)
			stateModel := minimalQemuVMModel("target node", 457)
			stateModel.ID = types.StringValue("target node/457")
			stateModel.NoCloudCDROMSlot = types.StringValue("ide2")
			stateModel.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("local:iso/seed-gen1.iso")})
			planModel := stateModel
			mutate(&planModel)

			updateState := tfsdk.State{Schema: schema.Schema}
			if diags := updateState.Set(context.Background(), stateModel); diags.HasError() {
				t.Fatalf("encode update state: %v", diags)
			}
			updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
			res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, planModel), State: updateState}, &updateResp)
			if !updateResp.Diagnostics.HasError() || !containsDiagnostic(updateResp.Diagnostics, "would replace an existing disk or unknown medium") || !containsDiagnostic(updateResp.Diagnostics, "ide3") {
				t.Fatalf("expected bare pseudo overwrite diagnostics: %v", updateResp.Diagnostics)
			}
			wantCalls := []string{
				"GET /api2/json/nodes/target%20node/qemu/457/config",
				"GET /api2/json/nodes/target%20node/storage",
				"GET /api2/json/nodes/target%20node/storage/local/content",
			}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("bare pseudo update must abort before the PUT: got %v want %v", calls, wantCalls)
			}
			handler.assert(t)
		})
	}
}

// TestQemuVMUpdateRefusesUnsafeMarkedSeedMutation proves an in-place marked
// seed mutation is validated against the live wire configuration before any
// PUT: an out-of-band foreign medium on the marked slot must refuse the
// update instead of silently overwriting it.
func TestQemuVMUpdateRefusesUnsafeMarkedSeedMutation(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage":
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage/local/content":
			handler.envelope(w, []map[string]any{{"volid": "local:iso/seed-gen2.iso", "format": "iso", "size": 393216, "content": "iso"}})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/450/config":
			// The marked slot was replaced out-of-band: the live medium is a
			// foreign ISO, not the seed Terraform attached.
			handler.envelope(w, map[string]any{"ide2": "local:iso/out-of-band.iso,media=cdrom"})
		default:
			handler.fail(w, "unsafe marked update must not issue further requests: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("target node", 450)
	stateModel.ID = types.StringValue("target node/450")
	stateModel.NoCloudCDROMSlot = types.StringValue("ide2")
	stateModel.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("local:iso/seed-gen1.iso")})
	planModel := stateModel
	planModel.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("local:iso/seed-gen2.iso")})

	updateState := tfsdk.State{Schema: schema.Schema}
	if diags := updateState.Set(context.Background(), stateModel); diags.HasError() {
		t.Fatalf("encode update state: %v", diags)
	}
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, planModel), State: updateState}, &updateResp)
	if !updateResp.Diagnostics.HasError() || !containsDiagnostic(updateResp.Diagnostics, "would replace an existing disk or unknown medium") {
		t.Fatalf("expected unsafe marked update diagnostics: %v", updateResp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/target%20node/qemu/450/config",
		"GET /api2/json/nodes/target%20node/storage",
		"GET /api2/json/nodes/target%20node/storage/local/content",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unsafe marked update must abort before the PUT: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

// TestQemuVMUpdateRefusesFormerlyOwnedSeedSwap proves the accepted refusal
// path: refreshed state is observed reality, not proof of which medium this
// resource attached, so an in-place swap of the formerly owned old seed for
// the next generation is refused before any PUT - seed changes replace the VM.
func TestQemuVMUpdateRefusesFormerlyOwnedSeedSwap(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage":
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage/local/content":
			handler.envelope(w, []map[string]any{
				{"volid": "local:iso/seed-gen1.iso", "format": "iso", "size": 393216, "content": "iso"},
				{"volid": "local:iso/seed-gen2.iso", "format": "iso", "size": 393216, "content": "iso"},
			})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/450/config":
			// The live medium is still the seed Terraform attached in an
			// earlier apply; the plan points at the next generation.
			handler.envelope(w, map[string]any{"ide2": "local:iso/seed-gen1.iso,media=cdrom"})
		default:
			handler.fail(w, "formerly owned seed swap must not issue a PUT or start: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("target node", 450)
	stateModel.ID = types.StringValue("target node/450")
	stateModel.NoCloudCDROMSlot = types.StringValue("ide2")
	stateModel.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("local:iso/seed-gen1.iso")})
	planModel := stateModel
	planModel.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("local:iso/seed-gen2.iso")})

	updateState := tfsdk.State{Schema: schema.Schema}
	if diags := updateState.Set(context.Background(), stateModel); diags.HasError() {
		t.Fatalf("encode update state: %v", diags)
	}
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, planModel), State: updateState}, &updateResp)
	if !updateResp.Diagnostics.HasError() || !containsDiagnostic(updateResp.Diagnostics, "would replace an existing disk or unknown medium") || !containsDiagnostic(updateResp.Diagnostics, "replacing the VM") {
		t.Fatalf("expected formerly owned seed swap refusal diagnostics: %v", updateResp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/target%20node/qemu/450/config",
		"GET /api2/json/nodes/target%20node/storage",
		"GET /api2/json/nodes/target%20node/storage/local/content",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("formerly owned seed swap must abort before the PUT: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

// TestQemuVMRefreshThenUpdateRefusesForeignISO reproduces the actual
// Terraform sequence: a refresh observes a foreign ISO substituted out of
// band and records the real drift, then a marked update pointing at the next
// generation must refuse instead of treating the observed foreign medium as
// one this resource attached - no PUT and no start may be issued.
func TestQemuVMRefreshThenUpdateRefusesForeignISO(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage":
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage/local/content":
			handler.envelope(w, []map[string]any{{"volid": "local:iso/seed-gen2.iso", "format": "iso", "size": 393216, "content": "iso"}})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/460/config":
			handler.envelope(w, map[string]any{"ide2": "local:iso/foreign.iso,media=cdrom"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/460/status/current":
			handler.envelope(w, map[string]any{"status": "stopped"})
		default:
			handler.fail(w, "refreshed foreign ISO must not be overwritten in place: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("target node", 460)
	stateModel.ID = types.StringValue("target node/460")
	stateModel.NoCloudCDROMSlot = types.StringValue("ide2")
	stateModel.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("local:iso/seed-gen1.iso")})
	before := testResourceState(t, schema, stateModel)

	read := resource.ReadResponse{State: before}
	res.Read(ctx, resource.ReadRequest{State: before}, &read)
	if read.Diagnostics.HasError() {
		t.Fatalf("refresh diagnostics: %v", read.Diagnostics)
	}
	// The refresh must keep observing the actual foreign medium as drift, not
	// echo fictitious old values to preserve an artificial ownership concept.
	var refreshed qemuVMModel
	if diags := read.State.Get(ctx, &refreshed); diags.HasError() {
		t.Fatalf("decode refreshed state: %v", diags)
	}
	refreshedDisks, diskDiags := expandQemuVMDiskModelMap(ctx, refreshed.Disk)
	if diskDiags.HasError() {
		t.Fatalf("decode refreshed disks: %v", diskDiags)
	}
	if got := refreshedDisks["ide2"].Volume.ValueString(); got != "local:iso/foreign.iso" {
		t.Fatalf("refresh must record the actual foreign medium as drift, got %q", got)
	}

	planModel := refreshed
	planModel.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("local:iso/seed-gen2.iso")})
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(ctx, resource.UpdateRequest{Plan: testResourcePlan(t, schema, planModel), State: read.State}, &updateResp)
	if !updateResp.Diagnostics.HasError() || !containsDiagnostic(updateResp.Diagnostics, "would replace an existing disk or unknown medium") || !containsDiagnostic(updateResp.Diagnostics, "replacing the VM") {
		t.Fatalf("expected foreign ISO refusal after refresh: %v", updateResp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/target%20node/qemu/460/config",
		"GET /api2/json/nodes/target%20node/qemu/460/status/current",
		"GET /api2/json/nodes/target%20node/qemu/460/config",
		"GET /api2/json/nodes/target%20node/storage",
		"GET /api2/json/nodes/target%20node/storage/local/content",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("refreshed foreign ISO update must refuse without PUT or start: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

// TestQemuVMUpdateSameSeedLifecyclePolicyOnlySucceeds proves a marked update
// that keeps the identical seed still succeeds: lifecycle-policy-only changes
// never restart the guest and the same-volume in-place path stays safe.
func TestQemuVMUpdateSameSeedLifecyclePolicyOnlySucceeds(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage":
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/storage/local/content":
			handler.envelope(w, []map[string]any{{"volid": "local:iso/seed-gen1.iso", "format": "iso", "size": 393216, "content": "iso"}})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/461/config":
			handler.envelope(w, map[string]any{"ide2": "local:iso/seed-gen1.iso,media=cdrom"})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/461/config":
			// The seed is unchanged relative to prior state, so the narrower
			// update contract no longer resends it: the PUT carries no disk
			// slots at all.
			if !handler.form(w, r, url.Values{}) {
				return
			}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/461/status/current":
			handler.envelope(w, map[string]any{"status": "stopped"})
		default:
			handler.fail(w, "same-seed policy update must not start the guest: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("target node", 461)
	stateModel.ID = types.StringValue("target node/461")
	stateModel.NoCloudCDROMSlot = types.StringValue("ide2")
	stateModel.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("local:iso/seed-gen1.iso")})
	planModel := stateModel
	planModel.StopOnDestroy = types.BoolValue(true)

	updateState := tfsdk.State{Schema: schema.Schema}
	if diags := updateState.Set(context.Background(), stateModel); diags.HasError() {
		t.Fatalf("encode update state: %v", diags)
	}
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, planModel), State: updateState}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("same-seed policy-only update diagnostics: %v", updateResp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/target%20node/qemu/461/config",
		"GET /api2/json/nodes/target%20node/storage",
		"GET /api2/json/nodes/target%20node/storage/local/content",
		"PUT /api2/json/nodes/target%20node/qemu/461/config",
		"GET /api2/json/nodes/target%20node/qemu/461/config",
		"GET /api2/json/nodes/target%20node/qemu/461/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected same-seed policy update calls: got %v want %v", calls, wantCalls)
	}
	var updated qemuVMModel
	if diags := updateResp.State.Get(context.Background(), &updated); diags.HasError() {
		t.Fatalf("decode updated state: %v", diags)
	}
	if !updated.StopOnDestroy.ValueBool() {
		t.Fatalf("update must persist lifecycle policy changes: %#v", updated)
	}
	updatedDisks, diskDiags := expandQemuVMDiskModelMap(context.Background(), updated.Disk)
	if diskDiags.HasError() {
		t.Fatalf("decode updated disks: %v", diskDiags)
	}
	if got := updatedDisks["ide2"].Volume.ValueString(); got != "local:iso/seed-gen1.iso" {
		t.Fatalf("same seed must stay attached, got %q", got)
	}
	handler.assert(t)
}

func TestQemuVMWireDiskHelpers(t *testing.T) {
	tests := []struct {
		raw           string
		volume        string
		isCDROM       bool
		cloudInitVMID int64
		isCloudInit   bool
	}{
		{raw: "local:iso/seed.iso,media=cdrom", volume: "local:iso/seed.iso", isCDROM: true, cloudInitVMID: 430, isCloudInit: false},
		{raw: "local:vm-430-cloudinit,media=cdrom", volume: "local:vm-430-cloudinit", isCDROM: true, cloudInitVMID: 430, isCloudInit: true},
		{raw: " local:vm-430-cloudinit , backend=unexpected , media=cdrom ", volume: "local:vm-430-cloudinit", isCDROM: true, cloudInitVMID: 430, isCloudInit: true},
		// File-backed (dir/NFS) storages write a format suffix and an owner
		// path segment; both LVM and file-backed same-VM forms are recognized.
		{raw: "local:430/vm-430-cloudinit.qcow2,media=cdrom", volume: "local:430/vm-430-cloudinit.qcow2", isCDROM: true, cloudInitVMID: 430, isCloudInit: true},
		{raw: "nfsvol:430/vm-430-cloudinit.raw,media=cdrom", volume: "nfsvol:430/vm-430-cloudinit.raw", isCDROM: true, cloudInitVMID: 430, isCloudInit: true},
		{raw: "local:430/vm-430-cloudinit,media=cdrom", volume: "local:430/vm-430-cloudinit", isCDROM: true, cloudInitVMID: 430, isCloudInit: true},
		// Without media=cdrom the drive is not the PVE-generated cloudinit form.
		{raw: "local-lvm:vm-430-cloudinit", volume: "local-lvm:vm-430-cloudinit", isCDROM: false, cloudInitVMID: 430, isCloudInit: false},
		// Foreign owners, unknown suffixes, and lookalike paths never match.
		{raw: "local:vm-999-cloudinit,media=cdrom", volume: "local:vm-999-cloudinit", isCDROM: true, cloudInitVMID: 430, isCloudInit: false},
		{raw: "local:430/vm-999-cloudinit.qcow2,media=cdrom", volume: "local:430/vm-999-cloudinit.qcow2", isCDROM: true, cloudInitVMID: 430, isCloudInit: false},
		{raw: "local:430/vm-430-cloudinit.qcow2.bak,media=cdrom", volume: "local:430/vm-430-cloudinit.qcow2.bak", isCDROM: true, cloudInitVMID: 430, isCloudInit: false},
		{raw: "local:430/vm-430-cloudinit.txt,media=cdrom", volume: "local:430/vm-430-cloudinit.txt", isCDROM: true, cloudInitVMID: 430, isCloudInit: false},
		{raw: "local:iso/vm-430-cloudinit,media=cdrom", volume: "local:iso/vm-430-cloudinit", isCDROM: true, cloudInitVMID: 430, isCloudInit: false},
		{raw: "cdrom,media=cdrom", volume: "cdrom", isCDROM: true, cloudInitVMID: 430, isCloudInit: false},
		{raw: "none,media=cdrom", volume: "none", isCDROM: true, cloudInitVMID: 430, isCloudInit: false},
		{raw: "local:iso/other.iso,backend=x", volume: "local:iso/other.iso", isCDROM: true, cloudInitVMID: 430, isCloudInit: false},
		{raw: "local-lvm:vm-430-disk-0,size=8G", volume: "local-lvm:vm-430-disk-0", isCDROM: false, cloudInitVMID: 430, isCloudInit: false},
		{raw: "local:iso/seed.ISO", volume: "local:iso/seed.ISO", isCDROM: true, cloudInitVMID: 430, isCloudInit: false},
	}
	for _, test := range tests {
		if got := qemuVMWireDiskVolume(test.raw); got != test.volume {
			t.Errorf("qemuVMWireDiskVolume(%q) = %q, want %q", test.raw, got, test.volume)
		}
		if got := qemuVMWireDiskIsCDROM(test.raw); got != test.isCDROM {
			t.Errorf("qemuVMWireDiskIsCDROM(%q) = %v, want %v", test.raw, got, test.isCDROM)
		}
		if got := qemuVMIsPVECloudInitDrive(test.raw, test.cloudInitVMID); got != test.isCloudInit {
			t.Errorf("qemuVMIsPVECloudInitDrive(%q, %d) = %v, want %v", test.raw, test.cloudInitVMID, got, test.isCloudInit)
		}
	}

	if qemuVMIsVolumeReference("local:iso/seed.iso") != true {
		t.Error("a storage volume must count as a volume reference")
	}
	for _, pseudo := range []string{"", "cdrom", "none"} {
		if qemuVMIsVolumeReference(pseudo) {
			t.Errorf("%q must not count as a volume reference", pseudo)
		}
	}
	for slot, want := range map[string]bool{"ide2": true, "sata5": true, "scsi30": true, "virtio0": false, "foo0": false, "ide": false, "idex": false} {
		if got := qemuVMSlotSupportsCDROM(slot); got != want {
			t.Errorf("qemuVMSlotSupportsCDROM(%q) = %v, want %v", slot, got, want)
		}
	}
}

func TestValidateQemuVMCDROMOwnership(t *testing.T) {
	tests := []struct {
		name      string
		vmID      int64
		marker    string
		planned   map[string]string
		inherited map[string]string
		contains  string
	}{
		{
			name:      "empty target slot is fine",
			planned:   map[string]string{"ide2": "local:iso/seed.iso"},
			inherited: map[string]string{"scsi0": "local-lvm:vm-430-disk-0,size=8G"},
		},
		{
			name:      "same slot same volume is fine",
			planned:   map[string]string{"ide2": "local:iso/seed.iso"},
			inherited: map[string]string{"ide2": "local:iso/seed.iso,media=cdrom"},
		},
		{
			name:      "replacing the LVM PVE cloud-init drive in its own slot is fine",
			planned:   map[string]string{"ide2": "local:iso/seed.iso"},
			inherited: map[string]string{"ide2": "local:vm-430-cloudinit,media=cdrom,backend=unexpected", "scsi0": "local-lvm:vm-430-disk-0,size=8G"},
		},
		{
			name:      "replacing the file-backed PVE cloud-init drive in its own slot is fine",
			planned:   map[string]string{"ide2": "local:iso/seed.iso"},
			inherited: map[string]string{"ide2": "local:430/vm-430-cloudinit.qcow2,media=cdrom"},
		},
		{
			name:      "file-backed cloud-init drive of another VM refuses",
			marker:    "ide2",
			planned:   map[string]string{"ide2": "local:iso/seed.iso"},
			inherited: map[string]string{"ide2": "local:430/vm-430-cloudinit.qcow2,media=cdrom"},
			vmID:      431,
			contains:  "would replace an existing disk or unknown medium",
		},
		{
			name:      "empty CD-ROM bay in target slot is fine",
			planned:   map[string]string{"ide2": "local:iso/seed.iso"},
			inherited: map[string]string{"ide2": "none,media=cdrom"},
		},
		{
			name:      "physical cdrom pseudo drive elsewhere is fine",
			planned:   map[string]string{"ide2": "local:iso/seed.iso"},
			inherited: map[string]string{"ide3": "cdrom,media=cdrom"},
		},
		{
			name:      "unmarked ordinary ISO replacement keeps baseline behavior",
			planned:   map[string]string{"ide2": "local:iso/installer-new.iso"},
			inherited: map[string]string{"ide2": "local:iso/installer-old.iso,media=cdrom"},
		},
		{
			name:      "foreign iso in target slot refuses",
			marker:    "ide2",
			planned:   map[string]string{"ide2": "local:iso/seed.iso"},
			inherited: map[string]string{"ide2": "local:iso/template-seed.iso,media=cdrom"},
			contains:  "would replace an existing disk or unknown medium",
		},
		{
			name:      "hard disk in target slot refuses",
			marker:    "ide2",
			planned:   map[string]string{"ide2": "local:iso/seed.iso"},
			inherited: map[string]string{"ide2": "local-lvm:vm-430-disk-0,size=8G"},
			contains:  "would replace an existing disk or unknown medium",
		},
		{
			name:      "cloud-init drive without cdrom media refuses",
			marker:    "ide2",
			planned:   map[string]string{"ide2": "local:iso/seed.iso"},
			inherited: map[string]string{"ide2": "local:vm-430-cloudinit"},
			contains:  "would replace an existing disk or unknown medium",
		},
		{
			name:      "cloud-init drive of another VM refuses",
			marker:    "ide2",
			planned:   map[string]string{"ide2": "local:iso/seed.iso"},
			inherited: map[string]string{"ide2": "local:vm-999-cloudinit,media=cdrom"},
			contains:  "would replace an existing disk or unknown medium",
		},
		{
			name:      "marked planned raw second seed refuses",
			marker:    "ide2",
			planned:   map[string]string{"ide2": "local:iso/seed.iso", "ide3": "local:iso/second-seed.iso"},
			inherited: map[string]string{"ide2": "local:vm-430-cloudinit,media=cdrom"},
			contains:  "more than one ISO/cloud-init drive",
		},
		{
			name:      "marked planned raw pseudo cannot clear an inherited hard disk",
			marker:    "ide2",
			planned:   map[string]string{"ide2": "local:iso/seed.iso", "ide3": "none"},
			inherited: map[string]string{"ide2": "local:vm-430-cloudinit,media=cdrom", "ide3": "local-lvm:vm-430-disk-0,size=8G"},
			contains:  "would replace an existing disk or unknown medium",
		},
		{
			name:      "unmarked second seed on another slot is allowed",
			planned:   map[string]string{"ide2": "local:iso/seed.iso"},
			inherited: map[string]string{"ide2": "local:vm-430-cloudinit,media=cdrom", "ide3": "local:iso/other-seed.iso,media=cdrom"},
		},
		{
			name:      "marked second seed on another slot refuses",
			marker:    "ide2",
			planned:   map[string]string{"ide2": "local:iso/seed.iso"},
			inherited: map[string]string{"ide2": "local:vm-430-cloudinit,media=cdrom", "ide3": "local:iso/other-seed.iso,media=cdrom"},
			contains:  "more than one ISO/cloud-init drive",
		},
		{
			name:      "marked second planned seed refuses",
			marker:    "ide2",
			planned:   map[string]string{"ide2": "local:iso/seed.iso", "ide3": "local:iso/other.iso"},
			inherited: map[string]string{"ide2": "local:vm-430-cloudinit,media=cdrom"},
			contains:  "more than one ISO/cloud-init drive",
		},
		{
			name:      "marked seed replacing same-slot inherited cloud-init is fine",
			marker:    "ide2",
			planned:   map[string]string{"ide2": "local:iso/seed.iso"},
			inherited: map[string]string{"ide2": "local:430/vm-430-cloudinit.qcow2,media=cdrom"},
		},
		{
			name:      "marked pseudo planned value cannot clear an inherited hard disk",
			marker:    "ide2",
			planned:   map[string]string{"ide2": "local:iso/seed.iso", "sata0": "cdrom"},
			inherited: map[string]string{"ide2": "local:vm-430-cloudinit,media=cdrom", "sata0": "local-lvm:vm-430-disk-0,size=8G"},
			contains:  "would replace an existing disk or unknown medium",
		},
		{
			name:      "marked pseudo planned value over a foreign iso refuses",
			marker:    "ide2",
			planned:   map[string]string{"ide2": "local:iso/seed.iso", "sata0": "none"},
			inherited: map[string]string{"ide2": "local:vm-430-cloudinit,media=cdrom", "sata0": "local:iso/template.iso,media=cdrom"},
			contains:  "would replace an existing disk or unknown medium",
		},
		{
			name:      "unmarked pseudo planned value keeps original unguarded scope",
			planned:   map[string]string{"ide2": "local:iso/seed.iso", "sata0": "none"},
			inherited: map[string]string{"ide2": "local:vm-430-cloudinit,media=cdrom", "sata0": "local-lvm:vm-430-disk-0,size=8G"},
		},
		{
			name:      "marked in-place swap of the formerly owned seed refuses",
			marker:    "ide2",
			planned:   map[string]string{"ide2": "local:iso/seed-gen2.iso"},
			inherited: map[string]string{"ide2": "local:iso/seed-gen1.iso,media=cdrom"},
			contains:  "would replace an existing disk or unknown medium",
		},
		{
			name:      "marked in-place update refuses a foreign medium",
			marker:    "ide2",
			planned:   map[string]string{"ide2": "local:iso/seed-gen2.iso"},
			inherited: map[string]string{"ide2": "local:iso/out-of-band.iso,media=cdrom"},
			contains:  "would replace an existing disk or unknown medium",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			vmID := test.vmID
			if vmID == 0 {
				vmID = 430
			}
			err := validateQemuVMCDROMOwnership(vmID, test.planned, test.inherited, test.marker)
			if test.contains == "" {
				if err != nil {
					t.Fatalf("expected acceptance, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("expected error containing %q, got %v", test.contains, err)
			}
		})
	}
}

// TestPlannedQemuVMWireCDROMs proves the effective planned attachment set:
// marked workflows merge planned raw extra_config CD-ROM slots into the typed
// planned CD-ROMs (reduced to bare volumes), while unmarked workflows keep
// the typed-only set so ordinary raw passthrough stays unguarded.
func TestPlannedQemuVMWireCDROMs(t *testing.T) {
	ctx := context.Background()

	raw := mustQemuVMRawValue(t, qemuVMRawModel{ExtraConfig: mustStringMapValue(t, map[string]string{
		"ide3":      "local:iso/second-seed.iso,media=cdrom",
		"sata0":     "none,media=cdrom",
		"sata1":     "cdrom",
		"virtio1":   "local:iso/drivers.iso",
		"scsi0":     "local-lvm:vm-458-disk-0,size=8G",
		"net0":      "virtio,bridge=vmbr0",
		"tpmstate0": "local-lvm:vm-458-tpmstate0",
	})})

	marked := minimalQemuVMModel("pve one", 458)
	marked.NoCloudCDROMSlot = types.StringValue("ide2")
	marked.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{
		"ide2": cdromDiskEntry("local:iso/seed.iso"),
		// Positional pseudo media with no explicit media still clear their
		// slot, so the marked workflow must inspect them.
		"sata2": {Volume: types.StringValue("none")},
	})
	marked.Raw = raw
	typed, diags := plannedQemuVMCDROMs(ctx, marked)
	if diags.HasError() {
		t.Fatalf("typed planned CD-ROMs: %v", diags)
	}
	merged, diags := plannedQemuVMWireCDROMs(ctx, marked, typed, "ide2")
	if diags.HasError() {
		t.Fatalf("marked wire CD-ROMs: %v", diags)
	}
	want := map[string]string{
		"ide2":    "local:iso/seed.iso",
		"ide3":    "local:iso/second-seed.iso",
		"sata0":   "none",
		"sata1":   "cdrom",
		"sata2":   "none",
		"virtio1": "local:iso/drivers.iso",
	}
	if !reflect.DeepEqual(merged, want) {
		t.Fatalf("marked merge must include planned raw CD-ROM slots reduced to bare volumes: got %v want %v", merged, want)
	}

	unmarked := minimalQemuVMModel("pve one", 458)
	unmarked.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("local:iso/seed.iso")})
	unmarked.Raw = raw
	unmarkedTyped, diags := plannedQemuVMCDROMs(ctx, unmarked)
	if diags.HasError() {
		t.Fatalf("unmarked typed planned CD-ROMs: %v", diags)
	}
	unmarkedMerged, diags := plannedQemuVMWireCDROMs(ctx, unmarked, unmarkedTyped, "")
	if diags.HasError() {
		t.Fatalf("unmarked wire CD-ROMs: %v", diags)
	}
	if !reflect.DeepEqual(unmarkedMerged, unmarkedTyped) {
		t.Fatalf("unmarked workflows must keep the typed-only set: got %v want %v", unmarkedMerged, unmarkedTyped)
	}
}
