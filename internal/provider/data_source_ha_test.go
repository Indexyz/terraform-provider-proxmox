// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestHAResourcesDataSourceSortsAndMaps(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/ha/resources": []map[string]any{
			{"sid": "vm:102", "state": "started", "comment": "db", "failback": 1, "auto-rebalance": 0, "max_restart": 2, "max_relocate": 3},
			{"sid": "ct:101", "state": "stopped"},
		},
	}
	readResp := runSDNDataSource(t, NewHAResourcesDataSource(), responses, map[string]any{})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state haResourcesDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	got := make([]string, 0, len(state.Resources))
	for _, record := range state.Resources {
		got = append(got, fmt.Sprintf("%s|%s|%s|%v|%v|%d|%d", record.SID.ValueString(), record.State.ValueString(), record.Comment.ValueString(), record.Failback.ValueBool(), record.AutoRebalance.ValueBool(), record.MaxRestart.ValueInt64(), record.MaxRelocate.ValueInt64()))
	}
	want := []string{"ct:101|stopped||false|false|0|0", "vm:102|started|db|true|false|2|3"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected projection:\n got %v\nwant %v", got, want)
	}
}

func TestHARulesDataSourceFiltersAndMaps(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/ha/rules": []map[string]any{
			{"rule": "spread", "type": "resource-affinity", "resources": "vm:102,vm:103", "affinity": "negative", "comment": "spread db"},
			{"rule": "prefer-a", "type": "node-affinity", "resources": "vm:101", "nodes": "node1:2,node2:1", "strict": 1, "disable": 1, "affinity": "positive", "errors": map[string]any{"vm:101": "unknown node"}},
		},
	}
	readResp := runSDNDataSource(t, NewHARulesDataSource(), responses, map[string]any{})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state haRulesDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if len(state.Rules) != 2 || state.Rules[0].Rule.ValueString() != "prefer-a" {
		t.Fatalf("unexpected order: %#v", state.Rules)
	}
	prefer := state.Rules[0]
	if prefer.Type.ValueString() != "node-affinity" || !prefer.Strict.ValueBool() || !prefer.Disable.ValueBool() {
		t.Fatalf("unexpected node-affinity projection: %#v", prefer)
	}
	if got := sdnListValues(prefer.Resources); fmt.Sprint(got) != fmt.Sprint([]string{"vm:101"}) {
		t.Fatalf("unexpected resources: %v", got)
	}
	if message, ok := prefer.Errors.Elements()["vm:101"].(types.String); !ok || message.ValueString() != "unknown node" {
		t.Fatalf("errors were not mapped: %#v", prefer.Errors)
	}

	filtered := runSDNDataSource(t, NewHARulesDataSource(), responses, map[string]any{"type": "resource-affinity"})
	var filteredState haRulesDataSourceModel
	if diags := filtered.State.Get(context.Background(), &filteredState); diags.HasError() || len(filteredState.Rules) != 1 || filteredState.Rules[0].Rule.ValueString() != "spread" {
		t.Fatalf("type filter failed: %#v (%v)", filteredState.Rules, diags)
	}
}

func TestHADataSourcesEmptyAndError(t *testing.T) {
	emptyResources := runSDNDataSource(t, NewHAResourcesDataSource(), map[string]any{"/api2/json/cluster/ha/resources": []any{}}, map[string]any{})
	var resources haResourcesDataSourceModel
	if diags := emptyResources.State.Get(context.Background(), &resources); diags.HasError() || len(resources.Resources) != 0 {
		t.Fatalf("empty resources must stay empty: %#v (%v)", resources.Resources, diags)
	}
	emptyRules := runSDNDataSource(t, NewHARulesDataSource(), map[string]any{"/api2/json/cluster/ha/rules": []any{}}, map[string]any{})
	var rules haRulesDataSourceModel
	if diags := emptyRules.State.Get(context.Background(), &rules); diags.HasError() || len(rules.Rules) != 0 {
		t.Fatalf("empty rules must stay empty: %#v (%v)", rules.Rules, diags)
	}

	for _, testCase := range []struct {
		name       string
		dataSource datasource.DataSource
	}{
		{"resources", NewHAResourcesDataSource()},
		{"rules", NewHARulesDataSource()},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			handler := &lifecycleHandler{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handler.auth(w, r)
				writeAPIError(t, w, http.StatusForbidden, "missing Sys.Audit")
			}))
			defer server.Close()
			var schemaResp datasource.SchemaResponse
			testCase.dataSource.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
			configurable, ok := testCase.dataSource.(datasource.DataSourceWithConfigure)
			if !ok {
				t.Fatalf("%T does not implement DataSourceWithConfigure", testCase.dataSource)
			}
			configurable.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: testLifecycleClient(t, server)}, &datasource.ConfigureResponse{})
			readResp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
			testCase.dataSource.Read(context.Background(), datasource.ReadRequest{Config: testDataSourceConfig(t, schemaResp, map[string]any{})}, &readResp)
			if len(readResp.Diagnostics) == 0 {
				t.Fatal("expected API error diagnostics")
			}
		})
	}
}
