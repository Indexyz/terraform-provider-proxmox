// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestQemuVMResourceValidateConfigCloneLinkedOverrides pins the Proxmox clone
// parameter rule that `clone.storage` and `clone.format` are only valid for
// full clones: an explicit `full = false` linked clone is rejected at plan
// time. An omitted `full` stays unvalidated because the effective Proxmox
// default depends on whether the source guest is a template.
func TestQemuVMResourceValidateConfigCloneLinkedOverrides(t *testing.T) {
	res := &QemuVMResource{}
	schema := testResourceSchema(t, res)

	tests := []struct {
		name          string
		clone         types.Object
		wantDetails   []string
		wantNoFailure bool
	}{
		{
			name:        "linked clone with storage override",
			clone:       mustQemuVMCloneValue(t, qemuVMCloneModel{SourceVMID: types.Int64Value(9000), Full: types.BoolValue(false), Storage: types.StringValue("local-lvm")}),
			wantDetails: []string{"clone.storage"},
		},
		{
			name:        "linked clone with format override",
			clone:       mustQemuVMCloneValue(t, qemuVMCloneModel{SourceVMID: types.Int64Value(9000), Full: types.BoolValue(false), Format: types.StringValue("qcow2")}),
			wantDetails: []string{"clone.format"},
		},
		{
			name:        "linked clone with both overrides",
			clone:       mustQemuVMCloneValue(t, qemuVMCloneModel{SourceVMID: types.Int64Value(9000), Full: types.BoolValue(false), Storage: types.StringValue("local-lvm"), Format: types.StringValue("qcow2")}),
			wantDetails: []string{"clone.storage", "clone.format"},
		},
		{
			name:          "linked clone alone is valid",
			clone:         mustQemuVMCloneValue(t, qemuVMCloneModel{SourceVMID: types.Int64Value(9000), Full: types.BoolValue(false)}),
			wantNoFailure: true,
		},
		{
			name:          "full clone with overrides is valid",
			clone:         mustQemuVMCloneValue(t, qemuVMCloneModel{SourceVMID: types.Int64Value(9000), Full: types.BoolValue(true), Storage: types.StringValue("local-lvm"), Format: types.StringValue("qcow2")}),
			wantNoFailure: true,
		},
		{
			// Omitted `full` lets Proxmox decide from the source guest, so the
			// overrides are valid for normal VMs and rejected only at apply time
			// for templates.
			name:          "omitted full with overrides is deferred",
			clone:         mustQemuVMCloneValue(t, qemuVMCloneModel{SourceVMID: types.Int64Value(9000), Storage: types.StringValue("local-lvm"), Format: types.StringValue("qcow2")}),
			wantNoFailure: true,
		},
		{
			// Unknown values can still resolve to a full clone, so the conflict
			// check is deferred instead of failing valid module configurations.
			name:          "unknown full with override is deferred",
			clone:         mustQemuVMCloneValue(t, qemuVMCloneModel{SourceVMID: types.Int64Value(9000), Full: types.BoolUnknown(), Storage: types.StringValue("local-lvm")}),
			wantNoFailure: true,
		},
		{
			name:          "linked clone with unknown storage is deferred",
			clone:         mustQemuVMCloneValue(t, qemuVMCloneModel{SourceVMID: types.Int64Value(9000), Full: types.BoolValue(false), Storage: types.StringUnknown()}),
			wantNoFailure: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := minimalQemuVMModel("pve one", 402)
			model.Clone = test.clone

			resp := resource.ValidateConfigResponse{}
			res.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, schema, model)}, &resp)

			if test.wantNoFailure {
				if resp.Diagnostics.HasError() {
					t.Fatalf("unexpected clone validation diagnostics: %v", resp.Diagnostics)
				}
				return
			}

			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected clone validation diagnostics for %v", test.wantDetails)
			}
			for _, want := range test.wantDetails {
				if !containsDiagnostic(resp.Diagnostics, want) {
					t.Fatalf("expected diagnostic mentioning %q: %v", want, resp.Diagnostics)
				}
			}
		})
	}
}

// TestLXCContainerResourceValidateConfigCloneLinkedStorage pins the same
// linked-clone storage rule for container clones.
func TestLXCContainerResourceValidateConfigCloneLinkedStorage(t *testing.T) {
	res := &LXCContainerResource{}
	schema := testResourceSchema(t, res)

	tests := []struct {
		name          string
		clone         types.Object
		wantDetail    string
		wantNoFailure bool
	}{
		{
			name:       "linked clone with storage override",
			clone:      mustLXCContainerCloneValue(t, lxcContainerCloneModel{SourceVMID: types.Int64Value(9000), Full: types.BoolValue(false), Storage: types.StringValue("local-lvm")}),
			wantDetail: "clone.storage",
		},
		{
			name:          "linked clone alone is valid",
			clone:         mustLXCContainerCloneValue(t, lxcContainerCloneModel{SourceVMID: types.Int64Value(9000), Full: types.BoolValue(false)}),
			wantNoFailure: true,
		},
		{
			name:          "full clone with storage override is valid",
			clone:         mustLXCContainerCloneValue(t, lxcContainerCloneModel{SourceVMID: types.Int64Value(9000), Full: types.BoolValue(true), Storage: types.StringValue("local-lvm")}),
			wantNoFailure: true,
		},
		{
			name:          "omitted full with storage override is deferred",
			clone:         mustLXCContainerCloneValue(t, lxcContainerCloneModel{SourceVMID: types.Int64Value(9000), Storage: types.StringValue("local-lvm")}),
			wantNoFailure: true,
		},
		{
			name:          "unknown full with storage override is deferred",
			clone:         mustLXCContainerCloneValue(t, lxcContainerCloneModel{SourceVMID: types.Int64Value(9000), Full: types.BoolUnknown(), Storage: types.StringValue("local-lvm")}),
			wantNoFailure: true,
		},
		{
			name:          "linked clone with unknown storage is deferred",
			clone:         mustLXCContainerCloneValue(t, lxcContainerCloneModel{SourceVMID: types.Int64Value(9000), Full: types.BoolValue(false), Storage: types.StringUnknown()}),
			wantNoFailure: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := minimalLXCContainerModel("pve one", 502)
			model.Clone = test.clone

			resp := resource.ValidateConfigResponse{}
			res.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, schema, model)}, &resp)

			if test.wantNoFailure {
				if resp.Diagnostics.HasError() {
					t.Fatalf("unexpected clone validation diagnostics: %v", resp.Diagnostics)
				}
				return
			}

			if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, test.wantDetail) {
				t.Fatalf("expected diagnostic mentioning %q: %v", test.wantDetail, resp.Diagnostics)
			}
		})
	}
}
