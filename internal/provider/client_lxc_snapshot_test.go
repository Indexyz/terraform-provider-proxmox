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

func TestClientLXCSnapshotMethods(t *testing.T) {
	ctx := context.Background()
	withLXCContainerTaskTiming(t, time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/lxc/101/snapshot" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{
				"snapname":    {"pre-deploy"},
				"description": {"before terraform apply"},
			})
			writeEnvelope(t, w, "UPID:pve-1:0001:snapshot:101:")
		case isLXCContainerTaskRequest(r, "UPID:pve-1:0001:snapshot:101:"):
			writeEnvelope(t, w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.URL.Path == "/api2/json/nodes/pve-1/lxc/101/snapshot" && r.Method == http.MethodGet:
			writeEnvelope(t, w, []map[string]any{
				{"name": "pre-deploy", "description": "before terraform apply", "parent": "base", "snaptime": 1700000000},
				{"name": "other", "description": "unrelated"},
			})
		case r.URL.Path == "/api2/json/nodes/pve-1/lxc/101/snapshot/pre-deploy/config" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{"description": {"updated"}})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/nodes/pve-1/lxc/101/snapshot/pre-deploy" && r.Method == http.MethodDelete:
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

	if err := client.CreateLXCSnapshot(ctx, CreateLXCSnapshotRequest{
		Node:        "pve-1",
		VMID:        101,
		Name:        "pre-deploy",
		Description: stringPtr("before terraform apply"),
	}); err != nil {
		t.Fatalf("CreateLXCSnapshot() unexpected error: %v", err)
	}

	snap, err := client.GetLXCSnapshot(ctx, "pve-1", 101, "pre-deploy")
	if err != nil {
		t.Fatalf("GetLXCSnapshot() unexpected error: %v", err)
	}
	if snap.Name != "pre-deploy" || snap.Description != "before terraform apply" || snap.Parent != "base" {
		t.Fatalf("unexpected snapshot: %#v", snap)
	}
	if snap.Snaptime.Ptr() == nil || *snap.Snaptime.Ptr() != 1700000000 {
		t.Fatalf("unexpected snaptime: %#v", snap.Snaptime)
	}

	if err := client.UpdateLXCSnapshot(ctx, "pve-1", 101, "pre-deploy", "updated"); err != nil {
		t.Fatalf("UpdateLXCSnapshot() unexpected error: %v", err)
	}

	if err := client.DeleteLXCSnapshot(ctx, "pve-1", 101, "pre-deploy"); err != nil {
		t.Fatalf("DeleteLXCSnapshot() unexpected error: %v", err)
	}
}

func TestClientLXCSnapshotGetNotFound(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, []map[string]any{})
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

	_, err = client.GetLXCSnapshot(ctx, "pve-1", 101, "missing")
	if err == nil {
		t.Fatal("expected error for missing snapshot")
	}
}

func TestParseLXCSnapshotImportID(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		input   string
		wantErr bool
	}{
		{"pve-1/101/pre-deploy", false},
		{"pve-1/101/", true},
		{"pve-1//pre-deploy", true},
		{"pve-1/101", true},
		{"pve-1/abc/pre-deploy", true},
	} {
		node, vmID, name, err := parseLXCSnapshotImportID(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("expected error for %q, got %q/%d/%q", tc.input, node, vmID, name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", tc.input, err)
		}
		if node != "pve-1" || vmID != 101 || name != "pre-deploy" {
			t.Fatalf("unexpected parse for %q: %q/%d/%q", tc.input, node, vmID, name)
		}
	}
}
