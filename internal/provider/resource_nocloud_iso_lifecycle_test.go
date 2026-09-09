// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	noCloudLifecycleNode     = "pve one"
	noCloudLifecycleStorage  = "local iso"
	noCloudLifecycleFilename = "seed gen1.iso"
	noCloudLifecycleVolumeID = "local iso:iso/seed gen1.iso"
	noCloudLifecycleUploadID = "UPID:pve-two:imgcopy-1"
	noCloudLifecycleDeleteID = "UPID:pve one:delete-1"
)

func noCloudLifecycleModel() noCloudISOModel {
	return noCloudISOModel{
		Node:          types.StringValue(noCloudLifecycleNode),
		Storage:       types.StringValue(noCloudLifecycleStorage),
		Filename:      types.StringValue(noCloudLifecycleFilename),
		UserData:      types.StringValue("#cloud-config\nruncmd:\n  - echo seeded\n"),
		MetaData:      types.StringValue("instance-id: iid-20260214\nlocal-hostname: shaula-runner\n"),
		NetworkConfig: types.StringValue("version: 2\nethernets:\n  nic0:\n    match:\n      name: en*\n    dhcp4: true\n"),
	}
}

// noCloudStorageEntries returns the node storage index entries used by the
// lifecycle tests for the given storage name.
func noCloudStorageEntries(storage string) []map[string]any {
	return []map[string]any{
		{"storage": storage, "type": "dir", "content": "iso,vztmpl", "active": 1, "enabled": 1, "shared": 0},
		{"storage": "local-zfs", "type": "zfspool", "content": "images,rootdir", "active": 1, "enabled": 1, "shared": 0},
	}
}

