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
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const nodePowerUPID = "UPID:pve-1:0000:0000:0000:stopall:::"

func TestNodeStatusCommandForm(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/pve-1/status" || r.Method != http.MethodPost {
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
			return
		}
		if !handler.form(w, r, url.Values{"command": {"reboot"}}) {
			return
		}
		handler.envelope(w, nil)
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)
	if err := client.NodeStatusCommand(context.Background(), "pve-1", "reboot"); err != nil {
		t.Fatalf("NodeStatusCommand() unexpected error: %v", err)
	}
	handler.assert(t)
}

func TestNodePowerActionForms(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve-1/stopall":
			if !handler.form(w, r, url.Values{"vms": {"300,301"}, "force-stop": {"1"}, "timeout": {"30"}, "max-workers": {"2"}}) {
				return
			}
			handler.envelope(w, nodePowerUPID)
		case "/api2/json/nodes/pve-1/suspendall":
			if !handler.form(w, r, url.Values{"vms": {"300,301"}}) {
				return
			}
			handler.envelope(w, nodePowerUPID)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)
	if _, err := client.NodePowerAction(context.Background(), "pve-1", "stopall", "300,301", true, 30, 2); err != nil {
		t.Fatalf("stopall unexpected error: %v", err)
	}
	if _, err := client.NodePowerAction(context.Background(), "pve-1", "suspendall", "300,301", false, 0, 0); err != nil {
		t.Fatalf("suspendall unexpected error: %v", err)
	}
	handler.assert(t)
}

func TestNodeStatusCommandLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":null}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"uptime":1}}`))
	}))
	defer server.Close()
	res := &NodeStatusCommandResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := nodeStatusCommandModel{
		Node: types.StringValue("pve-1"), Command: types.StringValue("reboot"),
		WaitSeconds: types.Int64Value(0), Triggers: testStringMap(t, map[string]string{"gen": "1"}),
	}
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("status command diagnostics: %v", resp.Diagnostics)
	}
	var state nodeStatusCommandModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.ID.ValueString() != "reboot/pve-1" {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestNodeStopAllLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/stopall":
			handler.envelope(w, "UPID:pve-1:0000:0000:0000:stopall:::")
		case strings.HasPrefix(r.URL.Path, "/api2/json/nodes/pve-1/tasks/"):
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	res := &NodeStopAllResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := nodePowerActionModel{
		Node: types.StringValue("pve-1"), VMs: types.SetValueMust(types.Int64Type, []attr.Value{types.Int64Value(300)}),
		ForceStop: types.BoolValue(true), Timeout: types.Int64Value(10), MaxWorkers: types.Int64Value(2),
		Triggers: testStringMap(t, map[string]string{"gen": "1"}),
	}
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("stopall diagnostics: %v", resp.Diagnostics)
	}
	var state nodePowerActionModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.ID.ValueString() != "pve-1" || state.UPID.ValueString() == "" {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestNodeSuspendAllLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/suspendall":
			handler.envelope(w, "UPID:pve-1:0000:0000:0000:suspendall:::")
		case strings.HasPrefix(r.URL.Path, "/api2/json/nodes/pve-1/tasks/"):
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	res := &NodeSuspendAllResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := nodeSuspendAllModel{
		Node: types.StringValue("pve-1"), VMs: types.SetValueMust(types.Int64Type, []attr.Value{}),
		MaxWorkers: types.Int64Value(0), Triggers: testStringMap(t, map[string]string{"gen": "1"}),
	}
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("suspendall diagnostics: %v", resp.Diagnostics)
	}
}

func TestWaitNodeOnline(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"uptime":1}}`))
	}))
	defer server.Close()
	client := testLifecycleClient(t, server)
	if err := client.WaitNodeOnline(context.Background(), "pve-1", 10*time.Second); err != nil {
		t.Fatalf("WaitNodeOnline() unexpected error: %v", err)
	}
	if calls < 3 {
		t.Fatalf("expected at least 3 probes, got %d", calls)
	}
}
