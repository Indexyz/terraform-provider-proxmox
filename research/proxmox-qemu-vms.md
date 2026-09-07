# Research: `proxmox_qemu_vms` list data source (search guests and templates by name)

## Question

Can users search the cluster for QEMU virtual machines — especially templates — by name and template flag, and feed the result into `proxmox_qemu_vm` reads and `clone.source_vmid` creates?

## Answer

Not today. The provider can only read a single guest by `node` + `vm_id` (`proxmox_qemu_vm`), or dump the raw `/cluster/resources` superset (`proxmox_cluster_resources`) and hand-filter it in HCL — which currently fails decoding on any response containing guest/storage entries, because the numeric `template`/`shared` flags hit the `*bool` decode defect described below. There is no list data source for QEMU guests. Proxmox VE has no server-side "find by name / is-template" endpoint, so any search is client-side filtering over a list endpoint — which the provider can wrap cleanly.

## Pinned PVE API contract (git.proxmox.com, pve-manager HEAD: `PVE/API2/Cluster.pm` `resources`, `PVE/API2Tools.pm` `extract_vm_stats`)

```
GET /cluster/resources[?type=<vm|storage|node|sdn>]
permissions: endpoint is `user => 'all'`; each guest entry is silently
             omitted without VM.Audit on /vms/<vmid>; `pool` is dropped
             without Pool.Audit on /pool/<pool>.
returns: array of resource records; per guest entry:
  id ("qemu/<vmid>"), vmid, node, type ('qemu'|'lxc'), status, name,
  template, tags, pool, maxcpu, cpu, maxmem, mem, maxdisk, disk,
  uptime, netin, netout, hastate, lock, ...
```

Verified serialization semantics that drive the design:

- The request `type` enum is exactly `['vm', 'storage', 'node', 'sdn']` — there are **no** `qemu`/`lxc` server-side filter values. `type=vm` is an umbrella block (`if (!$param->{type} || $param->{type} eq 'vm')`) that iterates the whole `vmlist`, which holds both qemu and lxc guests. The QEMU-only cut must happen client-side on the returned `type` field.
- `template` is **JSON number 0|1, not a JSON boolean, and it is optional**: `extract_vm_stats` sets it from RRD stats (`$entry->{template} = $d->[3] + 0`) only when an RRD sample exists. The returns schema declares `boolean, optional, default 0`, but PVE does not coerce serialization. The same entry populates `name`/`status` from RRD, so a freshly created guest can briefly miss `name`/`template` until the first `pvestatd` sample (~60s).
- Guest order is `for my $vmid (sort keys %$idlist)` — deterministic but **lexicographic on the vmid string** (`"100" < "99"`). The data source sorts numerically by `vmid` client-side so output ordering is stable and version-independent.
- `maxcpu`/`maxmem`/`maxdisk` are always numeric for guests; `tags`/`pool`/`lock` are absent when unset; `plugintype` is only set for storage entries, not guests.
- Per-node alternative `GET /nodes/{node}/qemu` (index) was rejected: it requires a `node` (templates should be findable cluster-wide) and `vmstatus()` does not carry `pool`, so the record would be strictly less useful.

## Current provider state

- `Client.ClusterResources(ctx, resourceType)` (`internal/provider/client.go`) already wraps `GET /cluster/resources` with an optional `type` query parameter and unwraps the `{"data": ...}` envelope. The `ClusterResource` struct already maps `vmid` (`VMID *int64`), `name`, `node`, `type` (`Type string`), `plugintype`, `template` (`Template *bool`), `status`, `tags`, `pool`, `maxcpu`, `maxmem`, `maxdisk`.
- **Latent decode defect (must fix in this slice)**: `ClusterResource.Template *bool` and `ClusterResource.Shared *bool` cannot decode real PVE responses — PVE serializes both as JSON numbers (`$d->[3] + 0` for guest `template` in `extract_vm_stats`, `$scfg->{shared} || 0` for storage `shared`), so plain `*bool` unmarshal fails on the first guest/storage entry. This is undetected because `internal/provider/e2e_smoke_test.go` only queries `type = "node"`, whose entries carry neither field. Fix: switch both fields to `proxmoxOptionalBool` (proven pattern at `internal/provider/client_qemu.go:18`, consumed via `.Ptr()` by the storages data source), update the `proxmox_cluster_resources` mapping and its mock fixtures, and add decode tests feeding numeric `0`/`1`.
- `proxmox_cluster_resources` data source exposes all 30+ fields and only passes the API-side `type` filter through; it has no name/template/node filters and forces users into `for` expressions to isolate templates.
- `proxmox_qemu_vm` data source takes `node` + `vm_id` only; the resource's `clone` block takes `source_vmid`. The natural search workflow today is hand-rolled HCL over `proxmox_cluster_resources`.
- Naming pairs (`storage/storages`, `user/users`, `role/roles`, `pool/pools`, `group/groups`) establish the singular/plural convention: `proxmox_qemu_vm` / **`proxmox_qemu_vms`**.

## Recommended design

### 1. Data source: `proxmox_qemu_vms` (new file `data_source_qemu_vms.go`)

