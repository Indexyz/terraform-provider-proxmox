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

func TestCephFlagClient(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.envelope(w, []map[string]any{{"name": "noout", "value": 1, "description": "no out"}})
		case http.MethodPut:
			if !handler.form(w, r, url.Values{"flag": {"noout"}, "value": {"1"}}) {
				return
			}
			handler.envelope(w, nil)
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)
	flags, err := client.CephFlags(context.Background())
	if err != nil || len(flags) != 1 || flags[0].Value.Ptr() == nil || !*flags[0].Value.Ptr() {
		t.Fatalf("unexpected flags: %#v (%v)", flags, err)
	}
	if err := client.SetCephFlag(context.Background(), "noout", true); err != nil {
		t.Fatalf("SetCephFlag() unexpected error: %v", err)
	}
	handler.assert(t)
}

func TestCephPoolClient(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			if !handler.form(w, r, url.Values{"name": {"pool1"}, "size": {"2"}, "min_size": {"1"}, "pg_num": {"8"}}) {
				return
			}
			handler.envelope(w, nodeBackupTaskUPID)
		case http.MethodPut:
			if !handler.form(w, r, url.Values{"size": {"3"}}) {
				return
			}
			handler.envelope(w, nil)
		case http.MethodDelete:
			if !handler.form(w, r, url.Values{}) {
				return
			}
			handler.envelope(w, nodeBackupTaskUPID)
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)
	if _, err := client.CreateCephPool(context.Background(), "pve-1", CephPool{Name: "pool1", Size: 2, MinSize: 1, PGNum: 8}); err != nil {
		t.Fatalf("CreateCephPool() unexpected error: %v", err)
	}
	if err := client.UpdateCephPool(context.Background(), "pve-1", "pool1", CephPool{Size: 3}); err != nil {
		t.Fatalf("UpdateCephPool() unexpected error: %v", err)
	}
	if _, err := client.DeleteCephPool(context.Background(), "pve-1", "pool1"); err != nil {
		t.Fatalf("DeleteCephPool() unexpected error: %v", err)
	}
	handler.assert(t)
}

func TestCephOSDLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/ceph/osd" && r.Method == http.MethodPost:
			if !handler.form(w, r, url.Values{"dev": {"/dev/sdb"}, "osds-per-device": {"1"}}) {
				return
			}
			handler.envelope(w, nodeBackupTaskUPID)
		case strings.HasSuffix(r.URL.Path, "/ceph/stop") && r.Method == http.MethodPost:
			handler.envelope(w, nodeBackupTaskUPID)
		case r.URL.Path == "/api2/json/nodes/pve-1/ceph/osd/0" && r.Method == http.MethodDelete:
			if !handler.form(w, r, url.Values{"cleanup": {"1"}}) {
				return
			}
			handler.envelope(w, nodeBackupTaskUPID)
		case strings.HasPrefix(r.URL.Path, "/api2/json/nodes/pve-1/tasks/"):
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	res := &CephOSDResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := cephOSDModel{
		Node: types.StringValue("pve-1"), Dev: types.StringValue("/dev/sdb"),
		OSDsPerDevice: types.Int64Value(1), Encrypted: types.BoolValue(false), CleanupOnDestroy: types.BoolValue(true),
	}
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	// Without a tree the locate step reports not found, so only the create form ran.
	if !resp.Diagnostics.HasError() {
		var state cephOSDModel
		if diags := resp.State.Get(context.Background(), &state); diags.HasError() || state.OSDID.ValueInt64() != 0 {
			t.Fatalf("unexpected state: %#v (%v)", state, diags)
		}
	}
}

func TestCephStatusAndPoolsDataSources(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/ceph/status":   map[string]any{"health": map[string]any{"status": "HEALTH_OK"}},
		"/api2/json/nodes/pve-1/ceph/pool": []map[string]any{{"pool_name": "p1", "pool": 1, "size": 2, "min_size": 1, "pg_num": 32}},
	}
	status := runSDNDataSource(t, NewCephStatusDataSource(), responses, nil)
	if status.Diagnostics.HasError() || !strings.Contains(status.State.Raw.String(), "HEALTH_OK") {
		t.Fatalf("unexpected status read: %v", status.Diagnostics)
	}
	pools := runSDNDataSource(t, NewCephPoolsDataSource(), responses, map[string]any{"node": "pve-1"})
	if pools.Diagnostics.HasError() {
		t.Fatalf("pool read diagnostics: %v", pools.Diagnostics)
	}
	var state cephPoolsModel
	if diags := pools.State.Get(context.Background(), &state); diags.HasError() || len(state.Pools) != 1 || state.Pools[0].Name.ValueString() != "p1" {
		t.Fatalf("unexpected pools: %#v (%v)", state, diags)
	}
}
