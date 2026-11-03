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

const bulkActionUPID = "UPID:pve-1:0000:0000:0000:bulkstart::root@pam:"

func TestClientRunBulkAction(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/cluster/bulk-action/guest/start" || r.Method != http.MethodPost {
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
			return
		}
		if !handler.form(w, r, url.Values{"vms": {"300,301"}, "max-workers": {"2"}}) {
			return
		}
		handler.envelope(w, bulkActionUPID)
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)
	upid, err := client.RunBulkAction(context.Background(), "start", []int64{300, 301}, "", false, false, 2)
	if err != nil || upid != bulkActionUPID {
		t.Fatalf("RunBulkAction() = %q, %v", upid, err)
	}
	handler.assert(t)
}

func TestClusterBulkActionValidation(t *testing.T) {
	set := types.SetValueMust(types.Int64Type, []attr.Value{types.Int64Value(300)})
	triggers := testStringMap(t, map[string]string{"gen": "1"})
	valid := clusterBulkActionModel{Action: types.StringValue("start"), VMs: set, Triggers: triggers}
	var ok resource.ValidateConfigResponse
	(&ClusterBulkActionResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &ClusterBulkActionResource{}), valid)}, &ok)
	if ok.Diagnostics.HasError() {
		t.Fatalf("valid diagnostics: %v", ok.Diagnostics)
	}
	invalid := map[string]clusterBulkActionModel{
		"bad action":        {Action: types.StringValue("pause"), VMs: set, Triggers: triggers},
		"migrate no target": {Action: types.StringValue("migrate"), VMs: set, Triggers: triggers},
		"empty set":         {Action: types.StringValue("start"), VMs: types.SetValueMust(types.Int64Type, []attr.Value{}), Triggers: triggers},
	}
	for name, model := range invalid {
		var resp resource.ValidateConfigResponse
		(&ClusterBulkActionResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &ClusterBulkActionResource{}), model)}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("%s: expected diagnostics", name)
		}
	}
}

func TestClusterBulkActionLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/cluster/bulk-action/guest/shutdown" && r.Method == http.MethodPost:
			handler.envelope(w, bulkActionUPID)
		case strings.HasPrefix(r.URL.Path, "/api2/json/nodes/pve-1/tasks/"):
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	res := &ClusterBulkActionResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := clusterBulkActionModel{
		Action: types.StringValue("shutdown"),
		VMs:    types.SetValueMust(types.Int64Type, []attr.Value{types.Int64Value(300), types.Int64Value(301)}),
		Online: types.BoolValue(false), WithLocalDisks: types.BoolValue(false), MaxWorkers: types.Int64Value(4),
		Triggers: testStringMap(t, map[string]string{"gen": "1"}),
	}
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("bulk diagnostics: %v", resp.Diagnostics)
	}
	var state clusterBulkActionModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.ID.ValueString() != "shutdown/300,301" || state.UPID.ValueString() != bulkActionUPID {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestNotificationTargetTestLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/cluster/notifications/targets/mail/test" || r.Method != http.MethodPost {
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
			return
		}
		handler.envelope(w, nil)
	}))
	defer server.Close()
	res := &NotificationTargetTestResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := notificationTargetTestModel{Name: types.StringValue("mail"), Triggers: testStringMap(t, map[string]string{"gen": "1"})}
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("target test diagnostics: %v", resp.Diagnostics)
	}
	var state notificationTargetTestModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.ID.ValueString() != "mail" {
		t.Fatalf("unexpected state: %#v", state)
	}
	handler.assert(t)
}
