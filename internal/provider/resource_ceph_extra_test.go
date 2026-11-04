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
)

func TestCephExtraClient(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/ceph/fs") && r.Method == http.MethodGet:
			handler.envelope(w, []map[string]any{{"name": "fs1", "data_pool": "d", "metadata_pool": "m"}})
		case strings.HasSuffix(r.URL.Path, "/ceph/fs/fs1") && r.Method == http.MethodPost:
			handler.form(w, r, url.Values{"name": {"fs1"}})
			handler.envelope(w, commandTaskUPID)
		case strings.HasSuffix(r.URL.Path, "/ceph/mds") && r.Method == http.MethodGet:
			handler.envelope(w, []map[string]any{{"name": "mds1", "fs_name": "fs1", "state": "up:active"}})
		case strings.HasSuffix(r.URL.Path, "/ceph/mgr") && r.Method == http.MethodGet:
			handler.envelope(w, []map[string]any{{"name": "mgr1", "state": "active"}})
		case strings.HasSuffix(r.URL.Path, "/ceph/crush"):
			handler.envelope(w, "crush text")
		case strings.HasSuffix(r.URL.Path, "/ceph/rules"):
			handler.envelope(w, []map[string]any{{"name": "replicated_rule"}})
		case strings.HasSuffix(r.URL.Path, "/ceph/cfg/db"):
			handler.envelope(w, []map[string]any{{"name": "x", "can_update_at_runtime": 1}})
		case strings.HasSuffix(r.URL.Path, "/ceph/cfg/raw"):
			handler.envelope(w, "raw cfg")
		case strings.HasSuffix(r.URL.Path, "/ceph/cfg/value"):
			handler.envelope(w, map[string]any{"global": map[string]any{"k": "v"}})
		case strings.HasSuffix(r.URL.Path, "/ceph/log"):
			handler.envelope(w, []map[string]any{{"n": 1, "t": "line"}})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)
	if fs, err := client.CephFS(context.Background(), "pve-1"); err != nil || len(fs) != 1 {
		t.Fatalf("CephFS() error: %v", err)
	}
	if _, err := client.CreateCephFS(context.Background(), "pve-1", "fs1", 0, false); err != nil {
		t.Fatalf("CreateCephFS() error: %v", err)
	}
	if crush, err := client.CephCrush(context.Background(), "pve-1"); err != nil || crush != "crush text" {
		t.Fatalf("CephCrush() = %q, %v", crush, err)
	}
	if rules, err := client.CephRules(context.Background(), "pve-1"); err != nil || len(rules) != 1 {
		t.Fatalf("CephRules() error: %v", err)
	}
	if entries, err := client.CephConfigDB(context.Background(), "pve-1"); err != nil || entries[0].CanUpdateAtRuntime.Ptr() == nil {
		t.Fatalf("CephConfigDB() error: %v", err)
	}
	if value, err := client.CephConfigValue(context.Background(), "pve-1", []string{"global:k"}); err != nil || !strings.Contains(value, "v") {
		t.Fatalf("CephConfigValue() = %q, %v", value, err)
	}
	handler.assert(t)
}

func TestCephExtraDataSources(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/ceph/fs":          []map[string]any{{"name": "fs1", "data_pool": "d", "metadata_pool": "m"}},
		"/api2/json/nodes/pve-1/ceph/mds":         []map[string]any{{"name": "mds1", "fs_name": "fs1", "state": "up:active"}},
		"/api2/json/nodes/pve-1/ceph/mgr":         []map[string]any{{"name": "mgr1", "state": "active"}},
		"/api2/json/nodes/pve-1/ceph/log?limit=2": []map[string]any{{"n": 1, "t": "line"}},
	}
	if resp := runSDNDataSource(t, NewCephFSDataSource(), responses, map[string]any{"node": "pve-1"}); resp.Diagnostics.HasError() {
		t.Fatalf("fs ds failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewCephMDSDataSource(), responses, map[string]any{"node": "pve-1"}); resp.Diagnostics.HasError() {
		t.Fatalf("mds ds failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewCephMGRDataSource(), responses, map[string]any{"node": "pve-1"}); resp.Diagnostics.HasError() {
		t.Fatalf("mgr ds failed: %v", resp.Diagnostics)
	}
	if resp := runSDNDataSource(t, NewCephLogDataSource(), responses, map[string]any{"node": "pve-1", "limit": int64(2)}); resp.Diagnostics.HasError() {
		t.Fatalf("log ds failed: %v", resp.Diagnostics)
	}
}
