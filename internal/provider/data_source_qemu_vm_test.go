// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

func TestQemuVMDataSourceMetadata(t *testing.T) {
	t.Parallel()

	ds := NewQemuVMDataSource()
	var resp datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "proxmox"}, &resp)

	if resp.TypeName != "proxmox_qemu_vm" {
		t.Fatalf("unexpected data source name: %q", resp.TypeName)
	}
}

func TestQemuVMDataSourceSchemaAttributes(t *testing.T) {
	t.Parallel()

	attrs := qemuVMDataSourceAttributes()
	for _, key := range []string{"node", "vm_id", "name", "template", "tpm_state", "status", "uptime"} {
		if _, ok := attrs[key]; !ok {
			t.Fatalf("expected data source attribute %q", key)
		}
	}
}

func TestQemuVMResourceSchemaAttributes(t *testing.T) {
	t.Parallel()

	attrs := qemuVMResourceAttributes()
	for _, key := range []string{"node", "vm_id", "name", "template", "tpm_state", "status", "uptime"} {
		if _, ok := attrs[key]; !ok {
			t.Fatalf("expected resource attribute %q", key)
		}
	}
}
