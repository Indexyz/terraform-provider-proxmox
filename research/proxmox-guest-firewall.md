# Research: guest firewall aliases and IP sets

Pinned: `pve-firewall` 5b69433 (`src/PVE/API2/Firewall/{VM,Host,Aliases,IPSet}.pm`,
`src/PVE/Firewall.pm` vm_option_properties) and the cluster object API already
implemented in `client_cluster_firewall_objects.go`.

## Endpoints

`PVE::API2::Firewall::Aliases` and `IPSet` are registered as subclasses both by
`Cluster.pm` (`/cluster/firewall/...`) and by `VM.pm`/`Host.pm`. For a guest the
paths are

- `GET/POST /nodes/{node}/qemu/{vmid}/firewall/aliases`
- `GET/PUT/DELETE /nodes/{node}/qemu/{vmid}/firewall/aliases/{name}`
- `GET/POST /nodes/{node}/qemu/{vmid}/firewall/ipset`
- `GET/DELETE /nodes/{node}/qemu/{vmid}/firewall/ipset/{name}` (POST is the
  update with `rename`, exactly like cluster scope)
- `GET/POST /nodes/{node}/qemu/{vmid}/firewall/ipset/{name}` (entries)
- `GET/PUT/DELETE .../ipset/{name}/{cidr}`

`lxc` replaces `qemu` for containers. Semantics, digests, `nomatch`, `rename`
and the `pve-fw-alias` / CIDR formats are identical to the cluster scope, so the
provider mirrors those contracts rather than inventing new ones.

## Permissions and identity

Guest objects carry no cluster-wide visibility: every request needs
`VM.Configure` (or `VM.Audit` for reads) on `/vms/{vmid}` and the guest must
exist. Identity is `node` + guest type + `vmid` + object name, and all three
scope fields therefore require replacement.

Aliases are not expanded by PVE (the firewall resolves them at compile time);
they only need to be unique per guest. Deleting an alias or set that is still
referenced by a rule succeeds — the rule then fails compilation until fixed,
which the rule resource already surfaces at its own apply.

## Slices

1. `proxmox_guest_firewall_alias` (`name`, `cidr`, `comment`, digest CAS).
2. `proxmox_guest_firewall_ip_set` (`name`, `comment`, POST-with-rename update).
3. `proxmox_guest_firewall_ip_set_entry` (`cidr`, `comment`, `nomatch`, digest).

Collection reads decide existence (missing item is not a 404 on every path),
guest switches are catalogued by the existing node-network node name pattern.
