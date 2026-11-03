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
	if want := []string{"proxmox_acl", "proxmox_acme_account", "proxmox_acme_plugin", "proxmox_backup_job", "proxmox_cluster_firewall_alias", "proxmox_cluster_firewall_ip_set", "proxmox_cluster_firewall_ip_set_entry", "proxmox_cluster_firewall_options", "proxmox_cluster_firewall_security_group", "proxmox_cluster_join", "proxmox_cluster_mapping_dir", "proxmox_cluster_mapping_pci", "proxmox_cluster_mapping_usb", "proxmox_cluster_metrics_server", "proxmox_cluster_node_remove", "proxmox_cluster_options", "proxmox_firewall_rule", "proxmox_group", "proxmox_guest_firewall_alias", "proxmox_guest_firewall_ip_set", "proxmox_guest_firewall_ip_set_entry", "proxmox_guest_firewall_options", "proxmox_ha_resource", "proxmox_ha_rule", "proxmox_lxc_container", "proxmox_lxc_snapshot", "proxmox_nocloud_iso", "proxmox_node_acme_certificate", "proxmox_node_apt_repository", "proxmox_node_apt_update", "proxmox_node_backup_prune", "proxmox_node_certificate", "proxmox_node_disk_directory", "proxmox_node_disk_lvm", "proxmox_node_disk_lvmthin", "proxmox_node_disk_wipe", "proxmox_node_disk_zfs", "proxmox_node_firewall_options", "proxmox_node_gpt_init", "proxmox_node_network_apply", "proxmox_node_network_interface", "proxmox_node_network_revert", "proxmox_node_service", "proxmox_node_vzdump", "proxmox_notification_endpoint_gotify", "proxmox_notification_endpoint_sendmail", "proxmox_notification_endpoint_smtp", "proxmox_notification_endpoint_webhook", "proxmox_notification_matcher", "proxmox_pool", "proxmox_qemu_snapshot", "proxmox_qemu_vm", "proxmox_realm", "proxmox_realm_sync", "proxmox_replication_job", "proxmox_role", "proxmox_sdn_apply", "proxmox_sdn_controller", "proxmox_sdn_dns", "proxmox_sdn_fabric", "proxmox_sdn_fabric_node", "proxmox_sdn_ipam", "proxmox_sdn_prefix_list", "proxmox_sdn_rollback", "proxmox_sdn_route_map_entry", "proxmox_sdn_subnet", "proxmox_sdn_vnet", "proxmox_sdn_vnet_firewall_options", "proxmox_sdn_zone", "proxmox_storage", "proxmox_storage_file_download", "proxmox_user", "proxmox_user_tfa", "proxmox_user_tfa_unlock", "proxmox_user_token"}; !reflect.DeepEqual(resourceNames, want) {
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
		"proxmox_acl",
		"proxmox_acme_challenge_schema",
		"proxmox_backup_job_included_volumes",
		"proxmox_backup_jobs",
		"proxmox_backup_not_backed_up",
		"proxmox_cluster_config",
		"proxmox_cluster_firewall_options",
		"proxmox_cluster_mapping",
		"proxmox_cluster_metrics_servers",
		"proxmox_cluster_options",
		"proxmox_cluster_realm_sync_jobs",
		"proxmox_cluster_resources",
		"proxmox_firewall_rules",
		"proxmox_group",
		"proxmox_groups",
		"proxmox_guest_firewall_options",
		"proxmox_ha_resources",
		"proxmox_ha_rules",
		"proxmox_lxc_container",
		"proxmox_lxc_containers",
		"proxmox_lxc_snapshots",
		"proxmox_node",
		"proxmox_node_apt_repositories",
		"proxmox_node_apt_versions",
		"proxmox_node_disk_smart",
		"proxmox_node_disks",
		"proxmox_node_dns",
		"proxmox_node_firewall_options",
		"proxmox_node_hardware_pci",
		"proxmox_node_hardware_usb",
		"proxmox_node_network_interfaces",
		"proxmox_node_services",
		"proxmox_node_time",
		"proxmox_nodes",
		"proxmox_pool",
		"proxmox_pools",
		"proxmox_qemu_snapshots",
		"proxmox_qemu_vm",
		"proxmox_qemu_vms",
		"proxmox_realm",
		"proxmox_realms",
		"proxmox_replication_jobs",
		"proxmox_role",
		"proxmox_roles",
		"proxmox_sdn_controller",
		"proxmox_sdn_controllers",
		"proxmox_sdn_dns",
		"proxmox_sdn_dns_providers",
		"proxmox_sdn_fabric",
		"proxmox_sdn_fabric_node",
		"proxmox_sdn_fabrics",
		"proxmox_sdn_ipam",
		"proxmox_sdn_ipams",
		"proxmox_sdn_prefix_list",
		"proxmox_sdn_prefix_lists",
		"proxmox_sdn_route_map_entries",
		"proxmox_sdn_route_map_entry",
		"proxmox_sdn_subnet",
		"proxmox_sdn_subnets",
		"proxmox_sdn_vnet",
		"proxmox_sdn_vnet_firewall_options",
		"proxmox_sdn_vnets",
		"proxmox_sdn_zone",
		"proxmox_sdn_zones",
		"proxmox_storage",
		"proxmox_storage_contents",
		"proxmox_storages",
		"proxmox_user",
		"proxmox_user_tfa_entries",
		"proxmox_user_tokens",
		"proxmox_users",
		"proxmox_version",
	}
	if !reflect.DeepEqual(dataSourceNames, want) {
		t.Fatalf("unexpected data sources: got %v want %v", dataSourceNames, want)
	}
}
