# Research: next Proxmox VE 9 provider feature

## Recommendation

Implement a single read-only `proxmox_realm` data source next.

This is the smallest complete user-facing gap after the external realm resource:

- the provider already has `Client.GetRealm` and tested PVE 9 realm decoding;
- the provider exports a realm resource but no realm data source;
- the operation is read-only and can be verified against the existing single-node PVE 9.2 CI guest;
- it completes lookup and cross-module reference workflows without adding synchronization, activation, placement, or network side effects;
- it does not require new secret state semantics.

The next substantial cluster feature after this slice should be HA resources and PVE 9 affinity rules. Node networking and SDN remain behind explicit activation design gates, while new QEMU compound devices should wait until removed config-slot deletion is supported.

## Compared candidates

| Rank | Candidate | Value | Main reason for position |
| --- | --- | --- | --- |
| 1 | Single realm data source | Medium-high | Existing client and mapping boundary, read-only, safe PVE 9.2 acceptance coverage, no new lifecycle machinery. |
| 2 | HA resources and affinity rules | High | Strong Terraform fit and PVE 9 value, but requires a multi-resource design and cannot be behaviorally validated by the single-node smoke environment. |
| 3 | QEMU config deletion foundation, then `rng` | Medium | Existing typed/raw parser pattern is reusable, but `UpdateQemuVM` currently has no removed-key `delete` diff, so adding slots first would produce incomplete lifecycle behavior. |
| 4 | SDN simple zone/VNet/subnet | High | Declarative object graph, but CRUD writes pending shared configuration and activation must remain explicit. Typed zone variants make this an epic. |
| 5 | Node networking | High | Pending interface CRUD is possible, but reload/revert are collection operations and an implicit reload can disconnect the management path. |

Advanced realm sync/group fields are not part of the first-ranked slice. They should be designed separately because `/access/domains/{realm}/sync` is a command that can create, modify, disable, or remove users, groups, properties, and ACLs.

## Verified PVE 9.2 realm contract

The current official API Viewer exposes:

- `GET /access/domains` for the login-visible realm index. It is world-readable and returns only `realm`, `type`, optional `comment`, and optional `tfa`.
- `GET /access/domains/{realm}` for full realm configuration. It requires either `Realm.Allocate` or `Sys.Audit` on `/access/realm`.
- `POST /access/domains/{realm}/sync` for LDAP/AD user and group synchronization. It requires `Realm.AllocateUser` on `/access/realm/{realm}` and `User.Modify` on `/access/groups`, returns a worker UPID, and supports destructive `remove-vanished` behavior.

The existing `Realm` API model intentionally omits `password`, `client-key`, `certkey`, and `tfa`. Unknown JSON fields are therefore discarded before Terraform state mapping. This boundary should remain unchanged.

## Proposed data-source boundary

### Identity and supported realm types

- Input: required `realm` identifier.
- Computed identity: `id`, equal to `realm`.
- Computed discriminator: `type`.
- Allow read-only lookup of all server realm types, including built-in `pam` and `pve` realms.

Built-in realms remain forbidden in the resource. Allowing them in the data source is not a compatibility fallback: the data source does not claim ownership and the PVE API explicitly supports reading them. It also enables a safe acceptance assertion against the standard `pam` realm in the PVE 9.2 CI image.

### Computed public fields

Expose the non-secret fields already decoded by `Realm`:

- common: `comment`, `default`;
- LDAP/AD: `server1`, `server2`, `port`, `mode`, `verify`, `ca_path`, `base_dn`, `user_attr`, `domain`, `bind_dn`;
- OpenID Connect: `issuer_url`, `client_id`, `autocreate`, `username_claim`, `scopes`, `prompt`, `query_userinfo`, `acr_values`, `audiences`.

Use a dedicated data-source model and computed schema rather than reusing the resource schema. The resource model contains WriteOnly inputs and rotation counters that have no meaning in a data source.

### State exclusions

Do not expose:

- `bind_password` or `client_key`;
- `bind_password_version` or `client_key_version`;
- `password`, `client-key`, `certkey`, or `tfa` API values;
- digest, because the data source does not perform mutation;
- synchronization controls or results.

Do not add a raw field. A raw realm payload can carry secrets and would defeat the current typed-only security boundary.

