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

func TestReplicationJobResourceFrameworkLifecycle(t *testing.T) {
	exists := true
	getCount := 0
	job := map[string]any{"id": "101-7", "target": "pve target", "comment": "created", "disable": 1, "rate": 12.5, "schedule": "*/10", "source": "pve-source", "guest": 101, "jobnum": 7, "type": "local"}
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.RawQuery != "" {
			handler.fail(w, "unexpected replication query: %s", r.URL.RawQuery)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/cluster/replication":
			if !handler.form(w, r, url.Values{"id": {"101-7"}, "target": {"pve target"}, "type": {"local"}, "comment": {"created"}, "disable": {"1"}, "rate": {"12.5"}, "schedule": {"*/10"}}) {
				return
			}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/cluster/replication/101-7":
			if !exists {
				http.Error(w, "missing replication job", http.StatusNotFound)
				return
			}
			getCount++
			response := make(map[string]any, len(job)+1)
			for key, value := range job {
				response[key] = value
			}
			response["digest"] = "digest-" + strconv.Itoa(getCount)
			handler.envelope(w, response)
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/cluster/replication/101-7":
			if !handler.form(w, r, url.Values{"delete": {"comment,rate,schedule"}, "digest": {"digest-2"}, "disable": {"0"}}) {
				return
			}
			job = map[string]any{"id": "101-7", "target": "pve target", "disable": 0, "source": "pve-source", "guest": 101, "jobnum": 7, "type": "local"}
			handler.envelope(w, nil)
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/cluster/replication/101-7":
			if !handler.form(w, r, url.Values{"force": {"1"}}) {
				return
			}
			if !exists {
				http.Error(w, "missing replication job", http.StatusNotFound)
				return
			}
			exists = false
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected replication request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &ReplicationJobResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := replicationJobModel{JobID: types.StringValue("101-7"), Target: types.StringValue("pve target"), Comment: types.StringValue("created"), Disable: types.BoolValue(true), Rate: types.Float64Value(12.5), Schedule: types.StringValue("*/10")}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial), Config: testResourceConfig(t, schema, initial)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("replication create diagnostics: %v", createResp.Diagnostics)
	}
	var created replicationJobModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode replication state: %v", diags)
	}
	if created.GuestID.ValueInt64() != 101 || created.JobNumber.ValueInt64() != 7 || created.Source.ValueString() != "pve-source" || created.Rate.ValueFloat64() != 12.5 {
		t.Fatalf("unexpected typed replication create state: %#v", created)
	}

	updated := replicationJobModel{JobID: types.StringValue("101-7"), Target: types.StringValue("pve target"), Disable: types.BoolValue(false)}
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}, Private: createResp.Private}
	res.Update(context.Background(), resource.UpdateRequest{Config: testResourceConfig(t, schema, updated), State: createResp.State, Private: createResp.Private}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("replication update diagnostics: %v", updateResp.Diagnostics)
	}
	var updatedState replicationJobModel
	if diags := updateResp.State.Get(context.Background(), &updatedState); diags.HasError() {
		t.Fatalf("decode replication update state: %v", diags)
	}
	if updatedState.Disable.ValueBool() || !updatedState.Comment.IsNull() || !updatedState.Rate.IsNull() || !updatedState.Schedule.IsNull() {
		t.Fatalf("unexpected typed replication update state: %#v", updatedState)
	}

	readResp := resource.ReadResponse{State: updateResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: updateResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("replication read diagnostics: %v", readResp.Diagnostics)
	}
	assertStateString(t, readResp.State, path.Root("type"), "local")
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("replication delete diagnostics: %v", deleteResp.Diagnostics)
	}
	missingResp := resource.ReadResponse{State: readResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: readResp.State}, &missingResp)
	if missingResp.Diagnostics.HasError() || !missingResp.State.Raw.IsNull() {
		t.Fatalf("missing replication job was not removed: diagnostics=%v raw=%v", missingResp.Diagnostics, missingResp.State.Raw)
	}
	var idempotentDelete resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &idempotentDelete)
	if idempotentDelete.Diagnostics.HasError() {
		t.Fatalf("idempotent replication delete diagnostics: %v", idempotentDelete.Diagnostics)
	}
	handler.assert(t)
	wantCalls := []string{"POST /api2/json/cluster/replication", "GET /api2/json/cluster/replication/101-7", "GET /api2/json/cluster/replication/101-7", "PUT /api2/json/cluster/replication/101-7", "GET /api2/json/cluster/replication/101-7", "GET /api2/json/cluster/replication/101-7", "DELETE /api2/json/cluster/replication/101-7", "GET /api2/json/cluster/replication/101-7", "DELETE /api2/json/cluster/replication/101-7"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected replication call order: got %v want %v", calls, wantCalls)
	}

	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, replicationJobModel{ID: types.StringNull(), JobID: types.StringNull()})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "101-7"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("replication import diagnostics: %v", importResp.Diagnostics)
	}
	assertStateString(t, importResp.State, path.Root("job_id"), "101-7")
}

