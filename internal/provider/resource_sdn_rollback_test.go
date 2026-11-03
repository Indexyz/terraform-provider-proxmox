// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestClientSDNRollback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		if r.Method != http.MethodPost || r.URL.Path != "/api2/json/cluster/sdn/rollback" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		assertFormValues(t, r, url.Values{"lock-token": {testSDNLockToken}, "release-lock": {"1"}})
		writeEnvelope(t, w, nil)
	}))
	defer server.Close()

	if err := newHAResourceTestClient(t, server.URL).RollbackSDN(context.Background(), testSDNLockToken); err != nil {
		t.Fatalf("RollbackSDN() unexpected error: %v", err)
	}
}

func TestSDNRollbackResourceLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	zones := []map[string]any{{"zone": "vz1", "type": "simple", "state": "new"}}
	var rolledBack bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/lock":
			handler.envelope(w, testSDNLockToken)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/lock":
			handler.envelope(w, nil)
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/rollback":
			rolledBack = true
			zones = nil
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/zones":
			handler.envelope(w, zones)
		default:
			handler.fail(w, "unexpected rollback request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &SDNRollbackResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)

	// A read before the rollback still reports the pending zone.
	state := sdnRollbackModel{
		ID: types.StringValue("sdn_rollback"), Triggers: testStringMap(t, map[string]string{"r": "1"}),
		Remaining: types.ListNull(types.StringType), RemainingZones: types.MapNull(types.StringType),
	}
	before := resource.ReadResponse{State: testResourceState(t, schema, state)}
	res.Read(ctx, resource.ReadRequest{State: before.State}, &before)
	var pending sdnRollbackModel
	if diags := before.State.Get(ctx, &pending); diags.HasError() || pending.Remaining.IsNull() {
		t.Fatalf("pending zone must be reported before the rollback: %#v (%v)", pending.Remaining, diags)
	}

	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, state)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("rollback create diagnostics: %v", createResp.Diagnostics)
	}
	if !rolledBack {
		t.Fatal("rollback endpoint was not called")
	}
	var cleared sdnRollbackModel
	if diags := createResp.State.Get(ctx, &cleared); diags.HasError() || !cleared.Remaining.IsNull() {
		t.Fatalf("pending list must be empty after rollback: %#v (%v)", cleared.Remaining, diags)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

func TestSDNRollbackRejectsEmptyTriggers(t *testing.T) {
	if diags := validateSDNRollback(sdnRollbackModel{Triggers: types.MapValueMust(types.StringType, nil)}); !diags.HasError() {
		t.Fatal("empty triggers must be rejected")
	}
}
