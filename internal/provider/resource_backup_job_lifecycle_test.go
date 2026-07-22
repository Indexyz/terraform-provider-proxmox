// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestBackupJobResourceFrameworkLifecycle(t *testing.T) {
	exists := true
	job := map[string]any{
		"id": "nightly unit", "all": 1, "comment": "created", "enabled": 1,
		"exclude": "900", "mode": "snapshot", "next-run": 1730000000,
		"prune-backups": map[string]any{"keep-last": 3, "keep-daily": 7},
		"remove":        1, "schedule": "mon..fri 02:30", "storage": "backup store",
	}
	handler := &lifecycleHandler{}
	var calls []string
	var forms []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.RawQuery != "" {
			handler.fail(w, "unexpected backup query: %s", r.URL.RawQuery)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/cluster/backup":
			want := url.Values{"id": {"nightly unit"}, "all": {"1"}, "comment": {"created"}, "enabled": {"1"}, "exclude": {"900"}, "mode": {"snapshot"}, "prune-backups": {"keep-last=3,keep-daily=7"}, "remove": {"1"}, "schedule": {"mon..fri 02:30"}, "storage": {"backup store"}}
			if !handler.form(w, r, want) {
				return
			}
			forms = append(forms, r.Form.Encode())
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/cluster/backup/nightly%20unit":
			if !exists {
				http.Error(w, `{"message":"missing backup job"}`, http.StatusNotFound)
				return
			}
			handler.envelope(w, job)
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/cluster/backup/nightly%20unit":
			want := url.Values{"delete": {"all,comment,exclude,prune-backups,schedule"}, "enabled": {"0"}, "mode": {"stop"}, "pool": {"critical"}, "remove": {"0"}, "storage": {"backup store"}}
			if !handler.form(w, r, want) {
				return
			}
			forms = append(forms, r.Form.Encode())
			job = map[string]any{"id": "nightly unit", "enabled": 0, "mode": "stop", "next-run": 1730000500, "pool": "critical", "remove": 0, "storage": "backup store"}
			handler.envelope(w, nil)
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/cluster/backup/nightly%20unit":
			if !handler.form(w, r, url.Values{}) {
				return
			}
			if !exists {
				http.Error(w, `{"message":"missing backup job"}`, http.StatusNotFound)
				return
			}
			exists = false
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected backup request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &BackupJobResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := backupJobModel{
		JobID: types.StringValue("nightly unit"), All: types.BoolValue(true),
		Comment: types.StringValue("created"), Enabled: types.BoolValue(true), ExcludeVMIDs: types.StringValue("900"),
		Mode: types.StringValue("snapshot"), PruneBackups: types.StringValue("keep-last=3,keep-daily=7"),
		Remove: types.BoolValue(true), Schedule: types.StringValue("mon..fri 02:30"), Storage: types.StringValue("backup store"),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial), Config: testResourceConfig(t, schema, initial)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("backup create diagnostics: %v", createResp.Diagnostics)
	}
	var created backupJobModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode backup create state: %v", diags)
	}
	if created.NextRun.ValueInt64() != 1730000000 || created.PruneBackups.ValueString() != "keep-daily=7,keep-last=3" || !created.All.ValueBool() {
		t.Fatalf("unexpected typed backup create state: %#v", created)
	}

	updated := backupJobModel{
		JobID: types.StringValue("nightly unit"), Enabled: types.BoolValue(false), Mode: types.StringValue("stop"),
		Pool: types.StringValue("critical"), Remove: types.BoolValue(false), Storage: types.StringValue("backup store"),
	}
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}, Private: createResp.Private}
	res.Update(context.Background(), resource.UpdateRequest{Config: testResourceConfig(t, schema, updated), State: createResp.State, Private: createResp.Private}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("backup update diagnostics: %v", updateResp.Diagnostics)
	}
	var updatedState backupJobModel
	if diags := updateResp.State.Get(context.Background(), &updatedState); diags.HasError() {
		t.Fatalf("decode backup update state: %v", diags)
	}
	if updatedState.Pool.ValueString() != "critical" || updatedState.Enabled.ValueBool() || updatedState.Remove.ValueBool() || !updatedState.Schedule.IsNull() || !updatedState.PruneBackups.IsNull() {
		t.Fatalf("unexpected typed backup update state: %#v", updatedState)
	}

	readResp := resource.ReadResponse{State: updateResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: updateResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("backup read diagnostics: %v", readResp.Diagnostics)
	}
	assertStateString(t, readResp.State, path.Root("mode"), "stop")
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("backup delete diagnostics: %v", deleteResp.Diagnostics)
	}
	missingResp := resource.ReadResponse{State: readResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: readResp.State}, &missingResp)
	if missingResp.Diagnostics.HasError() || !missingResp.State.Raw.IsNull() {
		t.Fatalf("missing backup was not removed: diagnostics=%v raw=%v", missingResp.Diagnostics, missingResp.State.Raw)
	}
	var idempotentDelete resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &idempotentDelete)
	if idempotentDelete.Diagnostics.HasError() {
		t.Fatalf("idempotent backup delete diagnostics: %v", idempotentDelete.Diagnostics)
	}
	handler.assert(t)
	wantCalls := []string{"POST /api2/json/cluster/backup", "GET /api2/json/cluster/backup/nightly%20unit", "PUT /api2/json/cluster/backup/nightly%20unit", "GET /api2/json/cluster/backup/nightly%20unit", "GET /api2/json/cluster/backup/nightly%20unit", "DELETE /api2/json/cluster/backup/nightly%20unit", "GET /api2/json/cluster/backup/nightly%20unit", "DELETE /api2/json/cluster/backup/nightly%20unit"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected backup call order: got %v want %v", calls, wantCalls)
	}
	if len(forms) != 2 {
		t.Fatalf("unexpected backup forms: %v", forms)
	}

	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, backupJobModel{ID: types.StringNull(), JobID: types.StringNull()})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "nightly unit"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("backup import diagnostics: %v", importResp.Diagnostics)
	}
	assertStateString(t, importResp.State, path.Root("job_id"), "nightly unit")
}

func TestBackupJobResourceReadPreservesAPIError(t *testing.T) {
	handler := &lifecycleHandler{}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls++
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api2/json/cluster/backup/nightly" || r.URL.RawQuery != "" {
			handler.fail(w, "unexpected backup error request: %s %s", r.Method, r.URL.String())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"errors":{"backup":"backup subsystem unavailable"}}`))
	}))
	defer server.Close()
	res := &BackupJobResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := testResourceState(t, schema, backupJobModel{ID: types.StringValue("nightly"), JobID: types.StringValue("nightly")})
	resp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "status 503") || !containsDiagnostic(resp.Diagnostics, "backup subsystem unavailable") {
		t.Fatalf("expected preserved backup API detail, got %v", resp.Diagnostics)
	}
	if calls != 1 || !resp.State.Raw.Equal(state.Raw) {
		t.Fatalf("backup API error caused unexpected calls or state mutation: calls=%d raw=%v", calls, resp.State.Raw)
	}
	handler.assert(t)
}
