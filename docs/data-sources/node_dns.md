---
page_title: "proxmox_node_dns Data Source"
description: |-
  Reads Proxmox VE node DNS settings.
---

# proxmox_node_dns

Reads DNS settings for a node from `/nodes/{node}/dns`.

## Example Usage

```terraform
data "proxmox_node_dns" "pve01" {
  node = "pve01"
}
```

## Schema

### Required

- `node` (String) Cluster node name.

### Read-Only

- `id` (String) Static identifier for this data source.
- `dns1` (String) First name server IP address.
- `dns2` (String) Second name server IP address.
- `dns3` (String) Third name server IP address.
- `search` (String) Search domain for host-name lookup.
