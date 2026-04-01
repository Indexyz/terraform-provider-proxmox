# Terraform Provider Proxmox

This repository contains a Terraform provider for Proxmox VE built on the Terraform Plugin Framework.

The current baseline is designed against the bundled `pve-docs/` reference and includes:

- A real Proxmox API client with ticket auth and API token auth
- Cluster inventory data sources for `/version`, `/nodes`, `/nodes/{node}/status`, and `/cluster/resources`
- A declarative `proxmox_pool` resource backed by `/pools`

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
