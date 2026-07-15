// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestBackupJobResourceSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewBackupJobResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 21; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	for _, name := range []string{"job_id", "schedule", "storage", "all", "pool", "vm_ids", "prune_backups", "next_run"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Fatalf("missing attribute %q", name)
		}
	}
}

func TestValidateBackupJobConfig(t *testing.T) {
	tests := []struct {
		name      string
		model     backupJobModel
		wantError bool
	}{
		{
			name:  "all guests",
			model: backupJobModel{JobID: types.StringValue("nightly"), All: types.BoolValue(true)},
		},
		{
			name:  "pool",
			model: backupJobModel{JobID: types.StringValue("nightly"), Pool: types.StringValue("production")},
		},
		{
			name:  "explicit guests",
			model: backupJobModel{JobID: types.StringValue("nightly"), VMIDs: types.StringValue("100,101")},
		},
		{
			name:      "missing selection",
			model:     backupJobModel{JobID: types.StringValue("nightly")},
			wantError: true,
		},
		{
			name:      "conflicting selection",
			model:     backupJobModel{JobID: types.StringValue("nightly"), All: types.BoolValue(true), Pool: types.StringValue("production")},
			wantError: true,
		},
		{
			name:      "exclude without all",
			model:     backupJobModel{JobID: types.StringValue("nightly"), Pool: types.StringValue("production"), ExcludeVMIDs: types.StringValue("100")},
			wantError: true,
		},
		{
			name:      "invalid mode",
			model:     backupJobModel{JobID: types.StringValue("nightly"), All: types.BoolValue(true), Mode: types.StringValue("live")},
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diags := validateBackupJobConfig(test.model)
			if diags.HasError() != test.wantError {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
		})
	}
}

func TestBackupJobDeleteKeys(t *testing.T) {
	prior := backupJobModel{
		Comment:      types.StringValue("nightly"),
		Enabled:      types.BoolValue(true),
		PruneBackups: types.StringValue("keep-last=3"),
		Storage:      types.StringValue("backup"),
	}
	config := backupJobModel{
		Comment:      types.StringNull(),
		Enabled:      types.BoolNull(),
		PruneBackups: types.StringNull(),
		Storage:      types.StringNull(),
	}
	want := []string{"comment", "enabled", "prune-backups", "storage"}
	got := backupJobDeleteKeys(config, prior, want)
	if len(got) != len(want) {
		t.Fatalf("unexpected delete keys: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected delete keys: got %v want %v", got, want)
		}
	}
	if got := backupJobDeleteKeys(config, prior, nil); len(got) != 0 {
		t.Fatalf("imported unmanaged fields must not be deleted: %v", got)
	}
	selectionConfig := backupJobModel{All: types.BoolValue(true)}
	selectionPrior := backupJobModel{Pool: types.StringValue("production")}
	got = backupJobDeleteKeys(selectionConfig, selectionPrior, nil)
	if len(got) != 1 || got[0] != "pool" {
		t.Fatalf("selector switch must delete prior selector: %v", got)
	}
}
