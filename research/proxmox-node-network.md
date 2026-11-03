# Research: node networking (`/nodes/{node}/network`)

## Question

PVE node networking is the next roadmap resource family. How does Proxmox actually
stage, apply, and revert interface configuration, and what must Terraform own so a
single interface resource cannot leave the node in a half-applied or disconnected
state?

## Answer

Proxmox separates **staged configuration** from **activation**:

- interface CRUD (`POST`/`PUT`/`DELETE /nodes/{node}/network[/{iface}]`) only rewrites
  the staged `/etc/network/interfaces.new`;
- `PUT /nodes/{node}/network` is a separate, explicit *apply* that renames the staged
  file over `/etc/network/interfaces`, runs `ifreload -a`, and returns a worker UPID;
- `DELETE /nodes/{node}/network` reverts every staged change by unlinking the staged
  file.

`GET /nodes/{node}/network` reads the staged *view* (merged: applied file plus pending
shadow) and returns the pending diff as a response-level `changes` attribute.

Therefore the provider must not reload behind every interface write. Deliver two
resources:

1. `proxmox_node_network_interface` — staged CRUD only, one interface per instance;
2. `proxmox_node_network_apply` — explicit node-scoped apply that waits for the reload
   task and exposes the still-pending diff.

This keeps a `vmbr0` port change or gateway move from taking effect midway through a
multi-resource apply, and lets an operator apply the whole staged set in one
`ifreload`, exactly like the GUI's "Apply Configuration" button.

## Verified PVE contract

Pinned to `pve-manager` `a725338` (`PVE/API2/Network.pm`), `pve-common` `a00b23b`
(`PVE/INotify.pm` shadow-file staging, `PVE/JSONSchema.pm` `pve-iface`), and the
response-attribute path `pve-manager` `a725338` (`PVE/HTTPServer.pm`).

### Endpoints and permissions

| Method | Endpoint | Permission | Result |
| --- | --- | --- | --- |
| GET | `/nodes/{node}/network` | none (`user => all`), proxied to the node | Staged view array; `changes` attribute carries the pending diff |
| GET | `/nodes/{node}/network/{iface}` | `Sys.Audit` on `/nodes/{node}` | One staged interface |
| POST | `/nodes/{node}/network` | `Sys.Modify` on `/nodes/{node}` | Synchronous staged create; returns `null` |
| PUT | `/nodes/{node}/network/{iface}` | `Sys.Modify` on `/nodes/{node}` | Synchronous staged update; returns `null` |
| DELETE | `/nodes/{node}/network/{iface}` | `Sys.Modify` on `/nodes/{node}` | Synchronous staged delete; returns `null` |
| PUT | `/nodes/{node}/network` | `Sys.Modify` on `/nodes/{node}` | Worker UPID of `srvreload`, `rename(.new)` + `ifreload -a` |
| DELETE | `/nodes/{node}/network` | `Sys.Modify` on `/nodes/{node}` | `unlink` of the staged file; returns `null` |

All mutation endpoints are `protected`, run under `/etc/network/.pve-interfaces.lock`,
and are proxied to the target node. Tokens are allowed. Writes therefore need the node
to be online, and read needs only an authenticated session.

There is **no digest and no 404**: missing interfaces raise a parameter exception
(`interface does not exist`, HTTP 400), duplicates raise `interface already exists`.
Reads must go through the collection; deletes need a collection pre-check to stay
idempotent.

### Staging and apply details

`PVE::INotify::write_file('interfaces', ...)` writes the registered shadow file
`/etc/network/interfaces.new` and every read merges shadow over the applied file while
computing `changes`. `PUT /nodes/{node}/network` renames the shadow away and runs
`ifreload -a`; `DELETE /nodes/{node}/network` drops the shadow, so *all* staged changes
are reverted, not one interface.

`regenerate-frr` (boolean, schema default `0`) is extracted before the worker and, being
schema-defaulted, is always defined, so `PUT` never falls back to the
`running_config_has_frr()` branch. FRR config is therefore only regenerated when the
caller sends `regenerate-frr=1`.

