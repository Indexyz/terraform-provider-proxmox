// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestClientQemuSnapshotMethods(t *testing.T) {
	ctx := context.Background()
	withLXCContainerTaskTiming(t, time.Millisecond, time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/qemu/101/snapshot" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{
				"snapname":    {"pre-deploy"},
				"description": {"before terraform apply"},
			})
			writeEnvelope(t, w, "UPID:pve-1:0001:snapshot:101:")
		case isLXCContainerTaskRequest(r, "UPID:pve-1:0001:snapshot:101:"):
			writeEnvelope(t, w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.URL.Path == "/api2/json/nodes/pve-1/qemu/101/snapshot" && r.Method == http.MethodGet:
			writeEnvelope(t, w, []map[string]any{
				{"name": "pre-deploy", "description": "before terraform apply", "parent": "base", "snaptime": 1700000000},
			})
		case r.URL.Path == "/api2/json/nodes/pve-1/qemu/101/snapshot/pre-deploy/config" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{"description": {"updated"}})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/nodes/pve-1/qemu/101/snapshot/pre-deploy" && r.Method == http.MethodDelete:
			writeEnvelope(t, w, "UPID:pve-1:0002:delsnapshot:101:")
		case isLXCContainerTaskRequest(r, "UPID:pve-1:0002:delsnapshot:101:"):
			writeEnvelope(t, w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewClient(ctx, ClientConfig{
		Endpoint:       server.URL,
		APITokenID:     "terraform@pve!provider",
		APITokenSecret: "token-secret",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}

	if err := client.CreateQemuSnapshot(ctx, CreateQemuSnapshotRequest{
		Node:        "pve-1",
		VMID:        101,
		Name:        "pre-deploy",
		Description: stringPtr("before terraform apply"),
	}); err != nil {
		t.Fatalf("CreateQemuSnapshot() unexpected error: %v", err)
	}

	snap, err := client.GetQemuSnapshot(ctx, "pve-1", 101, "pre-deploy")
	if err != nil {
		t.Fatalf("GetQemuSnapshot() unexpected error: %v", err)
	}
	if snap.Name != "pre-deploy" || snap.Description != "before terraform apply" || snap.Parent != "base" {
		t.Fatalf("unexpected snapshot: %#v", snap)
	}
	if snap.Snaptime.Ptr() == nil || *snap.Snaptime.Ptr() != 1700000000 {
		t.Fatalf("unexpected snaptime: %#v", snap.Snaptime)
	}

	if err := client.UpdateQemuSnapshot(ctx, "pve-1", 101, "pre-deploy", "updated"); err != nil {
		t.Fatalf("UpdateQemuSnapshot() unexpected error: %v", err)
	}

	if err := client.DeleteQemuSnapshot(ctx, "pve-1", 101, "pre-deploy"); err != nil {
		t.Fatalf("DeleteQemuSnapshot() unexpected error: %v", err)
	}
}

func TestParseQemuSnapshotImportID(t *testing.T) {
	t.Parallel()

	node, vmID, name, err := parseQemuSnapshotImportID("pve-1/101/pre-deploy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node != "pve-1" || vmID != 101 || name != "pre-deploy" {
		t.Fatalf("unexpected parse: %q/%d/%q", node, vmID, name)
	}

	if _, _, _, err := parseQemuSnapshotImportID("pve-1/101"); err == nil {
		t.Fatal("expected error for malformed import id")
	}
}
