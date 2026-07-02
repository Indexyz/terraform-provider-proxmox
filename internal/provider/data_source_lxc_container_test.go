// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

func TestLXCContainerDataSourceMetadata(t *testing.T) {
	t.Parallel()

	ds := NewLXCContainerDataSource()
	var resp datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "proxmox"}, &resp)

	if resp.TypeName != "proxmox_lxc_container" {
		t.Fatalf("unexpected data source name: %q", resp.TypeName)
	}
}

func TestLXCContainerDataSourceSchemaAttributes(t *testing.T) {
	t.Parallel()

	attrs := lxcContainerDataSourceAttributes()
	for _, key := range []string{"node", "vm_id", "ostemplate", "rootfs", "network", "mount_point", "raw", "status", "uptime"} {
		if _, ok := attrs[key]; !ok {
			t.Fatalf("expected data source attribute %q", key)
		}
	}
}
