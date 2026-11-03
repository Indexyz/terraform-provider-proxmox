# Research: Proxmox VE 9 HA rules

## Question

`proxmox_ha_resource` (PVE 9.2, `/cluster/ha/resources`) shipped and is the only HA
capability in the provider. PVE 9 replaced legacy HA groups with typed HA *rules*
(`node-affinity`, `resource-affinity`) on `/cluster/ha/rules`. What is the exact
PVE contract for those rules, and what does a single typed `proxmox_ha_rule`
resource have to do to be safe?

## Recommendation

Implement one resource, `proxmox_ha_rule`, with a required `type` discriminator and
typed-only variant fields. Keep the PVE-9-only and ownership boundaries established
by `proxmox_ha_resource`:

- rules reference canonical `vm:<vmid>` / `ct:<vmid>` SIDs that are already HA
  configured, so a rule always depends on `proxmox_ha_resource`;
- no legacy HA group support and no generic `raw` map;
- fresh shared digest on every mutation, per-rule private managed-field deletion,
  direct `rule` import;
- no data source, no runtime action (no migrate/relocate/CRM/arm-disarm), no wait
  for placement convergence;
- `disable` is part of the schema because disabling is the only way to keep an
  intentionally inactive rule without deleting it.

Negative *node* affinity (the `affinity` property on `node-affinity`) belongs in the
first slice: it is the current API shape, and omitting it would force users to
enumerate every node instead of the nodes to avoid. It is optional on the wire
(default `positive`), so sending it only when configured keeps the resource usable
on `pve-ha-manager` 5.2.4, which predates it.

## Why this is the next feature

| Candidate | Decision |
| --- | --- |
| `proxmox_ha_rule` | Implement next. Direct continuation of the HA epic, stable identity, synchronous configuration CRUD, server-side feasibility checking, and the only way to express placement constraints in PVE 9. |
| HA data sources (rules/resources) | Defer. The resource supports import; read-only inventory adds nothing for placement. |
| Dynamic CRS cluster options | Separate epic. Not required for rule authoring. |
| `proxmox_lxc_containers` list data source | Useful but small; independent of HA. |
| Node networking / SDN | Still blocked on explicit pending/apply boundaries. |
| QEMU compound devices | Still blocked on removed config-slot deletion. |

## Authoritative PVE contract

