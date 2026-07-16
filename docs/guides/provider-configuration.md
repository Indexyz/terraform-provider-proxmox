# Provider configuration and troubleshooting

This guide explains how to configure authentication, scope Proxmox permissions, and diagnose common connection and API failures. Attribute-level resource and data source documentation remains in [`docs/resources/`](../resources/) and [`docs/data-sources/`](../data-sources/).

## Endpoint and TLS

`endpoint` must be a complete URL with a scheme and host:

```hcl
provider "proxmox" {
  endpoint = "https://pve.example.com:8006"
}
```

The provider appends `/api2/json` when it is not already present. Query strings and fragments are not accepted. A reverse-proxy prefix is supported, but the resulting URL must still lead to the Proxmox API after `/api2/json` is appended.

TLS certificate verification is enabled by default. Prefer a certificate trusted by the machine running Terraform. Use `insecure = true` or `PROXMOX_VE_INSECURE=true` only for controlled development environments because it disables server certificate verification.

## Authentication

Configure exactly one of the following authentication methods.

### API token

API token authentication requires both the full token ID and its secret. The token ID has the form `user@realm!tokenid`.

```shell
export PROXMOX_VE_ENDPOINT='https://pve.example.com:8006'
export PROXMOX_VE_API_TOKEN_ID='terraform@pve!provider'
export PROXMOX_VE_API_TOKEN_SECRET='replace-me'
```

The provider sends the token on each API request and does not call the ticket login endpoint.

### Ticket authentication

Ticket authentication requires both a username and password. Set `PROXMOX_VE_OTP` as well when the account login requires a one-time password.

```shell
export PROXMOX_VE_ENDPOINT='https://pve.example.com:8006'
export PROXMOX_VE_USERNAME='terraform@pve'
export PROXMOX_VE_PASSWORD='replace-me'
# Optional:
export PROXMOX_VE_OTP='123456'
```

The provider logs in through `/access/ticket` during configuration, then uses the returned ticket and CSRF token for subsequent requests.

### Configuration rules

- API token fields and ticket fields are mutually exclusive, including when values come from a mixture of Terraform configuration and environment variables.
- Non-empty Terraform string values take precedence over their matching environment variables.
- `timeout_seconds` defaults to 30 seconds and controls individual HTTP requests. `PROXMOX_VE_TIMEOUT` is its environment-variable equivalent.
- `insecure` defaults to `false`.
- Invalid `PROXMOX_VE_INSECURE` or `PROXMOX_VE_TIMEOUT` values currently fall back to their defaults, so verify their spelling and values when a setting appears to be ignored.
- Do not commit credentials. Resource attributes marked sensitive can still be stored in Terraform state; protect the state backend and its access credentials.

See the [generated provider schema](../index.md) for the complete attribute and environment-variable list.

## Proxmox permissions

The provider does not ship a universal Proxmox role. Required privileges depend on the resources and data sources in the Terraform configuration, and many resources read current state before and after a write. The calling identity therefore needs both the relevant audit/read privileges and the privileges required by each mutation.

Use a dedicated Proxmox user and grant access only to the objects Terraform manages. When planning ACLs, account for every object involved in an operation:

- QEMU and LXC workflows can require access to the guest, its node, target storage, pool, and clone source.
- Storage downloads require access to both the node and target storage.
- Snapshot and guest firewall resources operate on an existing guest.
- Cluster firewall objects, backup jobs, replication jobs, metrics servers, and access-control resources use cluster-wide API families and need the corresponding cluster or access-management privileges.
- Data sources still require permission to read their API endpoints.

For API tokens with privilege separation enabled, configure token ACLs as well as user ACLs: Proxmox calculates effective permissions as the intersection of the user and token permissions. The [`proxmox_user_token`](../resources/user_token.md) resource defaults `privsep` to true. Its token value is returned only at creation and is retained in Terraform state.

Use the permission metadata for each endpoint in the [Proxmox VE API Viewer](https://pve.proxmox.com/pve-docs/api-viewer/) and the ACL model in the [Proxmox VE User Management documentation](https://pve.proxmox.com/pve-docs/chapter-pveum.html). Do not assume that a role sufficient for one resource family is sufficient for the entire provider.

## Troubleshooting

| Symptom | Checks |
| --- | --- |
| `Missing Proxmox Endpoint` | Set `endpoint` or `PROXMOX_VE_ENDPOINT` to a complete URL. |
| `Missing Authentication Settings` | Configure one complete authentication pair. |
| `Incomplete API Token Authentication Settings` | Set both `api_token_id` and `api_token_secret`; check matching environment variables too. |
| `Incomplete Ticket Authentication Settings` | Set both `username` and `password`. |
| `Conflicting Authentication Settings` | Remove the unused authentication pair from both Terraform configuration and the environment. A stale exported variable can cause this error. |
| `unable to login to Proxmox VE` | Check the username realm, password, OTP requirements, endpoint, and the underlying Proxmox response included in the error. |
| API status 400 | Read the Proxmox field-level errors included in the provider diagnostic and compare the configuration with the generated resource schema. |
| API status 401 | Check the full token ID, token secret, ticket credentials, OTP, and whether the credential is still enabled. The status alone does not identify one specific cause. |
| API status 403 | Check the user and token's effective ACLs on every object involved in the operation. The status alone does not identify the missing privilege. |
| Object not found or disappears from state after refresh | Verify the node, VMID, storage, job ID, and import ID, and confirm that the object still exists on the Proxmox endpoint used by this provider instance. |
| TLS or `x509` failure | Install a trusted certificate or CA chain. Use `insecure` only to confirm a certificate problem in a controlled environment. |
| Connection refused, DNS failure, or timeout | Verify routing and DNS from the Terraform runner, port 8006 or the reverse-proxy port, firewall policy, and `timeout_seconds`. A larger timeout does not fix an unreachable endpoint. |

Non-2xx responses normally include the Proxmox status and returned error body. Preserve that complete diagnostic when reporting an issue. If `TF_LOG` is enabled for additional Terraform diagnostics, review and redact the output before sharing it because logs can contain infrastructure details or credentials.
