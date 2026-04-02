# Terraform Provider Proxmox

This repository contains a Terraform provider for Proxmox VE built on the Terraform Plugin Framework.

The current baseline is designed against the bundled `pve-docs/` reference and includes:

- A real Proxmox API client with ticket auth and API token auth
- Cluster inventory data sources for `/version`, `/nodes`, `/nodes/{node}/status`, and `/cluster/resources`
- Declarative `proxmox_group`, `proxmox_pool`, and minimal `proxmox_qemu_vm` resources backed by `/access/groups`, `/pools`, and `/nodes/{node}/qemu`
- Inventory data sources for `proxmox_group`, `proxmox_groups`, `proxmox_pool`, `proxmox_pools`, `proxmox_qemu_vm`, `proxmox_node_dns`, `proxmox_node_time`, and `proxmox_cluster_metrics_servers`

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

- `proxmox_group`
- `proxmox_pool`
- `proxmox_qemu_vm`

## Supported Data Sources

- `proxmox_cluster_metrics_servers`
- `proxmox_cluster_resources`
- `proxmox_group`
- `proxmox_groups`
- `proxmox_node_dns`
- `proxmox_node_time`
- `proxmox_node`
- `proxmox_nodes`
- `proxmox_pool`
- `proxmox_pools`
- `proxmox_qemu_vm`
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

Use `proxmox_cluster_resources` for cluster-wide inventory, `data.proxmox_qemu_vm` for single-VM inspection, and `resource.proxmox_qemu_vm` for managed QEMU configuration including Phase 1A/1B clone, common, cloud-init, network, disk, and raw escape-hatch support.

When extending `proxmox_qemu_vm` beyond the minimal surface, keep these boundaries intact:

- `proxmox_cluster_resources` remains the bulk inventory surface; advanced VM management belongs on `proxmox_qemu_vm`.
- `status` and `uptime` stay observed-only. Declarative power management is a separate concern and must not be inferred from runtime reads.
- Clone inputs are create-mode only. They should select the initial provisioning path without becoming a permanent source of drift after the VM exists.
- Disk, network, and cloud-init domains should use stable slot identities (`scsi0`, `net0`, `ipconfig0`) instead of list-order identity.
- Typed nested blocks should cover the common cases, while raw escape hatches remain available only for uncovered long-tail Proxmox keys.
- Typed and raw configuration must not control the same Proxmox key or slot in the same plan; validation should fail fast before apply.
