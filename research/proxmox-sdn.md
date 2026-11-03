# Research: SDN zones, VNets and subnets

## Question

SDN is the next roadmap epic: Proxmox VE stores a whole declarative network model
(zones → VNets → subnets, plus controllers, IPAMs, DNS, fabrics) that is either *pending*
in `/etc/pve/sdn/*.cfg` or *running* (`*.cfg` snapshot applied by the cluster). How does
the API separate staging from activation, and what must Terraform own so a partial apply
cannot leave a cluster half-reconfigured?

## Answer

Proxmox gates the whole SDN model behind **one global config lock** and exposes the
pending/running split explicitly:

- every mutation requires the `lock-token` returned by `POST /cluster/sdn/lock`;
- `GET` on the collection or item returns the pending-vs-running `state`
  (`new`/`changed`/`deleted`) plus a nested `pending` diff, and accepts
  `running`/`pending` filters;
- `PUT /cluster/sdn` commits the pending config into the running snapshot and reloads
  every node's network (`reloadnetworkall` UPID);
- `POST /cluster/sdn/rollback` restores the running snapshot into the pending config;
- `GET /cluster/sdn/dry-run` shows the rendered interfaces diff.

Consequences for Terraform: stage with a short-lived lock and never reload per object.
Deliver a zone resource (pending CRUD) plus one global `proxmox_sdn_apply` resource, then
VNets, then subnets, then the remaining object families. This mirrors
`proxmox_node_network_interface`/`_apply` but with the cluster-wide lock as the concurrency
primitive instead of `interfaces.new` shadows.

## Verified PVE contract

Pinned to `pve-network` `4529016` (`src/PVE/API2/Network/SDN.pm`,
`src/PVE/API2/Network/SDN/{Zones,Vnets,Subnets}.pm`,
`src/PVE/Network/SDN.pm`, `src/PVE/Network/SDN/Zones/*.pm`).

### Locking, staging, activation

| Method | Endpoint | Permission | Semantics |
| --- | --- | --- | --- |
| POST | `/cluster/sdn/lock` | `SDN.Allocate` on `/sdn` | Returns `lock-token`; `allow-pending=0` refuses while pending changes exist |
| DELETE | `/cluster/sdn/lock` | `SDN.Allocate` on `/sdn` | Releases the lock, `force` ignores a missing token |
| PUT | `/cluster/sdn` | `SDN.Allocate` on `/sdn` | `commit_config()` then per-node network reload; returns `reloadnetworkall` UPID; `release-lock` defaults to `1` |
| POST | `/cluster/sdn/rollback` | `SDN.Allocate` on `/sdn` | Rewrites the pending config from the running snapshot |
| GET | `/cluster/sdn/dry-run` | `SDN.Audit` on `/sdn` | Rendered interface diff, no side effect |

`lock_sdn_config()` dies with `invalid lock token provided!` when a token file exists and
the caller sends none or a different one. Without a token file, mutations work
token-less — so the lock is mandatory *only* while something holds it (the GUI during an
edit session).

Pending detection is a real diff against the running snapshot (`has_pending_changes()`
via `pending_config()`), not a shadow file. Nothing is applied until `PUT`.

### Object endpoints (shape shared by zones, VNets, subnets)

| Method | Endpoint | Permission |
| --- | --- | --- |
| GET | `/cluster/sdn/zones` | `SDN.Audit` or `SDN.Allocate`, filtered per entry |
| GET | `/cluster/sdn/zones/{zone}` | `SDN.Audit` per object |
| POST | `/cluster/sdn/zones` | `SDN.Allocate` on `/sdn/zones` |
| PUT | `/cluster/sdn/zones/{zone}` | `SDN.Allocate` on `/sdn/zones/{zone}` |
| DELETE | `/cluster/sdn/zones/{zone}` | `SDN.Allocate` on `/sdn/zones/{zone}` |

Reads return `digest`, `state`, and `pending`; updates take a `digest` guarded by
`assert_if_modified`. Creates reject duplicate ids and check quorum. Zone delete refuses
while a VNet references the zone (`zone <id> is used by vnet <vnet>`), so there is no
cascade — Terraform dependency ordering via `zone` references is the correct model.
Subnets are nested under a VNet (`/cluster/sdn/vnets/{vnet}/subnets/{subnet}`).

### Zone types and fields

Common: `zone` (required id), `type` (required), plus `nodes`, `mtu`, `dns`, `reversedns`,
`dnszone`, `ipam`, `dhcp`. Type-specific: `vlan` requires `bridge` and adds
`bridge-disable-mac-learning`; `qinq` adds `tag`/`vlan-protocol`; `evpn` adds
`controller`/`vrf-vxlan`/`rt-import`/`exitnodes*`/`advertise-subnets`/
`disable-arp-nd-suppression`/`mac`; `vxlan`/`faucet` add their own peers/parameters.
`simple` has no extra required field.

