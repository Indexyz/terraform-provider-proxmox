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

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func storageLifecycleRaw(values map[string]string) types.Object {
	extra := make(map[string]attr.Value, len(values))
	for key, value := range values {
		extra[key] = types.StringValue(value)
	}
	return types.ObjectValueMust(storageRawAttrTypes(), map[string]attr.Value{
		"extra_config": types.MapValueMust(types.StringType, extra),
	})
}

func TestStorageResourceFrameworkLifecycle(t *testing.T) {
	exists := true
	storage := map[string]any{
		"storage": "pbs main", "type": "pbs", "content": "backup", "nodes": "pve-1,pve-2",
		"disable": 0, "shared": 0, "path": "/mnt/storage", "pool": "rbd-pool", "vgname": "vg0",
		"thinpool": "thin0", "server": "pbs.example.test", "export": "/exports/data", "share": "backups",
		"username": "backup@pbs", "monhost": "10.0.0.10", "datastore": "primary", "namespace": "terraform",
		"fingerprint": "AA:BB", "smbversion": "3.1.1", "options": "vers=4.2", "format": "raw", "mkdir": 0,
		"sparse": 0, "nocow": 0, "krbd": 0, "blocksize": "4k", "fs-name": "cephfs", "max-protected-backups": 5,
	}
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.RawQuery != "" {
			handler.fail(w, "unexpected storage query: %s", r.URL.RawQuery)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/storage":
			want := url.Values{
				"storage": {"pbs main"}, "type": {"pbs"}, "content": {"backup"}, "nodes": {"pve-1,pve-2"},
				"disable": {"0"}, "shared": {"0"}, "path": {"/mnt/storage"}, "pool": {"rbd-pool"}, "vgname": {"vg0"},
				"thinpool": {"thin0"}, "server": {"pbs.example.test"}, "export": {"/exports/data"}, "share": {"backups"},
				"username": {"backup@pbs"}, "password": {"create-secret"}, "monhost": {"10.0.0.10"}, "datastore": {"primary"},
				"namespace": {"terraform"}, "fingerprint": {"AA:BB"}, "smbversion": {"3.1.1"}, "options": {"vers=4.2"},
				"format": {"raw"}, "mkdir": {"0"}, "sparse": {"0"}, "nocow": {"0"}, "krbd": {"0"},
				"blocksize": {"4k"}, "fs-name": {"cephfs"}, "max-protected-backups": {"5"},
			}
			if !handler.form(w, r, want) {
				return
			}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/storage/pbs%20main":
			if !exists {
				http.Error(w, `{"message":"storage missing"}`, http.StatusNotFound)
				return
			}
			handler.envelope(w, storage)
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/storage/pbs%20main":
			want := url.Values{
				"delete":  {"blocksize,content,datastore,export,fingerprint,format,fs-name,krbd,mkdir,monhost,namespace,nocow,nodes,options,path,pool,server,share,shared,smbversion,sparse,thinpool,username,vgname"},
				"disable": {"0"}, "max-protected-backups": {"6"},
			}
			if !handler.form(w, r, want) {
				return
			}
			storage = map[string]any{"storage": "pbs main", "type": "pbs", "disable": 0, "max-protected-backups": 6}
			handler.envelope(w, nil)
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/storage/pbs%20main":
			if !handler.form(w, r, url.Values{}) {
				return
			}
			if !exists {
				http.Error(w, `{"message":"storage missing"}`, http.StatusNotFound)
				return
			}
			exists = false
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected storage request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &StorageResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := storageModel{
		Storage: types.StringValue("pbs main"), Type: types.StringValue("pbs"), Content: types.StringValue("backup"), Nodes: types.StringValue("pve-1,pve-2"),
		Disable: types.BoolValue(false), Shared: types.BoolValue(false), Path: types.StringValue("/mnt/storage"), Pool: types.StringValue("rbd-pool"),
		VGName: types.StringValue("vg0"), ThinPool: types.StringValue("thin0"), Server: types.StringValue("pbs.example.test"), Export: types.StringValue("/exports/data"),
		Share: types.StringValue("backups"), Username: types.StringValue("backup@pbs"), Password: types.StringValue("create-secret"), Monhost: types.StringValue("10.0.0.10"),
		Datastore: types.StringValue("primary"), Namespace: types.StringValue("terraform"), Fingerprint: types.StringValue("AA:BB"), SMBVersion: types.StringValue("3.1.1"),
		Options: types.StringValue("vers=4.2"), Format: types.StringValue("raw"), Mkdir: types.BoolValue(false), Sparse: types.BoolValue(false), NoCOW: types.BoolValue(false),
		KRBD: types.BoolValue(false), Blocksize: types.StringValue("4k"), FSName: types.StringValue("cephfs"), Raw: storageLifecycleRaw(map[string]string{"max-protected-backups": "5"}),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("storage create diagnostics: %v", createResp.Diagnostics)
	}
	var created storageModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode storage create state: %v", diags)
	}
	if created.ID.ValueString() != "pbs main" || created.Type.ValueString() != "pbs" || created.Password.ValueString() != "create-secret" || created.Disable.ValueBool() || created.Shared.ValueBool() || created.Mkdir.ValueBool() || created.Sparse.ValueBool() || created.NoCOW.ValueBool() || created.KRBD.ValueBool() {
		t.Fatalf("unexpected typed storage create state: %#v", created)
	}
	var createdRaw storageRawModel
	if diags := created.Raw.As(context.Background(), &createdRaw, qemuObjectAsOptions()); diags.HasError() {
		t.Fatalf("decode storage raw state: %v", diags)
	}
	var createdExtra map[string]string
	if diags := createdRaw.ExtraConfig.ElementsAs(context.Background(), &createdExtra, false); diags.HasError() || !reflect.DeepEqual(createdExtra, map[string]string{"max-protected-backups": "5"}) {
		t.Fatalf("unexpected storage raw state: values=%v diagnostics=%v", createdExtra, diags)
	}

	updated := storageModel{Storage: types.StringValue("pbs main"), Type: types.StringValue("pbs"), Disable: types.BoolValue(false), Raw: storageLifecycleRaw(map[string]string{"max-protected-backups": "6"})}
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, updated), State: createResp.State}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("storage update diagnostics: %v", updateResp.Diagnostics)
	}
	var updatedState storageModel
	if diags := updateResp.State.Get(context.Background(), &updatedState); diags.HasError() {
		t.Fatalf("decode storage update state: %v", diags)
	}
	if updatedState.Disable.ValueBool() || !updatedState.Shared.IsNull() || !updatedState.Content.IsNull() || !updatedState.Password.IsNull() {
		t.Fatalf("unexpected typed storage update state: %#v", updatedState)
	}

	readResp := resource.ReadResponse{State: updateResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: updateResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("storage read diagnostics: %v", readResp.Diagnostics)
	}
	assertStateString(t, readResp.State, path.Root("storage"), "pbs main")
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("storage delete diagnostics: %v", deleteResp.Diagnostics)
	}
	missingResp := resource.ReadResponse{State: readResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: readResp.State}, &missingResp)
	if missingResp.Diagnostics.HasError() || !missingResp.State.Raw.IsNull() {
		t.Fatalf("missing storage was not removed: diagnostics=%v raw=%v", missingResp.Diagnostics, missingResp.State.Raw)
	}
	var idempotentDelete resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &idempotentDelete)
	if idempotentDelete.Diagnostics.HasError() {
		t.Fatalf("idempotent storage delete diagnostics: %v", idempotentDelete.Diagnostics)
	}
	handler.assert(t)
	wantCalls := []string{
		"POST /api2/json/storage", "GET /api2/json/storage/pbs%20main", "PUT /api2/json/storage/pbs%20main",
		"GET /api2/json/storage/pbs%20main", "GET /api2/json/storage/pbs%20main", "DELETE /api2/json/storage/pbs%20main",
		"GET /api2/json/storage/pbs%20main", "DELETE /api2/json/storage/pbs%20main",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected storage call order: got %v want %v", calls, wantCalls)
	}

	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, storageModel{ID: types.StringNull(), Storage: types.StringNull(), Raw: types.ObjectNull(storageRawAttrTypes())})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "pbs main"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("storage import diagnostics: %v", importResp.Diagnostics)
	}
	assertStateString(t, importResp.State, path.Root("id"), "pbs main")
	assertStateString(t, importResp.State, path.Root("storage"), "pbs main")
}