func TestNoCloudISOResourceFrameworkLifecycle(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	seedFileExists := false
	uploadPolls := 0
	model := noCloudLifecycleModel()
	handler := &lifecycleHandler{}
	var calls []string
	var uploadedPayload []byte
	contentPath := "/api2/json/nodes/pve%20one/storage/local%20iso/content/local%20iso:iso%2Fseed%20gen1.iso"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/storage":
			handler.envelope(w, noCloudStorageEntries(noCloudLifecycleStorage))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/storage/local%20iso/content":
			if r.URL.Query().Get("content") != "iso" {
				handler.fail(w, "unexpected content query: %s", r.URL.RawQuery)
				return
			}
			items := []any{}
			if seedFileExists {
				items = append(items, map[string]any{"volid": noCloudLifecycleVolumeID, "format": "iso", "size": 393216, "content": "iso"})
			}
			handler.envelope(w, items)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/storage/local%20iso/upload":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				handler.fail(w, "parse upload form: %v", err)
				return
			}
			if got := r.FormValue("content"); got != "iso" {
				handler.fail(w, "unexpected upload content field: %q", got)
				return
			}
			file, header, err := r.FormFile("filename")
			if err != nil {
				handler.fail(w, "read filename part: %v", err)
				return
			}
			defer file.Close()
			if header.Filename != noCloudLifecycleFilename {
				handler.fail(w, "unexpected upload filename: %q", header.Filename)
				return
			}
			payload, err := io.ReadAll(file)
			if err != nil {
				handler.fail(w, "read upload payload: %v", err)
				return
			}
			uploadedPayload = payload
			// The accepted imgcopy task will materialize the file.
			seedFileExists = true
			handler.envelope(w, noCloudLifecycleUploadID)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-two/tasks/UPID:pve-two:imgcopy-1/status":
			uploadPolls++
			if uploadPolls == 1 {
				handler.envelope(w, map[string]any{"status": "running"})
				return
			}
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == contentPath:
			seedFileExists = false
			handler.envelope(w, noCloudLifecycleDeleteID)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:delete-1/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected nocloud request: %s %s raw=%q", r.Method, r.URL.String(), r.URL.RawPath)
		}
	}))
	defer server.Close()

	res := &NoCloudISOResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)

	// Create: visibility check, exclusive filename proof, in-memory ISO build,
	// multipart upload, and polling the task on its actual owner node.
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("nocloud create diagnostics: %v", createResp.Diagnostics)
	}
	var created noCloudISOModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode nocloud create state: %v", diags)
	}
	if created.ID.ValueString() != noCloudLifecycleNode+"/"+noCloudLifecycleStorage+"/"+noCloudLifecycleVolumeID || created.VolumeID.ValueString() != noCloudLifecycleVolumeID || created.UserData.ValueString() != model.UserData.ValueString() || created.NetworkConfig.ValueString() != model.NetworkConfig.ValueString() {
		t.Fatalf("unexpected typed nocloud create state: %#v", created)
	}

	// The uploaded bytes must be a CIDATA ISO whose root files carry exactly
	// the configured cloud-init contents.
	if len(uploadedPayload) == 0 {
		t.Fatal("upload payload was empty")
	}
	files := readISORootRecords(t, uploadedPayload)
	wantFiles := map[string]string{
		"user-data;1":      model.UserData.ValueString(),
		"meta-data;1":      model.MetaData.ValueString(),
		"network-config;1": model.NetworkConfig.ValueString(),
	}
	if len(files) != len(wantFiles) {
		t.Fatalf("unexpected uploaded root files: %v", files)
	}
	for identifier, content := range wantFiles {
		if string(files[identifier]) != content {
			t.Fatalf("uploaded %q content mismatch: got %q want %q", identifier, files[identifier], content)
		}
	}

	// Read keeps write-only inputs from state and survives refresh while the
	// file exists.
	readResp := resource.ReadResponse{State: createResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: createResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("nocloud read diagnostics: %v", readResp.Diagnostics)
	}
	assertStateString(t, readResp.State, path.Root("user_data"), model.UserData.ValueString())
	assertStateString(t, readResp.State, path.Root("meta_data"), model.MetaData.ValueString())
	assertStateString(t, readResp.State, path.Root("volume_id"), noCloudLifecycleVolumeID)

	// Every creation input requires replacement, so update can never carry a
	// real change; it must still re-read and preserve state.
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{State: readResp.State}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("nocloud update diagnostics: %v", updateResp.Diagnostics)
	}
	assertStateString(t, updateResp.State, path.Root("filename"), noCloudLifecycleFilename)

	// Delete removes the exact managed volume and waits for the delete task on
	// the storage node.
	deleteResp := resource.DeleteResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &deleteResp)
	res.Delete(context.Background(), resource.DeleteRequest{State: updateResp.State, Private: deleteResp.Private}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("nocloud delete diagnostics: %v", deleteResp.Diagnostics)
	}

	// Read after deletion removes the resource; a repeated delete stays
	// idempotent without issuing another DELETE.
	missingResp := resource.ReadResponse{State: updateResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: updateResp.State}, &missingResp)
	if missingResp.Diagnostics.HasError() || !missingResp.State.Raw.IsNull() {
		t.Fatalf("missing nocloud file was not removed: diagnostics=%v raw=%v", missingResp.Diagnostics, missingResp.State.Raw)
	}
	idempotentDelete := resource.DeleteResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &idempotentDelete)
	res.Delete(context.Background(), resource.DeleteRequest{State: updateResp.State, Private: idempotentDelete.Private}, &idempotentDelete)
	if idempotentDelete.Diagnostics.HasError() {
		t.Fatalf("idempotent nocloud delete diagnostics: %v", idempotentDelete.Diagnostics)
	}
	handler.assert(t)
	wantCalls := []string{
		"GET /api2/json/nodes/pve%20one/storage",
		"GET /api2/json/nodes/pve%20one/storage/local%20iso/content",
		"POST /api2/json/nodes/pve%20one/storage/local%20iso/upload",
		"GET /api2/json/nodes/pve-two/tasks/UPID:pve-two:imgcopy-1/status",
		"GET /api2/json/nodes/pve-two/tasks/UPID:pve-two:imgcopy-1/status",
		"GET /api2/json/nodes/pve%20one/storage/local%20iso/content",
		"GET /api2/json/nodes/pve%20one/storage/local%20iso/content",
		"GET /api2/json/nodes/pve%20one/storage/local%20iso/content",
		"DELETE " + contentPath,
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:delete-1/status",
		"GET /api2/json/nodes/pve%20one/storage/local%20iso/content",
		"GET /api2/json/nodes/pve%20one/storage/local%20iso/content",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected nocloud call order: got %v want %v", calls, wantCalls)
	}
}

func TestNoCloudISOCreateRefusesExistingFile(t *testing.T) {
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage":
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage/local/content":
			handler.envelope(w, []any{map[string]any{"volid": "local:iso/seed.iso", "format": "iso", "size": 2048, "content": "iso"}})
		default:
			handler.fail(w, "unexpected preflight request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &NoCloudISOResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := noCloudISOModel{Node: types.StringValue("pve"), Storage: types.StringValue("local"), Filename: types.StringValue("seed.iso"), UserData: types.StringValue("u"), MetaData: types.StringValue("m"), NetworkConfig: types.StringValue("n")}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "already exists") || !containsDiagnostic(resp.Diagnostics, "local:iso/seed.iso") {
		t.Fatalf("expected existing destination diagnostics, got %v", resp.Diagnostics)
	}
	if want := []string{"GET /api2/json/nodes/pve/storage", "GET /api2/json/nodes/pve/storage/local/content"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("existing destination must refuse before any upload: got %v", calls)
	}
	handler.assert(t)
}

