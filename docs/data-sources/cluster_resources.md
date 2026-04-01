---
page_title: "proxmox_cluster_resources Data Source"
description: |-
  Lists Proxmox VE cluster resources.
---

# proxmox_cluster_resources

Lists resources from `/cluster/resources`.

## Example Usage

```terraform
data "proxmox_cluster_resources" "vms" {
  type = "vm"
}
```
