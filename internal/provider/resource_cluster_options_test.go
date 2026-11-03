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

func TestClusterOptionsLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	options := map[string]any{"keyboard": "en-us", "description": "lab"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.URL.Path == "/api2/json/cluster/options" && r.Method == http.MethodGet:
			handler.envelope(w, options)
		case r.URL.Path == "/api2/json/cluster/options" && r.Method == http.MethodPut:
			if !handler.form(w, r, url.Values{"keyboard": {"de"}, "description": {"lab2"}}) {
				return
			}
			options["keyboard"] = "de"
			options["description"] = "lab2"
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &ClusterOptionsResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := emptyClusterOptionsModel()
	config.Keyboard = types.StringValue("de")
	config.Description = types.StringValue("lab2")
	createResp := resource.CreateResponse{State: testResourceState(t, schema, config)}
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("apply diagnostics: %v", createResp.Diagnostics)
	}
	var state clusterOptionsModel
	if diags := createResp.State.Get(ctx, &state); diags.HasError() || state.Keyboard.ValueString() != "de" || state.ID.ValueString() != "cluster" {
		t.Fatalf("unexpected state: %#v (%v)", state, diags)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

func emptyClusterOptionsModel() clusterOptionsModel {
	return clusterOptionsModel{
		ID: types.StringNull(), Description: types.StringNull(), Keyboard: types.StringNull(), Console: types.StringNull(),
		Language: types.StringNull(), EmailFrom: types.StringNull(), Fencing: types.StringNull(), Migration: types.StringNull(),
		MigrationUnsecure: types.BoolNull(), MaxWorkers: types.Int64Null(), TagStyle: types.StringNull(), HA: types.StringNull(),
		HTTPProxy: types.StringNull(), NextID: types.StringNull(), MACPrefix: types.StringNull(),
	}
}

func TestClusterOptionsValidation(t *testing.T) {
	valid := emptyClusterOptionsModel()
	valid.Console = types.StringValue("html5")
	valid.Fencing = types.StringValue("watchdog")
	valid.MaxWorkers = types.Int64Value(4)
	var resp resource.ValidateConfigResponse
	(&ClusterOptionsResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &ClusterOptionsResource{}), valid)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("valid diagnostics: %v", resp.Diagnostics)
	}
	for name, mutate := range map[string]func(*clusterOptionsModel){
		"bad console": func(m *clusterOptionsModel) { m.Console = types.StringValue("vt100") },
		"bad fencing": func(m *clusterOptionsModel) { m.Fencing = types.StringValue("power") },
		"bad workers": func(m *clusterOptionsModel) { m.MaxWorkers = types.Int64Value(0) },
	} {
		config := emptyClusterOptionsModel()
		mutate(&config)
		resp = resource.ValidateConfigResponse{}
		(&ClusterOptionsResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &ClusterOptionsResource{}), config)}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("%s: expected diagnostics", name)
		}
	}
}

func TestClusterConfigDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/config/totem":      map[string]any{"cluster_name": "pve", "config_version": 3},
		"/api2/json/cluster/config/apiversion": "3",
		"/api2/json/cluster/config/nodes":      []map[string]any{{"node": "b"}, {"node": "a"}},
	}
	readResp := runSDNDataSource(t, NewClusterConfigDataSource(), responses, map[string]any{})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state clusterConfigModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if len(state.Nodes.Elements()) != 2 || len(state.Totem.Elements()) != 2 || state.APIVersion.ValueString() != "3" {
		t.Fatalf("unexpected projection: %#v", state)
	}
}

func TestClusterJoinAndNodeRemove(t *testing.T) {
	handler := &lifecycleHandler{}
	joinUPID := "UPID:pve-1:0000:0000:0000:cfs_join:::"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.URL.Path == "/api2/json/cluster/config/join" && r.Method == http.MethodPost:
			if !handler.form(w, r, url.Values{"hostname": {"10.0.0.2"}, "password": {"secret"}, "fingerprint": {"AA:BB"}, "force": {"1"}}) {
				return
			}
			handler.envelope(w, joinUPID)
		case strings.HasPrefix(r.URL.Path, "/api2/json/nodes/pve-1/tasks/"):
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.URL.Path == "/api2/json/cluster/config/nodes/pve-2" && r.Method == http.MethodDelete:
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	join := &ClusterJoinResource{client: testLifecycleClient(t, server)}
	joinSchema := testResourceSchema(t, join)
	joinConfig := clusterJoinModel{ID: types.StringNull(), Hostname: types.StringValue("10.0.0.2"), Password: types.StringValue("secret"), Fingerprint: types.StringValue("AA:BB"), Force: types.BoolValue(true), Triggers: testStringMap(t, map[string]string{"x": "1"}), NodeID: types.Int64Null(), Votes: types.Int64Null(), Links: types.SetNull(types.StringType), UPID: types.StringNull()}
	joinResp := resource.CreateResponse{State: testResourceState(t, joinSchema, joinConfig)}
	join.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, joinSchema, joinConfig)}, &joinResp)
	if joinResp.Diagnostics.HasError() {
		t.Fatalf("join diagnostics: %v", joinResp.Diagnostics)
	}
	var joinState clusterJoinModel
	if diags := joinResp.State.Get(ctx, &joinState); diags.HasError() || joinState.UPID.ValueString() != joinUPID {
		t.Fatalf("unexpected join state: %#v (%v)", joinState, diags)
	}

	remove := &ClusterNodeRemoveResource{client: testLifecycleClient(t, server)}
	removeSchema := testResourceSchema(t, remove)
	removeConfig := clusterNodeRemoveModel{ID: types.StringNull(), Node: types.StringValue("pve-2"), Triggers: testStringMap(t, map[string]string{"x": "1"})}
	removeResp := resource.CreateResponse{State: testResourceState(t, removeSchema, removeConfig)}
	remove.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, removeSchema, removeConfig)}, &removeResp)
	if removeResp.Diagnostics.HasError() {
		t.Fatalf("remove diagnostics: %v", removeResp.Diagnostics)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}
