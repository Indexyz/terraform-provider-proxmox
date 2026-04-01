---
page_title: "proxmox_pool Data Source"
description: |-
  Reads a Proxmox VE pool.
---

# proxmox_pool

Reads a Proxmox VE pool from `/pools`.

## Example Usage

```terraform
data "proxmox_pool" "platform" {
  pool_id = "platform"
}
```

## Schema

### Required

- `pool_id` (String) Pool identifier.

### Read-Only

- `id` (String) Pool identifier.
- `comment` (String) Optional pool comment.
- `vm_ids` (Set of Number) Guest VMIDs currently assigned to the pool.
- `storage_ids` (Set of String) Storage IDs currently assigned to the pool.
- `members` (Attributes List)
  - `id` (String) Proxmox object identifier.
  - `node` (String) Node that owns the member.
  - `storage_id` (String) Storage identifier when the member is a storage.
  - `type` (String) Member type.
  - `vm_id` (Number) VMID when the member is a guest.
