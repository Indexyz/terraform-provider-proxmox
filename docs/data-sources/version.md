---
page_title: "proxmox_version Data Source"
description: |-
  Reads Proxmox VE version details.
---

# proxmox_version

Reads cluster version details from `/version`.

## Example Usage

```terraform
data "proxmox_version" "current" {}
```
