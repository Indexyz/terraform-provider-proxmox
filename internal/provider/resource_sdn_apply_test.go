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

const sdnApplyUPID = "UPID:node1:00001234:00000000:00000000:reloadnetworkall::root@pam:"

func sdnApplyModelWith(t *testing.T, triggers map[string]string) sdnApplyModel {
	t.Helper()
	model := sdnApplyModel{
		Triggers:       testStringMap(t, triggers),
		Pending:        types.ListNull(types.StringType),
		PendingObjects: types.MapNull(types.StringType),
	}
	return model
}

func TestSDNApplySchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewSDNApplyResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 4; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	triggers, ok := resp.Schema.Attributes["triggers"].(resourceschema.MapAttribute)
	if !ok || !triggers.IsRequired() || len(triggers.PlanModifiers) == 0 {
		t.Fatal("triggers must be a required map with a replacement plan modifier")
	}
	for _, name := range []string{"id", "pending", "pending_objects"} {
		if !resp.Schema.Attributes[name].IsComputed() {
			t.Fatalf("%s must be computed", name)
		}
	}
}

func TestSDNApplyValidateConfig(t *testing.T) {
	if diags := validateSDNApply(sdnApplyModelWith(t, map[string]string{"revision": "1"})); diags.HasError() {
		t.Fatalf("valid triggers diagnostics: %v", diags)
	}
	if diags := validateSDNApply(sdnApplyModel{Triggers: types.MapValueMust(types.StringType, nil)}); !diags.HasError() {
		t.Fatal("empty triggers must be rejected")
	}
	if diags := validateSDNApply(sdnApplyModel{Triggers: types.MapUnknown(types.StringType)}); diags.HasError() {
		t.Fatalf("unknown triggers must defer: %v", diags)
	}
}

func TestSDNApplyLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	var reloads atomic.Int64
	pending := []map[string]any{{"zone": "vz1", "type": "vlan", "state": "new"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/lock":
			handler.envelope(w, testSDNLockToken)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/sdn":
			if !handler.form(w, r, url.Values{"lock-token": {testSDNLockToken}, "release-lock": {"1"}}) {
				return
			}
			reloads.Add(1)
			pending = nil
			handler.envelope(w, sdnApplyUPID)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/node1/tasks/"+sdnApplyUPID+"/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/zones":
			handler.envelope(w, pending)
		default:
			handler.fail(w, "unexpected SDN apply request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &SDNApplyResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	plan := sdnApplyModelWith(t, map[string]string{"revision": "1"})
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, plan)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("apply create diagnostics: %v", createResp.Diagnostics)
	}
	var created sdnApplyModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() {
		t.Fatalf("decode apply state: %v", diags)
	}
	if !created.Pending.IsNull() || created.ID.ValueString() != "sdn_apply" {
		t.Fatalf("committed apply must have no pending objects: %#v", created)
	}
	if got := reloads.Load(); got != 1 {
		t.Fatalf("unexpected reload count: %d", got)
	}

	deleteResp := resource.DeleteResponse{State: createResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: createResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() || reloads.Load() != 1 {
		t.Fatalf("delete must not reload: diagnostics=%v reloads=%d", deleteResp.Diagnostics, reloads.Load())
	}
}

func TestSDNApplyReadReportsPendingObjects(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		handler.envelope(w, []map[string]any{
			{"zone": "vz2", "type": "simple", "state": "changed"},
			{"zone": "vz1", "type": "simple", "state": "new"},
			{"zone": "vz3", "type": "simple"},
		})
	}))
	defer server.Close()

	res := &SDNApplyResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := sdnApplyModelWith(t, map[string]string{"revision": "1"})
	state.ID = types.StringValue("sdn_apply")
	readResp := resource.ReadResponse{State: testResourceState(t, schema, state)}
	res.Read(context.Background(), resource.ReadRequest{State: readResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var refreshed sdnApplyModel
	if diags := readResp.State.Get(context.Background(), &refreshed); diags.HasError() {
		t.Fatalf("decode read state: %v", diags)
	}
	var pending []string
	if diags := refreshed.Pending.ElementsAs(context.Background(), &pending, false); diags.HasError() || len(pending) != 2 {
		t.Fatalf("unexpected pending list: %#v (%v)", pending, diags)
	}
	if pending[0] != "zone vz1 (new)" || pending[1] != "zone vz2 (changed)" {
		t.Fatalf("pending list is not sorted: %#v", pending)
	}
	var objects map[string]string
	if diags := refreshed.PendingObjects.ElementsAs(context.Background(), &objects, false); diags.HasError() || objects["vz2"] != "changed" {
		t.Fatalf("unexpected pending objects: %#v (%v)", objects, diags)
	}
}

func TestSDNApplyCreateFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("empty upid", func(t *testing.T) {
		handler := &lifecycleHandler{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !handler.auth(w, r) {
				return
			}
			if r.Method == http.MethodPost {
				handler.envelope(w, testSDNLockToken)
				return
			}
			handler.envelope(w, nil)
		}))
		defer server.Close()
		res := &SDNApplyResource{client: testLifecycleClient(t, server)}
		schema := testResourceSchema(t, res)
		createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
		res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, sdnApplyModelWith(t, map[string]string{"r": "1"}))}, &createResp)
		if !createResp.Diagnostics.HasError() || !strings.Contains(createResp.Diagnostics[0].Detail(), "no UPID") {
			t.Fatalf("empty acknowledgement was accepted: %v", createResp.Diagnostics)
		}
	})

	t.Run("task failure", func(t *testing.T) {
		handler := &lifecycleHandler{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !handler.auth(w, r) {
				return
			}
			if r.Method == http.MethodPost {
				handler.envelope(w, testSDNLockToken)
				return
			}
			if r.Method == http.MethodPut {
				handler.envelope(w, sdnApplyUPID)
				return
			}
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "ERROR"})
		}))
		defer server.Close()
		res := &SDNApplyResource{client: testLifecycleClient(t, server)}
		schema := testResourceSchema(t, res)
		createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
		res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, sdnApplyModelWith(t, map[string]string{"r": "1"}))}, &createResp)
		if !createResp.Diagnostics.HasError() || !strings.Contains(createResp.Diagnostics[0].Detail(), "ERROR") {
			t.Fatalf("failed reload task was accepted: %v", createResp.Diagnostics)
		}
	})
}

func TestSDNApplyImportIsNotRequired(t *testing.T) {
	// Apply state carries no server-side identity beyond triggers, so it stays
	// un-importable; assert the schema does not advertise an import path.
	var resp resource.SchemaResponse
	NewSDNApplyResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if _, ok := NewSDNApplyResource().(resource.ResourceWithImportState); ok {
		t.Fatal("SDN apply must not implement import")
	}
	_ = tftypes.NewValue
}
