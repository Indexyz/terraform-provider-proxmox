// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestProviderConfigFromModelUsesDefaultsAndEnvFallback(t *testing.T) {
	t.Setenv(envEndpoint, "https://pve.example.com:8006")
	t.Setenv(envAPITokenID, "terraform@pve!provider")
	t.Setenv(envAPITokenSecret, "token-secret")
	t.Setenv(envInsecure, "true")
	t.Setenv(envTimeout, "45")

	cfg, diags := providerConfigFromModel(ProxmoxProviderModel{}, "test")
	if diags.HasError() {
		t.Fatalf("providerConfigFromModel() unexpected diagnostics: %v", diags)
	}

	if cfg.Endpoint != "https://pve.example.com:8006" {
		t.Fatalf("unexpected endpoint: %q", cfg.Endpoint)
	}
	if cfg.APITokenID != "terraform@pve!provider" || cfg.APITokenSecret != "token-secret" {
		t.Fatalf("unexpected token auth config: %#v", cfg)
	}
	if cfg.Timeout != 45*time.Second {
		t.Fatalf("unexpected timeout: %s", cfg.Timeout)
	}
	if !cfg.Insecure {
		t.Fatalf("expected insecure flag to be true")
	}
	if cfg.UserAgent != "terraform-provider-proxmox/test" {
		t.Fatalf("unexpected user agent: %q", cfg.UserAgent)
	}
}

func TestProviderConfigFromModelValidatesAuthentication(t *testing.T) {
	tests := []struct {
		name  string
		model ProxmoxProviderModel
		want  string
	}{
		{
			name: "conflicting auth",
			model: ProxmoxProviderModel{
				Endpoint:       types.StringValue("https://pve.example.com:8006"),
				Username:       types.StringValue("root@pam"),
				Password:       types.StringValue("secret"),
				APITokenID:     types.StringValue("terraform@pve!provider"),
				APITokenSecret: types.StringValue("token-secret"),
			},
			want: "Conflicting Authentication Settings",
		},
		{
			name: "incomplete token auth",
			model: ProxmoxProviderModel{
				Endpoint:   types.StringValue("https://pve.example.com:8006"),
				APITokenID: types.StringValue("terraform@pve!provider"),
			},
			want: "Incomplete API Token Authentication Settings",
		},
		{
			name: "incomplete ticket auth",
			model: ProxmoxProviderModel{
				Endpoint: types.StringValue("https://pve.example.com:8006"),
				Username: types.StringValue("root@pam"),
			},
			want: "Incomplete Ticket Authentication Settings",
		},
		{
			name: "missing auth",
			model: ProxmoxProviderModel{
				Endpoint: types.StringValue("https://pve.example.com:8006"),
			},
			want: "Missing Authentication Settings",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, diags := providerConfigFromModel(test.model, "test")
			if !diags.HasError() {
				t.Fatalf("expected diagnostics for %s", test.name)
			}
			if got := diags[0].Summary(); got != test.want {
				t.Fatalf("unexpected diagnostic summary: got %q want %q", got, test.want)
			}
		})
	}
}

func TestProviderExportsResourcesAndDataSources(t *testing.T) {
	ctx := context.Background()
	provider := &ProxmoxProvider{}

	resourceNames := make([]string, 0)
	for _, factory := range provider.Resources(ctx) {
		res := factory()
		var resp resource.MetadataResponse
		res.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "proxmox"}, &resp)
		resourceNames = append(resourceNames, resp.TypeName)
	}
	sort.Strings(resourceNames)
	if want := []string{"proxmox_acl", "proxmox_backup_job", "proxmox_cluster_firewall_alias", "proxmox_cluster_firewall_ip_set", "proxmox_cluster_firewall_ip_set_entry", "proxmox_cluster_firewall_options", "proxmox_cluster_firewall_security_group", "proxmox_cluster_metrics_server", "proxmox_firewall_rule", "proxmox_group", "proxmox_guest_firewall_options", "proxmox_lxc_container", "proxmox_lxc_snapshot", "proxmox_node_firewall_options", "proxmox_pool", "proxmox_qemu_snapshot", "proxmox_qemu_vm", "proxmox_realm", "proxmox_replication_job", "proxmox_role", "proxmox_storage", "proxmox_storage_file_download", "proxmox_user", "proxmox_user_token"}; !reflect.DeepEqual(resourceNames, want) {
		t.Fatalf("unexpected resources: got %v want %v", resourceNames, want)
	}

	dataSourceNames := make([]string, 0)
	for _, factory := range provider.DataSources(ctx) {
		ds := factory()
		var resp datasource.MetadataResponse
		ds.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "proxmox"}, &resp)
		dataSourceNames = append(dataSourceNames, resp.TypeName)
	}
	sort.Strings(dataSourceNames)
	want := []string{
		"proxmox_cluster_metrics_servers",
		"proxmox_cluster_resources",
		"proxmox_group",
		"proxmox_groups",
		"proxmox_lxc_container",
		"proxmox_node",
		"proxmox_node_dns",
		"proxmox_node_time",
		"proxmox_nodes",
		"proxmox_pool",
		"proxmox_pools",
		"proxmox_qemu_vm",
		"proxmox_realm",
		"proxmox_role",
		"proxmox_roles",
		"proxmox_storage",
		"proxmox_storages",
		"proxmox_user",
		"proxmox_users",
		"proxmox_version",
	}
	if !reflect.DeepEqual(dataSourceNames, want) {
		t.Fatalf("unexpected data sources: got %v want %v", dataSourceNames, want)
	}
}
