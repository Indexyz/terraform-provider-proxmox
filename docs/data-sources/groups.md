---
page_title: "proxmox_groups Data Source"
description: |-
  Lists Proxmox VE access groups.
---

# proxmox_groups

Lists Proxmox VE access groups from `/access/groups`.

## Example Usage

```terraform
data "proxmox_groups" "all" {}
```

## Schema

### Read-Only

- `id` (String) Static identifier for this data source.
- `groups` (Attributes List)
  - `group_id` (String) Proxmox group identifier.
  - `comment` (String) Optional group comment.
  - `users` (List of String) Users currently assigned to the group.
