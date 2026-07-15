# Terraform Provider Proxmox

This repository contains a Terraform provider for Proxmox VE built on the Terraform Plugin Framework.

The current baseline is designed against the bundled `pve-docs/` reference and includes:

- A real Proxmox API client with ticket auth and API token auth
- Cluster inventory data sources for `/version`, `/nodes`, `/nodes/{node}/status`, and `/cluster/resources`
- Declarative QEMU VM and LXC container management, including clone workflows, typed device configuration, snapshots, and raw configuration escape hatches
- Storage, pool, RBAC, API token, ACL, and cluster/node/guest firewall management
- Inventory data sources for guests, storage, pools, access control objects, node settings, metrics servers, and cluster resources

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.24

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

Provider configuration also supports ticket-based authentication with `username` and `password`.

## Supported Resources

- `proxmox_acl`
- `proxmox_firewall_rule`
- `proxmox_group`
- `proxmox_guest_firewall_options`
- `proxmox_lxc_container`
- `proxmox_lxc_snapshot`
- `proxmox_node_firewall_options`
- `proxmox_pool`
- `proxmox_qemu_snapshot`
- `proxmox_qemu_vm`
- `proxmox_role`
- `proxmox_storage`
- `proxmox_user`
- `proxmox_user_token`

## Supported Data Sources

- `proxmox_cluster_metrics_servers`
- `proxmox_cluster_resources`
- `proxmox_group`
- `proxmox_groups`
- `proxmox_lxc_container`
- `proxmox_node`
- `proxmox_node_dns`
- `proxmox_node_time`
- `proxmox_nodes`
- `proxmox_pool`
- `proxmox_pools`
- `proxmox_qemu_vm`
- `proxmox_role`
- `proxmox_roles`
- `proxmox_storage`
- `proxmox_storages`
- `proxmox_user`
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

## QEMU/KVM Workflow

Use `proxmox_cluster_resources` for cluster-wide inventory, `data.proxmox_qemu_vm` for single-VM inspection, and `resource.proxmox_qemu_vm` for managed QEMU configuration including clone, common, cloud-init, network, disk, EFI, TPM, and raw escape-hatch workflows. Manage VM snapshots separately with `proxmox_qemu_snapshot`.

When extending `proxmox_qemu_vm` beyond the minimal surface, keep these boundaries intact:

- `proxmox_cluster_resources` remains the bulk inventory surface; advanced VM management belongs on `proxmox_qemu_vm`.
- `status` and `uptime` stay observed-only. Declarative power management is a separate concern and must not be inferred from runtime reads.
- Clone inputs are create-mode only. They should select the initial provisioning path without becoming a permanent source of drift after the VM exists.
- Disk, network, and cloud-init domains should use stable slot identities (`scsi0`, `net0`, `ipconfig0`) instead of list-order identity.
- Typed nested blocks should cover the common cases, while raw escape hatches remain available only for uncovered long-tail Proxmox keys.
- Typed and raw configuration must not control the same Proxmox key or slot in the same plan; validation should fail fast before apply.
