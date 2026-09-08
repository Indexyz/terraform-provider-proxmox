# Research: filters and capacity selection for the `proxmox_storages` data source

## Question

Can users filter the `proxmox_storages` list by storage type and select the largest storage by capacity, the same way `proxmox_qemu_vms` filters guests?

## Answer

Yes, with one merge step. The current data source reads `GET /storage` (datacenter storage **configuration**), which carries `type`, `content`, `nodes`, `disable`, and `shared` but **no capacity at all**. Capacity lives in `GET /cluster/resources?type=storage`, which reports per **node × storage** entries with RRD-derived `maxdisk`/`disk`. The design is therefore: keep `/storage` as the base list, enrich each record with an aggregated capacity from the cluster-resources call, and apply `type` + `largest` filters client-side — mirroring the `proxmox_qemu_vms` pattern.

## Pinned PVE API contract

Endpoints already mapped by the provider:

```
GET /storage                      (client_storage.go: Storages)
returns one record per configured storage:
  storage, type (plugin type: dir/zfs/lvmthin/nfs/cephfs/...),
  content, nodes, disable, shared, path/pool/vgname/..., ...
This is configuration only — no capacity, no per-node status.

GET /cluster/resources?type=storage   (client.go: ClusterResources)
permissions: each storage is silently omitted without Datastore.Audit
             on /storage/<storeid>; endpoint itself is user => 'all'.
returns one entry per enabled node × storage:
  id ("storage/<node>/<storeid>"), storage, node, type='storage',
  plugintype, content, shared (JSON number 0|1 — decoded by the
  proxmoxOptionalBool shipped with the qemu_vms work), status
  ('available' only when an RRD sample exists, else 'unknown'),
  maxdisk/disk (JSON numbers, present only with an RRD sample).
```

Verified semantics that drive the design (pve-manager HEAD, `PVE/API2/Cluster.pm` storage block + `PVE/API2Tools.pm` `extract_storage_stats`):

- A storage enabled on N nodes produces **N entries**. Shared storages (nfs, cephfs, …) report the same underlying size on every node; node-local storages (dir, zfs) report per-node sizes.
- `maxdisk` comes from RRD stats (`$d->[1] + 0`) and is **absent** until the storage has been indexed; `status` stays `'unknown'` in that case.
- Disabled storages: `/storage` still lists them (configuration), while `storage_check_enabled` may keep their node entries out of `/cluster/resources` — so a disabled or never-indexed storage simply has **no capacity data**.
- Entry order follows the storage config order; no reliance on it is needed.

## Current provider state

- `Client.Storages(ctx)` (`GET /storage`) feeds `proxmox_storages`; records expose `storage`, `type`, `content`, `nodes`, `disable`, `shared` — all nullable-aware via `proxmoxOptionalBool`/helpers.
- `Client.ClusterResources(ctx, resourceType)` (`GET /cluster/resources`) already accepts `type=storage`; the `ClusterResource` struct maps `storage`, `node`, `plugintype`, `shared`, `maxdisk`, `status`, and — since the qemu_vms slice — decodes numeric booleans correctly.
- No filters exist on `proxmox_storages` today; users hand-filter in HCL and cannot select by capacity at all.

## Recommended design

### 1. Schema additions to `proxmox_storages`

Optional filters (client-side, matching the `proxmox_qemu_vms` conventions):

| Attribute | Type | Semantics |
| --- | --- | --- |
| `type` | string | Exact, case-sensitive match on the storage plugin type (`zfs`, `dir`, `nfs`, …). Empty value disables the filter. |
| `largest` | bool | After other filters, keep only storages tied at the highest `capacity_bytes`. Storages without reported capacity are excluded by this filter; if no storage reports capacity the result is an empty list. Ties between distinct storages are all returned — assert uniqueness with Terraform `one()`. |

New computed field on each record:

