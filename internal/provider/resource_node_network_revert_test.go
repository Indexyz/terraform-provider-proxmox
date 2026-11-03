// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const nodeNetworkRevertUPID = "UPID:node1:00001234:00000000:00000000:srvreload:networking:root@pam:"

func TestClientRevertNodeNetwork(t *testing.T) {
	ctx := context.Background()
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, w, "")
	}))
	if _, err := newHAResourceTestClient(t, empty.URL).RevertNodeNetwork(ctx, "node1"); err == nil {
		t.Fatal("RevertNodeNetwork() with empty acknowledgement returned no error")
	}
	empty.Close()

	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if len(r.Form) != 0 {
			t.Fatalf("unexpected revert form: %#v", r.Form)
		}
		writeEnvelope(t, w, nodeNetworkRevertUPID)
	}))
	defer server.Close()

	upid, err := newHAResourceTestClient(t, server.URL).RevertNodeNetwork(ctx, "node1")
	if err != nil {
		t.Fatalf("RevertNodeNetwork() unexpected error: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api2/json/nodes/node1/network" {
		t.Fatalf("unexpected revert request: %q %q", gotMethod, gotPath)
	}
	if upid != nodeNetworkRevertUPID {
		t.Fatalf("unexpected UPID: %q", upid)
	}
}

func TestNodeNetworkRevertSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewNodeNetworkRevertResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 4; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	node, ok := resp.Schema.Attributes["node"].(resourceschema.StringAttribute)
	if !ok || !node.IsRequired() || len(node.PlanModifiers) == 0 {
		t.Fatal("node must be required with a replacement plan modifier")
	}
	triggers, ok := resp.Schema.Attributes["triggers"].(resourceschema.MapAttribute)
	if !ok || !triggers.IsRequired() || len(triggers.PlanModifiers) == 0 {
		t.Fatal("triggers must be a required map with a replacement plan modifier")
	}
	if !resp.Schema.Attributes["pending_changes"].IsComputed() || !resp.Schema.Attributes["id"].IsComputed() {
		t.Fatal("pending_changes and id must be computed")
	}
}

func TestNodeNetworkRevertValidateConfig(t *testing.T) {
	if diags := validateNodeNetworkRevert(nodeNetworkRevertModel{Node: types.StringValue("node1")}); diags.HasError() {
		t.Fatalf("valid config diagnostics: %v", diags)
	}
	if diags := validateNodeNetworkRevert(nodeNetworkRevertModel{Node: types.StringValue("node_1")}); !diags.HasError() {
		t.Fatal("node_1 must be rejected as node name")
	}
	if diags := validateNodeNetworkRevert(nodeNetworkRevertModel{Node: types.StringUnknown()}); diags.HasError() {
		t.Fatalf("unknown node must defer: %v", diags)
	}
}

func TestNodeNetworkRevertLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	var reverts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/nodes/node1/network":
			reverts.Add(1)
			handler.envelope(w, nodeNetworkRevertUPID)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/node1/tasks/"+nodeNetworkRevertUPID+"/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/node1/network":
			_, _ = w.Write([]byte(`{"data":[],"changes":[]}`))
		default:
			handler.fail(w, "unexpected revert request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &NodeNetworkRevertResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := nodeNetworkRevertModel{
		Node:           types.StringValue("node1"),
		Triggers:       testStringMap(t, map[string]string{"staged": "generation-1"}),
		PendingChanges: types.ListNull(types.StringType),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("revert create diagnostics: %v", createResp.Diagnostics)
	}
	var created nodeNetworkRevertModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() {
		t.Fatalf("decode revert state: %v", diags)
	}
	if created.ID.ValueString() != "node1" || !created.PendingChanges.IsNull() {
		t.Fatalf("unexpected revert state: %#v", created)
	}
	trigger, ok := created.Triggers.Elements()["staged"].(types.String)
	if !ok || trigger.ValueString() != "generation-1" {
		t.Fatalf("revert dropped triggers: %#v", created)
	}
	if got := reverts.Load(); got != 1 {
		t.Fatalf("unexpected revert count: %d", got)
	}

	deleteResp := resource.DeleteResponse{State: createResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: createResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() || reverts.Load() != 1 {
		t.Fatalf("delete must be a no-op: diagnostics=%v reverts=%d", deleteResp.Diagnostics, reverts.Load())
	}
}

