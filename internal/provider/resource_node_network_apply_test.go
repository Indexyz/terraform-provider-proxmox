// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const nodeNetworkApplyUPID = "UPID:node1:00001234:00000000:00000000:srvreload:networking:root@pam:"

func testStringMap(t *testing.T, values map[string]string) types.Map {
	t.Helper()
	value, diags := types.MapValueFrom(context.Background(), types.StringType, values)
	if diags.HasError() {
		t.Fatalf("create string map: %v", diags)
	}
	return value
}

func TestNodeNetworkApplySchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewNodeNetworkApplyResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 5; got != want {
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
	regenerate, ok := resp.Schema.Attributes["regenerate_frr"].(resourceschema.BoolAttribute)
	if !ok || !regenerate.IsOptional() || len(regenerate.PlanModifiers) == 0 {
		t.Fatal("regenerate_frr must be optional with a replacement plan modifier")
	}
	if !resp.Schema.Attributes["pending_changes"].IsComputed() || !resp.Schema.Attributes["id"].IsComputed() {
		t.Fatal("pending_changes and id must be computed")
	}
	if _, ok := resp.Schema.Attributes["digest"]; ok {
		t.Fatal("apply schema must not expose a digest")
	}
}

func TestNodeNetworkApplyValidateConfig(t *testing.T) {
	valid := nodeNetworkApplyModel{Node: types.StringValue("node1"), Triggers: testStringMap(t, map[string]string{"interfaces": "abc"})}
	if diags := validateNodeNetworkApply(valid); diags.HasError() {
		t.Fatalf("valid config diagnostics: %v", diags)
	}
	if diags := validateNodeNetworkApply(nodeNetworkApplyModel{Node: types.StringValue("node_1")}); !diags.HasError() {
		t.Fatal("node_1 must be rejected as node name")
	}
	if diags := validateNodeNetworkApply(nodeNetworkApplyModel{Node: types.StringUnknown()}); diags.HasError() {
		t.Fatalf("unknown node must defer: %v", diags)
	}
}

func TestNodeNetworkApplyLifecycle(t *testing.T) {
	pending := []string{"+ vmbr7: bridge_ports eth9"}
	handler := &lifecycleHandler{}
	var reloads atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/nodes/node1/network":
			if !handler.form(w, r, url.Values{"regenerate-frr": {"1"}}) {
				return
			}
			reloads.Add(1)
			pending = nil
			handler.envelope(w, nodeNetworkApplyUPID)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/node1/tasks/"+nodeNetworkApplyUPID+"/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/node1/network":
			if pending == nil {
				handler.envelope(w, []any{})
				return
			}
			_, _ = w.Write([]byte(`{"data":[],"changes":["+ vmbr7: bridge_ports eth9"]}`))
		default:
			handler.fail(w, "unexpected apply request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &NodeNetworkApplyResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := nodeNetworkApplyModel{
		Node:           types.StringValue("node1"),
		Triggers:       testStringMap(t, map[string]string{"interfaces": "generation-1"}),
		RegenerateFRR:  types.BoolValue(true),
		PendingChanges: types.ListNull(types.StringType),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("apply create diagnostics: %v", createResp.Diagnostics)
	}
	var created nodeNetworkApplyModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() {
		t.Fatalf("decode apply state: %v", diags)
	}
	if created.ID.ValueString() != "node1" || !created.PendingChanges.IsNull() {
		t.Fatalf("unexpected apply state: %#v", created)
	}
	if !created.RegenerateFRR.ValueBool() {
		t.Fatalf("apply dropped regenerate_frr: %#v", created)
	}
	trigger, ok := created.Triggers.Elements()["interfaces"].(types.String)
	if !ok || trigger.ValueString() != "generation-1" {
		t.Fatalf("apply dropped triggers: %#v", created)
	}
	if got := reloads.Load(); got != 1 {
		t.Fatalf("unexpected reload count: %d", got)
	}

	// Destroying the apply record never reloads.
	deleteResp := resource.DeleteResponse{State: createResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: createResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() || reloads.Load() != 1 {
		t.Fatalf("delete must be a no-op: diagnostics=%v reloads=%d", deleteResp.Diagnostics, reloads.Load())
	}
}

func TestNodeNetworkApplyReadReportsPendingChanges(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"data":[],"changes":["- vmbr7","+ vmbr8: bridge_ports eth9"]}`))
	}))
	defer server.Close()

	res := &NodeNetworkApplyResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := nodeNetworkApplyModel{
		ID:             types.StringValue("node1"),
		Node:           types.StringValue("node1"),
		Triggers:       testStringMap(t, map[string]string{"interfaces": "generation-1"}),
		RegenerateFRR:  types.BoolNull(),
		PendingChanges: types.ListNull(types.StringType),
	}
	readResp := resource.ReadResponse{State: testResourceState(t, schema, state)}
	res.Read(context.Background(), resource.ReadRequest{State: readResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var refreshed nodeNetworkApplyModel
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

func TestNodeNetworkApplyCreateFailures(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.Method == http.MethodPut {
			handler.envelope(w, nil) // empty acknowledgement
			return
		}
		handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "ERROR"})
	}))
	defer server.Close()

	ctx := context.Background()
	res := &NodeNetworkApplyResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := nodeNetworkApplyModel{Node: types.StringValue("node1"), Triggers: testStringMap(t, map[string]string{}), PendingChanges: types.ListNull(types.StringType)}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, config)}, &createResp)
	if !createResp.Diagnostics.HasError() || !strings.Contains(createResp.Diagnostics[0].Detail(), "no UPID") {
		t.Fatalf("empty acknowledgement was accepted: %v", createResp.Diagnostics)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			handler.envelope(w, nodeNetworkApplyUPID)
			return
		}
		handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "ERROR"})
	}))
	defer failing.Close()
	res = &NodeNetworkApplyResource{client: testLifecycleClient(t, failing)}
	createResp = resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, config)}, &createResp)
	if !createResp.Diagnostics.HasError() || !strings.Contains(createResp.Diagnostics[0].Detail(), "ERROR") {
		t.Fatalf("failed reload task was accepted: %v", createResp.Diagnostics)
	}
}

func TestNodeNetworkApplyImportValidation(t *testing.T) {
	for _, id := range []string{"", "node_1", "node/1"} {
		var resp resource.ImportStateResponse
		(&NodeNetworkApplyResource{}).ImportState(context.Background(), resource.ImportStateRequest{ID: id}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected import diagnostic for %q", id)
		}
	}
	ctx := context.Background()
	schemaResp := testResourceSchema(t, &NodeNetworkApplyResource{})
	var resp resource.ImportStateResponse
	resp.State = tfsdk.State{Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil), Schema: schemaResp.Schema}
	(&NodeNetworkApplyResource{}).ImportState(ctx, resource.ImportStateRequest{ID: "node1"}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}
	var state nodeNetworkApplyModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("unable to read imported state: %v", diags)
	}
	if state.ID.ValueString() != "node1" || state.Node.ValueString() != "node1" {
		t.Fatalf("unexpected imported state: %#v", state)
	}
}
