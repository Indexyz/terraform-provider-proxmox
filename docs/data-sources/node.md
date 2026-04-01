---
page_title: "proxmox_node Data Source"
description: |-
  Reads detailed status for a Proxmox VE node.
---

# proxmox_node

Reads detailed node status from `/nodes/{node}/status`.

## Example Usage

```terraform
data "proxmox_node" "pve01" {
  node = "pve01"
}
```
