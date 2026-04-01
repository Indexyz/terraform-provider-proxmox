---
page_title: "proxmox_group Data Source"
description: |-
  Reads a Proxmox VE access group.
---

# proxmox_group

Reads a Proxmox VE access group from `/access/groups/{groupid}`.

## Example Usage

```terraform
data "proxmox_group" "developers" {
  group_id = "developers"
}
```

## Schema

### Required

- `group_id` (String) Proxmox group identifier.

### Read-Only

- `id` (String) Group identifier.
- `comment` (String) Optional group comment.
- `members` (List of String) Users currently assigned to the group.
