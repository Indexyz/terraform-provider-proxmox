---
page_title: "proxmox_group Resource"
description: |-
  Manages Proxmox VE access groups.
---

# proxmox_group

Manages Proxmox VE access groups through the `/access/groups` API.

## Example Usage

```terraform
resource "proxmox_group" "developers" {
  group_id = "developers"
  comment  = "Managed by Terraform"
}
```

## Schema

### Required

- `group_id` (String) Proxmox group identifier.

### Optional

- `comment` (String) Optional group comment.

### Read-Only

- `id` (String) Group identifier.
- `members` (List of String) Users currently assigned to the group.

## Import

Import by group identifier:

```shell
terraform import proxmox_group.developers developers
```
