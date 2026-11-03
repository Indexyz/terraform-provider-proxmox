// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestACMEChallengeSchemaDataSourceFiltersAndSorts(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/acme/challenge-schema": []map[string]any{
			{"id": "zz", "type": "dns", "schema": map[string]any{"fields": map[string]any{"ZZ_TOKEN": map[string]any{"type": "string"}}}},
			{"id": "cf", "type": "dns", "schema": map[string]any{"fields": map[string]any{"CF_API_TOKEN": map[string]any{"type": "string"}, "CF_ZONE": map[string]any{"type": "string"}}}},
			{"id": "standalone", "type": "standalone", "schema": map[string]any{}},
		},
	}
	readResp := runSDNDataSource(t, NewACMEChallengeSchemaDataSource(), responses, map[string]any{})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state acmeChallengeSchemaModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if got := sdnListValues(state.Plugins); len(got) != 3 || got[0] != "cf" || got[2] != "zz" {
		t.Fatalf("plugins were not sorted: %v", got)
	}
	fields, ok := state.Fields.Elements()["cf"].(types.String)
	if !ok || fields.ValueString() != "CF_API_TOKEN,CF_ZONE" {
		t.Fatalf("unexpected fields: %#v", state.Fields.Elements())
	}

	filtered := runSDNDataSource(t, NewACMEChallengeSchemaDataSource(), responses, map[string]any{"type": "standalone"})
	var filteredState acmeChallengeSchemaModel
	if diags := filtered.State.Get(context.Background(), &filteredState); diags.HasError() || len(filteredState.Plugins.Elements()) != 1 {
		t.Fatalf("filter failed: %#v (%v)", filteredState.Plugins, diags)
	}
}
