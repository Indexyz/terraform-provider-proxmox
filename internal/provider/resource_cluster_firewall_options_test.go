// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestClusterFirewallOptionsResourceSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewClusterFirewallOptionsResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
	}
	want := []string{"ebtables", "enable", "id", "log_ratelimit", "policy_forward", "policy_in", "policy_out"}
	got := slices.Sorted(maps.Keys(resp.Schema.Attributes))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected attributes: got %v want %v", got, want)
	}
}

func TestClusterFirewallOptionsDeleteKeys(t *testing.T) {
	prior := clusterFirewallOptionsModel{
		Enable:       types.BoolValue(true),
		Ebtables:     types.BoolValue(true),
		LogRateLimit: types.StringValue("enable=1"),
		PolicyIn:     types.StringValue("DROP"),
	}
	plan := clusterFirewallOptionsModel{
		Enable:       types.BoolNull(),
		Ebtables:     types.BoolNull(),
		LogRateLimit: types.StringNull(),
		PolicyIn:     types.StringNull(),
	}
	want := []string{"enable", "ebtables", "log_ratelimit", "policy_in"}
	if got := clusterFirewallOptionsDeleteKeys(plan, prior); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected delete keys: got %v want %v", got, want)
	}
}

func TestClusterFirewallOptionsImportRejectsUnexpectedID(t *testing.T) {
	var resp resource.ImportStateResponse
	(&ClusterFirewallOptionsResource{}).ImportState(
		context.Background(),
		resource.ImportStateRequest{ID: "wrong"},
		&resp,
	)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected invalid import identifier diagnostic")
	}
}