func TestNoCloudISOCreateRejectsUnusableStorage(t *testing.T) {
	tests := []struct {
		name     string
		entries  []map[string]any
		contains string
	}{
		{"missing", noCloudStorageEntries("unrelated"), "not visible"},
		{"disabled", []map[string]any{{"storage": "local", "type": "dir", "content": "iso", "active": 1, "enabled": 0, "shared": 0}}, "disabled"},
		{"inactive", []map[string]any{{"storage": "local", "type": "dir", "content": "iso", "active": 0, "enabled": 1, "shared": 0}}, "not active"},
		{"no iso content", []map[string]any{{"storage": "local", "type": "zfspool", "content": "images,rootdir", "active": 1, "enabled": 1, "shared": 0}}, "does not support iso content"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := &lifecycleHandler{}
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !handler.auth(w, r) {
					return
				}
				calls = append(calls, r.Method+" "+r.URL.EscapedPath())
				if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api2/json/nodes/pve/storage" {
					handler.fail(w, "unexpected storage request: %s %s", r.Method, r.URL.String())
					return
				}
				handler.envelope(w, test.entries)
			}))
			defer server.Close()

			res := &NoCloudISOResource{client: testLifecycleClient(t, server)}
			schema := testResourceSchema(t, res)
			model := noCloudISOModel{Node: types.StringValue("pve"), Storage: types.StringValue("local"), Filename: types.StringValue("seed.iso"), UserData: types.StringValue("u"), MetaData: types.StringValue("m"), NetworkConfig: types.StringValue("n")}
			resp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
			res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
			if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, test.contains) {
				t.Fatalf("expected unusable storage diagnostics (%s), got %v", test.contains, resp.Diagnostics)
			}
			if len(calls) != 1 {
				t.Fatalf("unusable storage must abort before any upload: calls %v", calls)
			}
			handler.assert(t)
		})
	}
}

func TestNoCloudISOCreateUploadTaskFailurePreserved(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage":
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage/local/content":
			handler.envelope(w, []any{})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage/local/upload":
			handler.envelope(w, "UPID:pve:imgcopy-fail")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/tasks/UPID:pve:imgcopy-fail/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "imgcopy failed - no space left on device"})
		default:
			handler.fail(w, "unexpected upload failure request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &NoCloudISOResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := noCloudISOModel{Node: types.StringValue("pve"), Storage: types.StringValue("local"), Filename: types.StringValue("seed.iso"), UserData: types.StringValue("u"), MetaData: types.StringValue("m"), NetworkConfig: types.StringValue("n")}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &resp)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "imgcopy failed") {
		t.Fatalf("expected preserved upload task failure, got %v", resp.Diagnostics)
	}

	// The upload was accepted, so the failed create must still track the
	// resource identity, inputs, and the accepted task for reconciliation.
	var after noCloudISOModel
	if diags := resp.State.Get(context.Background(), &after); diags.HasError() {
		t.Fatalf("decode retained create state: %v", diags)
	}
	if after.ID.ValueString() != "pve/local/local:iso/seed.iso" || after.VolumeID.ValueString() != "local:iso/seed.iso" || after.UserData.ValueString() != "u" || after.NetworkConfig.ValueString() != "n" {
		t.Fatalf("unexpected retained create state: %#v", after)
	}
	retained, privateDiags := resp.Private.GetKey(context.Background(), noCloudISOTasksKey)
	if privateDiags.HasError() || string(retained) != `{"upload_task_upid":"UPID:pve:imgcopy-fail"}` {
		t.Fatalf("expected retained upload task, got %q diags %v", retained, privateDiags)
	}
	handler.assert(t)
}

