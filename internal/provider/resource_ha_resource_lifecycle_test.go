// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestHAResourceFrameworkLifecycleFreshDigestAndSafeDelete(t *testing.T) {
	exists := true
	getCount := 0
	haState := map[string]any{"sid": "vm:120", "state": "started", "comment": "created", "failback": 0, "auto-rebalance": 0, "max_restart": 2, "max_relocate": 3}
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.RawQuery != "" {
			handler.fail(w, "unexpected HA query: %s", r.URL.RawQuery)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/cluster/ha/resources":
			if !handler.form(w, r, url.Values{"sid": {"vm:120"}, "state": {"started"}, "comment": {"created"}, "failback": {"0"}, "auto-rebalance": {"0"}, "max_restart": {"2"}, "max_relocate": {"3"}}) {
				return
			}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/cluster/ha/resources":
			getCount++
			if !exists {
				handler.envelope(w, []any{})
				return
			}
			response := make(map[string]any, len(haState)+1)
			for key, value := range haState {
				response[key] = value
			}
			response["digest"] = "ha-digest-" + strconv.Itoa(getCount)
			handler.envelope(w, []map[string]any{response})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/cluster/ha/resources/vm:120":
			if !handler.form(w, r, url.Values{"state": {"stopped"}, "auto-rebalance": {"0"}, "max_relocate": {"3"}, "delete": {"comment,failback,max_restart"}, "digest": {"ha-digest-2"}}) {
				return
			}
			haState = map[string]any{"sid": "vm:120", "state": "stopped", "auto-rebalance": 0, "max_relocate": 3}
			handler.envelope(w, nil)
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/cluster/ha/resources/vm:120":
			if !handler.form(w, r, url.Values{"purge": {"0"}}) {
				return
			}
			if !exists {
				handler.fail(w, "HA delete must not be called after collection reported missing")
				return
			}
			exists = false
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected HA request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &HAResourceResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := haResourceModel{ResourceID: types.StringValue("vm:120"), State: types.StringValue("started"), Comment: types.StringValue("created"), Failback: types.BoolValue(false), AutoRebalance: types.BoolValue(false), MaxRestart: types.Int64Value(2), MaxRelocate: types.Int64Value(3)}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial), Config: testResourceConfig(t, schema, initial)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("HA create diagnostics: %v", createResp.Diagnostics)
	}
	var created haResourceModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode HA create state: %v", diags)
	}
	if created.State.ValueString() != "started" || created.Failback.ValueBool() || created.AutoRebalance.ValueBool() || created.MaxRestart.ValueInt64() != 2 || created.MaxRelocate.ValueInt64() != 3 {
		t.Fatalf("unexpected typed HA create state: %#v", created)
	}

	updated := haResourceModel{ResourceID: types.StringValue("vm:120"), State: types.StringValue("stopped"), AutoRebalance: types.BoolValue(false), MaxRelocate: types.Int64Value(3)}
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}, Private: createResp.Private}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, updated), Config: testResourceConfig(t, schema, updated), State: createResp.State, Private: createResp.Private}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("HA update diagnostics: %v", updateResp.Diagnostics)
	}
	var updatedState haResourceModel
	if diags := updateResp.State.Get(context.Background(), &updatedState); diags.HasError() {
		t.Fatalf("decode HA update state: %v", diags)
	}
	if updatedState.State.ValueString() != "stopped" || updatedState.AutoRebalance.ValueBool() || updatedState.MaxRelocate.ValueInt64() != 3 || !updatedState.Comment.IsNull() || !updatedState.Failback.ValueBool() || updatedState.MaxRestart.ValueInt64() != 1 {
		t.Fatalf("unexpected typed HA update/default state: %#v", updatedState)
	}

	readResp := resource.ReadResponse{State: updateResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: updateResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("HA read diagnostics: %v", readResp.Diagnostics)
	}
	assertStateString(t, readResp.State, path.Root("state"), "stopped")
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("HA delete diagnostics: %v", deleteResp.Diagnostics)
	}
	missingResp := resource.ReadResponse{State: readResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: readResp.State}, &missingResp)
	if missingResp.Diagnostics.HasError() || !missingResp.State.Raw.IsNull() {
		t.Fatalf("missing HA resource was not removed: diagnostics=%v raw=%v", missingResp.Diagnostics, missingResp.State.Raw)
	}
	var idempotentDelete resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &idempotentDelete)
	if idempotentDelete.Diagnostics.HasError() {
		t.Fatalf("idempotent HA delete diagnostics: %v", idempotentDelete.Diagnostics)
	}
	handler.assert(t)
	wantCalls := []string{"POST /api2/json/cluster/ha/resources", "GET /api2/json/cluster/ha/resources", "GET /api2/json/cluster/ha/resources", "PUT /api2/json/cluster/ha/resources/vm:120", "GET /api2/json/cluster/ha/resources", "GET /api2/json/cluster/ha/resources", "GET /api2/json/cluster/ha/resources", "DELETE /api2/json/cluster/ha/resources/vm:120", "GET /api2/json/cluster/ha/resources", "GET /api2/json/cluster/ha/resources"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected HA call order: got %v want %v", calls, wantCalls)
	}

	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, haResourceModel{ID: types.StringNull(), ResourceID: types.StringNull()})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "ct:121"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("HA import diagnostics: %v", importResp.Diagnostics)
	}
	assertStateString(t, importResp.State, path.Root("resource_id"), "ct:121")
}

func TestHAResourceUpdatePreservesFreshReadError(t *testing.T) {
	handler := &lifecycleHandler{}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls++
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api2/json/cluster/ha/resources" || r.URL.RawQuery != "" {
			handler.fail(w, "unexpected HA error request: %s %s", r.Method, r.URL.String())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"errors":{"ha":"HA manager quorum unavailable"}}`))
	}))
	defer server.Close()
	res := &HAResourceResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := haResourceModel{ID: types.StringValue("vm:120"), ResourceID: types.StringValue("vm:120"), State: types.StringValue("started")}
	resp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &resp)
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, state), Config: testResourceConfig(t, schema, state), State: testResourceState(t, schema, state), Private: resp.Private}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "status 409") || !containsDiagnostic(resp.Diagnostics, "HA manager quorum unavailable") {
		t.Fatalf("expected preserved HA API detail, got %v", resp.Diagnostics)
	}
	if calls != 1 || !resp.State.Raw.IsNull() {
		t.Fatalf("HA API error caused unexpected calls or state mutation: calls=%d raw=%v", calls, resp.State.Raw)
	}
	handler.assert(t)
}
