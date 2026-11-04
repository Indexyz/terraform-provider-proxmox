// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestClientHAResourceActionAndStatus(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch r.URL.EscapedPath() {
		case "/api2/json/cluster/ha/resources/vm:100/migrate":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			assertFormValues(t, r, url.Values{"node": {"pve2"}})
			writeEnvelope(t, w, map[string]any{"sid": "vm:100", "requested-node": "pve2"})
		case "/api2/json/cluster/ha/status":
			writeEnvelope(t, w, []map[string]any{{"id": "quorum", "type": "quorum", "node": ""}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)

	result, err := client.HAResourceAction(ctx, "vm:100", "migrate", "pve2")
	if err != nil || !strings.Contains(result, `"requested-node":"pve2"`) {
		t.Fatalf("HAResourceAction() = %q, %v", result, err)
	}
	summary, err := client.HAStatusSummary(ctx)
	if err != nil || !strings.Contains(summary, "quorum") {
		t.Fatalf("HAStatusSummary() = %q, %v", summary, err)
	}
}

func TestClientCephHealthMuteAndMetadata(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch r.URL.Path {
		case "/api2/json/cluster/ceph/health-mute":
			writeEnvelope(t, w, []map[string]any{{"code": "OSD_NEAR_FULL", "summary": "near full", "ttl": "1h", "sticky": 1}})
		case "/api2/json/cluster/ceph/health-mute/OSD_NEAR_FULL":
			assertFormValues(t, r, url.Values{"value": {"1"}, "ttl": {"1h"}, "sticky": {"1"}})
			writeEnvelope(t, w, nil)
		case "/api2/json/cluster/ceph/metadata":
			if got := r.URL.Query().Get("scope"); got != "mds" {
				t.Fatalf("scope = %q", got)
			}
			writeEnvelope(t, w, map[string]any{"mds": map[string]any{}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)

	mutes, err := client.CephHealthMutes(ctx)
	if err != nil || len(mutes) != 1 || mutes[0].Code != "OSD_NEAR_FULL" || mutes[0].Sticky.Ptr() == nil || !*mutes[0].Sticky.Ptr() {
		t.Fatalf("CephHealthMutes() = %#v, %v", mutes, err)
	}
	if err := client.SetCephHealthMute(ctx, "OSD_NEAR_FULL", true, "1h", 1); err != nil {
		t.Fatalf("SetCephHealthMute() unexpected error: %v", err)
	}
	metadata, err := client.CephMetadata(ctx, "mds")
	if err != nil || !strings.Contains(metadata, "mds") {
		t.Fatalf("CephMetadata() = %q, %v", metadata, err)
	}
}

func TestClientCephServiceAndRestartBulk(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch r.URL.Path {
		case "/api2/json/nodes/pve/ceph/start":
			assertFormValues(t, r, url.Values{"service": {"osd.0"}})
			writeEnvelope(t, w, "UPID:pve:x")
		case "/api2/json/nodes/pve/ceph/stop":
			assertFormValues(t, r, url.Values{"service": {"osd.0"}})
			writeEnvelope(t, w, "UPID:pve:x")
		case "/api2/json/cluster/ceph/restart-bulk":
			assertFormValues(t, r, url.Values{"service-type": {"osd"}, "force": {"1"}, "only-outdated": {"1"}, "timeout": {"60"}})
			writeEnvelope(t, w, "UPID:pve:x")
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)

	for _, action := range []string{"start", "stop"} {
		upid, err := client.CephServiceAction(ctx, "pve", "osd.0", action)
		if err != nil || upid != "UPID:pve:x" {
			t.Fatalf("CephServiceAction(%s) = %q, %v", action, upid, err)
		}
	}
	upid, err := client.CephRestartBulkCluster(ctx, "osd", true, true, 60)
	if err != nil || upid != "UPID:pve:x" {
		t.Fatalf("CephRestartBulkCluster() = %q, %v", upid, err)
	}
}

func TestClientGuestActionsExactForms(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch r.URL.Path {
		case "/api2/json/nodes/pve/qemu/100/status/shutdown":
			assertFormValues(t, r, url.Values{"timeout": {"30"}, "forceStop": {"1"}})
			writeEnvelope(t, w, "UPID:pve:x")
		case "/api2/json/nodes/pve/qemu/100/status/suspend":
			assertFormValues(t, r, url.Values{"todisk": {"1"}, "statestorage": {"local"}})
			writeEnvelope(t, w, "UPID:pve:x")
		case "/api2/json/nodes/pve/lxc/101/status/reboot":
			assertFormValues(t, r, url.Values{})
			writeEnvelope(t, w, "UPID:pve:x")
		case "/api2/json/nodes/pve/qemu/100/template":
			assertFormValues(t, r, url.Values{"disk": {"scsi0,scsi1"}})
			writeEnvelope(t, w, nil)
		case "/api2/json/nodes/pve/lxc/101/template":
			assertFormValues(t, r, url.Values{})
			writeEnvelope(t, w, nil)
		case "/api2/json/nodes/pve/qemu/100/cloudinit":
			if r.Method != http.MethodPut {
				t.Fatalf("method = %s", r.Method)
			}
			writeEnvelope(t, w, nil)
		case "/api2/json/nodes/pve/qemu/100/dbus-vmstate":
			assertFormValues(t, r, url.Values{"action": {"start"}})
			writeEnvelope(t, w, nil)
		case "/api2/json/nodes/pve/qemu/100/unlink":
			assertFormValues(t, r, url.Values{"idlist": {"scsi0,scsi1"}, "force": {"1"}})
			writeEnvelope(t, w, nil)
		case "/api2/json/nodes/pve/lxc/101/resize":
			assertFormValues(t, r, url.Values{"disk": {"rootfs"}, "size": {"+2G"}, "digest": {"abc"}})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)

	if upid, err := client.GuestStatusAction(ctx, "pve", "qemu", "100", "shutdown", 30, 1, 0, ""); err != nil || upid != "UPID:pve:x" {
		t.Fatalf("shutdown = %q, %v", upid, err)
	}
	if _, err := client.GuestStatusAction(ctx, "pve", "qemu", "100", "suspend", 0, 0, 1, "local"); err != nil {
		t.Fatalf("suspend unexpected error: %v", err)
	}
	if _, err := client.GuestStatusAction(ctx, "pve", "lxc", "101", "reboot", 0, 0, 0, ""); err != nil {
		t.Fatalf("reboot unexpected error: %v", err)
	}
	if err := client.GuestTemplate(ctx, "pve", "qemu", "100", []string{"scsi0", "scsi1"}); err != nil {
		t.Fatalf("GuestTemplate() unexpected error: %v", err)
	}
	if err := client.GuestTemplate(ctx, "pve", "lxc", "101", []string{"scsi0"}); err != nil {
		t.Fatalf("GuestTemplate(lxc) unexpected error: %v", err)
	}
	if err := client.GuestCloudinitRegenerate(ctx, "pve", "100"); err != nil {
		t.Fatalf("GuestCloudinitRegenerate() unexpected error: %v", err)
	}
	if err := client.GuestDBusVMState(ctx, "pve", "100", "start"); err != nil {
		t.Fatalf("GuestDBusVMState() unexpected error: %v", err)
	}
	if err := client.GuestUnlink(ctx, "pve", "100", []string{"scsi0", "scsi1"}, true); err != nil {
		t.Fatalf("GuestUnlink() unexpected error: %v", err)
	}
	if err := client.LXCResize(ctx, "pve", "101", "rootfs", "+2G", "abc"); err != nil {
		t.Fatalf("LXCResize() unexpected error: %v", err)
	}
}

func TestClientGuestReads(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch r.URL.Path {
		case "/api2/json/nodes/pve/qemu/100/feature":
			if r.URL.Query().Get("feature") != "snapshot" || r.URL.Query().Get("snapname") != "s1" {
				t.Fatalf("query = %v", r.URL.Query())
			}
			writeEnvelope(t, w, map[string]any{"hasFeature": true, "nodes": []string{"pve"}})
		case "/api2/json/nodes/pve/qemu/100/firewall/refs":
			writeEnvelope(t, w, []map[string]any{{"type": "ipset", "ref": "mgmt", "name": "x", "scope": "cluster"}})
		case "/api2/json/nodes/pve/qemu/100/pending":
			writeEnvelope(t, w, []map[string]any{{"key": "memory", "value": "2048", "pending": true}})
		case "/api2/json/nodes/pve/qemu/100/rrd":
			if r.URL.Query().Get("ds") != "cpu" {
				t.Fatalf("query = %v", r.URL.Query())
			}
			writeEnvelope(t, w, map[string]any{"image": "base64png", "filename": "x.png"})
		case "/api2/json/nodes/pve/qemu/100/rrddata":
			writeEnvelope(t, w, []map[string]any{{"time": 1, "cpu": 0.1}})
		case "/api2/json/nodes/pve/qemu/100/vncwebsocket":
			writeEnvelope(t, w, map[string]any{"port": "5900"})
		case "/api2/json/nodes/pve/vncwebsocket":
			writeEnvelope(t, w, map[string]any{"port": "5901"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)

	feature, err := client.GuestFeature(ctx, "pve", "qemu", "100", "snapshot", "s1")
	if err != nil || feature.HasFeature.Ptr() == nil || !*feature.HasFeature.Ptr() || len(feature.Nodes) != 1 {
		t.Fatalf("GuestFeature() = %#v, %v", feature, err)
	}
	refs, err := client.GuestFirewallRefs(ctx, "pve", "qemu", "100", "")
	if err != nil || len(refs) != 1 || refs[0].Ref != "mgmt" {
		t.Fatalf("GuestFirewallRefs() = %#v, %v", refs, err)
	}
	pending, err := client.GuestPending(ctx, "pve", "qemu", "100")
	if err != nil || !strings.Contains(pending, "memory") {
		t.Fatalf("GuestPending() = %q, %v", pending, err)
	}
	if image, err := client.GuestRRDImage(ctx, "pve", "qemu", "100", "AVERAGE", "cpu", "hour"); err != nil || image != "base64png" {
		t.Fatalf("GuestRRDImage() = %q, %v", image, err)
	}
	if data, err := client.GuestRRDData(ctx, "pve", "qemu", "100", "AVERAGE", "hour"); err != nil || !strings.Contains(data, "cpu") {
		t.Fatalf("GuestRRDData() = %q, %v", data, err)
	}
	if port, err := client.GuestVNCWebsocketPort(ctx, "pve", "qemu", "100", "ticket", "5900"); err != nil || port != "5900" {
		t.Fatalf("GuestVNCWebsocketPort() = %s, %v", port, err)
	}
	if port, err := client.NodeVNCWebsocketPort(ctx, "pve", "ticket", "5901"); err != nil || port != "5901" {
		t.Fatalf("NodeVNCWebsocketPort() = %s, %v", port, err)
	}
}

func TestClientStorageTransfer(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch r.URL.Path {
		case "/api2/json/nodes/pve/storage/local/download-url":
			assertFormValues(t, r, url.Values{"url": {"https://example.invalid/x.iso"}, "content": {"iso"}, "filename": {"x.iso"}, "checksum": {"abc"}, "checksum-algorithm": {"sha256"}, "compression": {"gz"}})
			writeEnvelope(t, w, "UPID:pve:x")
		case "/api2/json/nodes/pve/storage/local/upload":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("ParseMultipartForm() error: %v", err)
			}
			if r.FormValue("content") != "iso" || r.FormValue("checksum") != "abc" || r.FormValue("checksum-algorithm") != "sha256" {
				t.Fatalf("content = %q checksum = %q", r.FormValue("content"), r.FormValue("checksum"))
			}
			file, header, err := r.FormFile("filename")
			if err != nil {
				t.Fatalf("FormFile() error: %v", err)
			}
			data, _ := io.ReadAll(file)
			if header.Filename != "x.iso" || string(data) != "payload" {
				t.Fatalf("uploaded %q %q", header.Filename, data)
			}
			writeEnvelope(t, w, "UPID:pve:x")
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)

	upid, err := client.DownloadStorageURL(ctx, "pve", "local", "https://example.invalid/x.iso", "iso", "x.iso", "abc", "sha256", "gz", true)
	if err != nil || upid != "UPID:pve:x" {
		t.Fatalf("DownloadStorageURL() = %q, %v", upid, err)
	}
	upid, owner, err := client.UploadStorageFile(ctx, UploadStorageFileRequest{Node: "pve", Storage: "local", Content: "iso", Filename: "x.iso", Data: []byte("payload"), Checksum: "abc", ChecksumAlgorithm: "sha256"})
	if err != nil || upid != "UPID:pve:x" || owner != "pve" {
		t.Fatalf("UploadStorageFile() = %q %q, %v", upid, owner, err)
	}
}

func TestClientAccessTicketAndTFA(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/access/ticket":
			if r.FormValue("otp") != "" {
				if r.FormValue("realm") != "pam" {
					t.Fatalf("realm = %q", r.FormValue("realm"))
				}
			}
			writeEnvelope(t, w, map[string]any{"ticket": "PVE:ticket", "CSRFPreventionToken": "csrf", "username": "root@pam", "clustername": "pve"})
		case "/api2/json/access/tfa":
			assertTokenAuth(t, r)
			writeEnvelope(t, w, []map[string]any{{"userid": "root@pam", "entries": []map[string]any{{"id": "t1", "type": "totp", "enable": 1, "created": 1}}}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	anonymous, err := NewClient(ctx, ClientConfig{Endpoint: server.URL, Username: "root@pam", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	var ticket struct {
		CSRF     string `json:"CSRFPreventionToken"`
		Ticket   string `json:"ticket"`
		Username string `json:"username"`
	}
	form := url.Values{"username": {"root@pam"}, "password": {"secret"}, "otp": {"123456"}, "realm": {"pam"}}
	if err := anonymous.do(ctx, http.MethodPost, "/access/ticket", nil, form, &ticket); err != nil || ticket.Ticket != "PVE:ticket" {
		t.Fatalf("ticket = %#v, %v", ticket, err)
	}

	client := newHAResourceTestClient(t, server.URL)
	var entries []map[string]any
	if err := client.do(ctx, http.MethodGet, "/access/tfa", nil, nil, &entries); err != nil || len(entries) != 1 {
		t.Fatalf("tfa entries = %#v, %v", entries, err)
	}
	encoded, _ := json.Marshal(entries)
	if !strings.Contains(string(encoded), "root@pam") {
		t.Fatalf("entries = %s", encoded)
	}
}
