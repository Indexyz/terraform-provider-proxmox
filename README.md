# Terraform Provider Proxmox

This repository contains a Terraform provider for Proxmox VE built on the Terraform Plugin Framework.

The current baseline is designed against the bundled `pve-docs/` reference and includes:

- A real Proxmox API client with ticket auth and API token auth
- Cluster inventory data sources for `/version`, `/nodes`, `/nodes/{node}/status`, and `/cluster/resources`
- Declarative `proxmox_group` and `proxmox_pool` resources backed by `/access/groups` and `/pools`
- Inventory data sources for `proxmox_group`, `proxmox_groups`, `proxmox_pool`, `proxmox_pools`, `proxmox_node_dns`, `proxmox_node_time`, and `proxmox_cluster_metrics_servers`

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