### Interface fields

`json_config_properties()` spreads a fixed `confdesc` over create/update:

- `type` (required enum `bridge`, `bond`, `eth`, `alias`, `vlan`, `fabric`,
  `OVSBridge`, `OVSBond`, `OVSPort`, `OVSIntPort`, `vnet`), `iface` (required,
  `^[a-z][a-z0-9_]{1,20}([:.]\d+)?$` case-insensitive, 2–20 chars);
- common: `autostart`, `comments`, `comments6`, `mtu` (1280–65520);
- IPv4/IPv6: `address`+`netmask` / `address6`+`netmask6` (mutually `requires`), `cidr`
  / `cidr6` (converted to address+netmask and dropped), `gateway`, `gateway6`;
- bridge: `bridge_ports` (`pve-iface-list`), `bridge_vlan_aware`, `bridge_vids`;
- bond: `slaves`, `bond_mode`, `bond_xmit_hash_policy`, `bond-primary`;
- vlan: `vlan-id`, `vlan-raw-device`;
- OVS: `ovs_ports`, `ovs_tag`, `ovs_options`, `ovs_bridge`, `ovs_bonds`;
- update-only `delete` (`pve-configid-list`).

`update_network` overwrites every supplied key, applies `delete` first, then recomputes
`method`, `method6` and `families` from the resulting addresses and stores them. Those
derived keys also appear in reads; the resource does not expose them.

Duplicate default gateways and duplicate ports across interfaces are rejected. OVS
interface creation also rewrites the parent bridge's `ovs_ports`, and `OVSIntPort` /
`OVSBond` deletion rewrites it again.

The interface writer emits an empty port list as `bridge-ports none` /
`bond-slaves none`, so reads normalize that sentinel back to an empty set; without
that, clearing the last port would look like a permanent diff.

## Delivered slice

Linux interfaces (`bridge`, `bond`, `vlan`, `eth`) plus the OVS types delivered on top:
`OVSBridge`, `OVSPort`, `OVSIntPort`, `OVSBond`. OVS specifics: `ovs_ports` is
computed-only on bridges because Proxmox maintains it from the port members;
`ovs_bridge` is required for `OVSIntPort`/`OVSBond`; OVS ports reject `autostart` (the
writer forces `0`); and `bond_mode` is validated against the OVS-only mode set
(`balance-slb`, `lacp-balance-slb`, `lacp-balance-tcp`, `active-backup`) instead of the
Linux modes. SDN `vnet` / `fabric` entries are owned by the SDN epic and merely show up
in reads.

### `proxmox_node_network_interface`

| Attribute | Mode |
| --- | --- |
| `id` | Computed, `"<node>/<iface>"`. |
| `node` | Required, `RequiresReplace`. |
| `iface` | Required, `RequiresReplace`. |
| `type` | Required, `RequiresReplace`, enum `bridge`/`bond`/`vlan`/`eth`/`OVSBridge`/`OVSPort`/`OVSIntPort`/`OVSBond`. |
| `ovs_tag`, `ovs_options`, `ovs_bridge` | Optional + Computed, OVS port types only; `ovs_bridge` required for `OVSIntPort`/`OVSBond`. |
| `ovs_ports` | Computed, `OVSBridge` only. |
| `ovs_bonds` (set) | Optional + Computed, `OVSBond` only. |
| `autostart`, `mtu`, `comments`, `comments6` | Optional + Computed. |
| `address`, `netmask`, `gateway`, `address6`, `netmask6`, `gateway6` | Optional + Computed. |
| `bridge_ports` (set), `bridge_vlan_aware`, `bridge_vids` | Optional + Computed, bridge only. |
| `slaves` (set), `bond_mode`, `bond_xmit_hash_policy`, `bond_primary` | Optional + Computed, bond only. |
| `vlan_id`, `vlan_raw_device` | Optional + Computed, vlan only. |

Wire mapping: hyphenated keys stay hyphenated (`bond-primary`, `vlan-id`,
`vlan-raw-device`); `bridge_ports`/`slaves` are space-joined sorted lists; `cidr`/`cidr6`
stay unexposed in favour of the canonical address/netmask pairs the API reads back.

