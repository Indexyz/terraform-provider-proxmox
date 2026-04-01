---
page_title: "proxmox_cluster_metrics_servers Data Source"
description: |-
  Lists configured Proxmox VE cluster metrics servers.
---

# proxmox_cluster_metrics_servers

Lists configured cluster metrics servers from `/cluster/metrics/server`.

## Example Usage

```terraform
data "proxmox_cluster_metrics_servers" "all" {}
```

## Schema

### Read-Only

- `id` (String) Static identifier for this data source.
- `servers` (Attributes List)
  - `disable` (Boolean) Whether the metrics plugin is disabled.
  - `id` (String) Metrics server identifier.
  - `port` (Number) Server network port.
  - `server` (String) Server DNS name or IP address.
  - `type` (String) Plugin type.
