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

func TestStorageFileDownloadResourceFrameworkLifecycle(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	exists := true
	downloadPolls := 0
	handler := &lifecycleHandler{}
	var calls []string
	const contentPath = "/api2/json/nodes/pve%20one/storage/local%20iso/content/local%20iso:iso%2Fdebian.iso"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.RawQuery != "" {
			handler.fail(w, "unexpected download query: %s", r.URL.RawQuery)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/storage/local%20iso/download-url":
			want := url.Values{"content": {"iso"}, "filename": {"debian.iso"}, "url": {"https://user:secret@example.test/debian.iso.zst"}, "checksum": {"abc123"}, "checksum-algorithm": {"sha256"}, "compression": {"zstd"}, "verify-certificates": {"0"}}
			if !handler.form(w, r, want) {
				return
			}
			handler.envelope(w, "UPID:pve one:download")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:download/status":
			downloadPolls++
			if downloadPolls == 1 {
				handler.envelope(w, map[string]any{"status": "running"})
				return
			}
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == contentPath:
			if !exists {
				http.Error(w, "missing storage content", http.StatusNotFound)
				return
			}
			handler.envelope(w, map[string]any{"format": "iso", "path": "/var/lib/vz/template/iso/debian.iso", "size": 4096, "used": 4096, "notes": "downloaded", "protected": 1})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == contentPath:
			if !handler.form(w, r, url.Values{}) {
				return
			}
			if !exists {
				http.Error(w, "missing storage content", http.StatusNotFound)
				return
			}
			exists = false
			handler.envelope(w, "UPID:pve one:delete")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:delete/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected download request: %s %s raw=%q", r.Method, r.URL.String(), r.URL.RawPath)
		}
	}))
	defer server.Close()

	res := &StorageFileDownloadResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := storageFileDownloadModel{Node: types.StringValue("pve one"), Storage: types.StringValue("local iso"), Content: types.StringValue("iso"), Filename: types.StringValue("debian.iso"), URL: types.StringValue("https://user:secret@example.test/debian.iso.zst"), Checksum: types.StringValue("abc123"), ChecksumAlgorithm: types.StringValue("sha256"), Compression: types.StringValue("zstd"), VerifyCertificates: types.BoolValue(false)}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("download create diagnostics: %v", createResp.Diagnostics)
	}
	var created storageFileDownloadModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode download create state: %v", diags)
	}
	if created.ID.ValueString() != "pve one/local iso/local iso:iso/debian.iso" || created.VolumeID.ValueString() != "local iso:iso/debian.iso" || created.Format.ValueString() != "iso" || created.Size.ValueInt64() != 4096 || !created.Protected.ValueBool() || created.URL.ValueString() != "https://user:secret@example.test/debian.iso.zst" || created.Checksum.ValueString() != "abc123" {
		t.Fatalf("unexpected typed download create state: %#v", created)
	}

	readResp := resource.ReadResponse{State: createResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: createResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("download read diagnostics: %v", readResp.Diagnostics)
	}
	assertStateString(t, readResp.State, path.Root("url"), "https://user:secret@example.test/debian.iso.zst")
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{State: readResp.State}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("download update diagnostics: %v", updateResp.Diagnostics)
	}
	assertStateString(t, updateResp.State, path.Root("notes"), "downloaded")

	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("download delete diagnostics: %v", deleteResp.Diagnostics)
	}
	missingResp := resource.ReadResponse{State: updateResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: updateResp.State}, &missingResp)
	if missingResp.Diagnostics.HasError() || !missingResp.State.Raw.IsNull() {
		t.Fatalf("missing downloaded file was not removed: diagnostics=%v raw=%v", missingResp.Diagnostics, missingResp.State.Raw)
	}
	var idempotentDelete resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: updateResp.State}, &idempotentDelete)
	if idempotentDelete.Diagnostics.HasError() {
		t.Fatalf("idempotent download delete diagnostics: %v", idempotentDelete.Diagnostics)
	}
	handler.assert(t)
	wantCalls := []string{
		"POST /api2/json/nodes/pve%20one/storage/local%20iso/download-url",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:download/status",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:download/status",
		"GET " + contentPath,
		"GET " + contentPath,
		"GET " + contentPath,
		"DELETE " + contentPath,
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:delete/status",
		"GET " + contentPath,
		"DELETE " + contentPath,
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected download call order: got %v want %v", calls, wantCalls)
	}
}

func TestStorageFileDownloadDeletePreservesTaskFailure(t *testing.T) {
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage/local/content/local:iso%2Funit.iso" && r.URL.RawQuery == "":
			if !handler.form(w, r, url.Values{}) {
				return
			}
			handler.envelope(w, "UPID:pve:delete-failed")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/tasks/UPID:pve:delete-failed/status" && r.URL.RawQuery == "":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "permission denied"})
		default:
			handler.fail(w, "unexpected task failure request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	res := &StorageFileDownloadResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := storageFileDownloadModel{ID: types.StringValue("pve/local/local:iso/unit.iso"), Node: types.StringValue("pve"), Storage: types.StringValue("local"), VolumeID: types.StringValue("local:iso/unit.iso")}
	var resp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: testResourceState(t, schema, state)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "UPID:pve:delete-failed") || !containsDiagnostic(resp.Diagnostics, "permission denied") {
		t.Fatalf("expected preserved task failure detail, got %v", resp.Diagnostics)
	}
	if want := []string{"DELETE /api2/json/nodes/pve/storage/local/content/local:iso%2Funit.iso", "GET /api2/json/nodes/pve/tasks/UPID:pve:delete-failed/status"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected failed delete task order: got %v want %v", calls, want)
	}
	handler.assert(t)
}

func TestStorageFileDownloadReadPreservesAPIError(t *testing.T) {
	handler := &lifecycleHandler{}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls++
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api2/json/nodes/pve%20one/storage/local/content/local:iso%2Funit.iso" || r.URL.RawQuery != "" {
			handler.fail(w, "unexpected storage error request: %s %s", r.Method, r.URL.String())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":{"permission":"missing Datastore.Audit"}}`))
	}))
	defer server.Close()
	res := &StorageFileDownloadResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := testResourceState(t, schema, storageFileDownloadModel{ID: types.StringValue("pve one/local/local:iso/unit.iso"), Node: types.StringValue("pve one"), Storage: types.StringValue("local"), VolumeID: types.StringValue("local:iso/unit.iso")})
	resp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "status 403") || !containsDiagnostic(resp.Diagnostics, "missing Datastore.Audit") {
		t.Fatalf("expected preserved storage API detail, got %v", resp.Diagnostics)
	}
	if calls != 1 || !resp.State.Raw.Equal(state.Raw) {
		t.Fatalf("storage API error caused unexpected calls or state mutation: calls=%d raw=%v", calls, resp.State.Raw)
	}
	handler.assert(t)
}
