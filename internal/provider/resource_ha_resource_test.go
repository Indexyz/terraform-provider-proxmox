// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestHAResourceResourceSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewHAResourceResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 8; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	if !resp.Schema.Attributes["id"].IsComputed() || !resp.Schema.Attributes["resource_id"].IsRequired() || !resp.Schema.Attributes["state"].IsRequired() {
		t.Fatal("id must be computed and resource_id/state must be required")
	}
	for _, name := range []string{"comment", "failback", "auto_rebalance", "max_restart", "max_relocate"} {
		attribute := resp.Schema.Attributes[name]
		if !attribute.IsOptional() || !attribute.IsComputed() {
			t.Fatalf("%s must be optional and computed", name)
		}
	}
	resourceID, ok := resp.Schema.Attributes["resource_id"].(resourceschema.StringAttribute)
	if !ok || len(resourceID.PlanModifiers) == 0 {
		t.Fatal("resource_id must have a replacement plan modifier")
	}
	for _, excluded := range []string{"group", "digest", "purge", "type", "node", "status", "raw"} {
		if _, ok := resp.Schema.Attributes[excluded]; ok {
			t.Fatalf("HA resource schema must not expose %s", excluded)
		}
	}
}

func TestValidateHAResourceConfig(t *testing.T) {
	valid := haResourceModel{
		ResourceID:    types.StringValue("vm:120"),
		State:         types.StringValue("ignored"),
		Comment:       types.StringValue("database guest"),
		MaxRestart:    types.Int64Value(0),
		MaxRelocate:   types.Int64Value(25),
		AutoRebalance: types.BoolValue(false),
		Failback:      types.BoolValue(true),
	}
	if diags := validateHAResourceConfig(valid); diags.HasError() {
		t.Fatalf("valid config diagnostics: %v", diags)
	}
	for _, state := range []string{"started", "stopped", "disabled", "ignored"} {
		config := valid
		config.State = types.StringValue(state)
		if diags := validateHAResourceConfig(config); diags.HasError() {
			t.Fatalf("valid state %q diagnostics: %v", state, diags)
		}
	}

	invalid := []haResourceModel{
		{ResourceID: types.StringValue("120"), State: valid.State},
		{ResourceID: types.StringValue("vm:0"), State: valid.State},
		{ResourceID: types.StringValue("VM:120"), State: valid.State},
		{ResourceID: types.StringValue("node/vm:120"), State: valid.State},
		{ResourceID: types.StringValue("storage:120"), State: valid.State},
		{ResourceID: valid.ResourceID, State: types.StringValue("enabled")},
		{ResourceID: valid.ResourceID, State: types.StringValue("running")},
		{ResourceID: valid.ResourceID, State: valid.State, Comment: types.StringValue(strings.Repeat("x", 4097))},
		{ResourceID: valid.ResourceID, State: valid.State, MaxRestart: types.Int64Value(-1)},
		{ResourceID: valid.ResourceID, State: valid.State, MaxRelocate: types.Int64Value(-1)},
	}
	for _, config := range invalid {
		if diags := validateHAResourceConfig(config); !diags.HasError() {
			t.Fatalf("expected invalid config diagnostics: %#v", config)
		}
	}
}

func TestHAResourceRequestUsesOnlyExplicitConfiguration(t *testing.T) {
	config := haResourceModel{
		ResourceID: types.StringValue("vm:120"),
		State:      types.StringValue("disabled"),
		Comment:    types.StringValue("managed"),
	}
	plan := config
	plan.Failback = types.BoolValue(true)
	plan.AutoRebalance = types.BoolValue(true)
	plan.MaxRestart = types.Int64Value(1)
	plan.MaxRelocate = types.Int64Value(1)

	request := haResourceRequestFromModels(config, plan)
	if request.State != "disabled" || request.Comment == nil || *request.Comment != "managed" {
		t.Fatalf("request omitted explicit configuration: %#v", request)
	}
	if request.Failback != nil || request.AutoRebalance != nil || request.MaxRestart != nil || request.MaxRelocate != nil {
		t.Fatalf("request sent computed-only plan values: %#v", request)
	}
}

func TestHAResourceManagedFieldDeletion(t *testing.T) {
	config := haResourceModel{
		State:         types.StringValue("ignored"),
		AutoRebalance: types.BoolValue(false),
		MaxRelocate:   types.Int64Value(3),
	}
	got := haResourceDeleteKeys(config, []string{"comment", "failback", "auto-rebalance", "max_restart", "max_relocate"})
	want := []string{"comment", "failback", "max_restart"}
	if len(got) != len(want) {
		t.Fatalf("unexpected delete keys: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected delete keys: got %v want %v", got, want)
		}
	}
	if got := haResourceDeleteKeys(config, nil); len(got) != 0 {
		t.Fatalf("imported unmanaged fields must not be deleted: %v", got)
	}
}

