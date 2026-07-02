# LXC Container Support Design

## Context

The provider currently supports cluster/node/pool APIs and a first-class `proxmox_qemu_vm` resource/data source. It has no first-class LXC container resource or data source. The Proxmox API exposes LXC containers under `/nodes/{node}/lxc`, `/nodes/{node}/lxc/{vmid}/config`, `/nodes/{node}/lxc/{vmid}/status/current`, and `/nodes/{node}/lxc/{vmid}`. Generated Proxmox docs in `pve-docs/generated/pct.conf.5-opts.adoc` define common LXC config keys such as `hostname`, `description`, `tags`, `onboot`, `protection`, `startup`, `arch`, `cores`, `memory`, `swap`, `unprivileged`, `features`, `ostype`, `rootfs`, `net[n]`, and `mp[n]`.

## Scope

Add first-class Terraform support for LXC containers with:

- `proxmox_lxc_container` resource for create, read, update, delete, import, and raw conflict validation.
- `proxmox_lxc_container` data source for reading an existing container config and runtime status.
- Client methods for LXC config/status/create/update/delete.
- Typed scalar attributes for the commonly used top-level LXC config keys.
- Raw Proxmox string maps for complex, slot-keyed LXC grammars (`network` for `net[n]`, `mount_point` for `mp[n]`) instead of over-parsing those grammars in this increment.
- `raw.extra_config` as an escape hatch only for unsupported LXC config keys.

Out of scope for this increment: LXC clone, template conversion, start/stop/reboot state management, snapshots, resize/move-volume, firewall, console/proxy APIs, detailed parsing of `rootfs`, `net[n]`, `mp[n]`, `features`, or `startup`, and any QEMU changes.

## Terraform Schema

### Resource attributes

- Required identity inputs: `node` (string), `vm_id` (int64). Changes require replacement.
- Create inputs: `ostemplate` (string, optional resource attribute), `rootfs` (string, optional/computed resource attribute), `arch` (optional/computed string), and `unprivileged` (optional/computed bool). The provider enforces `ostemplate` and `rootfs` in `Create` with explicit `resp.Diagnostics.AddAttributeError` diagnostics before calling Proxmox; they remain schema-optional so imported resources and data source reads can have null `ostemplate`. `arch` and `unprivileged` are optional create inputs; if omitted, Terraform lets Proxmox choose defaults and then reads the API values back (`arch` null if omitted by API, `unprivileged` false if omitted by API). Changes to `ostemplate`, `rootfs`, `arch`, or `unprivileged` require replacement. Resource refresh preserves the prior `ostemplate` and prior configured `rootfs` when prior state has them, because Proxmox does not return `ostemplate` and can normalize rootfs allocation shorthand like `local-lvm:10` into a concrete volume ID.
- Optional/computed top-level config: `hostname`, `description`, `tags`, `cores`, `memory`, `swap`, `onboot`, `protection`, `startup`, `features`, `ostype`, `nameserver`, `searchdomain`, `timezone`. `cores`, `memory`, `swap`, and `uptime` are int64 values; `onboot`, `protection`, and `unprivileged` are bool values; the other config attributes, including `rootfs`, `features`, and `startup`, are raw string passthrough values. `arch` and `unprivileged` are also exposed but are create-time replacement attributes.
- Optional/computed maps: `network` (`map(string)` keyed by `net0`, `net1`, ...), `mount_point` (`map(string)` keyed by `mp0`, `mp1`, ...), and `raw.extra_config` (`map(string)`).
- Computed runtime fields: `id`, `status`, `uptime`.

### Data source attributes

- Required lookup inputs: `node`, `vm_id`.
- Computed config/runtime attributes mirror the resource, including `ostemplate` as null because Proxmox config does not return the create template.

## API and Mapping Rules

