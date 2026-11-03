// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestExecuteCommands(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/pve-1/execute" || r.Method != http.MethodPost {
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		var calls []NodeCommandCall
		if err := json.Unmarshal([]byte(r.PostForm.Get("commands")), &calls); err != nil {
			t.Fatalf("decode commands: %v", err)
		}
		if len(calls) != 2 || calls[0].Path != "status" || calls[1].Method != "POST" || calls[1].Args["force"] != "1" {
			t.Fatalf("unexpected calls: %#v", calls)
		}
		handler.envelope(w, []map[string]any{{"status": 200, "data": map[string]any{"uptime": 1}}, {"status": 200}})
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)
	results, err := client.ExecuteCommands(context.Background(), "pve-1", []NodeCommandCall{
		{Method: "GET", Path: "status"},
		{Method: "POST", Path: "stopall", Args: map[string]string{"force": "1"}},
	})
	if err != nil || len(results) != 2 {
		t.Fatalf("ExecuteCommands() = %d results, %v", len(results), err)
	}
	handler.assert(t)
}

func TestExecuteCommandsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"status":500,"message":"boom"}]}`))
	}))
	defer server.Close()
	client := testLifecycleClient(t, server)
	if _, err := client.ExecuteCommands(context.Background(), "pve-1", []NodeCommandCall{{Method: "GET", Path: "status"}}); err == nil {
		t.Fatal("expected failure for status 500")
	}
}

func TestNodeExecuteLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.envelope(w, []map[string]any{{"status": 200, "data": map[string]any{"uptime": 3}}})
	}))
	defer server.Close()
	res := &NodeExecuteResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := nodeExecuteModel{
		Node:     types.StringValue("pve-1"),
		Calls:    []nodeCommandModel{{Method: types.StringValue("GET"), Path: types.StringValue("status"), Args: testStringMap(t, map[string]string{})}},
		Triggers: testStringMap(t, map[string]string{"gen": "1"}),
	}
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("execute diagnostics: %v", resp.Diagnostics)
	}
	var state nodeExecuteModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.ID.ValueString() != "pve-1" || state.Results.ValueString() == "" {
		t.Fatalf("unexpected state: %#v", state)
	}
	_ = attr.Value(nil)
}

func TestMetricsExportDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/metrics/export?history=1&node-list=pve-1": map[string]any{"data": []map[string]any{
			{"id": "node/pve-1", "metric": "uptime", "type": "gauge", "timestamp": 10, "value": 15},
			{"id": "node/pve-1", "metric": "cpu", "type": "gauge", "timestamp": 10, "value": 0.5},
		}},
	}
	readResp := runSDNDataSource(t, NewMetricsExportDataSource(), responses, map[string]any{"node_list": "pve-1", "history": true})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state metricsExportModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() || len(state.Metrics) != 2 || state.Metrics[1].Value.ValueFloat64() != 0.5 {
		t.Fatalf("unexpected projection: %#v (%v)", state.Metrics, diags)
	}
}
