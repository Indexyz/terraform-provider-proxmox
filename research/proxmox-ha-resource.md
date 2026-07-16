# Research: Proxmox VE 9.2 HA resource

## Recommendation

Implement a standalone `proxmox_ha_resource` next, and defer affinity rules to a second change.

This is independently useful: it enrolls an existing QEMU VM or LXC container in HA, owns requested HA state and recovery policy, supports direct import, and can be destroyed without deleting the guest. It is smaller than a combined HA resource/rule delivery and establishes the stable `vm:<vmid>` / `ct:<vmid>` identities that later rules will reference.

The first slice must be PVE-9-only:

- canonical resource IDs only; do not accept the bare-VMID shortcut;
- no legacy HA `group` field or `/cluster/ha/groups` fallback;
- no `enabled` state alias; `disabled` remains a distinct supported state;
- no migrate, relocate, CRM start/stop, arm/disarm, status-convergence wait, or affinity-rule CRUD;
- destroy always uses `purge=0` and never deletes or stops the VM/CT.

## Why this is the next feature

| Candidate | Decision |
| --- | --- |
| `proxmox_ha_resource` | Implement next. High value, stable identity, synchronous configuration CRUD, manageable test surface, and useful without rules. |
| `proxmox_ha_rule` | Follow immediately afterward. It depends on already HA-managed resource IDs and adds typed variants plus feasibility interactions. |
| HA data sources | Defer. Read-only inventory is lower value than closing HA enrollment/configuration; the resource supports import. |
| Dynamic CRS cluster options | Separate epic. PVE 9.2 adds cluster-wide scheduler controls, but they are not required for basic HA enrollment. |
| Node networking / SDN | Still deferred behind explicit activation boundaries. |
| QEMU compound devices | Still deferred until removed config-slot deletion is implemented. |