func TestNoCloudISOCreateUploadPollErrorPreserved(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage":
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage/local/content":
			handler.envelope(w, []any{})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage/local/upload":
			handler.envelope(w, "UPID:pve:imgcopy-hang")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/tasks/UPID:pve:imgcopy-hang/status":
			// Poll failure must surface as an error, never as success.
			http.Error(w, "task status became unavailable", http.StatusInternalServerError)
		default:
			handler.fail(w, "unexpected poll error request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &NoCloudISOResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := noCloudISOModel{Node: types.StringValue("pve"), Storage: types.StringValue("local"), Filename: types.StringValue("seed.iso"), UserData: types.StringValue("u"), MetaData: types.StringValue("m"), NetworkConfig: types.StringValue("n")}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &resp)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "imgcopy-hang") {
		t.Fatalf("expected preserved poll error, got %v", resp.Diagnostics)
	}
	var after noCloudISOModel
	if diags := resp.State.Get(context.Background(), &after); diags.HasError() {
		t.Fatalf("decode retained create state: %v", diags)
	}
	if after.ID.ValueString() != "pve/local/local:iso/seed.iso" || after.VolumeID.ValueString() != "local:iso/seed.iso" {
		t.Fatalf("unexpected retained create state: %#v", after)
	}
	retained, privateDiags := resp.Private.GetKey(context.Background(), noCloudISOTasksKey)
	if privateDiags.HasError() || string(retained) != `{"upload_task_upid":"UPID:pve:imgcopy-hang"}` {
		t.Fatalf("expected retained upload task, got %q diags %v", retained, privateDiags)
	}
	handler.assert(t)
}

// TestNoCloudISOFailedUploadRecovery walks the full recovery chain for an
// upload whose wait failed: the create stays tracked, a refresh never declares
// the file absent or schedules cleanup while the accepted task runs, and the
// eventual cleanup removes the materialized media instead of orphaning it.
func TestNoCloudISOFailedUploadRecovery(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	// The interrupted create below can leave its last task-status handler
	// running after the cancelled client call returns, so handler state is
	// shared across goroutines and must be synchronized.
	var callsMu sync.Mutex
	var recorded []string
	recordCall := func(method, path string) {
		callsMu.Lock()
		defer callsMu.Unlock()
		recorded = append(recorded, method+" "+path)
	}
	takeCalls := func() []string {
		callsMu.Lock()
		defer callsMu.Unlock()
		taken := recorded
		recorded = nil
		return taken
	}
	taskRunning := atomic.Bool{}
	taskRunning.Store(true)
	seedFileExists := atomic.Bool{}
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		recordCall(r.Method, r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage":
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage/local/content":
			items := []any{}
			if seedFileExists.Load() {
				items = append(items, map[string]any{"volid": "local:iso/seed.iso", "format": "iso", "size": 2048, "content": "iso"})
			}
			handler.envelope(w, items)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage/local/upload":
			handler.envelope(w, "UPID:pve:imgcopy-hang")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/tasks/UPID:pve:imgcopy-hang/status":
			if taskRunning.Load() {
				handler.envelope(w, map[string]any{"status": "running"})
				return
			}
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "imgcopy failed"})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage/local/content/local:iso%2Fseed.iso":
			seedFileExists.Store(false)
			handler.envelope(w, "UPID:pve:delete-1")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/tasks/UPID:pve:delete-1/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected recovery request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &NoCloudISOResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := noCloudISOModel{Node: types.StringValue("pve"), Storage: types.StringValue("local"), Filename: types.StringValue("seed.iso"), UserData: types.StringValue("u"), MetaData: types.StringValue("m"), NetworkConfig: types.StringValue("n")}

	// Create accepts the upload, then the wait times out: the failure is
	// retained with identity and accepted task instead of orphaning the seed.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &createResp)
	if !createResp.Diagnostics.HasError() || !containsDiagnostic(createResp.Diagnostics, "UPID:pve:imgcopy-hang") {
		t.Fatalf("expected interrupted upload diagnostics, got %v", createResp.Diagnostics)
	}
	var afterCreate noCloudISOModel
	if diags := createResp.State.Get(context.Background(), &afterCreate); diags.HasError() {
		t.Fatalf("decode retained create state: %v", diags)
	}
	if afterCreate.ID.ValueString() != "pve/local/local:iso/seed.iso" || afterCreate.VolumeID.ValueString() != "local:iso/seed.iso" {
		t.Fatalf("unexpected retained create state: %#v", afterCreate)
	}
	retained, privateDiags := createResp.Private.GetKey(context.Background(), noCloudISOTasksKey)
	if privateDiags.HasError() || string(retained) != `{"upload_task_upid":"UPID:pve:imgcopy-hang"}` {
		t.Fatalf("expected retained upload task, got %q diags %v", retained, privateDiags)
	}
	takeCalls()

	// Refresh while the accepted task runs: state is kept, no absence
	// decision, and no cleanup or existence read happens yet.
	readResp := resource.ReadResponse{State: createResp.State, Private: createResp.Private}
	res.Read(context.Background(), resource.ReadRequest{State: createResp.State, Private: createResp.Private}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("refresh during running upload diagnostics: %v", readResp.Diagnostics)
	}
	if !readResp.State.Raw.Equal(createResp.State.Raw) {
		t.Fatal("refresh during a running upload mutated state")
	}
	calls := takeCalls()
	if len(calls) != 1 || calls[0] != "GET /api2/json/nodes/pve/tasks/UPID:pve:imgcopy-hang/status" {
		t.Fatalf("running upload refresh must only poll the accepted task: %v", calls)
	}
	takeCalls()

	// The task stops with a failure and the media materialized anyway. The
	// refresh must keep the tracked state so cleanup stays possible.
	taskRunning.Store(false)
	seedFileExists.Store(true)
	res.Read(context.Background(), resource.ReadRequest{State: readResp.State, Private: readResp.Private}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("refresh after failed upload diagnostics: %v", readResp.Diagnostics)
	}
	if readResp.State.Raw.IsNull() {
		t.Fatal("materialized media after a failed upload was dropped from state")
	}
	cleared, privateDiags := readResp.Private.GetKey(context.Background(), noCloudISOTasksKey)
	if privateDiags.HasError() || string(cleared) != `{}` {
		t.Fatalf("expected settled upload task to be cleared, got %q diags %v", cleared, privateDiags)
	}
	calls = takeCalls()
	want := []string{
		"GET /api2/json/nodes/pve/tasks/UPID:pve:imgcopy-hang/status",
		"GET /api2/json/nodes/pve/storage/local/content",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected post-failure refresh calls: got %v want %v", calls, want)
	}
	takeCalls()

	// Destroy cleans up the materialized media exactly once.
	deleteResp := resource.DeleteResponse{State: readResp.State, Private: readResp.Private}
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State, Private: readResp.Private}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("recovery delete diagnostics: %v", deleteResp.Diagnostics)
	}
	if seedFileExists.Load() {
		t.Fatal("orphaned seed media was not cleaned up")
	}
	calls = takeCalls()
	want = []string{
		"GET /api2/json/nodes/pve/storage/local/content",
		"DELETE /api2/json/nodes/pve/storage/local/content/local:iso%2Fseed.iso",
		"GET /api2/json/nodes/pve/tasks/UPID:pve:delete-1/status",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected recovery delete calls: got %v want %v", calls, want)
	}
	handler.assert(t)
}

