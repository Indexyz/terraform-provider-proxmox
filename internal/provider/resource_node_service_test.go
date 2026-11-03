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

const nodeServiceUPID = "UPID:pve-1:0000:0000:0000:startall:::"

func TestClientNodeServiceAndAPT(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/services/spiceproxy/start" && r.Method == http.MethodPost:
			handler.envelope(w, nodeServiceUPID)
		case r.URL.Path == "/api2/json/nodes/pve-1/services/spiceproxy/state" && r.Method == http.MethodGet:
			handler.envelope(w, map[string]any{"service": "spiceproxy", "active-state": "active", "state": "running"})
		case r.URL.Path == "/api2/json/nodes/pve-1/apt/repositories" && r.Method == http.MethodGet:
			handler.envelope(w, map[string]any{"digest": "d1", "files": []map[string]any{{"path": "/etc/apt/sources.list", "digest": []int{102, 49}, "repositories": []map[string]any{{"Types": []string{"deb"}, "URIs": []string{"http://download.proxmox.com"}, "Enabled": 1}}}}})
		case r.URL.Path == "/api2/json/nodes/pve-1/apt/repositories" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{"path": {"/etc/apt/sources.list"}, "index": {"0"}, "enabled": {"0"}, "digest": {"d1"}})
			handler.envelope(w, nil)
		case r.URL.Path == "/api2/json/nodes/pve-1/apt/update" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{"quiet": {"1"}})
			handler.envelope(w, "UPID:pve-1:0000:0000:0000:aptupdate:::")
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)

	upid, err := client.NodeServiceAction(ctx, "pve-1", "spiceproxy", "start")
	if err != nil || upid != nodeServiceUPID {
		t.Fatalf("NodeServiceAction() = %q, %v", upid, err)
	}
	state, err := client.NodeServiceState(ctx, "pve-1", "spiceproxy")
	if err != nil || state.ActiveState != "active" {
		t.Fatalf("unexpected service state: %#v (%v)", state, err)
	}
	repositories, err := client.NodeAPTRepositories(ctx, "pve-1")
	if err != nil || len(repositories.Files) != 1 || hexEncode(repositories.Files[0].Digest) != "6631" {
		t.Fatalf("unexpected repositories: %#v (%v)", repositories, err)
	}
	if err := client.ToggleAPTRepository(ctx, "pve-1", "/etc/apt/sources.list", 0, false, "d1"); err != nil {
		t.Fatalf("ToggleAPTRepository() unexpected error: %v", err)
	}
	if _, err := client.NodeAPTUpdate(ctx, "pve-1", true, false); err != nil {
		t.Fatalf("NodeAPTUpdate() unexpected error: %v", err)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

func TestNodeServiceLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPost {
			handler.envelope(w, nodeServiceUPID)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/state") {
			handler.envelope(w, map[string]any{"service": "spiceproxy", "active-state": "inactive"})
			return
		}
		handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
	}))
	defer server.Close()

	ctx := context.Background()
	res := &NodeServiceResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := nodeServiceModel{ID: types.StringNull(), Node: types.StringValue("pve-1"), Service: types.StringValue("spiceproxy"), State: types.StringValue("stopped"), Triggers: types.MapNull(types.StringType)}
	createResp := resource.CreateResponse{State: testResourceState(t, schema, config)}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	if !strings.Contains(strings.Join(calls, ","), "POST /api2/json/nodes/pve-1/services/spiceproxy/stop") {
		t.Fatalf("stop action was not used: %v", calls)
	}
}

func TestNodeAPTUpdateCommand(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.Method == http.MethodPost {
			handler.envelope(w, "UPID:pve-1:0000:0000:0000:aptupdate:::")
			return
		}
		handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
	}))
	defer server.Close()

	ctx := context.Background()
	res := &NodeAPTUpdateResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := nodeAPTUpdateModel{ID: types.StringNull(), Node: types.StringValue("pve-1"), Quiet: types.BoolValue(true), Notify: types.BoolValue(false), Triggers: testStringMap(t, map[string]string{"x": "1"}), UPID: types.StringNull()}
	createResp := resource.CreateResponse{State: testResourceState(t, schema, config)}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("apt update diagnostics: %v", createResp.Diagnostics)
	}
	var state nodeAPTUpdateModel
	if diags := createResp.State.Get(ctx, &state); diags.HasError() || state.UPID.ValueString() == "" {
		t.Fatalf("unexpected state: %#v (%v)", state, diags)
	}
}

func TestNodeAPTRepositoryLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	enabled := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.Method == http.MethodGet {
			handler.envelope(w, map[string]any{"digest": "d1", "files": []map[string]any{{"path": "/etc/apt/sources.list", "digest": []int{102, 49}, "repositories": []map[string]any{{"Enabled": boolFlag(enabled)}}}}})
			return
		}
		enabled = false
		handler.envelope(w, nil)
	}))
	defer server.Close()

	ctx := context.Background()
	res := &NodeAPTRepositoryResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := nodeAPTRepositoryModel{ID: types.StringNull(), Node: types.StringValue("pve-1"), Path: types.StringValue("/etc/apt/sources.list"), Index: types.Int64Value(0), Enabled: types.BoolValue(false)}
	createResp := resource.CreateResponse{State: testResourceState(t, schema, config)}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("toggle diagnostics: %v", createResp.Diagnostics)
	}
	var readState nodeAPTRepositoryModel
	readResp := resource.ReadResponse{State: createResp.State}
	res.Read(ctx, resource.ReadRequest{State: createResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	if diags := readResp.State.Get(ctx, &readState); diags.HasError() || readState.Enabled.ValueBool() {
		t.Fatalf("unexpected refreshed state: %#v (%v)", readState, diags)
	}
}

func TestNodeAPTRepositoriesDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/apt/repositories": map[string]any{
			"digest": "d1",
			"files": []map[string]any{
				{"path": "/etc/apt/sources.list.d/pve.list", "digest": []int{102, 50}, "repositories": []map[string]any{{"Types": []string{"deb"}, "URIs": []string{"http://download.proxmox.com"}, "Enabled": 1}}},
				{"path": "/etc/apt/sources.list", "digest": []int{102, 49}, "repositories": []map[string]any{{"Types": []string{"deb"}, "URIs": []string{"http://deb.debian.org"}, "Enabled": 0}}},
			},
		},
	}
	readResp := runSDNDataSource(t, NewNodeAPTRepositoriesDataSource(), responses, map[string]any{"node": "pve-1"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state nodeAPTRepositoriesModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() || len(state.Files) != 2 || state.Files[0].Path.ValueString() != "/etc/apt/sources.list" || state.Files[1].Repositories[0].Enabled.ValueBool() != true {
		t.Fatalf("unexpected projection: %#v (%v)", state.Files, diags)
	}
}
