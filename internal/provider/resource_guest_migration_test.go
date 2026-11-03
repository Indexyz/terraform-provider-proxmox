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

const migrationUPID = "UPID:pve-1:0000:0000:0000:qmigrate:300:root@pam:"

func TestMigrateGuestForms(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve-1/qemu/300/migrate":
			if !handler.form(w, r, url.Values{"target": {"pve-2"}, "online": {"1"}, "with-local-disks": {"1"}, "targetstorage": {"backup"}}) {
				return
			}
			handler.envelope(w, migrationUPID)
		case "/api2/json/nodes/pve-1/lxc/400/migrate":
			if !handler.form(w, r, url.Values{"target": {"pve-2"}, "restart": {"0"}, "target-storage": {"backup"}, "timeout": {"60"}}) {
				return
			}
			handler.envelope(w, migrationUPID)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)
	if _, err := client.MigrateGuest(context.Background(), "qemu", "pve-1", 300, "pve-2", true, true, true, 60, "backup"); err != nil {
		t.Fatalf("qemu migrate unexpected error: %v", err)
	}
	if _, err := client.MigrateGuest(context.Background(), "lxc", "pve-1", 400, "pve-2", false, false, false, 60, "backup"); err != nil {
		t.Fatalf("lxc migrate unexpected error: %v", err)
	}
	handler.assert(t)
}

func TestGuestMigrationLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/migrate"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":"` + migrationUPID + `"}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"status":"stopped","exitstatus":"OK"}}`))
		}
	}))
	defer server.Close()
	res := &GuestMigrationResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := guestMigrationModel{
		Type: types.StringValue("qemu"), Node: types.StringValue("pve-1"), VMID: types.Int64Value(300), Target: types.StringValue("pve-2"),
		Online: types.BoolValue(true), WithLocalDisks: types.BoolValue(false), Restart: types.BoolValue(true), Timeout: types.Int64Value(180),
		Triggers: testStringMap(t, map[string]string{"gen": "1"}),
	}
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("migration diagnostics: %v", resp.Diagnostics)
	}
	var state guestMigrationModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.ID.ValueString() != "qemu/300" || state.UPID.ValueString() != migrationUPID {
		t.Fatalf("unexpected state: %#v", state)
	}
}