VNet fields: `vnet` (id), `zone`, `alias`, `tag`, `vlanaware`, `isolate-ports`, `mtu`.
Subnet fields: `subnet` (CIDR id), `type`, `gateway`, `snat`, `dnszoneprefix`,
`dhcp-range*`.

## Delivery slices

1. **`proxmox_sdn_zone`** (types `simple` and `vlan` only) + **`proxmox_sdn_apply`**.
   Common fields typed, `bridge` for `vlan`, variant validation, digest CAS, pending-state
   read, `zone` import. This unblocks everything else because VNets need a zone.
2. `proxmox_sdn_vnet` (`zone` reference, `tag`, `vlanaware`, `isolate-ports`, `alias`).
3. `proxmox_sdn_subnet` (CIDR identity, gateway/snat/dhcp ranges, `dnszoneprefix`).
4. Remaining zone types (`qinq`, `vxlan`, `evpn`, `faucet`) and object families
   (`ipam`, `dns`, `controller`, and eventually fabrics/route-maps/prefix-lists).

## Lock policy for Terraform

Per mutation: `POST /cluster/sdn/lock?allow-pending=1` → run the CRUD call with the token
→ `DELETE /cluster/sdn/lock` (best effort during cleanup). Pending changes therefore
survive the operation, and a second object can still be edited while the first is pending.

`allow-pending=1` is required, otherwise the second edit of an unapplied batch would
fail to acquire the lock. A foreign lock (GUI session) fails the mutation with
`invalid lock token provided!`; that is preserved, never retried into someone else's
session. If the provider dies between lock and release, the lock file stays and later
operations must be unlocked manually (`DELETE /cluster/sdn/lock?force=1`) — documented.

`proxmox_sdn_apply` mirrors `proxmox_node_network_apply`: `triggers` (Required,
`RequiresReplace`), no-op destroy, `pending_changes`-like computed state from the index
`state`/`pending` data, `PUT /cluster/sdn` and wait for the returned UPID. The commit step
holds the lock itself (`PUT` with `release-lock=1`).

## Read mapping

- Read the collection (missing items raise parameter exceptions, not 404) and select by
  id; `state`/`pending` come free with the entry.
- Map only running+pending-effective values into typed state; keep the `pending` diff for
  computed output (`pending` on the resource) so an unapplied edit stays visible.
- `digest` stays private and is refreshed from a collection read immediately before
  every update.

## Safety boundaries

- Applying SDN reconfigures every node's network; a wrong zone/VNet can drop the
  management path. Never apply implicitly, never rollback automatically.
- The provider does not call `/cluster/sdn/dry-run`, `/lock` beyond mutations, or
  `commit_config` outside apply.
- No fabric/controller/IPAM support in the first slices; unknown zone types are rejected
  at plan time instead of being half-managed.
- Zones with unreleased foreign locks are errors, not silent retries.

## Validation plan

- client: lock/unlock token plumbing, exact-form CRUD, digest CAS, `state`/`pending`
  decoding, apply UPID handling and nested reload failure, rollback helper if added;
- resource: schema modes, variant matrix for `simple`/`vlan`, id syntax, digest update
  order, absent-object read, idempotent delete (parameter-exception mapping), import;
- apply: trigger replacement, no-op destroy, pending computed output, lock-token errors;
- registration, examples, generated docs, README/codebase/roadmap counts.

## Non-goals

- no VNets/subnets/remaining zone types in the first slice;
- no IPAM/controller/DNS/fabric/route-map/prefix-list objects;
- no dry-run resource, no automatic rollback resource;
- no lock *holding* across plan/apply boundaries.

## Sources

- [`PVE/API2/Network/SDN.pm`](https://git.proxmox.com/?p=pve-network.git;a=blob;f=src/PVE/API2/Network/SDN.pm;hb=4529016) — lock, rollback, reload, dry-run
- [`PVE/API2/Network/SDN/Zones.pm`](https://git.proxmox.com/?p=pve-network.git;a=blob;f=src/PVE/API2/Network/SDN/Zones.pm;hb=4529016) — zone CRUD, digest, lock-token, delete refusal
- [`PVE/API2/Network/SDN/Vnets.pm`](https://git.proxmox.com/?p=pve-network.git;a=blob;f=src/PVE/API2/Network/SDN/Vnets.pm;hb=4529016), [`Subnets.pm`](https://git.proxmox.com/?p=pve-network.git;a=blob;f=src/PVE/API2/Network/SDN/Subnets.pm;hb=4529016) — nested contract
- [`PVE/Network/SDN.pm`](https://git.proxmox.com/?p=pve-network.git;a=blob;f=src/PVE/Network/SDN.pm;hb=4529016) — `lock_sdn_config`, `has_pending_changes`, `commit_config`
- [PVE SDN documentation](https://pve.proxmox.com/pve-docs/chapter-pvesdn.html)
- [PVE API Viewer: SDN](https://pve.proxmox.com/pve-docs/api-viewer/#/cluster/sdn)
