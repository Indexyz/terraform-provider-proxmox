---
page_title: "proxmox Provider"
description: |-
  Terraform provider for Proxmox VE.
---

# proxmox Provider

## Example Usage

```terraform
provider "proxmox" {
  endpoint         = "https://pve.example.com:8006"
  api_token_id     = "terraform@pve!provider"
  api_token_secret = var.proxmox_api_token_secret
}
```

## Schema

### Optional

- `api_token_id` (String) Full Proxmox API token identifier in the form `user@realm!tokenid`. Can also be set with `PROXMOX_VE_API_TOKEN_ID`.
- `api_token_secret` (String, Sensitive) Proxmox API token secret. Can also be set with `PROXMOX_VE_API_TOKEN_SECRET`.
- `endpoint` (String) Proxmox VE API endpoint. Accepts a base URL such as `https://pve.example.com:8006` and auto-appends `/api2/json` if needed. Can also be set with `PROXMOX_VE_ENDPOINT`.
- `insecure` (Boolean) Disable TLS certificate verification for the Proxmox endpoint. Can also be set with `PROXMOX_VE_INSECURE`.
- `otp` (String, Sensitive) One-time password used with ticket-based authentication. Can also be set with `PROXMOX_VE_OTP`.
- `password` (String, Sensitive) Password for ticket-based authentication. Can also be set with `PROXMOX_VE_PASSWORD`.
- `timeout_seconds` (Number) HTTP request timeout in seconds. Defaults to `30`. Can also be set with `PROXMOX_VE_TIMEOUT`.
- `user_agent` (String) Optional custom HTTP user agent string sent to the Proxmox API.
- `username` (String) Proxmox user ID for ticket-based authentication, for example `root@pam`. Can also be set with `PROXMOX_VE_USERNAME`.


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