func TestReplicationJobResourceImportedUpdateUsesEmptyPrivateState(t *testing.T) {
	handler := &lifecycleHandler{}
	comment := "external"
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/cluster/replication/101-7" && r.URL.RawQuery == "":
			handler.envelope(w, map[string]any{"id": "101-7", "target": "pve-target", "comment": comment, "source": "pve-source", "guest": 101, "jobnum": 7, "type": "local", "digest": "import-digest"})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/cluster/replication/101-7" && r.URL.RawQuery == "":
			if !handler.form(w, r, url.Values{"comment": {"adopted"}, "digest": {"import-digest"}}) {
				return
			}
			comment = "adopted"
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected imported replication update request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &ReplicationJobResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	prior := replicationJobModel{ID: types.StringValue("101-7"), JobID: types.StringValue("101-7"), Target: types.StringValue("pve-target"), Comment: types.StringValue("external"), Source: types.StringValue("pve-source"), GuestID: types.Int64Value(101), JobNumber: types.Int64Value(7), Type: types.StringValue("local")}
	config := replicationJobModel{JobID: types.StringValue("101-7"), Target: types.StringValue("pve-target"), Comment: types.StringValue("adopted")}
	resp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &resp)
	res.Update(context.Background(), resource.UpdateRequest{Config: testResourceConfig(t, schema, config), State: testResourceState(t, schema, prior), Private: resp.Private}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("imported replication update diagnostics: %v", resp.Diagnostics)
	}
	assertStateString(t, resp.State, path.Root("comment"), "adopted")
	if want := []string{"GET /api2/json/cluster/replication/101-7", "PUT /api2/json/cluster/replication/101-7", "GET /api2/json/cluster/replication/101-7"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected imported replication update order: got %v want %v", calls, want)
	}
	handler.assert(t)
}

func TestReplicationJobResourceReadPreservesAPIError(t *testing.T) {
	handler := &lifecycleHandler{}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls++
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api2/json/cluster/replication/101-7" || r.URL.RawQuery != "" {
			handler.fail(w, "unexpected replication error request: %s %s", r.Method, r.URL.String())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":{"replication":"cluster replication configuration unavailable"}}`))
	}))
	defer server.Close()
	res := &ReplicationJobResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := testResourceState(t, schema, replicationJobModel{ID: types.StringValue("101-7"), JobID: types.StringValue("101-7")})
	resp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "status 500") || !containsDiagnostic(resp.Diagnostics, "cluster replication configuration unavailable") {
		t.Fatalf("expected preserved replication API detail, got %v", resp.Diagnostics)
	}
	if calls != 1 || !resp.State.Raw.Equal(state.Raw) {
		t.Fatalf("replication API error caused unexpected calls or state mutation: calls=%d raw=%v", calls, resp.State.Raw)
	}
	handler.assert(t)
}