func TestNodeNetworkRevertReadReportsPendingChanges(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"data":[],"changes":["- vmbr7","+ vmbr8: bridge_ports eth9"]}`))
	}))
	defer server.Close()

	res := &NodeNetworkRevertResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := nodeNetworkRevertModel{
		ID:             types.StringValue("node1"),
		Node:           types.StringValue("node1"),
		Triggers:       testStringMap(t, map[string]string{"staged": "generation-1"}),
		PendingChanges: types.ListNull(types.StringType),
	}
	readResp := resource.ReadResponse{State: testResourceState(t, schema, state)}
	res.Read(context.Background(), resource.ReadRequest{State: readResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var refreshed nodeNetworkRevertModel
	if diags := readResp.State.Get(context.Background(), &refreshed); diags.HasError() {
		t.Fatalf("decode read state: %v", diags)
	}
	var pending []string
	if diags := refreshed.PendingChanges.ElementsAs(context.Background(), &pending, false); diags.HasError() || len(pending) != 2 {
		t.Fatalf("unexpected pending changes: %#v (%v)", pending, diags)
	}
	if refreshed.Node.ValueString() != "node1" || refreshed.Triggers.IsNull() {
		t.Fatalf("read dropped Terraform-side values: %#v", refreshed)
	}
}

func TestNodeNetworkRevertCreateFailures(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.Method == http.MethodDelete {
			handler.envelope(w, nil) // empty acknowledgement
			return
		}
		handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "ERROR"})
	}))
	defer server.Close()

	ctx := context.Background()
	res := &NodeNetworkRevertResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := nodeNetworkRevertModel{Node: types.StringValue("node1"), Triggers: testStringMap(t, map[string]string{}), PendingChanges: types.ListNull(types.StringType)}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, config)}, &createResp)
	if !createResp.Diagnostics.HasError() || !strings.Contains(createResp.Diagnostics[0].Detail(), "no UPID") {
		t.Fatalf("empty acknowledgement was accepted: %v", createResp.Diagnostics)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			handler.envelope(w, nodeNetworkRevertUPID)
			return
		}
		handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "ERROR"})
	}))
	defer failing.Close()
	res = &NodeNetworkRevertResource{client: testLifecycleClient(t, failing)}
	createResp = resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, config)}, &createResp)
	if !createResp.Diagnostics.HasError() || !strings.Contains(createResp.Diagnostics[0].Detail(), "ERROR") {
		t.Fatalf("failed revert task was accepted: %v", createResp.Diagnostics)
	}
}

func TestNodeNetworkRevertImportState(t *testing.T) {
	res, ok := NewNodeNetworkRevertResource().(*NodeNetworkRevertResource)
	if !ok {
		t.Fatal("constructor returned a different type")
	}
	schema := testResourceSchema(t, res)
	importResp := resource.ImportStateResponse{State: tfsdk.State{Raw: tftypes.NewValue(schema.Schema.Type().TerraformType(context.Background()), nil), Schema: schema.Schema}}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "node1"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", importResp.Diagnostics)
	}
	var imported nodeNetworkRevertModel
	if diags := importResp.State.Get(context.Background(), &imported); diags.HasError() {
		t.Fatalf("decode import state: %v", diags)
	}
	if imported.ID.ValueString() != "node1" || imported.Node.ValueString() != "node1" {
		t.Fatalf("unexpected import state: %#v", imported)
	}

	badResp := resource.ImportStateResponse{State: tfsdk.State{Raw: tftypes.NewValue(schema.Schema.Type().TerraformType(context.Background()), nil), Schema: schema.Schema}}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "node_1"}, &badResp)
	if !badResp.Diagnostics.HasError() {
		t.Fatal("node_1 must be rejected on import")
	}
}
