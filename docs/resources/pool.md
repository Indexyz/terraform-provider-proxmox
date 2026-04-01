---
page_title: "proxmox_pool Resource"
description: |-
  Manages Proxmox VE pools.
---

# proxmox_pool

Manages Proxmox VE pools through the `/pools` API.

## Example Usage

```terraform
resource "proxmox_pool" "platform" {
  pool_id     = "platform"
  comment     = "Managed by Terraform"
  vm_ids      = [101, 102]
  storage_ids = ["local-lvm"]
}
```
