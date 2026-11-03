# Research: `proxmox_lxc_containers` list data source

## Question

`proxmox_qemu_vms` lists QEMU guests with client-side filters. LXC has no list
counterpart, so searching containers or LXC templates means querying
`/cluster/resources` by hand. What has to differ from the QEMU implementation?

## Answer

Almost nothing: the contract is the same endpoint and the same filters. LXC only
changes the type discriminator and the vocabulary.

Verified on `pve-manager` `a725338` and `pve-common` `a00b23b`:

- `GET /cluster/resources?type=vm` returns QEMU **and** LXC guests; the endpoint has no
  server-side `qemu`/`lxc` split, so the client must filter on the `type` field exactly
  like `proxmox_qemu_vms` does (`type == "lxc"` here);
- guest entries carry `vm_id`, `node`, `name`, `template`, `status`, `tags`, `pool`,
  `maxcpu`, `maxmem`, `maxdisk`; `name`/`template` come from RRD stats and can be
  missing for a guest created moments ago;
- the numeric `0|1` encoding of `template` (and `shared`) required the tolerant
  `proxmoxOptionalBool` decoding that already exists;
- permission `VM.Audit` on `/vms` filters the result; entries the identity cannot audit
  are omitted by Proxmox, not by the provider.

Decoding pitfalls (numeric booleans, missing `template` treated as the API default `0`,
missing `name`) are already solved and covered by
[`research/proxmox-qemu-vms.md`](./proxmox-qemu-vms.md); this slice reuses them.

## Proposed boundary

Mirror `proxmox_qemu_vms` one-to-one so the two lists stay predictable:

- filters `template` (bool, absent `template` counts as non-template), `name` (exact,
  case-sensitive, empty disables), `node` (exact, empty disables);
- `containers` output sorted by `vm_id` ascending (numeric, not string order);
- empty result is a legal empty list; uniqueness is the caller's `one()` concern;
- record fields identical to the QEMU list: `vm_id`, `name`, `node`, `template`,
  `status`, `tags`, `pool`, `max_cpu`, `max_memory`, `max_disk`;
- no server-side `type` parameter trick, no second request, no raw payload.

Also add the data source to the read-only PVE 9 smoke; the CI guest has no containers,
which is exactly the empty-list case.

## Non-goals

- no LXC config detail (`proxmox_lxc_container` already covers one container);
- no aggregate counts, no `largest`-style selection;
- no change to `proxmox_qemu_vms` or to the shared `ClusterResource` model.

## Validation plan

- metadata/schema attribute presence;
- filter matrix (none/`template` true/false/`name` hit+miss/`node`/combined) over an
  unsorted fixture that mixes `qemu` and `lxc` entries, proving the client-side type
  split;
- numeric vs boolean `template` decoding, missing `template`/`name` projection;
- `99` vs `100` numeric ordering;
- filter echo into state and null when unset;
- registration list, example, generated docs, README/codebase/roadmap counts.
