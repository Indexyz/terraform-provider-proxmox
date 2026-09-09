// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestNoCloudISOResourceSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewNoCloudISOResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 8; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	for _, name := range []string{"node", "storage", "filename", "user_data", "meta_data", "network_config"} {
		if !resp.Schema.Attributes[name].IsRequired() {
			t.Fatalf("%s must be required", name)
		}
	}
	for _, name := range []string{"user_data", "meta_data", "network_config"} {
		if !resp.Schema.Attributes[name].IsSensitive() {
			t.Fatalf("%s must be sensitive because cloud-init content holds credentials", name)
		}
	}
	for _, name := range []string{"id", "volume_id"} {
		if !resp.Schema.Attributes[name].IsComputed() {
			t.Fatalf("%s must be computed", name)
		}
	}
}

func TestValidateNoCloudISOConfig(t *testing.T) {
	valid := noCloudISOModel{
		Node:          types.StringValue("pve"),
		Storage:       types.StringValue("local"),
		Filename:      types.StringValue("shaula-runner-seed-2026.02.iso"),
		UserData:      types.StringValue("#cloud-config\n"),
		MetaData:      types.StringValue("instance-id: iid-1\n"),
		NetworkConfig: types.StringValue("version: 2\n"),
	}
	if diags := validateNoCloudISOConfig(valid); diags.HasError() {
		t.Fatalf("valid config diagnostics: %v", diags)
	}

	for name, filename := range map[string]string{
		"path":        "seeds/seed.iso",
		"parent":      "../seed.iso",
		"extension":   "seed.img",
		"no name":     ".iso",
		"unsafe char": "seed $2026.iso",
		"missing iso": "seed",
	} {
		invalid := valid
		invalid.Filename = types.StringValue(filename)
		if diags := validateNoCloudISOConfig(invalid); !diags.HasError() {
			t.Fatalf("expected filename diagnostics for %s (%q)", name, filename)
		}
	}
}
