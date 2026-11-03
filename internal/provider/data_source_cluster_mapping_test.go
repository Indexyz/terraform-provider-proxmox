// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestClusterMappingDataSourceReads(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/mapping/dir": []map[string]any{{"id": "share", "map": []string{"node=pve-1,path=/srv/share"}, "description": "share", "digest": "d1"}},
	}
	readResp := runSDNDataSource(t, NewClusterMappingDataSource(), responses, map[string]any{"kind": "dir", "id": "share"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state clusterMappingDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.ID.ValueString() != "share" || state.Description.ValueString() != "share" || len(state.Map.Elements()) != 1 {
		t.Fatalf("unexpected projection: %#v", state)
	}
}

func TestClusterMappingDataSourceRejectsBadKind(t *testing.T) {
	if diags := validateClusterMappingDSKind(types.StringValue("xen")); !diags.HasError() {
		t.Fatal("invalid kind accepted")
	}
	if diags := validateClusterMappingDSKind(types.StringUnknown()); diags.HasError() {
		t.Fatalf("unknown kind must defer: %v", diags)
	}
}
