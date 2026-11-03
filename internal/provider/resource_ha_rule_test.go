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
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func testInt64Map(t *testing.T, values map[string]int64) types.Map {
	t.Helper()
	value, diags := types.MapValueFrom(context.Background(), types.Int64Type, values)
	if diags.HasError() {
		t.Fatalf("create int map: %v", diags)
	}
	return value
}

func TestHARuleResourceSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewHARuleResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 11; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	if !resp.Schema.Attributes["id"].IsComputed() || !resp.Schema.Attributes["rule"].IsRequired() || !resp.Schema.Attributes["type"].IsRequired() || !resp.Schema.Attributes["resources"].IsRequired() {
		t.Fatal("id must be computed and rule/type/resources must be required")
	}
	for _, name := range []string{"nodes", "affinity", "strict", "disable", "comment"} {
		attribute := resp.Schema.Attributes[name]
		if !attribute.IsOptional() || !attribute.IsComputed() {
			t.Fatalf("%s must be optional and computed", name)
		}
	}
	for _, name := range []string{"order", "feasibility_errors"} {
		if !resp.Schema.Attributes[name].IsComputed() {
			t.Fatalf("%s must be computed", name)
		}
	}
	for _, name := range []string{"rule", "type"} {
		attribute, ok := resp.Schema.Attributes[name].(resourceschema.StringAttribute)
		if !ok || len(attribute.PlanModifiers) == 0 {
			t.Fatalf("%s must have a replacement plan modifier", name)
		}
	}
	if !resp.Schema.Attributes["resources"].IsRequired() {
		t.Fatal("resources must be required")
	}
	if _, ok := resp.Schema.Attributes["resources"].(resourceschema.SetAttribute); !ok {
		t.Fatal("resources must be a set attribute")
	}
	if _, ok := resp.Schema.Attributes["nodes"].(resourceschema.MapAttribute); !ok {
		t.Fatal("nodes must be a map attribute")
	}
	for _, excluded := range []string{"digest", "purge", "group", "raw", "state"} {
		if _, ok := resp.Schema.Attributes[excluded]; ok {
			t.Fatalf("HA rule schema must not expose %s", excluded)
		}
	}
}