Recent ecosystem evidence confirms demand: BPG issue [#2774](https://github.com/bpg/terraform-provider-proxmox/issues/2774) requested per-resource PVE 9 `failback` because CLI changes did not scale, and its implementation was tested on a three-node cluster in PR [#2865](https://github.com/bpg/terraform-provider-proxmox/pull/2865). Telmate continues to expose guest-coupled `hastate`/`hagroup` fields. A standalone resource is safer and more composable because its destroy means “leave HA,” not “destroy guest.”

## Authoritative PVE 9.2 contract

Research used the current official PVE 9.2 API Viewer and official `pve-ha-manager` source at commit [`858625dfce33ecf1a5ddcbca576fcb1ac90fbe9b`](https://git.proxmox.com/?p=pve-ha-manager.git;a=commit;h=858625dfce33ecf1a5ddcbca576fcb1ac90fbe9b), whose changelog reports package version `5.2.4`.

### Endpoints and permissions

| Method | Endpoint | Permission | Result |
| --- | --- | --- | --- |
| GET | `/cluster/ha/resources` | `Sys.Audit` on `/` | Resource configuration array with shared digest. |
| GET | `/cluster/ha/resources/{sid}` | `Sys.Audit` on `/` | Single resource configuration with shared digest. |
| POST | `/cluster/ha/resources` | `Sys.Console` on `/` | Synchronous config write; returns `null`. |
| PUT | `/cluster/ha/resources/{sid}` | `Sys.Console` on `/` | Synchronous config write; returns `null`. |
| DELETE | `/cluster/ha/resources/{sid}` | `Sys.Console` on `/` | Synchronous config removal; returns `null`. |

API tokens are allowed. The permissions are the exact current API checks; do not document a guessed `Sys.Modify` role or `/cluster/ha` ACL path.

Create calls the cluster quorum check, verifies that the referenced guest exists, verifies that `vm:` refers to QEMU and `ct:` refers to LXC, then writes `/etc/pve/ha/resources.cfg` under the HA configuration lock. It does not return a UPID.

### Configuration fields

| API field | Type | Default | Terraform decision |
| --- | --- | --- | --- |
| `sid` | `vm:<vmid>` or `ct:<vmid>`; API also accepts a bare VMID | none | Required canonical `resource_id`; replacement/import identity. Do not support the shortcut. |
| `state` | `started`, `stopped`, `enabled`, `disabled`, `ignored` | `started` | Required and explicit; allow `started`, `stopped`, `disabled`, `ignored`. Exclude alias `enabled`. |
| `comment` | string, max 4096 | absent | Optional + Computed, managed-field deletion. |
| `failback` | boolean | `true` | Optional + Computed; map absent read value to effective default `true`. |
| `auto-rebalance` | boolean | `true` | Optional + Computed `auto_rebalance`; preserve exact hyphenated wire key. |
| `max_restart` | integer, minimum 0 | `1` | Optional + Computed, validate only `>= 0`. |
| `max_relocate` | integer, minimum 0 | `1` | Optional + Computed, validate only `>= 0`. |
| `group` | transitional legacy field | none | Omit. A migrated PVE 9 cluster rejects setting it. |
| `type` | optional create discriminator/read value | inferred from SID | Omit. Canonical identity already carries it. |
| `digest` | shared config digest | none | Internal concurrency value; never user-configurable or persisted as public state. |
| `delete` | comma-separated config keys | none | Internal update control for removed managed fields. |
| `purge` | delete boolean | **`true`** | Never expose; destroy always sends `false`. |

Do not copy BPG's upper bound of 10 for restart/relocate counts: the official PVE schema has no upper bound. Validate canonical IDs against `^(vm|ct):[1-9][0-9]+$`; guest existence and type are external-system checks and remain authoritative in PVE.

### Requested-state effects

`state` is configuration, but CRM/LRM acts on it asynchronously:

- `started`: CRM tries to start and recover the resource.
- `stopped`: CRM keeps it stopped but can still relocate it after node failure.
- `disabled`: CRM keeps it stopped and does not relocate it after node failure; it is also the path out of HA error state.
- `ignored`: CRM/LRM stops tracking and touching the resource.
- `enabled`: only an alias for `started`; excluding it avoids a legacy spelling in state.

Make `state` required rather than silently accepting PVE's `started` default. This forces plans to acknowledge potential start/stop behavior. The Provider still must not call `/migrate`, `/relocate`, guest status start/stop endpoints, CRM commands, or HA arm/disarm from CRUD.

`failback=true` permits migration toward a higher-priority node when it returns. `auto_rebalance=true` permits PVE 9.2 dynamic CRS movement. These are explicit HA policies; they do not justify the Provider calling a migration endpoint or waiting for placement convergence.

## Safe Terraform lifecycle

### Read through the collection

Use `GET /cluster/ha/resources` and select the exact canonical SID instead of relying on item GET for absence detection.

Official item-read source raises `die "no such resource"` for a missing section rather than an explicit HTTP 404. PVE commonly renders that as HTTP 500. This Provider intentionally maps only real 404 responses to `errNotFound`; adding message matching for `no such resource` would be a brittle compatibility fallback. Collection lookup gives an unambiguous missing result while returning the same shared digest and fields.

Read mapping must apply effective PVE defaults because `read_resources_config()` returns raw section properties and may omit default-valued keys:

- missing `state` -> `started`;
- returned alias `enabled` -> canonical `started` so the unsupported alias never enters public state;
- missing `failback` -> `true`;
- missing `auto-rebalance` -> `true`;
- missing `max_restart` / `max_relocate` -> `1`;
- missing `comment` -> null.

Do not expose current node, CRM/LRM state, recovery error, runtime status, or placement.

### Create

1. Validate canonical `resource_id` and explicit `state`.
2. POST `sid`, `state`, and only explicitly configured optional fields.
3. Do not send redundant `type`.
4. Read the collection and select the canonical SID to capture effective state.
5. Store only explicitly configured optional API field names in private managed-field state.

PVE verifies guest existence/type. Preserve its complete API error in the Terraform diagnostic.

### Update

1. Decode config, plan, prior state, and private managed fields following the replication-job/realm pattern.
2. Fresh-read the collection immediately before mutation.
3. Use its shared `digest` on PUT.
4. Send required `state` from the plan. For each Optional + Computed field, use Terraform configuration/private ownership to decide presence: send the planned value only when configuration explicitly contains that field, never merely because the computed plan carries a remote/default value.
5. Send sorted `delete` keys only for previously managed optional fields now removed from configuration.
6. Read back and refresh private managed fields from explicit configuration presence.

Managed API keys are `comment`, `failback`, `auto-rebalance`, `max_restart`, and `max_relocate`. Imported resources start with an empty private list, so omitting a remote optional field from configuration does not silently delete it.

### Destroy

1. Collection-read first; confirmed absence is success.
2. DELETE `/cluster/ha/resources/{sid}` with form value `purge=0`.
3. Do not send digest: the current DELETE schema accepts only `sid` and `purge`.
4. Do not call QEMU/LXC delete or stop APIs and do not wait for HA runtime status.

PVE's default `purge=1` removes the SID from every referring rule and deletes a rule only when no resources remain. It can therefore silently rewrite a separately managed rule and can leave a formerly multi-resource rule infeasible. `purge=0` preserves ownership boundaries. If a rule still references the removed resource, that visible rule error is preferable to hidden cross-resource mutation; Terraform dependencies should destroy rules first.

There is an unavoidable narrow race if an external actor removes HA membership after the pre-delete collection read. Do not add error-string matching to hide it; a retry will observe absence.

## Proposed public schema

```hcl
resource "proxmox_ha_resource" "database" {
  resource_id   = "vm:120"
  state         = "started"
  comment       = "Managed by Terraform"
  failback      = true
  auto_rebalance = false
  max_restart   = 2
  max_relocate  = 2
}
```

| Attribute | Mode |
| --- | --- |
| `id` | Computed; same as `resource_id`. |
| `resource_id` | Required, `RequiresReplace`, canonical SID. |
| `state` | Required, mutable, validated enum without `enabled`. |
| `comment` | Optional + Computed. |
| `failback` | Optional + Computed. |
| `auto_rebalance` | Optional + Computed. |
| `max_restart` | Optional + Computed, minimum 0. |
| `max_relocate` | Optional + Computed, minimum 0. |

Explicit non-fields: `group`, `type`, `digest`, `purge`, runtime node/status, migrate/relocate target, guest power controls, arm/disarm controls, and raw configuration.

## Implementation footprint

Implemented files:

- `internal/provider/client_ha_resource.go`
- `internal/provider/client_ha_resource_test.go`
- `internal/provider/resource_ha_resource.go`
- `internal/provider/resource_ha_resource_test.go`
- registration updates in `internal/provider/provider.go` and `provider_unit_test.go`
- `examples/resources/proxmox_ha_resource/resource.tf`
- generated `docs/resources/ha_resource.md`
- `README.md`, `docs/codebase.md`, and `docs/roadmap.md`

The implementation reuses the existing `ReplicationJob`/`Realm` patterns for pointer requests, fresh digest, private managed fields, import, diagnostics, and generated docs without adding a generic cluster-section abstraction.

## Validation plan

### Exact HTTP tests

- collection GET decodes VM/CT resources, mixed Proxmox boolean/integer encodings, effective defaults, and shared digest;
- missing canonical SID in a successful collection response maps to `errNotFound`;
- POST sends exact canonical SID/state and exact optional field names, including `auto-rebalance`;
- PUT sends fresh digest and sorted `delete` keys;
- DELETE sends exact `purge=0` form and never calls guest or rule endpoints;
- API failures retain status and PVE error details;
- no migrate, relocate, status, arm/disarm, or guest endpoint is requested.

### Resource tests

- schema modes, replacement identity, comment length, state enum, SID syntax, and non-negative limits;
- `enabled`, bare VMID, unknown prefix, node-scoped guest ID, empty/zero/negative values rejected;
- API-to-state default mapping;
- managed-field deletion and import with empty private ownership;
- read disappearance removes state;
- import accepts canonical `vm:` and `ct:` only;
- provider exact registration list updated.

### Documentation and CI

- add example and generated reference, then verify generation idempotence;
- run Go tests/build/lint and tools tests;
- keep the existing PVE 9.2 smoke read-only.

The current single-node E2E image has no disposable guest and cannot prove recovery, failover, migration, rule feasibility, or safe guest preservation. A future isolated HA acceptance test should create a disposable stopped guest, enroll it first as `ignored`, exercise config/digest/delete, assert the guest still exists after destroy, and reserve multi-node behavior for a dedicated environment.

## Follow-up feature: typed HA rules

After `proxmox_ha_resource`, implement one `proxmox_ha_rule` resource with a required discriminator:

- common: stable `rule`, `type`, set of canonical `resources`, `disable`, `comment`;
- `node-affinity`: non-empty node-priority map plus `strict`;
- `resource-affinity`: at least two resources plus `positive`/`negative` affinity;
- fresh shared rule digest, managed-field deletion, import, and PVE feasibility errors preserved;
- no legacy HA groups and no generic raw map.

Rules require already HA-managed resources, so this order creates a natural Terraform dependency and avoids bundling two lifecycle families.

## Evidence and caveats

### Official sources

- [PVE API Viewer: HA resources](https://pve.proxmox.com/pve-docs/api-viewer/#/cluster/ha/resources)
- [PVE 9.2 roadmap: dynamic HA load balancing](https://pve.proxmox.com/wiki/Roadmap#Proxmox_VE_9.2)
- [PVE 9.0 roadmap: HA affinity rules and group migration](https://pve.proxmox.com/wiki/Roadmap#Proxmox_VE_9.0)
- [`PVE/API2/HA/Resources.pm` at researched source revision](https://git.proxmox.com/?p=pve-ha-manager.git;a=blob;f=src/PVE/API2/HA/Resources.pm;hb=858625dfce33ecf1a5ddcbca576fcb1ac90fbe9b)
- [`PVE/HA/Resources.pm` at researched source revision](https://git.proxmox.com/?p=pve-ha-manager.git;a=blob;f=src/PVE/HA/Resources.pm;hb=858625dfce33ecf1a5ddcbca576fcb1ac90fbe9b)
- [`PVE/HA/Config.pm` safe delete and digest logic](https://git.proxmox.com/?p=pve-ha-manager.git;a=blob;f=src/PVE/HA/Config.pm;hb=858625dfce33ecf1a5ddcbca576fcb1ac90fbe9b)
- [Proxmox HA Manager chapter](https://pve.proxmox.com/pve-docs/chapter-ha-manager.html)

### Comparative sources

- [BPG HA resource implementation at inspected revision](https://github.com/bpg/terraform-provider-proxmox/blob/04894bdf3c238ae25104b2fe708e08d9caabb9a6/fwprovider/cluster/ha/resource_haresource.go)
- [BPG HA client at inspected revision](https://github.com/bpg/terraform-provider-proxmox/blob/04894bdf3c238ae25104b2fe708e08d9caabb9a6/proxmox/cluster/ha/resources/resources.go)
- [BPG failback request #2774](https://github.com/bpg/terraform-provider-proxmox/issues/2774)
- [Telmate QEMU HA fields at inspected revision](https://github.com/Telmate/terraform-provider-proxmox/blob/f4d11caf3fc13468ac69807c332b63f5db95a57d/proxmox/resource_vm_qemu.go)

BPG confirms standalone-resource value and direct SID import, but its current implementation is not a contract to copy: it still exposes legacy `group`, omits PVE 9.2 per-resource `auto-rebalance`, does not send the shared digest on update, and omits `purge=0` on delete. This Provider should preserve its PVE-9-only and cross-resource ownership boundaries instead.

The current API Viewer is a moving PVE 9.2 document. Before implementation acceptance, retain exact-form tests for every field and, if the CI image package revision changes, re-check `pve-ha-manager` package/schema differences. The official API/source facts above override conflicting third-party behavior.