func TestNoCloudISODeleteRetainsAcceptedTaskForRetry(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	// The interrupted delete below can leave its last task-status handler
	// running after the cancelled client call returns, so handler state is
	// shared across goroutines and must be synchronized.
	var callsMu sync.Mutex
	var recorded []string
	recordCall := func(method, path string) {
		callsMu.Lock()
		defer callsMu.Unlock()
		recorded = append(recorded, method+" "+path)
	}
	takeCalls := func() []string {
		callsMu.Lock()
		defer callsMu.Unlock()
		taken := recorded
		recorded = nil
		return taken
	}
	deleteTaskRunning := atomic.Bool{}
	deleteTaskRunning.Store(true)
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		recordCall(r.Method, r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage/local/content":
			handler.envelope(w, []any{map[string]any{"volid": "local:iso/seed.iso", "format": "iso", "size": 2048, "content": "iso"}})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage/local/content/local:iso%2Fseed.iso":
			handler.envelope(w, "UPID:pve:delete-1")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/tasks/UPID:pve:delete-1/status":
			if deleteTaskRunning.Load() {
				handler.envelope(w, map[string]any{"status": "running"})
				return
			}
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected retry request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &NoCloudISOResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := noCloudISOModel{ID: types.StringValue("pve/local/local:iso/seed.iso"), Node: types.StringValue("pve"), Storage: types.StringValue("local"), Filename: types.StringValue("seed.iso"), UserData: types.StringValue("u"), MetaData: types.StringValue("m"), NetworkConfig: types.StringValue("n"), VolumeID: types.StringValue("local:iso/seed.iso")}

	// First attempt: the delete task is accepted, then the wait is interrupted.
	// The state must stay and the accepted UPID must be retained privately.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	firstResp := resource.DeleteResponse{State: testResourceState(t, schema, stateModel)}
	initializeResourcePrivate(t, &firstResp)
	res.Delete(ctx, resource.DeleteRequest{State: firstResp.State, Private: firstResp.Private}, &firstResp)
	if !firstResp.Diagnostics.HasError() || !containsDiagnostic(firstResp.Diagnostics, "UPID:pve:delete-1") {
		t.Fatalf("expected interrupted delete diagnostics, got %v", firstResp.Diagnostics)
	}
	retained, privateDiags := firstResp.Private.GetKey(context.Background(), noCloudISOTasksKey)
	if privateDiags.HasError() || string(retained) != `{"delete_task_upid":"UPID:pve:delete-1"}` {
		t.Fatalf("expected retained delete task UPID, got %q diags %v", retained, privateDiags)
	}
	calls := takeCalls()
	if len(calls) < 4 || calls[0] != "GET /api2/json/nodes/pve/storage/local/content" || calls[1] != "DELETE /api2/json/nodes/pve/storage/local/content/local:iso%2Fseed.iso" {
		t.Fatalf("unexpected interrupted delete calls: %v", calls)
	}
	for _, call := range calls[2:] {
		if call != "GET /api2/json/nodes/pve/tasks/UPID:pve:delete-1/status" {
			t.Fatalf("interrupted delete must only poll the accepted task: %v", calls)
		}
	}

	// Retry: the retained task has finished, so cleanup proceeds instead of
	// blocking on the stale task or issuing a duplicate delete untracked.
	takeCalls()
	deleteTaskRunning.Store(false)
	retryResp := resource.DeleteResponse{State: firstResp.State, Private: firstResp.Private}
	res.Delete(context.Background(), resource.DeleteRequest{State: retryResp.State, Private: retryResp.Private}, &retryResp)
	if retryResp.Diagnostics.HasError() {
		t.Fatalf("retry delete diagnostics: %v", retryResp.Diagnostics)
	}
	handler.assert(t)
	calls = takeCalls()
	wantCalls := []string{
		"GET /api2/json/nodes/pve/tasks/UPID:pve:delete-1/status",
		"GET /api2/json/nodes/pve/storage/local/content",
		"DELETE /api2/json/nodes/pve/storage/local/content/local:iso%2Fseed.iso",
		"GET /api2/json/nodes/pve/tasks/UPID:pve:delete-1/status",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected retry call order: got %v want %v", calls, wantCalls)
	}
}

func TestNoCloudISODeleteProceedsAfterFailedRetainedTask(t *testing.T) {
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/tasks/UPID:pve:delete-1/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "volume is busy"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage/local/content":
			handler.envelope(w, []any{})
		default:
			handler.fail(w, "unexpected stale task request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &NoCloudISOResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := noCloudISOModel{ID: types.StringValue("pve/local/local:iso/seed.iso"), Node: types.StringValue("pve"), Storage: types.StringValue("local"), Filename: types.StringValue("seed.iso"), VolumeID: types.StringValue("local:iso/seed.iso")}
	resp := resource.DeleteResponse{State: testResourceState(t, schema, stateModel)}
	initializeResourcePrivate(t, &resp)
	if diags := resp.Private.SetKey(context.Background(), noCloudISOTasksKey, []byte(`{"delete_task_upid":"UPID:pve:delete-1"}`)); diags.HasError() {
		t.Fatalf("seed private state: %v", diags)
	}
	res.Delete(context.Background(), resource.DeleteRequest{State: resp.State, Private: resp.Private}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("delete after failed retained task diagnostics: %v", resp.Diagnostics)
	}
	if want := []string{"GET /api2/json/nodes/pve/tasks/UPID:pve:delete-1/status", "GET /api2/json/nodes/pve/storage/local/content"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("failed retained task must not block cleanup or re-delete: got %v", calls)
	}
	handler.assert(t)
}

// TestNoCloudISORetainedTasksPollOwnerNodeNotResourceNode walks the full
// retained-task chain with a task owner that differs from the resource node:
// an accepted upload whose wait timed out must be polled on the owner node
// encoded in its UPID during every retry, refresh, and cleanup, never on the
// destination node, with no early cleanup and no orphaned media.
func TestNoCloudISORetainedTasksPollOwnerNodeNotResourceNode(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	const uploadUPID = "UPID:pve-two:imgcopy-owner"
	const deleteUPID = "UPID:pve-two:delete-owner"

	var callsMu sync.Mutex
	var recorded []string
	recordCall := func(method, path string) {
		callsMu.Lock()
		defer callsMu.Unlock()
		recorded = append(recorded, method+" "+path)
	}
	takeCalls := func() []string {
		callsMu.Lock()
		defer callsMu.Unlock()
		taken := recorded
		recorded = nil
		return taken
	}
	taskRunning := atomic.Bool{}
	taskRunning.Store(true)
	seedFileExists := atomic.Bool{}
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		// Polling the retained upload task on the resource node instead of
		// its owner is the regression this test exists to catch.
		if r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve-two:imgcopy-owner/status" {
			handler.fail(w, "retained upload task polled on the wrong node: %s", r.URL.EscapedPath())
			return
		}
		recordCall(r.Method, r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/storage":
			handler.envelope(w, noCloudStorageEntries("local"))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/storage/local/content":
			items := []any{}
			if seedFileExists.Load() {
				items = append(items, map[string]any{"volid": "local:iso/seed.iso", "format": "iso", "size": 2048, "content": "iso"})
			}
			handler.envelope(w, items)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/storage/local/upload":
			handler.envelope(w, uploadUPID)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-two/tasks/UPID:pve-two:imgcopy-owner/status":
			if taskRunning.Load() {
				handler.envelope(w, map[string]any{"status": "running"})
				return
			}
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "imgcopy failed"})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/storage/local/content/local:iso%2Fseed.iso":
			seedFileExists.Store(false)
			handler.envelope(w, deleteUPID)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-two/tasks/UPID:pve-two:delete-owner/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected owner-node request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &NoCloudISOResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := noCloudISOModel{Node: types.StringValue("pve one"), Storage: types.StringValue("local"), Filename: types.StringValue("seed.iso"), UserData: types.StringValue("u"), MetaData: types.StringValue("m"), NetworkConfig: types.StringValue("n")}

	// Create accepts the upload on the destination node, then the wait times
	// out; the failure retains identity plus the accepted owner-node task.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &createResp)
	if !createResp.Diagnostics.HasError() || !containsDiagnostic(createResp.Diagnostics, uploadUPID) {
		t.Fatalf("expected interrupted upload diagnostics, got %v", createResp.Diagnostics)
	}
	retained, privateDiags := createResp.Private.GetKey(context.Background(), noCloudISOTasksKey)
	if privateDiags.HasError() || string(retained) != `{"upload_task_upid":"UPID:pve-two:imgcopy-owner"}` {
		t.Fatalf("expected retained upload task, got %q diags %v", retained, privateDiags)
	}
	takeCalls()

	// Refresh while the accepted task runs: only the owner node is polled,
	// the state is kept, and no existence read or cleanup happens yet.
	readResp := resource.ReadResponse{State: createResp.State, Private: createResp.Private}
	res.Read(context.Background(), resource.ReadRequest{State: createResp.State, Private: createResp.Private}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("refresh during running upload diagnostics: %v", readResp.Diagnostics)
	}
	if !readResp.State.Raw.Equal(createResp.State.Raw) {
		t.Fatal("refresh during a running upload mutated state")
	}
	if calls := takeCalls(); !reflect.DeepEqual(calls, []string{"GET /api2/json/nodes/pve-two/tasks/UPID:pve-two:imgcopy-owner/status"}) {
		t.Fatalf("running upload refresh must only poll the task owner node: %v", calls)
	}

	// The task stops with a failure on the owner node while the media
	// materialized anyway; the refresh keeps the tracked state for cleanup.
	taskRunning.Store(false)
	seedFileExists.Store(true)
	res.Read(context.Background(), resource.ReadRequest{State: readResp.State, Private: readResp.Private}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("refresh after failed upload diagnostics: %v", readResp.Diagnostics)
	}
	if readResp.State.Raw.IsNull() {
		t.Fatal("materialized media after a failed upload was dropped from state")
	}
	if calls := takeCalls(); !reflect.DeepEqual(calls, []string{
		"GET /api2/json/nodes/pve-two/tasks/UPID:pve-two:imgcopy-owner/status",
		"GET /api2/json/nodes/pve%20one/storage/local/content",
	}) {
		t.Fatalf("unexpected post-failure refresh calls: %v", calls)
	}

	// Destroy awaits nothing (the upload settled), then cleans up the
	// materialized media and polls the fresh delete task on its owner.
	deleteResp := resource.DeleteResponse{State: readResp.State, Private: readResp.Private}
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State, Private: readResp.Private}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("recovery delete diagnostics: %v", deleteResp.Diagnostics)
	}
	if seedFileExists.Load() {
		t.Fatal("orphaned seed media was not cleaned up")
	}
	if calls := takeCalls(); !reflect.DeepEqual(calls, []string{
		"GET /api2/json/nodes/pve%20one/storage/local/content",
		"DELETE /api2/json/nodes/pve%20one/storage/local/content/local:iso%2Fseed.iso",
		"GET /api2/json/nodes/pve-two/tasks/UPID:pve-two:delete-owner/status",
	}) {
		t.Fatalf("unexpected owner-node delete calls: %v", calls)
	}
	handler.assert(t)
}