func TestValidateHARuleConfig(t *testing.T) {
	valid := []haRuleModel{
		{
			Rule:      types.StringValue("db-together"),
			Type:      types.StringValue(haRuleTypeNodeAffinity),
			Resources: testStringSet(t, "vm:120", "ct:201"),
			Nodes:     testInt64Map(t, map[string]int64{"node1": 2, "node2": 0}),
			Affinity:  types.StringValue("positive"),
			Strict:    types.BoolValue(true),
			Disable:   types.BoolValue(false),
			Comment:   types.StringValue("database pair"),
		},
		{
			Rule:      types.StringValue("avoid-backup"),
			Type:      types.StringValue(haRuleTypeNodeAffinity),
			Resources: testStringSet(t, "vm:120"),
			Nodes:     testInt64Map(t, map[string]int64{"backup1": 0}),
			Affinity:  types.StringValue("negative"),
		},
		{
			Rule:      types.StringValue("db-apart"),
			Type:      types.StringValue(haRuleTypeResourceAffinity),
			Resources: testStringSet(t, "vm:120", "ct:201"),
			Affinity:  types.StringValue("negative"),
		},
		{
			Rule:      types.StringValue("unknown-values"),
			Type:      types.StringUnknown(),
			Resources: types.SetUnknown(types.StringType),
			Nodes:     types.MapUnknown(types.Int64Type),
			Affinity:  types.StringUnknown(),
		},
	}
	for _, config := range valid {
		if diags := validateHARuleConfig(config); diags.HasError() {
			t.Fatalf("valid config diagnostics: %v (%#v)", diags, config)
		}
	}

	invalid := []haRuleModel{
		{Rule: types.StringValue("1rule"), Type: types.StringValue(haRuleTypeNodeAffinity), Resources: testStringSet(t, "vm:120"), Nodes: testInt64Map(t, map[string]int64{"node1": 0})},
		{Rule: types.StringValue("r"), Type: types.StringValue(haRuleTypeNodeAffinity), Resources: testStringSet(t, "vm:120"), Nodes: testInt64Map(t, map[string]int64{"node1": 0})},
		{Rule: types.StringValue("bad id"), Type: types.StringValue(haRuleTypeNodeAffinity), Resources: testStringSet(t, "vm:120"), Nodes: testInt64Map(t, map[string]int64{"node1": 0})},
		{Rule: types.StringValue("rule"), Type: types.StringValue("affinity"), Resources: testStringSet(t, "vm:120")},
		{Rule: types.StringValue("rule"), Type: types.StringValue(haRuleTypeNodeAffinity), Resources: testStringSet(t), Nodes: testInt64Map(t, map[string]int64{"node1": 0})},
		{Rule: types.StringValue("rule"), Type: types.StringValue(haRuleTypeNodeAffinity), Resources: testStringSet(t, "120"), Nodes: testInt64Map(t, map[string]int64{"node1": 0})},
		{Rule: types.StringValue("rule"), Type: types.StringValue(haRuleTypeNodeAffinity), Resources: testStringSet(t, "VM:120"), Nodes: testInt64Map(t, map[string]int64{"node1": 0})},
		{Rule: types.StringValue("rule"), Type: types.StringValue(haRuleTypeNodeAffinity), Resources: testStringSet(t, "vm:120")},
		{Rule: types.StringValue("rule"), Type: types.StringValue(haRuleTypeNodeAffinity), Resources: testStringSet(t, "vm:120"), Nodes: testInt64Map(t, map[string]int64{})},
		{Rule: types.StringValue("rule"), Type: types.StringValue(haRuleTypeNodeAffinity), Resources: testStringSet(t, "vm:120"), Nodes: testInt64Map(t, map[string]int64{"node_1": 0})},
		{Rule: types.StringValue("rule"), Type: types.StringValue(haRuleTypeNodeAffinity), Resources: testStringSet(t, "vm:120"), Nodes: testInt64Map(t, map[string]int64{"node1": -1})},
		{Rule: types.StringValue("rule"), Type: types.StringValue(haRuleTypeNodeAffinity), Resources: testStringSet(t, "vm:120"), Nodes: testInt64Map(t, map[string]int64{"node1": 2}), Affinity: types.StringValue("negative")},
		{Rule: types.StringValue("rule"), Type: types.StringValue(haRuleTypeNodeAffinity), Resources: testStringSet(t, "vm:120"), Nodes: testInt64Map(t, map[string]int64{"node1": 0}), Affinity: types.StringValue("mixed")},
		{Rule: types.StringValue("rule"), Type: types.StringValue(haRuleTypeResourceAffinity), Resources: testStringSet(t, "vm:120", "ct:201"), Nodes: testInt64Map(t, map[string]int64{"node1": 0}), Affinity: types.StringValue("positive")},
		{Rule: types.StringValue("rule"), Type: types.StringValue(haRuleTypeResourceAffinity), Resources: testStringSet(t, "vm:120", "ct:201"), Strict: types.BoolValue(true), Affinity: types.StringValue("positive")},
		{Rule: types.StringValue("rule"), Type: types.StringValue(haRuleTypeResourceAffinity), Resources: testStringSet(t, "vm:120", "ct:201")},
		{Rule: types.StringValue("rule"), Type: types.StringValue(haRuleTypeResourceAffinity), Resources: testStringSet(t, "vm:120"), Affinity: types.StringValue("positive")},
		{Rule: types.StringValue("rule"), Type: types.StringValue(haRuleTypeNodeAffinity), Resources: testStringSet(t, "vm:120"), Nodes: testInt64Map(t, map[string]int64{"node1": 0}), Comment: types.StringValue(strings.Repeat("x", 4097))},
	}
	for _, config := range invalid {
		if diags := validateHARuleConfig(config); !diags.HasError() {
			t.Fatalf("expected invalid config diagnostics: %#v", config)
		}
	}
}