## Implementation outline

Likely files:

- add `internal/provider/data_source_realm.go`;
- add focused data-source schema/read tests;
- register `NewRealmDataSource` in `internal/provider/provider.go`;
- update the exact data-source list in `provider_unit_test.go`;
- add `examples/data-sources/proxmox_realm/data-source.tf`;
- generate `docs/data-sources/realm.md`;
- extend the existing PVE 9 smoke test with a read of `pam` and assert `type = "pam"`;
- update `README.md`, `docs/codebase.md`, and `docs/roadmap.md` counts and coverage.

Avoid extracting a large shared realm abstraction. A small public-field mapper is justified only if it directly removes duplicated API-to-state assignments between the resource and data source.

## Required tests

1. Metadata and schema: `realm` required; public fields computed; secret/version/TFA fields absent.
2. Client/read: exact `GET /access/domains/{realm}` and correct state for LDAP, AD, OpenID, and built-in realm fixtures.
3. Secret filtering: include `password`, `client-key`, `certkey`, and `tfa` in a fake API response and prove none can enter the data-source schema/state.
4. Error propagation: preserve the complete API error context in diagnostics.
5. Registration: update the provider's exact exported data-source list.
6. Documentation generation: example and generated reference are stable across a second generation pass.
7. Acceptance: read `pam` from the PVE 9.2 guest and assert its realm and type; do not create an external realm in the shared smoke environment.

## Explicit non-goals

- no realm list data source in the same change;
- no new resource fields;
- no client certificate or TFA management;
- no LDAP group/sync field expansion;
- no call to `/sync`;
- no authentication attempt against LDAP, AD, or OpenID Connect;
- no provider-wide minimum-version change.

## Follow-up: HA resources and rules

PVE 9.2 exposes stable CRUD families at `/cluster/ha/resources[/{sid}]` and `/cluster/ha/rules[/{rule}]`.

Important verified constraints for the later design:

- resource identity is `vm:<vmid>` or `ct:<vmid>`;
- resource update accepts digest and delete fields; configuration includes requested state, failback, auto-rebalance, restart/relocate limits, and comment;
- deleting an HA resource only removes HA configuration, but API `purge` defaults to true and can mutate or delete rules that reference the resource;
- a provider destroy should not delete the VM/CT and should not silently cascade into separately managed rules;
- PVE 9 rules have typed `node-affinity` and `resource-affinity` variants;
- node affinity requires resources and nodes, with optional strict placement and node priorities;
- resource affinity requires at least two managed HA resources and positive or negative affinity;
- rule writes use the shared rules digest and PVE performs feasibility checks;
- migrate, relocate, arm/disarm, start, stop, and recovery are operational commands and do not belong in ordinary config CRUD;
- runtime placement and failover cannot be proven by the current single-node read-only CI environment.

This should be delivered as a separately reviewed design, not bundled into the realm data-source change.

## Sources

Official Proxmox sources:

- [Proxmox VE API Viewer](https://pve.proxmox.com/pve-docs/api-viewer/)
- [Proxmox VE 9.2 roadmap](https://pve.proxmox.com/wiki/Roadmap#Proxmox_VE_9.2)
- [Proxmox VE 9.0 HA affinity rule release notes](https://pve.proxmox.com/wiki/Roadmap#Proxmox_VE_9.0)
- [Proxmox HA Manager documentation](https://pve.proxmox.com/pve-docs/chapter-ha-manager.html)
- [Proxmox user management documentation](https://pve.proxmox.com/pve-docs/chapter-pveum.html)
- [Proxmox node networking documentation](https://pve.proxmox.com/pve-docs/chapter-sysadmin.html#sysadmin_network_configuration)
- [Proxmox SDN documentation](https://pve.proxmox.com/pve-docs/chapter-pvesdn.html)
- [Official `pve-ha-manager` source](https://git.proxmox.com/?p=pve-ha-manager.git;a=tree)

Local evidence:

- `internal/provider/provider.go`
- `internal/provider/client_realm.go`
- `internal/provider/realm_schema.go`
- `internal/provider/client_qemu.go`
- `internal/provider/qemu_vm_mapping.go`
- `internal/provider/e2e_smoke_test.go`
- `docs/roadmap.md`