// TestNoCloudISODeleteRejectsUnverifiedAcknowledgement proves a no-delay
// content DELETE that answers HTTP 200 without a usable UPID is an error that
// keeps the resource in state for a retry: PVE only returns null from this
// endpoint when a `delay` parameter is supplied, which the client never sends,
// so an empty or malformed acknowledgement means the deletion is unverified —
// never a silent success — and no task poll may run against it.
func TestNoCloudISODeleteRejectsUnverifiedAcknowledgement(t *testing.T) {
	tests := []struct {
		name string
		ack  any
	}{
		{"null data", nil},
		{"empty string", ""},
		{"malformed upid", "not-a-upid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := &lifecycleHandler{}
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !handler.auth(w, r) {
					return
				}
				calls = append(calls, r.Method+" "+r.URL.EscapedPath())
				switch {
				case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage/local/content":
					handler.envelope(w, []any{map[string]any{"volid": "local:iso/seed.iso", "format": "iso", "size": 2048, "content": "iso"}})
				case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage/local/content/local:iso%2Fseed.iso":
					handler.envelope(w, test.ack)
				default:
					handler.fail(w, "unverified acknowledgement must not poll: %s %s", r.Method, r.URL.String())
				}
			}))
			defer server.Close()

			res := &NoCloudISOResource{client: testLifecycleClient(t, server)}
			schema := testResourceSchema(t, res)
			stateModel := noCloudISOModel{ID: types.StringValue("pve/local/local:iso/seed.iso"), Node: types.StringValue("pve"), Storage: types.StringValue("local"), Filename: types.StringValue("seed.iso"), UserData: types.StringValue("u"), MetaData: types.StringValue("m"), NetworkConfig: types.StringValue("n"), VolumeID: types.StringValue("local:iso/seed.iso")}
			resp := resource.DeleteResponse{State: testResourceState(t, schema, stateModel)}
			initializeResourcePrivate(t, &resp)
			res.Delete(context.Background(), resource.DeleteRequest{State: resp.State, Private: resp.Private}, &resp)
			if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "UPID") {
				t.Fatalf("expected unverified delete acknowledgement diagnostics, got %v", resp.Diagnostics)
			}
			if resp.State.Raw.IsNull() {
				t.Fatal("unverified delete must keep the resource state for a retry")
			}
			retained, privateDiags := resp.Private.GetKey(context.Background(), noCloudISOTasksKey)
			if privateDiags.HasError() || strings.Contains(string(retained), "delete_task_upid") {
				t.Fatalf("an unacknowledged delete must retain no task UPID, got %q diags %v", retained, privateDiags)
			}
			wantCalls := []string{
				"GET /api2/json/nodes/pve/storage/local/content",
				"DELETE /api2/json/nodes/pve/storage/local/content/local:iso%2Fseed.iso",
			}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("invalid delete acknowledgement must not poll task status: got %v want %v", calls, wantCalls)
			}
			handler.assert(t)
		})
	}
}