func TestHARuleRequestEncodesOnlyExplicitConfiguration(t *testing.T) {
	config := haRuleModel{
		Rule:      types.StringValue("db-together"),
		Type:      types.StringValue(haRuleTypeNodeAffinity),
		Resources: testStringSet(t, "vm:120", "ct:201"),
		Nodes:     testInt64Map(t, map[string]int64{"node2": 0, "node1": 2}),
		Comment:   types.StringValue("database pair"),
	}
	plan := config
	plan.Affinity = types.StringValue("positive")
	plan.Strict = types.BoolValue(true)
	plan.Disable = types.BoolValue(false)

	request, diags := haRuleRequestFromModels(context.Background(), config, plan)
	if diags.HasError() {
		t.Fatalf("haRuleRequestFromModels() unexpected diagnostics: %v", diags)
	}
	if request.Type != haRuleTypeNodeAffinity || request.Comment == nil || *request.Comment != "database pair" {
		t.Fatalf("request omitted explicit configuration: %#v", request)
	}
	if request.Affinity != nil || request.Strict != nil || request.Disable != nil {
		t.Fatalf("request sent computed-only plan values: %#v", request)
	}
	if got, want := strings.Join(request.Resources, ","), "ct:201,vm:120"; got != want {
		t.Fatalf("resources were not sorted: got %s want %s", got, want)
	}
	form := haRuleForm(request)
	if got, want := form.Get("nodes"), "node1:2,node2"; got != want {
		t.Fatalf("nodes were not encoded with sorted names and omitted zero priorities: got %s want %s", got, want)
	}
	if got, want := form.Get("resources"), "ct:201,vm:120"; got != want {
		t.Fatalf("resources form value: got %s want %s", got, want)
	}
}

func TestHARuleManagedFieldDeletion(t *testing.T) {
	config := haRuleModel{
		Type:      types.StringValue(haRuleTypeNodeAffinity),
		Disable:   types.BoolValue(false),
		Affinity:  types.StringValue("positive"),
		Resources: testStringSet(t, "vm:120"),
	}
	got := haRuleDeleteKeys(config, []string{"affinity", "comment", "disable", "nodes", "strict"})
	want := []string{"comment", "nodes", "strict"}
	if len(got) != len(want) {
		t.Fatalf("unexpected delete keys: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected delete keys: got %v want %v", got, want)
		}
	}
	if got := haRuleDeleteKeys(config, nil); len(got) != 0 {
		t.Fatalf("imported unmanaged fields must not be deleted: %v", got)
	}
}