| Attribute | Type | Source |
| --- | --- | --- |
| `capacity_bytes` | int64 | Aggregated `maxdisk`: the **maximum** across the storage's `/cluster/resources` node entries; null when the storage has no reported capacity (disabled, unauditable, or present without `maxdisk` before RRD indexing). The maximum is an aggregate hint and does not guarantee this capacity on any specific deployment node. |

The existing six fields (`storage`, `type`, `content`, `nodes`, `disable`, `shared`) and the API record order stay unchanged — surgical change, no break to current consumers.

### 2. Read logic

1. `Storages(ctx)` — base configuration list (unchanged source of truth).
2. `ClusterResources(ctx, "storage")` — capacity lookup; build `map[storageID]max(maxdisk)` taking the maximum across per-node entries (identical for shared storages, largest instance for node-local ones). A storage may be present in the response **without** `maxdisk` (entry exists, `status='unknown'`, capacity absent until RRD indexing) — treat that the same as an absent entry: null capacity.
3. Apply the `type` filter.
4. Attach `capacity_bytes` (null when absent from the map).
5. If `largest` is true: drop records with null capacity, find the maximum, keep all records tied at it.
6. Echo filters into state; empty result is a valid empty list.

Both endpoints are plain GETs; failures surface with their existing error context. Two GETs per read is the cost of capacity data that the config endpoint does not carry.

### 3. Why not switch the base to `/cluster/resources?type=storage`

It would silently drop disabled and unauditable storages, change record semantics for existing users (a breaking change), and still not carry the config-level `nodes`/`disable` detail that the current records expose. Note the distinction: a storage **enabled** on a node keeps its entry in the response even before RRD indexing (with `status='unknown'` and no `maxdisk`), while a **disabled or unauditable** storage may have no entries at all. Enrichment keeps the existing contract intact and treats both capacity-absence shapes identically.

### 4. Scope

Only `proxmox_storages` gains filters/capacity in this slice. `proxmox_storage` (single) and other list data sources (`nodes`, `qemu_vms`) already expose the fields their workflows need (`max_cpu`/`max_memory` on qemu_vms; nodes have their own capacity surface); extending them is follow-up work, not part of this ask.

## Tests

- `data_source_storages_test.go` (new): metadata + schema-attribute presence; table-driven reads over mock `/storage` + `/cluster/resources?type=storage` covering: capacity merge (max across duplicate node entries), present entry without `maxdisk` (enabled but not yet indexed → null capacity), reported zero capacity preserved, `type` filter match/miss and empty-string disable, `largest` single winner / tie returns all / excludes null-capacity storages / all-capacities-missing yields empty list / combined with `type`, filter echo into state, non-null empty list, null `capacity_bytes` for storages absent from the resources response. Numeric capacity assertions live in these dedicated tests (the shared lifecycle table asserts string attributes only).
- Update the existing `storages` case in the shared `TestDataSourceSuccessfulReads` table: the read now calls both endpoints, so the fixture must mock `/cluster/resources?type=storage` as well (keeping its storage-name assertion).
- Existing `proxmox_storages` consumers keep passing unchanged.

## Docs and examples

Extend `examples/data-sources/proxmox_storages/data-source.tf` with a type + largest example (e.g. pick the largest `zfs` storage for image placement), then regenerate `docs/data-sources/storages.md` via tfplugindocs.

## Known risks

- **Auditing**: storages without Datastore.Audit are silently omitted from `/cluster/resources`, so their `capacity_bytes` is null and they can never win a `largest` selection; documented in the schema description.
- **RRD latency**: a storage created moments ago may lack `maxdisk` until indexed; it still appears in the list with null capacity.
- **Tie semantics**: `largest` returning multiple tied records is deliberate — it keeps the data source a pure filter; users assert uniqueness with `one()`.

## Out of scope

- Free-space (`maxdisk - disk`) selection — "容量" here means total capacity; free-space selection is a separate decision.
- A `content` filter (iso/vztmpl/images/…) — useful follow-up if requested.
- Per-node storage records.

## Effort estimate

Small: one schema/Read change in `data_source_storages.go`, no client changes (`ClusterResources` is reused), new tests, example extension, docs regeneration. No state migration risk.