// TestNoCloudISORetainedTaskUnknownStatusIsError proves a retained task whose
// status reply is neither running nor stopped is a retryable error instead of
// an endless wait or a silent success: the destroy aborts after the single
// poll and never reaches the existence check or a DELETE.
func TestNoCloudISORetainedTaskUnknownStatusIsError(t *testing.T) {
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch r.URL.EscapedPath() {
		case "/api2/json/nodes/pve/tasks/UPID:pve:delete-1/status":
			handler.envelope(w, map[string]any{"status": "zombie"})
		default:
			handler.fail(w, "unexpected unknown-status request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &NoCloudISOResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := noCloudISOModel{ID: types.StringValue("pve/local/local:iso/seed.iso"), Node: types.StringValue("pve"), Storage: types.StringValue("local"), Filename: types.StringValue("seed.iso"), VolumeID: types.StringValue("local:iso/seed.iso")}
	resp := resource.DeleteResponse{State: testResourceState(t, schema, stateModel)}
	initializeResourcePrivate(t, &resp)
	if diags := resp.Private.SetKey(context.Background(), noCloudISOTasksKey, []byte(`{"delete_task_upid":"UPID:pve:delete-1"}`)); diags.HasError() {
		t.Fatalf("seed private state: %v", diags)
	}
	res.Delete(context.Background(), resource.DeleteRequest{State: resp.State, Private: resp.Private}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "unknown status") || !containsDiagnostic(resp.Diagnostics, "delete-1") {
		t.Fatalf("expected unknown retained status diagnostics, got %v", resp.Diagnostics)
	}
	if want := []string{"GET /api2/json/nodes/pve/tasks/UPID:pve:delete-1/status"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("unknown retained status must abort after one poll: got %v", calls)
	}
	handler.assert(t)
}