func TestHARuleStateFromAPIAppliesDefaultsAndDecodesVariant(t *testing.T) {
	ctx := context.Background()
	state, diags := haRuleStateFromAPI(ctx, HARule{
		Rule:      "db-together",
		Type:      haRuleTypeNodeAffinity,
		Resources: "ct:201, vm:120",
		Nodes:     "node1:2,node2",
		Comment:   "database pair",
		Order:     proxmoxOptionalInt64{value: intPtr64(3)},
		Errors:    map[string]string{"nodes": "node1 is offline"},
	})
	if diags.HasError() {
		t.Fatalf("haRuleStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "db-together" || state.Rule.ValueString() != "db-together" || state.Type.ValueString() != haRuleTypeNodeAffinity {
		t.Fatalf("unexpected identity: %#v", state)
	}
	if got, want := state.Order.ValueInt64(), int64(3); got != want {
		t.Fatalf("unexpected order: got %d want %d", got, want)
	}
	if state.Affinity.ValueString() != "positive" || state.Strict.ValueBool() || state.Disable.ValueBool() {
		t.Fatalf("unexpected defaults: %#v", state)
	}
	if !state.Disable.IsNull() && state.Disable.ValueBool() {
		t.Fatalf("disable must default to false: %#v", state.Disable)
	}
	var resources []string
	if diags := state.Resources.ElementsAs(ctx, &resources, false); diags.HasError() || len(resources) != 2 {
		t.Fatalf("unexpected resources: %#v (%v)", resources, diags)
	}
	var nodes map[string]int64
	if diags := state.Nodes.ElementsAs(ctx, &nodes, false); diags.HasError() || nodes["node1"] != 2 || nodes["node2"] != 0 {
		t.Fatalf("unexpected nodes: %#v (%v)", nodes, diags)
	}
	var feasibility map[string]string
	if diags := state.FeasibilityErrors.ElementsAs(ctx, &feasibility, false); diags.HasError() || feasibility["nodes"] != "node1 is offline" {
		t.Fatalf("unexpected feasibility errors: %#v (%v)", feasibility, diags)
	}

	apart, diags := haRuleStateFromAPI(ctx, HARule{Rule: "db-apart", Type: haRuleTypeResourceAffinity, Resources: "vm:120,ct:201", Affinity: "negative"})
	if diags.HasError() {
		t.Fatalf("haRuleStateFromAPI() resource-affinity diagnostics: %v", diags)
	}
	if !apart.Nodes.IsNull() || !apart.Strict.IsNull() || apart.Affinity.ValueString() != "negative" {
		t.Fatalf("resource-affinity must not expose nodes/strict: %#v", apart)
	}
	if !apart.FeasibilityErrors.IsNull() || apart.Order.ValueInt64() != 0 {
		t.Fatalf("unexpected empty-computed values: %#v", apart)
	}
}

func TestHARuleStateFromAPIRejectsMalformedNodes(t *testing.T) {
	_, diags := haRuleStateFromAPI(context.Background(), HARule{Rule: "rule", Type: haRuleTypeNodeAffinity, Nodes: "node1:x"})
	if !diags.HasError() || !strings.Contains(diags[0].Detail(), "node1:x") {
		t.Fatalf("malformed nodes were not rejected: %v", diags)
	}
}

func TestHARuleReadStateUsesCollectionAndPreservesErrors(t *testing.T) {
	t.Run("read", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/cluster/ha/rules" {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
			}
			writeEnvelope(t, w, []map[string]any{{
				"affinity":  "positive",
				"resources": "vm:120",
				"rule":      "db-together",
				"type":      "node-affinity",
				"nodes":     "node1",
			}})
		}))
		defer server.Close()
		state, diags := (&HARuleResource{client: newHAResourceTestClient(t, server.URL)}).readState(context.Background(), "db-together")
		if diags.HasError() {
			t.Fatalf("readState() unexpected diagnostics: %v", diags)
		}
		if state.Rule.ValueString() != "db-together" || state.Affinity.ValueString() != "positive" {
			t.Fatalf("unexpected state: %#v", state)
		}
	})

	t.Run("missing", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeEnvelope(t, w, []any{})
		}))
		defer server.Close()
		state, diags := (&HARuleResource{client: newHAResourceTestClient(t, server.URL)}).readState(context.Background(), "gone")
		if diags.HasError() || !state.ID.IsNull() {
			t.Fatalf("missing readState() = %#v, %v", state, diags)
		}
	})

	t.Run("api error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"rule":"permission denied"},"data":null}`))
		}))
		defer server.Close()
		_, diags := (&HARuleResource{client: newHAResourceTestClient(t, server.URL)}).readState(context.Background(), "db-together")
		if !diags.HasError() || !strings.Contains(diags[0].Detail(), "status 500") || !strings.Contains(diags[0].Detail(), "permission denied") {
			t.Fatalf("readState() did not preserve API error: %v", diags)
		}
	})
}

func TestHARuleImportValidation(t *testing.T) {
	for _, id := range []string{"1rule", "r", "bad id", "vm/1"} {
		var resp resource.ImportStateResponse
		(&HARuleResource{}).ImportState(context.Background(), resource.ImportStateRequest{ID: id}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected import diagnostic for %q", id)
		}
	}
	ctx := context.Background()
	schemaResp := testResourceSchema(t, &HARuleResource{})
	for _, id := range []string{"db-together", "db_apart"} {
		var resp resource.ImportStateResponse
		resp.State = tfsdk.State{
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
			Schema: schemaResp.Schema,
		}
		(&HARuleResource{}).ImportState(ctx, resource.ImportStateRequest{ID: id}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("valid import ID %q diagnostics: %v", id, resp.Diagnostics)
		}
		var state haRuleModel
		if diags := resp.State.Get(ctx, &state); diags.HasError() {
			t.Fatalf("unable to read imported state for %q: %v", id, diags)
		}
		if state.ID.ValueString() != id || state.Rule.ValueString() != id {
			t.Fatalf("unexpected imported state for %q: %#v", id, state)
		}
	}
}

