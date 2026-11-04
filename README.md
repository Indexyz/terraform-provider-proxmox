# Terraform Provider Proxmox

This repository contains a Terraform provider for Proxmox VE built on the Terraform Plugin Framework.

The current baseline is designed against the official [Proxmox VE documentation](https://pve.proxmox.com/pve-docs/) and includes:

- A real Proxmox API client with ticket auth and API token auth
- Cluster inventory data sources for `/version`, `/nodes`, `/nodes/{node}/status`, and `/cluster/resources`
- Declarative QEMU VM and LXC container management, including clone workflows, typed device configuration (disks, network, serial, hostpci, usb, rng, virtiofs), snapshots, raw configuration escape hatches, and opt-in declarative `power` reconciliation (graceful shutdown with a server-side forced-stop fallback)
- Storage, pool, Proxmox VE 9 high-availability enrollment and placement rules, external authentication realm with directory sync and uploaded client certificates, user TFA factors, RBAC, API token, ACL, cluster/node/guest firewall management, staged node network interfaces and staged SDN zones (all zone types), VNets and subnets with explicit apply steps
- Inventory data sources for guests, storage, pools, access control objects, node settings, metrics servers, and cluster resources; `proxmox_qemu_vms` and `proxmox_lxc_containers` list guests with client-side filters

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0; Terraform >= 1.11 is required when using the write-only `proxmox_realm` secret attributes
- [Go](https://golang.org/doc/install) >= 1.26.4

## Building the Provider

```shell
go install
```

## Using the Provider

```terraform
terraform {
  required_providers {
    proxmox = {
      source = "indexyz/proxmox"
    }
  }
}

provider "proxmox" {
  endpoint         = "https://pve.example.com:8006"
  api_token_id     = "terraform@pve!provider"
  api_token_secret = var.proxmox_api_token_secret
}
```

Provider configuration also supports ticket-based authentication with `username` and `password`, plus an optional `otp`. Ticket authentication and API token authentication are mutually exclusive, and each selected method requires its complete credential pair.

See [Provider configuration and troubleshooting](docs/guides/provider-configuration.md) for endpoint normalization, environment-variable precedence, permission planning, state security, and common API errors.

## Supported Resources

- `proxmox_acl`
- `proxmox_acme_account`
- `proxmox_acme_plugin`
- `proxmox_backup_job`
- `proxmox_cluster_bulk_action`
- `proxmox_cluster_ceph_flag`
- `proxmox_cluster_firewall_alias`
- `proxmox_cluster_firewall_ip_set`
- `proxmox_cluster_firewall_ip_set_entry`
- `proxmox_cluster_firewall_options`
- `proxmox_cluster_firewall_security_group`
- `proxmox_cluster_join`
- `proxmox_cluster_mapping_dir`
- `proxmox_cluster_mapping_pci`
- `proxmox_cluster_mapping_usb`
- `proxmox_cluster_metrics_server`
- `proxmox_cluster_node_remove`
- `proxmox_cluster_options`
- `proxmox_custom_cpu_model`
- `proxmox_firewall_rule`
- `proxmox_group`
- `proxmox_guest_firewall_alias`
- `proxmox_guest_firewall_ip_set`
- `proxmox_guest_firewall_ip_set_entry`
- `proxmox_guest_firewall_options`
- `proxmox_guest_migration`
- `proxmox_guest_move_disk`
- `proxmox_guest_move_volume`
- `proxmox_guest_remote_migrate`
- `proxmox_ha_armed`
- `proxmox_ha_resource`
- `proxmox_ha_rule`
- `proxmox_lxc_container`
- `proxmox_lxc_snapshot`
- `proxmox_nocloud_iso`
- `proxmox_node_acme_certificate`
- `proxmox_node_apl_download`
- `proxmox_node_apt_repository`
- `proxmox_node_apt_update`
- `proxmox_node_backup_prune`
- `proxmox_node_ceph_fs`
- `proxmox_node_ceph_init`
- `proxmox_node_ceph_mds`
- `proxmox_node_ceph_mgr`
- `proxmox_node_ceph_monitor`
- `proxmox_node_ceph_osd`
- `proxmox_node_ceph_osd_scrub`
- `proxmox_node_ceph_pool`
- `proxmox_node_ceph_restart`
- `proxmox_node_ceph_restart_bulk`
- `proxmox_node_certificate`
- `proxmox_node_disk_directory`
- `proxmox_node_disk_lvm`
- `proxmox_node_disk_lvmthin`
- `proxmox_node_disk_wipe`
- `proxmox_node_disk_zfs`
- `proxmox_node_dns`
- `proxmox_node_execute`
- `proxmox_node_firewall_options`
- `proxmox_node_gpt_init`
- `proxmox_node_hosts`
- `proxmox_node_migrateall`
- `proxmox_node_network_apply`
- `proxmox_node_network_interface`
- `proxmox_node_network_revert`
- `proxmox_node_oci_pull`
- `proxmox_node_service`
- `proxmox_node_startall`
- `proxmox_node_status_command`
- `proxmox_node_stopall`
- `proxmox_node_suspendall`
- `proxmox_node_time`
- `proxmox_node_vzdump`
- `proxmox_node_wakeonlan`
- `proxmox_notification_endpoint_gotify`
- `proxmox_notification_endpoint_sendmail`
- `proxmox_notification_endpoint_smtp`
- `proxmox_notification_endpoint_webhook`
- `proxmox_notification_matcher`
- `proxmox_notification_target_test`
- `proxmox_pool`
- `proxmox_qemu_agent_exec`
- `proxmox_qemu_agent_file_write`
- `proxmox_qemu_agent_fsfreeze`
- `proxmox_qemu_agent_fstrim`
- `proxmox_qemu_cloudinit_pending`
- `proxmox_qemu_monitor`
- `proxmox_qemu_sendkey`
- `proxmox_qemu_snapshot`
- `proxmox_qemu_vm`
- `proxmox_realm`
- `proxmox_realm_sync`
- `proxmox_replication_job`
- `proxmox_role`
- `proxmox_sdn_apply`
- `proxmox_sdn_controller`
- `proxmox_sdn_dns`
- `proxmox_sdn_fabric`
- `proxmox_sdn_fabric_node`
- `proxmox_sdn_ipam`
- `proxmox_sdn_prefix_list`
- `proxmox_sdn_rollback`
- `proxmox_sdn_route_map_entry`
- `proxmox_sdn_subnet`
- `proxmox_sdn_vnet`
- `proxmox_sdn_vnet_firewall_options`
- `proxmox_sdn_zone`
- `proxmox_storage`
- `proxmox_storage_file_download`
- `proxmox_storage_volume`
- `proxmox_storage_volume_transfer`
- `proxmox_user`
- `proxmox_user_tfa`
- `proxmox_user_tfa_unlock`
- `proxmox_user_token`

## Supported Data Sources

- `proxmox_access_permissions`
- `proxmox_access_vncticket`
- `proxmox_acl`
- `proxmox_acme_challenge_schema`
- `proxmox_apl_templates`
- `proxmox_backup_job_included_volumes`
- `proxmox_backup_jobs`
- `proxmox_backup_not_backed_up`
- `proxmox_ceph_flags`
- `proxmox_ceph_status`
- `proxmox_cluster_config`
- `proxmox_cluster_firewall_options`
- `proxmox_cluster_log`
- `proxmox_cluster_mapping`
- `proxmox_cluster_metrics_servers`
- `proxmox_cluster_options`
- `proxmox_cluster_realm_sync_jobs`
- `proxmox_cluster_resources`
- `proxmox_cluster_status`
- `proxmox_cluster_tasks`
- `proxmox_cpu_flags`
- `proxmox_custom_cpu_models`
- `proxmox_firewall_rules`
- `proxmox_group`
- `proxmox_groups`
- `proxmox_guest_firewall_options`
- `proxmox_guest_mtunnel`
- `proxmox_guest_proxy`
- `proxmox_ha_groups`
- `proxmox_ha_manager_status`
- `proxmox_ha_resources`
- `proxmox_ha_rules`
- `proxmox_ha_status`
- `proxmox_lxc_container`
- `proxmox_lxc_containers`
- `proxmox_lxc_interfaces`
- `proxmox_lxc_snapshots`
- `proxmox_metrics_export`
- `proxmox_node`
- `proxmox_node_apt_repositories`
- `proxmox_node_apt_versions`
- `proxmox_node_capabilities`
- `proxmox_node_ceph_config`
- `proxmox_node_ceph_config_raw`
- `proxmox_node_ceph_config_value`
- `proxmox_node_ceph_crush`
- `proxmox_node_ceph_filesystems`
- `proxmox_node_ceph_log`
- `proxmox_node_ceph_mds_daemons`
- `proxmox_node_ceph_mgr_daemons`
- `proxmox_node_ceph_monitors`
- `proxmox_node_ceph_osd_lv_info`
- `proxmox_node_ceph_osd_metadata`
- `proxmox_node_ceph_osd_tree`
- `proxmox_node_ceph_pools`
- `proxmox_node_ceph_rules`
- `proxmox_node_certificates_info`
- `proxmox_node_disk_smart`
- `proxmox_node_disks`
- `proxmox_node_dns`
- `proxmox_node_firewall_options`
- `proxmox_node_hardware_pci`
- `proxmox_node_hardware_usb`
- `proxmox_node_journal`
- `proxmox_node_netstat`
- `proxmox_node_network_interfaces`
- `proxmox_node_report`
- `proxmox_node_rrd_image`
- `proxmox_node_rrddata`
- `proxmox_node_sdn`
- `proxmox_node_services`
- `proxmox_node_spiceshell`
- `proxmox_node_subscription`
- `proxmox_node_syslog`
- `proxmox_node_tasks`
- `proxmox_node_termproxy`
- `proxmox_node_time`
- `proxmox_node_vncshell`
- `proxmox_nodes`
- `proxmox_notification_targets`
- `proxmox_openid_auth_url`
- `proxmox_openid_login`
- `proxmox_pool`
- `proxmox_pools`
- `proxmox_qemu_agent`
- `proxmox_qemu_agent_file`
- `proxmox_qemu_agent_fsfreeze_status`
- `proxmox_qemu_cloudinit_dump`
- `proxmox_qemu_rrddata`
- `proxmox_qemu_snapshots`
- `proxmox_qemu_vm`
- `proxmox_qemu_vms`
- `proxmox_qemu_vncproxy`
- `proxmox_query_oci_repo_tags`
- `proxmox_query_url_metadata`
- `proxmox_realm`
- `proxmox_realms`
- `proxmox_replication_jobs`
- `proxmox_role`
- `proxmox_roles`
- `proxmox_schedule_analyze`
- `proxmox_sdn_controller`
- `proxmox_sdn_controllers`
- `proxmox_sdn_dns`
- `proxmox_sdn_dns_providers`
- `proxmox_sdn_fabric`
- `proxmox_sdn_fabric_node`
- `proxmox_sdn_fabrics`
- `proxmox_sdn_ipam`
- `proxmox_sdn_ipams`
- `proxmox_sdn_prefix_list`
- `proxmox_sdn_prefix_lists`
- `proxmox_sdn_route_map_entries`
- `proxmox_sdn_route_map_entry`
- `proxmox_sdn_subnet`
- `proxmox_sdn_subnets`
- `proxmox_sdn_vnet`
- `proxmox_sdn_vnet_firewall_options`
- `proxmox_sdn_vnets`
- `proxmox_sdn_zone`
- `proxmox_sdn_zones`
- `proxmox_storage`
- `proxmox_storage_contents`
- `proxmox_storage_file_restore_download`
- `proxmox_storage_file_restore_list`
- `proxmox_storage_identity`
- `proxmox_storage_import_metadata`
- `proxmox_storage_rrd_image`
- `proxmox_storage_rrddata`
- `proxmox_storages`
- `proxmox_user`
- `proxmox_user_tfa_entries`
- `proxmox_user_tokens`
- `proxmox_users`
- `proxmox_version`

Supported environment variables:

- `PROXMOX_VE_ENDPOINT`
- `PROXMOX_VE_USERNAME`
- `PROXMOX_VE_PASSWORD`
- `PROXMOX_VE_OTP`
- `PROXMOX_VE_API_TOKEN_ID`
- `PROXMOX_VE_API_TOKEN_SECRET`
- `PROXMOX_VE_INSECURE`
- `PROXMOX_VE_TIMEOUT`

## Developing the Provider

To keep dependencies clean:

```shell
go mod tidy
```

To run tests:

```shell
go test ./...
```

The CI tooling is a separate Go module, and the real Proxmox smoke test requires QEMU/KVM and additional host setup. See [`tools/ci/README.md`](tools/ci/README.md) for local reproduction and diagnostics.

## QEMU/KVM Workflow

Use `proxmox_cluster_resources` for cluster-wide inventory, `proxmox_qemu_vms` to search guests and templates by name/node/template flag, `data.proxmox_qemu_vm` for single-VM inspection, and `resource.proxmox_qemu_vm` for managed QEMU configuration including clone, common, cloud-init, network, disk, EFI, TPM, and raw escape-hatch workflows. Manage VM snapshots separately with `proxmox_qemu_snapshot`.

For cloud-init NoCloud delivery, `proxmox_nocloud_iso` generates a `CIDATA` seed ISO - staging the cloud-init files as plaintext in a private (`0700`) temporary directory before assembling the image, with the staging directory cleaned on every normal path - and manages the uploaded storage content, while `proxmox_qemu_vm` attaches it as CD-ROM media with create-time attachment safety checks plus `start_on_create`/`stop_on_destroy` lifecycle hooks. See [Provisioning a cloud-init seeded QEMU VM](docs/guides/nocloud-runner-vm.md) for the full template → seed → clone → destroy chain, seed-driven VM replacement ordering, and a real-environment acceptance checklist.

When extending `proxmox_qemu_vm` beyond the minimal surface, keep these boundaries intact:

- `proxmox_cluster_resources` remains the bulk inventory surface; advanced VM management belongs on `proxmox_qemu_vm`.
- `status` and `uptime` stay observed-only. Runtime reads never infer power management; the explicit `power` attribute is the only declarative knob, and it acts solely during apply/create/update, never on refresh.
- Clone inputs are create-mode only. They should select the initial provisioning path without becoming a permanent source of drift after the VM exists.
- Disk, network, and cloud-init domains should use stable slot identities (`scsi0`, `net0`, `ipconfig0`) instead of list-order identity.
- Typed nested blocks should cover the common cases, while raw escape hatches remain available only for uncovered long-tail Proxmox keys.
- Typed and raw configuration must not control the same Proxmox key or slot in the same plan; validation should fail fast before apply.