func TestStorageResourceNoOpUpdateAndCreateValidation(t *testing.T) {
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api2/json/storage/local" || r.URL.RawQuery != "" {
			handler.fail(w, "unexpected no-op storage request: %s %s", r.Method, r.URL.String())
			return
		}
		handler.envelope(w, map[string]any{"storage": "local", "type": "dir"})
	}))
	defer server.Close()
	res := &StorageResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := storageModel{ID: types.StringValue("local"), Storage: types.StringValue("local"), Type: types.StringValue("dir"), Raw: types.ObjectNull(storageRawAttrTypes())}
	resp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, model), State: testResourceState(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("storage no-op update diagnostics: %v", resp.Diagnostics)
	}
	if want := []string{"GET /api2/json/storage/local"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected no-op storage calls: got %v want %v", calls, want)
	}

	invalid := storageModel{Storage: types.StringValue(""), Type: types.StringValue("dir"), Raw: types.ObjectNull(storageRawAttrTypes())}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, invalid)}, &createResp)
	if !createResp.Diagnostics.HasError() || !containsDiagnostic(createResp.Diagnostics, "non-empty value") || len(calls) != 1 {
		t.Fatalf("expected local storage identifier validation, calls=%v diagnostics=%v", calls, createResp.Diagnostics)
	}
	handler.assert(t)
}

func TestStorageResourceReadPreservesAPIError(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api2/json/storage/broken" || r.URL.RawQuery != "" {
			handler.fail(w, "unexpected storage error request: %s %s", r.Method, r.URL.String())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"errors":{"storage":"storage subsystem unavailable"}}`))
	}))
	defer server.Close()
	res := &StorageResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := testResourceState(t, schema, storageModel{ID: types.StringValue("broken"), Storage: types.StringValue("broken"), Type: types.StringValue("dir"), Raw: types.ObjectNull(storageRawAttrTypes())})
	resp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "status 503") || !containsDiagnostic(resp.Diagnostics, "storage subsystem unavailable") {
		t.Fatalf("expected preserved storage API detail, got %v", resp.Diagnostics)
	}
	if !resp.State.Raw.Equal(state.Raw) {
		t.Fatalf("storage API error unexpectedly mutated state: %v", resp.State.Raw)
	}
	handler.assert(t)
}