func TestHAResourceStateAppliesPVE9Defaults(t *testing.T) {
	state := haResourceStateFromAPI(HAResource{SID: "ct:121"})
	if state.ID.ValueString() != "ct:121" || state.ResourceID.ValueString() != "ct:121" || state.State.ValueString() != "started" {
		t.Fatalf("unexpected identity/default state: %#v", state)
	}
	if !state.Failback.ValueBool() || !state.AutoRebalance.ValueBool() || state.MaxRestart.ValueInt64() != 1 || state.MaxRelocate.ValueInt64() != 1 {
		t.Fatalf("unexpected PVE defaults: %#v", state)
	}
	if !state.Comment.IsNull() {
		t.Fatalf("missing comment must remain null: %#v", state.Comment)
	}
	if alias := haResourceStateFromAPI(HAResource{SID: "vm:122", State: "enabled"}); alias.State.ValueString() != "started" {
		t.Fatalf("enabled alias was not normalized: %#v", alias)
	}

	explicit := haResourceStateFromAPI(HAResource{
		SID:           "vm:120",
		State:         "disabled",
		Comment:       "database guest",
		Failback:      proxmoxOptionalBool{value: boolPtr(false)},
		AutoRebalance: proxmoxOptionalBool{value: boolPtr(false)},
		MaxRestart:    proxmoxOptionalInt64{value: intPtr64(0)},
		MaxRelocate:   proxmoxOptionalInt64{value: intPtr64(0)},
	})
	if explicit.State.ValueString() != "disabled" || explicit.Failback.ValueBool() || explicit.AutoRebalance.ValueBool() || explicit.MaxRestart.ValueInt64() != 0 || explicit.MaxRelocate.ValueInt64() != 0 {
		t.Fatalf("explicit API values were not preserved: %#v", explicit)
	}
}

func TestHAResourceReadStateUsesCollectionAndPreservesErrors(t *testing.T) {
	t.Run("read", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/cluster/ha/resources" {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
			}
			writeEnvelope(t, w, []map[string]any{{"sid": "vm:120", "state": "ignored"}})
		}))
		defer server.Close()
		state, diags := (&HAResourceResource{client: newHAResourceTestClient(t, server.URL)}).readState(context.Background(), "vm:120")
		if diags.HasError() {
			t.Fatalf("readState() unexpected diagnostics: %v", diags)
		}
		if state.ResourceID.ValueString() != "vm:120" || state.State.ValueString() != "ignored" {
			t.Fatalf("unexpected state: %#v", state)
		}
	})

	t.Run("missing", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeEnvelope(t, w, []any{})
		}))
		defer server.Close()
		state, diags := (&HAResourceResource{client: newHAResourceTestClient(t, server.URL)}).readState(context.Background(), "vm:404")
		if diags.HasError() || !state.ID.IsNull() {
			t.Fatalf("missing readState() = %#v, %v", state, diags)
		}
	})

	t.Run("api error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"digest":"configuration modified"},"data":null}`))
		}))
		defer server.Close()
		_, diags := (&HAResourceResource{client: newHAResourceTestClient(t, server.URL)}).readState(context.Background(), "vm:120")
		if !diags.HasError() || !strings.Contains(diags[0].Detail(), "status 500") || !strings.Contains(diags[0].Detail(), "configuration modified") {
			t.Fatalf("readState() did not preserve API error: %v", diags)
		}
	})
}

func TestHAResourceImportValidation(t *testing.T) {
	for _, id := range []string{"120", "vm:0", "VM:120", "node/vm:120", "storage:120"} {
		var resp resource.ImportStateResponse
		(&HAResourceResource{}).ImportState(context.Background(), resource.ImportStateRequest{ID: id}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected import diagnostic for %q", id)
		}
	}
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	NewHAResourceResource().Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	for _, id := range []string{"vm:120", "ct:121"} {
		var resp resource.ImportStateResponse
		resp.State = tfsdk.State{
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
			Schema: schemaResp.Schema,
		}
		(&HAResourceResource{}).ImportState(ctx, resource.ImportStateRequest{ID: id}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("canonical import ID %q diagnostics: %v", id, resp.Diagnostics)
		}
		var state haResourceModel
		if diags := resp.State.Get(ctx, &state); diags.HasError() {
			t.Fatalf("unable to read imported state for %q: %v", id, diags)
		}
		if state.ID.ValueString() != id || state.ResourceID.ValueString() != id {
			t.Fatalf("unexpected imported state for %q: %#v", id, state)
		}
	}
}
