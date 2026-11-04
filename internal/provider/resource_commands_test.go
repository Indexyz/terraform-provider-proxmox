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

const commandTaskUPID = "UPID:pve-1:0000:0000:0000:cmd:::"

func TestNodeCommandResources(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/wakeonlan":
			handler.envelope(w, "aa:bb:cc:dd:ee:ff")
		case r.URL.Path == "/api2/json/nodes/pve-1/startall":
			handler.form(w, r, url.Values{"vms": {"100"}, "force": {"1"}})
			handler.envelope(w, commandTaskUPID)
		case r.URL.Path == "/api2/json/nodes/pve-1/migrateall":
			handler.form(w, r, url.Values{"vms": {"100"}, "target": {"pve-2"}})
			handler.envelope(w, commandTaskUPID)
		case strings.HasPrefix(r.URL.Path, "/api2/json/nodes/pve-1/tasks/"):
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)
	mac, err := client.WakeOnLAN(context.Background(), "pve-1")
	if err != nil || mac != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("WakeOnLAN() = %q, %v", mac, err)
	}
	if _, err := client.StartAll(context.Background(), "pve-1", []int64{100}, true, 0); err != nil {
		t.Fatalf("StartAll() unexpected error: %v", err)
	}
	if _, err := client.MigrateAll(context.Background(), "pve-1", "pve-2", []int64{100}, false, 0); err != nil {
		t.Fatalf("MigrateAll() unexpected error: %v", err)
	}
	handler.assert(t)
}

func TestGuestCommandResources(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendkey"):
			handler.form(w, r, url.Values{"key": {"a"}})
			handler.envelope(w, nil)
		case strings.HasSuffix(r.URL.Path, "/monitor"):
			handler.form(w, r, url.Values{"command": {"info status"}})
			handler.envelope(w, "VM status: running")
		case strings.HasSuffix(r.URL.Path, "/status/current"):
			handler.envelope(w, map[string]any{"status": "running"})
		case strings.HasSuffix(r.URL.Path, "/agent/fsfreeze-freeze"):
			handler.envelope(w, map[string]any{"result": nil})
		case strings.HasSuffix(r.URL.Path, "/agent/fsfreeze-thaw"):
			handler.envelope(w, map[string]any{"result": nil})
		case strings.HasSuffix(r.URL.Path, "/agent/fstrim"):
			handler.envelope(w, map[string]any{"result": nil})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)
	if err := client.SendKey(context.Background(), "pve-1", 100, "a"); err != nil {
		t.Fatalf("SendKey() unexpected error: %v", err)
	}
	if result, err := client.QemuMonitor(context.Background(), "pve-1", 100, "info status"); err != nil || result != "VM status: running" {
		t.Fatalf("QemuMonitor() = %q, %v", result, err)
	}
	if _, err := client.AgentFSFreeze(context.Background(), "pve-1", 100, "freeze"); err != nil {
		t.Fatalf("AgentFSFreeze() unexpected error: %v", err)
	}
	if _, err := client.AgentFSFreeze(context.Background(), "pve-1", 100, "thaw"); err != nil {
		t.Fatalf("AgentFSFreeze(thaw) unexpected error: %v", err)
	}
	if err := client.AgentFstrim(context.Background(), "pve-1", 100); err != nil {
		t.Fatalf("AgentFstrim() unexpected error: %v", err)
	}
	handler.assert(t)
}

func TestMoveAndRemoteMigrateClient(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve-1/qemu/100/move_disk":
			handler.form(w, r, url.Values{"disk": {"scsi0"}, "storage": {"target"}, "delete": {"1"}})
			handler.envelope(w, commandTaskUPID)
		case "/api2/json/nodes/pve-1/lxc/200/move_volume":
			handler.form(w, r, url.Values{"volume": {"rootfs"}, "storage": {"target"}})
			handler.envelope(w, commandTaskUPID)
		case "/api2/json/nodes/pve-1/qemu/100/remote_migrate":
			handler.form(w, r, url.Values{"target-endpoint": {"host=x"}, "target-storage": {"local"}, "target-bridge": {"vmbr0"}})
			handler.envelope(w, commandTaskUPID)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)
	if _, err := client.MoveQemuDisk(context.Background(), "pve-1", 100, QemuMoveDisk{Disk: "scsi0", Storage: "target", Delete: true}); err != nil {
		t.Fatalf("MoveQemuDisk() unexpected error: %v", err)
	}
	if _, err := client.MoveGuestVolume(context.Background(), "pve-1", 200, GuestVolumeMove{GuestType: "lxc", Volume: "rootfs", Storage: "target"}); err != nil {
		t.Fatalf("MoveGuestVolume() unexpected error: %v", err)
	}
	if _, err := client.RemoteMigrateGuest(context.Background(), "pve-1", 100, GuestRemoteMigrate{GuestType: "qemu", TargetEndpoint: "host=x", TargetStorage: "local", TargetBridge: "vmbr0"}); err != nil {
		t.Fatalf("RemoteMigrateGuest() unexpected error: %v", err)
	}
	handler.assert(t)
}

func TestCustomCPUModelLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/cluster/qemu/custom-cpu-models" && r.Method == http.MethodPost:
			handler.form(w, r, url.Values{"cputype": {"custom-x"}, "reported-model": {"qemu64"}})
			handler.envelope(w, nil)
		case strings.Contains(r.URL.Path, "/custom-cpu-models/custom-x"):
			handler.envelope(w, map[string]any{"cputype": "custom-x", "reported-model": "qemu64", "hidden": 0, "digest": "d1"})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	res := &CustomCPUModelResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := customCPUModelModel{CPUType: types.StringValue("custom-x"), ReportedModel: types.StringValue("qemu64")}
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("cpu model diagnostics: %v", resp.Diagnostics)
	}
}

func TestProxyDataSources(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/termproxy": map[string]any{"port": "5900", "ticket": "t", "user": "u", "upid": "UPID:x"},
		"/api2/json/access/vncticket":      nil,
	}
	read := runSDNDataSource(t, NewNodeTermProxyDataSource(), responses, map[string]any{"node": "pve-1"})
	if read.Diagnostics.HasError() {
		t.Fatalf("termproxy read failed: %v", read.Diagnostics)
	}
	var state proxyModel
	if diags := read.State.Get(context.Background(), &state); diags.HasError() || state.Port.ValueString() != "5900" {
		t.Fatalf("unexpected proxy state: %#v (%v)", state, diags)
	}
	ticket := runSDNDataSource(t, NewAccessVNCTicketDataSource(), responses, map[string]any{"userid": "u", "path": "/vms/1", "vncticket": "t", "privs": "VM.Console"})
	if ticket.Diagnostics.HasError() {
		t.Fatalf("vncticket read failed: %v", ticket.Diagnostics)
	}
}

func TestOpenIDAndMTunnelDataSources(t *testing.T) {
	responses := map[string]any{
		"/api2/json/access/openid/auth-url":       "https://idp/auth",
		"/api2/json/access/openid/login":          map[string]any{"username": "u", "ticket": "t", "CSRFPreventionToken": "c"},
		"/api2/json/nodes/pve-1/qemu/300/mtunnel": map[string]any{"socket": "/run/x", "ticket": "t", "upid": "UPID:x"},
	}
	auth := runSDNDataSource(t, NewOpenIDAuthURLDataSource(), responses, map[string]any{"realm": "r", "redirect_url": "https://pve"})
	if auth.Diagnostics.HasError() {
		t.Fatalf("auth url failed: %v", auth.Diagnostics)
	}
	login := runSDNDataSource(t, NewOpenIDLoginDataSource(), responses, map[string]any{"code": "c", "redirect_url": "https://pve", "state": "s"})
	if login.Diagnostics.HasError() {
		t.Fatalf("openid login failed: %v", login.Diagnostics)
	}
	tunnel := runSDNDataSource(t, NewGuestMTunnelDataSource(), responses, map[string]any{"type": "qemu", "node": "pve-1", "vmid": int64(300)})
	if tunnel.Diagnostics.HasError() {
		t.Fatalf("mtunnel failed: %v", tunnel.Diagnostics)
	}
}

func TestStorageFileRestoreAndQueryDataSources(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/storage/backup/file-restore/list?volume=backup%3Abackup%2Fb.vma.zst":                                []map[string]any{{"filepath": "/etc/hostname", "type": "f", "size": 5, "leaf": 1}},
		"/api2/json/nodes/pve-1/storage/backup/file-restore/download?filepath=%2Fetc%2Fhostname&volume=backup%3Abackup%2Fb.vma.zst": "pve1",
		"/api2/json/nodes/pve-1/query-url-metadata?url=https%3A%2F%2Fexample.com":                                                   map[string]any{"filename": "example", "size": 1, "mimetype": "text/html"},
	}
	list := runSDNDataSource(t, NewFileRestoreListDataSource(), responses, map[string]any{"node": "pve-1", "storage": "backup", "volume": "backup:backup/b.vma.zst"})
	if list.Diagnostics.HasError() {
		t.Fatalf("file restore list failed: %v", list.Diagnostics)
	}
	download := runSDNDataSource(t, NewFileRestoreDownloadDataSource(), responses, map[string]any{"node": "pve-1", "storage": "backup", "volume": "backup:backup/b.vma.zst", "filepath": "/etc/hostname"})
	if download.Diagnostics.HasError() {
		t.Fatalf("file restore download failed: %v", download.Diagnostics)
	}
	query := runSDNDataSource(t, NewQueryURLMetadataDataSource(), responses, map[string]any{"node": "pve-1", "url": "https://example.com"})
	if query.Diagnostics.HasError() {
		t.Fatalf("query metadata failed: %v", query.Diagnostics)
	}
}

func TestGuestProxyDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/qemu/300/vncproxy": map[string]any{"port": "5901", "ticket": "t", "user": "u", "upid": "UPID:x"},
	}
	resp := runSDNDataSource(t, NewGuestProxyDataSource(), responses, map[string]any{"type": "qemu", "node": "pve-1", "vmid": int64(300), "kind": "vncproxy"})
	if resp.Diagnostics.HasError() {
		t.Fatalf("guest proxy failed: %v", resp.Diagnostics)
	}
}