Lifecycle:

- Create → `POST` with `iface`, `type` and explicitly configured fields; existing
  interfaces are not adopted;
- Read → collection lookup, absent means gone from state;
- Update → `PUT` with `type`, configured fields and `delete` for previously managed
  fields that are gone; a fresh collection read happens first so a vanished interface
  fails fast instead of silently succeeding;
- Delete → collection pre-check, then `DELETE`; absence is success;
- Import → `node/iface`.

Every optional field is owned through private managed-field state, so an import does not
strip remote settings.

### `proxmox_node_network_apply`

| Attribute | Mode |
| --- | --- |
| `id` | Computed, the node name. |
| `node` | Required, `RequiresReplace`. |
| `triggers` | Required `map(string)`, `RequiresReplace`. |
| `regenerate_frr` | Optional, `RequiresReplace`. |
| `pending_changes` | Computed `list(string)` from the API `changes` attribute. |

Create → `PUT /nodes/{node}/network`, validate the UPID acknowledgement, wait for the
`srvreload` task. Update → replacement (delete is a no-op because a reload is never
undone), so a trigger change re-runs it. Delete → no HTTP call. Read → list the node's
staged view and mirror `changes`, which lets a forgotten apply stay visible without
ever reloading implicitly.

`triggers` is user-supplied (`{ interfaces = sha1(jsonencode(...)) }` or the managed
interface ids plus a revision counter) because a reload is a command, not a stored
resource; nothing else can decide when it is due.

## Safety boundaries

- `vmbr0` and the management path: staging is inert, apply is explicit. A bad apply can
  still drop the management connection, and `ifreload -a` is not transactional — the
  node may need console access. Documented, not defended against.
- No quorum or node-online pre-check: the API itself proxies and fails.
- No automatic revert resource in the first slice; `DELETE /nodes/{node}/network` is the
  manual escape hatch.
- No digest; concurrent writers are last-write-wins under the file lock. Two interfaces
  of the same node in one apply are fine, two operators are not.
- Physical NICs are only managed after import; renaming/altnames stay out of scope.

## Validation plan

- exact-form HTTP: create/update/delete paths and bodies, hyphenated keys, space-joined
  sorted port lists, `delete` keys, apply `PUT` form and UPID handling (empty/garbage
  acknowledgement, task failure, task status 404), `changes` capture;
- resource: schema modes, replacement identity, variant field matrix (bridge/bond/vlan/
  eth), address/netmask pairing, `mtu`/`vlan_id` ranges, iface/node syntax, managed-field
  deletion, default mapping, absent-interface read and idempotent delete;
- apply: no-op destroy, trigger replacement, `pending_changes` mapping, reload error
  propagation;
- registration, examples, generated docs, README/codebase counts.

Single-node e2e can exercise bridge create → apply → delete on a spare `vmbr` name
(`vmbr` beyond the existing ones) without touching the management bridge; `target`/route
behaviour still needs manual verification.

## Sources

- [`PVE/API2/Network.pm`](https://git.proxmox.com/?p=pve-manager.git;a=blob;f=PVE/API2/Network.pm;hb=a725338)
- [`PVE/INotify.pm` shadow staging](https://git.proxmox.com/?p=pve-common.git;a=blob;f=src/PVE/INotify.pm;hb=a00b23b)
- [`PVE/JSONSchema.pm` `pve-iface`](https://git.proxmox.com/?p=pve-common.git;a=blob;f=src/PVE/JSONSchema.pm;hb=a00b23b)
- [`PVE/HTTPServer.pm` `changes` result attribute](https://git.proxmox.com/?p=pve-manager.git;a=blob;f=PVE/HTTPServer.pm;hb=a725338)
- [PVE node network documentation](https://pve.proxmox.com/pve-docs/chapter-sysadmin.html#sysadmin_network_configuration)
- [PVE API Viewer: node network](https://pve.proxmox.com/pve-docs/api-viewer/#/nodes/{node}/network)
