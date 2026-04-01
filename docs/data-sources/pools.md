---
page_title: "proxmox_pools Data Source"
description: |-
  Lists Proxmox VE pools.
---

# proxmox_pools

Lists Proxmox VE pools from `/pools`.

## Example Usage

```terraform
data "proxmox_pools" "all" {}
```

## Schema

### Read-Only

- `id` (String) Static identifier for this data source.
- `pools` (Attributes List)
  - `pool_id` (String) Pool identifier.
  - `comment` (String) Optional pool comment.
  - `vm_ids` (Set of Number) Guest VMIDs currently assigned to the pool.
  - `storage_ids` (Set of String) Storage IDs currently assigned to the pool.
  - `members` (Attributes List)
    - `id` (String) Proxmox object identifier.
    - `node` (String) Node that owns the member.
    - `storage_id` (String) Storage identifier when the member is a storage.
    - `type` (String) Member type.
    - `vm_id` (Number) VMID when the member is a guest.