func TestHARuleResourceLifecycleKeepsPUTShape(t *testing.T) {
	rule := map[string]string{
		"affinity":  "positive",
		"comment":   "database pair",
		"disable":   "1",
		"nodes":     "node1:2,node2",
		"resources": "ct:201,vm:120",
		"strict":    "1",
		"type":      "node-affinity",
	}
	exists := true
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/ha/rules":
			if !handler.form(w, r, url.Values{
				"affinity":  {"positive"},
				"comment":   {"database pair"},
				"disable":   {"1"},
				"nodes":     {"node1:2,node2"},
				"resources": {"ct:201,vm:120"},
				"rule":      {"db-together"},
				"strict":    {"1"},
				"type":      {"node-affinity"},
			}) {
				return
			}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/ha/rules":
			if !exists {
				handler.envelope(w, []any{})
				return
			}
			payload := map[string]any{"digest": "digest-1", "order": 1, "rule": "db-together"}
			for key, value := range rule {
				payload[key] = value
			}
			handler.envelope(w, []any{payload})
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/ha/rules/db-together":
			if !handler.form(w, r, url.Values{
				"affinity":  {"negative"},
				"comment":   {"database pair updated"},
				"delete":    {"disable,strict"},
				"digest":    {"digest-1"},
				"nodes":     {"node1,node2:1"},
				"resources": {"ct:201,vm:120,vm:121"},
				"rule":      {"db-together"},
			}) {
				return
			}
			handler.envelope(w, nil)
			rule = map[string]string{
				"affinity":  "negative",
				"comment":   "database pair updated",
				"nodes":     "node1,node2:1",
				"resources": "ct:201,vm:120,vm:121",
				"type":      "node-affinity",
			}
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/ha/rules/db-together":
			if !handler.form(w, r, url.Values{}) {
				return
			}
			exists = false
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected HA rule request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &HARuleResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := haRuleModel{
		Rule:              types.StringValue("db-together"),
		Type:              types.StringValue(haRuleTypeNodeAffinity),
		Resources:         testStringSet(t, "vm:120", "ct:201"),
		Nodes:             testInt64Map(t, map[string]int64{"node1": 2, "node2": 0}),
		Comment:           types.StringValue("database pair"),
		Affinity:          types.StringValue("positive"),
		Strict:            types.BoolValue(true),
		Disable:           types.BoolValue(true),
		FeasibilityErrors: types.MapNull(types.StringType),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("HA rule create diagnostics: %v", createResp.Diagnostics)
	}
	var created haRuleModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() {
		t.Fatalf("decode created HA rule state: %v", diags)
	}
	if created.Order.ValueInt64() != 1 || !created.Strict.ValueBool() || created.Affinity.ValueString() != "positive" {
		t.Fatalf("unexpected created state: %#v", created)
	}

	updated := config
	updated.Resources = testStringSet(t, "vm:120", "vm:121", "ct:201")
	updated.Nodes = testInt64Map(t, map[string]int64{"node1": 0, "node2": 1})
	updated.Comment = types.StringValue("database pair updated")
	updated.Affinity = types.StringValue("negative")
	updated.Strict = types.BoolNull()
	updated.Disable = types.BoolNull()
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}, Private: createResp.Private}
	res.Update(ctx, resource.UpdateRequest{
		Config:  testResourceConfig(t, schema, updated),
		Plan:    testResourcePlan(t, schema, updated),
		State:   createResp.State,
		Private: createResp.Private,
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("HA rule update diagnostics: %v", updateResp.Diagnostics)
	}
	var refreshed haRuleModel
	if diags := updateResp.State.Get(ctx, &refreshed); diags.HasError() {
		t.Fatalf("decode updated HA rule state: %v", diags)
	}
	if refreshed.Affinity.ValueString() != "negative" || !refreshed.Disable.IsNull() && refreshed.Disable.ValueBool() {
		t.Fatalf("unexpected updated state: %#v", refreshed)
	}

	deleteResp := resource.DeleteResponse{State: updateResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("HA rule delete diagnostics: %v", deleteResp.Diagnostics)
	}

	readResp := resource.ReadResponse{State: updateResp.State}
	res.Read(ctx, resource.ReadRequest{State: updateResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("HA rule read-after-delete diagnostics: %v", readResp.Diagnostics)
	}
	if got := readResp.State.Raw.IsNull(); !got {
		t.Fatal("missing HA rule must be removed from state")
	}

	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}