```
data "proxmox_qemu_vms" "ubuntu_template" {
  template = true
  name     = "ubuntu-24.04-cloudinit"
}

resource "proxmox_qemu_vm" "app" {
  node  = one(data.proxmox_qemu_vms.ubuntu_template.vms[*].node)
  clone = {
    source_vmid = one(data.proxmox_qemu_vms.ubuntu_template.vms[*].vm_id)
    full        = true
  }
  # ...
}
```

### 2. Schema

Optional filters (all client-side; empty/omitted = no filter):

| Attribute | Type | Semantics |
| --- | --- | --- |
| `template` | bool | `true` → only templates; `false` → only non-templates (a missing `template` field counts as non-template, per the API's `default => 0`); unset → all |
| `name` | string | Exact, case-sensitive match against the guest name |
| `node` | string | Exact match against the hosting node |

Computed output — one compact `vms` list, sorted by `vm_id` ascending:

| Attribute | Type | Source |
| --- | --- | --- |
| `vm_id` | int64 | `vmid` — feeds `clone.source_vmid` / `proxmox_qemu_vm.vm_id` |
| `name` | string | `name` |
| `node` | string | `node` |
| `template` | bool | `template` |
| `status` | string | `status` |
| `tags` | string | `tags` (PVE semicolon-separated string, as-is) |
| `pool` | string | `pool` |
| `max_cpu` | float64 | `maxcpu` |
| `max_memory` | int64 | `maxmem` |
| `max_disk` | int64 | `maxdisk` |

`id` is a computed string marker (`"qemu_vms"`), matching `proxmox_cluster_resources` style. Output `template` preserves a null when the API omits the field (see read logic step 5); the `template` filter treats that absent value as a non-template per the API's `default => 0`. High-frequency counters (`cpu` utilization, `uptime`, `netin/out`) are deliberately excluded: they churn every read and belong to the single-VM data source; the full superset remains in `proxmox_cluster_resources`.

### 3. Read logic

1. `ClusterResources(ctx, "vm")` — umbrella call, no server-side qemu/lxc filter exists.
2. Drop entries where `Type != "qemu"` (excludes LXC containers admitted by the `vm` umbrella).
3. Apply `node`, `name`, `template` filters in that order. A guest entry without a `template` field counts as a non-template **for filtering only**, matching the API's declared `default => 0`; the mapped output keeps it null.
4. Sort survivors by `vm_id` ascending (numerically; the server's vmid-string order is lexicographic).
5. Map to records via existing `helpers.go` `*OrNull` helpers (template through `proxmoxOptionalBool.Ptr()`); empty result is a valid empty list, not an error (consumers can use Terraform `one()` to assert uniqueness).

Client changes are limited to the `Template`/`Shared` struct fix above; registration appends `NewQemuVMsDataSource` to `DataSources()`.

### 4. Tests

- `data_source_qemu_vms_test.go`: metadata + schema-attribute presence tests (pattern of `data_source_qemu_vm_test.go`), plus table-driven read tests over a mock `/cluster/resources?type=vm` response covering: QEMU/LXC split, `template` filter true/false/unset, missing-`template`-field treated as non-template, numeric `0`/`1` template encoding (and a boolean fixture for forward tolerance), `name` exact match and miss, `node` filter, empty result, `vm_id` numeric sort order (including the `100` vs `99` lexicographic trap), null `pool`/`tags` handling. Plus client-level decode tests for the `Template`/`Shared` → `proxmoxOptionalBool` fix.
- Extend the shared `TestDataSourceSuccessfulReads` table with a `qemu vms` case so the data source is covered by the standard lifecycle harness.

### 5. Docs and examples

`examples/data-sources/proxmox_qemu_vms/data-source.tf` demonstrating template search feeding `clone.source_vmid`, then `make generate` to regenerate `docs/data-sources/qemu_vms.md` (generated docs drop the provider prefix).

## Known risks

- **Ambiguous matches**: name collisions across nodes return multiple records by design; the data source stays a list and consumers disambiguate via `node` or `one()`. A "must match exactly one" error variant is intentionally not built.
- **RRD latency**: `name` and `template` on guest entries come from RRD stats, so a guest created or converted to a template within the last ~60s can briefly lack both fields and be missed by name/template search. Document in the schema description.
- **Permission-dependent results**: PVE silently omits guests the caller cannot audit (VM.Audit on `/vms/<vmid>`), and drops `pool` without Pool.Audit. A missing template looks like an empty result; document this in the data source description to help users triage credentials vs. reality.
- **Runtime churn**: `status` changes between reads re-run data sources every plan anyway; high-frequency counters are excluded from the output, so plan diffs stay attributable to real changes.
- **PVE version drift**: the design relies only on `type=vm` umbrella plus fields stable across PVE 7→9. The `template` numeric encoding is verified against pve-manager HEAD; `proxmoxOptionalBool` also tolerates a future boolean switch without changes.

## Out of scope

- LXC counterpart (`proxmox_lxc_containers`) — separate follow-up if wanted.
- Regex / `tags` / `status` filters — follow bpg's `filter` block only when the exact-match matrix proves insufficient.
- Single-record search variant with hard uniqueness error.

## Effort estimate

Small: the `Template`/`Shared` → `proxmoxOptionalBool` struct fix plus cluster_resources mapping/mock updates, one data source file (~150 lines), tests, example, provider registration entry, `make generate` docs. No resource or state migration risk; the struct fix is additive to the decode layer only.
