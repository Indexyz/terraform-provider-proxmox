---
page_title: "proxmox_node_time Data Source"
description: |-
  Reads Proxmox VE node time settings.
---

# proxmox_node_time

Reads server time and timezone settings from `/nodes/{node}/time`.

## Example Usage

```terraform
data "proxmox_node_time" "pve01" {
  node = "pve01"
}
```

## Schema

### Required

- `node` (String) Cluster node name.

### Read-Only

- `id` (String) Static identifier for this data source.
- `local_time` (Number) Seconds since 1970-01-01 00:00:00 in local time.
- `time` (Number) Seconds since 1970-01-01 00:00:00 UTC.
- `timezone` (String) Configured node timezone.