Pinned to official `pve-ha-manager` at commit
[`c73364c`](https://git.proxmox.com/?p=pve-ha-manager.git;a=commit;h=c73364c)
(package version `5.2.5`, the first release containing negative node affinity),
plus `pve-common` at HEAD for `PVE::SectionConfig` schema generation and
`pve-configid`.

### Endpoints and permissions

| Method | Endpoint | Permission | Semantics |
| --- | --- | --- | --- |
| GET | `/cluster/ha/rules` | `Sys.Audit` on `/` | Array of full rule configs, ordered by the stored ordinal, each with shared `digest`, `order`, and optional per-property `errors`. Also accepts `type` / `resource` filters. |
| GET | `/cluster/ha/rules/{rule}` | `Sys.Audit` on `/` | Single rule with the same payload. |
| POST | `/cluster/ha/rules` | `Sys.Console` on `/` | Synchronous, `protected`, under the HA config lock; checks quorum. Returns `null`. |
| PUT | `/cluster/ha/rules/{rule}` | `Sys.Console` on `/` | Synchronous, `protected`, under the HA config lock. Returns `null`. |
| DELETE | `/cluster/ha/rules/{rule}` | `Sys.Console` on `/` | Synchronous, `protected`. Returns `null`. |

API tokens are allowed. `Sys.Console` on `/` is the exact check for every mutation;
do not document a guessed role.

Create assigns the next `order` ordinal, rejects a duplicate `rule` id
(`HA rule '<rule>' already defined`), and rejects unknown resources and nodes. Both
POST and PUT additionally refuse to run while legacy HA groups still exist
(`cannot create ha rule: ha groups have not been migrated yet`); DELETE does not
check. DELETE has **no existence check**, so deleting a missing rule succeeds.

`GET /cluster/ha/rules/{rule}` dies with `no such ha rule '<rule>'` instead of
returning 404 — the same PVE behaviour that made `proxmox_ha_resource` read through
the collection. Read must do the same here.

### Request bodies

Create (`PVE::HA::Rules->createSchema()`) is `allOf` of
`{ rule }` + a `oneOf` discriminated by `type`:

| Field | node-affinity | resource-affinity | Default | Terraform decision |
| --- | --- | --- | --- | --- |
| `rule` | required | required | none | Required, `RequiresReplace`, import identity. `pve-configid`: `^[a-zA-Z][a-zA-Z0-9_-]+$`. |
| `type` | required (`node-affinity`) | required (`resource-affinity`) | none | Required, `RequiresReplace`. Sent on POST only. |
| `resources` | required, non-empty | required, ≥2 | none | Required set of canonical SIDs; serialized sorted, comma-joined. |
| `nodes` | required, non-empty `<node>[:<priority>]` list | absent | none | Optional + Computed `map(string→int64)`, plan-time required for node-affinity. |
| `affinity` | optional, enum `positive`/`negative`, default `positive` | required, enum `positive`/`negative` | `positive` (node-affinity only) | Optional + Computed for node-affinity, required for resource-affinity. |
| `strict` | optional boolean, default `0` | absent | `0` | Optional + Computed. |
| `disable` | optional boolean, effective default `false` | same | absent | Optional + Computed, managed-field deletion. |
| `comment` | optional, max 4096 | same | absent | Optional + Computed, managed-field deletion. |
| `digest` | update only | update only | none | Internal CAS value; never public state. |
| `delete` | update only, comma list | update only | none | Internal control for removed managed fields. |

Update (`PVE::HA::Rules->updateSchema()`) keeps the `oneOf` by `type` but marks every
variant property optional and drops `type`, so:

- `type` must **not** be sent on PUT (the schema has `additionalProperties: 0` and no
  `type` property); the plugin type is taken from the stored rule.
- `rule` stays a required body property on PUT and must be sent even though it is also
  the URL segment.
- `digest` is schema-optional, but `assert_if_modified` is what makes updates
  concurrency-safe, so the provider always sends a freshly read digest.

Boolean fields are accepted as `0`/`1`. Create and update both drop a falsy `disable`
before parsing, so clearing `disable` requires putting it into `delete`; the other
optional keys are cleared the same way (with `delete_from_config` ignoring required
keys such as `resources` and `nodes`).

### Wire value formats

- `resources` — comma-joined SIDs, written sorted, returned sorted; decoded into a set.
  Canonical form is `vm:<vmid>` / `ct:<vmid>`; the API's own validator is looser
  (`^[a-z]+:\S+$`), and each SID must already be HA configured.
- `nodes` — comma-joined `node` or `node:priority` entries; priority `0` is written
  without the suffix, and a missing priority decodes to `0`. Node names match
  `^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`; higher numbers are more preferred.
- `comment` — stored escaped (`PVE::Tools::encode_text`), returned decoded.

### Feasibility model

POST/PUT run `check_feasibility` over the whole rule set with the current node list and
abort with HTTP 400 when *the touched rule* is infeasible:

```
Rule '<rule>' is invalid.
{"errors": {"<property>": "<joined messages>"}, "data": null}
```

Currently registered checks (enabled rules only):

| Check | Rule type | Rejection |
| --- | --- | --- |
| single node-affinity reference per resource | node-affinity | a resource is already used in another node-affinity rule |
| minimum resource count | resource-affinity | fewer than two resources |
| negative resources vs. nodes | resource-affinity (negative) | more resources than cluster nodes |
| positive/negative overlap | resource-affinity | a positive and a negative rule share two or more resources |
| negative node set vs. nodes | node-affinity (negative) | the rule lists every cluster node |
| negative priorities | node-affinity (negative) | at least one node has a non-zero priority |
| node-affinity × resource-affinity | both | a resource-affinity rule touches a multi-priority node-affinity rule (`strict` or not), or a positively affine resource set touches more than one node-affinity rule |

Two consequences matter for Terraform:

1. Feasibility is **not transactional across rules.** Only the touched rule is
   asserted, so a new or changed rule can make a *different* existing rule
   infeasible; that only shows up as `errors` on the next read. The scheduler then
   compiles with the infeasible rules dropped (`Drop rule '<rule>', because ...`), so
   an infeasible rule silently does nothing.
2. Because of (1), the resource should surface PVE's `errors` map as computed state
   instead of validating rule interactions locally.

The guest state `ignored` (owned by `proxmox_ha_resource`) and the rule property
`disable` are distinct: both are pure configuration, and the provider never starts,
stops, migrates, or waits for anything.

### Resource and node preconditions

- Every SID in `resources` must already be configured in `/etc/pve/ha/resources`
  (created by `proxmox_ha_resource`), otherwise POST/PUT fail with
  `cannot use unmanaged resource(s) ...`.
- Every node in `nodes` must exist in the cluster; the API skips the existence check
  only for the node the request runs on.
- Destroying an HA resource with `purge=0` leaves the SID inside referencing rules, so
  a rule that outlives its resource becomes infeasible (and stops being applied).
  Terraform dependency from `resources` to `proxmox_ha_resource.*.resource_id` gives
  the correct destroy order.

### Legacy HA group migration

On a cluster whose `/etc/pve/ha/groups.cfg` still exists, GET synthesizes a
`node-affinity` rule per group with the id `ha-group-<group>`, derived from group
members, `restricted`, and `comment` (`Generated from HA group '<group>'.`). Those
synthesized rules:

- are not in `/etc/pve/ha/rules.cfg`, so PUT/DELETE appear to work but the migration
  regenerates them from the group on the next read — writes to a group-derived id are
  not persistent;
- block POST/PUT entirely until the groups are removed.

Decision: document the `ha-group-*` ids as usable only after the group has been
migrated away, and keep no group fields on the resource.

## Safe Terraform lifecycle

### Read

1. `GET /cluster/ha/rules`, select the exact `rule`.
2. Missing id → remove from state.
3. Map fields, applying effective defaults for absent optional keys (`affinity` →
   `positive` only for node-affinity, `strict` → `false`, `disable` → `false`,
   `comment` → null), decode the `resources` set and the `nodes` map, keep `digest`
   private and `order`/`errors` public.

A malformed `resources`/`nodes` payload is an error, not a silent skip: the provider
writes both itself.

### Create

1. Read configuration; validate variant requirements.
2. POST `rule`, `type`, sorted `resources`, and only explicitly configured optional
   fields — never redundant defaults.
3. Read back through the collection and persist, together with the managed-field list.

PVE's own feasibility error must reach the diagnostic unchanged.

### Update

1. Decode config, plan, prior state, and private managed fields.
2. Fresh collection read to obtain the digest and confirm existence.
3. PUT `rule` (required by the body schema), plan `resources`, planned values only for
   fields the *configuration* contains, the fresh `digest`, and sorted `delete` keys for
   previously managed fields that are gone from configuration. Never send `type`.
4. Read back, refresh state, rewrite managed fields from configuration presence.

Managed API keys: `comment`, `disable`, `nodes`, `strict`, `affinity`. `resources` is
always configured and `type` is sent on create only. Imported resources start with an
empty managed-field list, so an import does not silently delete remote optional fields.

Presence decisions use the configuration, not the plan: an Optional + Computed field
is pre-filled by Terraform with the prior remote value, and resending that would
overwrite concurrent out-of-band changes.

### Destroy

Plain `DELETE /cluster/ha/rules/{rule}`. No pre-read (missing rules already succeed),
no digest, no `purge`, no wait. The rule simply stops constraining placement; guests
are untouched.

### Import

`rule` only, validated against `pve-configid`. `type`, `resources`, and the variant
fields are filled by the following read.

## Proposed public schema

```hcl
resource "proxmox_ha_rule" "web_on_fast_nodes" {
  rule      = "web-fast-nodes"
  type      = "node-affinity"
  resources = [proxmox_ha_resource.web.resource_id]
  nodes     = { node1 = 2, node2 = 1 }
  strict    = false
  comment   = "Keep web VMs on the fast nodes"
}

resource "proxmox_ha_rule" "db_apart" {
  rule      = "db-apart"
  type      = "resource-affinity"
  resources = [proxmox_ha_resource.db_a.resource_id, proxmox_ha_resource.db_b.resource_id]
  affinity  = "negative"
}

resource "proxmox_ha_rule" "avoid_backup_node" {
  rule      = "avoid-backup-node"
  type      = "node-affinity"
  resources = [proxmox_ha_resource.batch.resource_id]
  nodes     = { backup1 = 0 }
  affinity  = "negative"
}
```

| Attribute | Mode |
| --- | --- |
| `id` | Computed; same as `rule`. |
| `rule` | Required, `RequiresReplace`. |
| `type` | Required, `RequiresReplace`, enum `node-affinity`/`resource-affinity`. |
| `resources` | Required set of canonical SIDs; non-empty, ≥2 for resource-affinity. |
| `nodes` | Optional + Computed map; schema-Optional so the variant requirement is enforced at plan time, where node-affinity must supply it and resource-affinity must not. |
| `affinity` | Optional + Computed defaulting to `positive` for node-affinity; required for resource-affinity. |
| `strict` | Optional + Computed for node-affinity; node-affinity only. |
| `disable` | Optional + Computed, effective default `false`. |
| `comment` | Optional + Computed, max 4096. |
| `order` | Computed; server-assigned creation ordinal. |
| `feasibility_errors` | Computed map of PVE property → message. |

Explicit non-fields: `digest`, `type` on PUT, `purge`, legacy `group`, cluster node
inventory, runtime placement/CRM state, and scheduling knob overrides.

### Plan-time validation

`ValidateConfig` covers the whole matrix (the repository does not use framework
validators elsewhere); every unknown value defers the check it would need.

- `rule`: `^[a-zA-Z][a-zA-Z0-9_-]+$` (at least two characters).
- `resources`: every entry matches `^(vm|ct):[1-9][0-9]+$`; non-empty; ≥2 for
  resource-affinity.
- `nodes`: non-empty and only for node-affinity (schema-Optional + Computed, so the
  variant requirement is a plan-time check); keys match the node-name pattern; values
  ≥ 0; for `affinity = "negative"` all values must be `0` (PVE rejects priorities
  there).
- `affinity`: `positive`/`negative`, required for resource-affinity, rejected for
  nothing on node-affinity; `strict` and `nodes` rejected for resource-affinity.
- `comment` ≤ 4096.

Checks that need cluster knowledge stay server-side: node existence, node-set
complement, resource HA membership, resource-interaction feasibility, and
`negative` resources vs. node count.

## Implementation footprint

- `internal/provider/client_ha_rule.go` + test
- `internal/provider/resource_ha_rule.go` (schema, validation, lifecycle) + test
- registration in `internal/provider/provider.go` and `provider_unit_test.go`
- `examples/resources/proxmox_ha_rule/resource.tf`, generated
  `docs/resources/ha_rule.md`
- `README.md`, `docs/codebase.md`, `docs/roadmap.md`

Reuse the `Realm`/`ReplicationJob`/`HAResource` patterns (pointer request struct,
fresh digest, private managed fields, import, collection read).

## Validation plan

### Exact HTTP contract tests

- collection GET decodes a node-affinity rule (with `nodes` priorities, `strict`,
  `affinity`) and a resource-affinity rule (with `affinity`), including mixed
  Proxmox boolean/integer encodings, `order`, `errors`, and digest;
- missing rule id in a successful collection maps to `errNotFound`;
- POST sends `rule`, `type`, sorted `resources`, sorted `node` / `node:pri` list, and
  omits unconfigured optional keys; `disable` dropped-when-false behaviour is asserted
  through the API's own echo;
- PUT sends `rule`, planned `resources`, fresh `digest`, sorted `delete` keys, and no
  `type`;
- DELETE sends no body/purge and is idempotent for a missing rule;
- HTTP 400 with an `errors` map keeps status, message, and per-property detail.

### Resource tests

- schema modes, replacement identity for `rule`/`type`, set/map element types;
- validation matrix: bad rule ids, bare VMIDs, empty sets, `<2` resources for
  resource-affinity, bad node names, non-zero priorities on a negative node-affinity
  rule, variant cross-fields, unknown values deferred;
- API-to-state default mapping (absent `affinity`/`strict`/`disable`);
- managed-field deletion, import with empty ownership, and read of a disappeared rule;
- provider registration list.

### Terraform Core harness

An `httptest` mock with `plan`/`apply`/`destroy` through the real CLI is not necessary
here (no computed identity, no generation dance). A drive-it-by-framework lifecycle
test plus the exact HTTP tests cover the surface; the mock must still assert that PUT
without `rule` would fail.

### Acceptance boundary

The single-node PVE 9.2 e2e host cannot prove placement, but a future isolated case
can enroll two stopped guests as `ignored`, then create a node-affinity rule and a
*positive* resource-affinity rule (both legal on one node), assert `order`/read-back,
and delete. A *negative* resource-affinity rule needs at least two nodes (2 resources
> 1 node is rejected). The e2e host package must provide `pve-ha-manager` ≥ 5.2.5 to
exercise `affinity`; exact-form HTTP tests stay offline either way.

## Evidence and caveats

### Official sources

- [PVE API Viewer: HA rules](https://pve.proxmox.com/pve-docs/api-viewer/#/cluster/ha/rules) (documents the `oneOf` types, `affinity` on node-affinity, and `Sys.Console` checks)
- [`PVE/API2/HA/Rules.pm`](https://git.proxmox.com/?p=pve-ha-manager.git;a=blob;f=src/PVE/API2/HA/Rules.pm;hb=c73364c) — handlers, permissions, feasibility assertions, idempotent DELETE
- [`PVE/HA/Rules.pm`](https://git.proxmox.com/?p=pve-ha-manager.git;a=blob;f=src/PVE/HA/Rules.pm;hb=c73364c) — `resources`/`comment` codecs, defaults, registered cross-type checks
- [`PVE/HA/Rules/NodeAffinity.pm`](https://git.proxmox.com/?p=pve-ha-manager.git;a=blob;f=src/PVE/HA/Rules/NodeAffinity.pm;hb=c73364c) — `nodes` codec, `affinity`/`strict` defaults and checks
- [`PVE/HA/Rules/ResourceAffinity.pm`](https://git.proxmox.com/?p=pve-ha-manager.git;a=blob;f=src/PVE/HA/Rules/ResourceAffinity.pm;hb=c73364c) — affinity enum and resource-count checks
- [`PVE/HA/Config.pm`](https://git.proxmox.com/?p=pve-ha-manager.git;a=blob;f=src/PVE/HA/Config.pm;hb=c73364c) — `have_groups_been_migrated` gate
- [`PVE/HA/Groups.pm`](https://git.proxmox.com/?p=pve-ha-manager.git;a=blob;f=src/PVE/HA/Groups.pm;hb=c73364c) — `migrate_groups_to_rules`
- [`PVE/HA/Tools.pm`](https://git.proxmox.com/?p=pve-ha-manager.git;a=blob;f=src/PVE/HA/Tools.pm;hb=c73364c) — `pve-ha-node-list`, `pve-ha-resource-id-list`, `parse_node_priority`
- `pve-common` HEAD `PVE::SectionConfig` (`createSchema`/`updateSchema`, required vs. optional variant properties) and `PVE::JSONSchema` `$CONFIGID_RE`
- [`pve-ha-manager` changelog 5.2.5](https://git.proxmox.com/?p=pve-ha-manager.git;a=blob;f=debian/changelog;hb=c73364c) — negative node affinity introduction

### Comparative sources

- [BPG `resource_harule.go` at inspected revision](https://github.com/bpg/terraform-provider-proxmox/blob/main/fwprovider/cluster/ha/resource_harule.go)

BPG already models `rule`/`type`/`resources` set/`nodes` map/`strict`/`affinity`/
`disable`, which is a good sanity check for the shape. It is not a contract to copy:
it sends `type` on PUT (not in the update schema), sends no `digest` (so concurrent
`rules.cfg` edits are silently overwritten), deletes only `comment`, restricts
`affinity` to resource-affinity, and turns a missing rule on delete into a warning
instead of using the API's idempotent DELETE.

### Caveats

- `pve-ha-manager` < 5.2.5 rejects `affinity` on node-affinity; the resource only sends
  it when configured, so older hosts keep working with positive node affinity.
- `GET` also returns the internally merged/implicit rule ids only in compiled form;
  `rules.cfg` keeps the authored ids, so `order` and `errors` are the only
  server-generated values surfaced.
- Rule feasibility depends on the current node list and HA resource configuration, so
  `feasibility_errors` can appear without any Terraform change.
- Guest-level HA state (`started`/`stopped`/`disabled`/`ignored`) is owned by
  `proxmox_ha_resource`; a rule does not imply anything about power state.
- VMID reuse does not leak into rules: a rule references the SID, and a replacement VM
  with the same VMID silently inherits it.