- `GetLXCContainerConfig(ctx, node, vmID)` calls `GET /nodes/{node}/lxc/{vmid}/config` and decodes known scalar keys into typed fields.
- `GetLXCContainerStatus(ctx, node, vmID)` calls `GET /nodes/{node}/lxc/{vmid}/status/current` and decodes `status` and `uptime`.
- `CreateLXCContainer(ctx, node, req)` calls `POST /nodes/{node}/lxc`, sends `vmid`, `ostemplate`, typed scalar form parameters, every `network` map entry as its bare `net[n]` form key, every `mount_point` map entry as its bare `mp[n]` form key, and every `raw.extra_config` entry as its exact Proxmox key after conflict validation.
- `UpdateLXCContainer(ctx, node, vmID, req)` calls `PUT /nodes/{node}/lxc/{vmid}/config` and sends all current known updatable typed scalar values, every current `network` map entry as `net[n]`, every current `mount_point` map entry as `mp[n]`, every current `raw.extra_config` entry as its exact Proxmox key, plus the comma-joined `delete` parameter for removals. Conflict validation runs before request mapping, so typed/network/mount keys take precedence by rejecting overlapping `raw.extra_config` rather than silently overwriting.
- `DeleteLXCContainer(ctx, node, vmID)` calls `DELETE /nodes/{node}/lxc/{vmid}` and performs the same returned-UPID task wait as create/update. Resource delete tolerates `errNotFound` from the initial DELETE call, matching existing QEMU behavior.
- Mutating LXC client methods intentionally diverge from current QEMU client behavior by waiting for returned Proxmox tasks. Decode the Proxmox response envelope `data` field as a JSON string UPID such as `"UPID:..."`; if no non-empty UPID string is decoded because `data` is `null`, absent, or an empty string, no task wait is performed. For a non-empty UPID, URL-path-escape the UPID, then poll `GET /nodes/{node}/tasks/{upid}/status` every 2 seconds until `status == "stopped"` and require `exitstatus == "OK"`. Stop polling when the request context is canceled/deadline-exceeded and return an error that includes that context error. Also stop after 10 minutes if the caller supplied no shorter context deadline and return a timeout error naming the UPID. If the task stops with any non-`OK` exit status, return an error naming the UPID and exit status. If any HTTP/decode/poll operation fails, return that lower-level error with context.
- Use existing optional Proxmox bool/int decoders so API values like `1`, `"1"`, and `true` decode consistently.
- Decode `net[n]` keys into the `Network` map, `mp[n]` keys into the `MountPoint` map, and all unsupported non-empty string/int/bool values into `ExtraConfig`. `features`, `startup`, and `rootfs` are raw string passthrough attributes, not structured nested types; `rootfs` is additionally marked replacement-only in the resource schema.
- Keep `ostemplate` from prior Terraform state on refresh; it is a create input that Proxmox does not return from `/config`. Keep `rootfs` from prior Terraform state on resource refresh when prior state has a known configured value, so Proxmox rootfs shorthand normalization does not cause replacement loops; data source reads and imports without prior state use the API `rootfs` value.
- Resource read removes the Terraform resource from state when either config or status read returns `errNotFound`; other read errors produce diagnostics preserving the underlying error text.
- Normalize omitted/null Proxmox defaults for `onboot`, `protection`, and `unprivileged` to `false` in Terraform state. Other omitted scalar strings/integers remain null.
- Update requests intentionally diverge from current QEMU update behavior by using the LXC `delete` form parameter for removals. Compare prior Terraform state with the planned model. When a previously known updatable optional scalar becomes null, or a previous `network`, `mount_point`, or `raw.extra_config` key is absent from the plan, include the bare Proxmox key name (for example `hostname`, `net0`, or `mp1`, not `key=value`) in the comma-joined `delete` form parameter. Do not use the delete parameter on create. Never send `ostemplate`, `rootfs`, `arch`, or `unprivileged` in update requests or delete parameters; schema replacement handles changes to those fields.

## Single Source of Truth

`raw.extra_config` must not overlap with typed LXC fields. Reject conflicts for these always-reserved keys even when the typed attribute is omitted: `hostname`, `description`, `tags`, `cores`, `memory`, `swap`, `onboot`, `protection`, `startup`, `features`, `ostype`, `nameserver`, `searchdomain`, `timezone`, `arch`, `unprivileged`, `ostemplate`, `rootfs`, every `net[n]`, and every `mp[n]`. Users should put network and mount-point Proxmox strings in `network` and `mount_point`, not in `raw.extra_config`.

## Files and Docs

- Add LXC client, schema, mapping, resource, data source, and tests under `internal/provider/`, reusing existing naming/style where it fits but explicitly implementing the LXC-only task wait and delete-parameter behavior described above without sharing abstractions prematurely.
- Register the resource and data source in `provider.go` and update provider export tests.
- Import ID syntax is exactly `node/vmid`, matching QEMU. Import rejects missing slash, empty node, empty VMID, and non-integer VMID.
- Add examples under `examples/resources/proxmox_lxc_container/resource.tf` and `examples/data-sources/proxmox_lxc_container/data-source.tf`.
- Run docs generation after schema/example updates. If `make generate` is blocked by the missing Terraform CLI formatter, run the `tfplugindocs` generation command directly and keep only intended generated docs changes.
- Update `docs/roadmap.md` to record LXC support and list focused follow-up work.

## Acceptance Criteria

- Unit tests cover LXC client methods, task wait success/non-OK/canceled-or-timeout behavior, config decode classification, state mapping, request mapping including delete-parameter removals, import ID parsing, raw conflict validation including conflicts when the typed attribute is omitted, resource metadata/read missing behavior, data source/resource schema attributes, and provider registration.
- `proxmox_lxc_container` resource can create, refresh, update, delete, and import an LXC container through the Proxmox LXC endpoints.
- `proxmox_lxc_container` data source can read config and status for an existing LXC container.
- Typed fields and `raw.extra_config` have one source of truth for all supported LXC keys.
- Examples and generated docs include the new resource and data source.
- No QEMU behavior, Lyre audio behavior, peer mesh audio behavior, or legacy fallback is added.

## Safety and Migration

This is additive provider surface area. No existing resource type changes behavior. The new LXC resource intentionally rejects `raw.extra_config` conflicts from its first release rather than supporting duplicate sources. Rollback is a normal git revert of the implementation commit.
