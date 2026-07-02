# QEMU VM Protection Typed Field Design

## Goal

Add a first-class `protection` boolean to `proxmox_qemu_vm` resource and data source so Terraform can read and manage the Proxmox VE QEMU VM protection flag without using `raw.extra_config`.

## Context and Proxmox Alignment

The project roadmap says the next QEMU work should extend `proxmox_qemu_vm` typed fields while keeping typed fields and `raw.extra_config` as a single source of truth. The bundled Proxmox reference documents `protection` in `pve-docs/generated/qm.conf.5-opts.adoc` as:

> `protection`: `<boolean>` (`default = 0`): Sets the protection flag of the VM. This will disable the remove VM and remove disk operations.

The same option appears in `pve-docs/api-viewer/apidata.js` as an optional boolean on the QEMU config API. The provider does not currently expose `protection`; users must put it in `raw.extra_config`, which bypasses typed bool handling and conflicts with the roadmap direction.

## Scope

Implement only the QEMU VM protection flag:

- Data source: expose computed `protection` from `/nodes/{node}/qemu/{vmid}/config`.
- Resource: expose optional+computed `protection`, send known planned values on create/update, and read omitted Proxmox values as `false` because Proxmox defines `protection` default `0`.
- Raw conflict detection: reject `raw.extra_config["protection"]` unconditionally because `protection` is now a typed provider field rather than an uncovered raw key.
- Documentation/examples: update generated docs and the QEMU example to show the typed field.
- Roadmap: record the completed feature and the next follow-up.

## Non-Goals

- Do not add a destroy-time workaround that automatically disables protection. If a protected VM cannot be deleted, that is Proxmox-aligned behavior; users can set `protection = false` before destroying.
- Do not add typed fields for `tablet`, `scsihw`, `vga`, serial devices, RNG, audio, USB, or virtiofs in this change.
- Do not change the existing raw escape hatch except to reserve `protection` as a typed key. Existing `raw.extra_config["protection"]` usage must migrate to the typed `protection` attribute.
- Do not change tag separators, power-state management, clone semantics, or disk/network parsing.

## Considered Approaches

1. **Typed `protection` boolean (selected).** Small, high-value, non-destructive, and directly supported by Proxmox as a boolean config key. It reuses the provider's existing scalar schema/client/form plumbing, but intentionally normalizes omitted API values to `false` and reserves the raw key because `protection` has a Proxmox default and is no longer an uncovered raw field.
2. **Typed display/input fields (`tablet`, `vga`, `serial[n]`, `scsihw`).** Also useful, but serial and VGA require nested/string grammar choices and device slot handling. They are better as a separate design.
3. **Typed RNG/audio/virtiofs devices.** Valuable for richer VM hardware, but slot-based device grammars need parser/encoder work and broader tests. This is larger than the next incremental alignment step.

## Design

### Schema

Add `Protection types.Bool` to `qemuVMModel` with `tfsdk:"protection"`.

Add a top-level `protection` attribute beside `onboot`/`startup`:

- Data source: `Computed: true`, description says it is the Proxmox protection flag that disables remove VM and remove disk operations.
- Resource: `Optional: true, Computed: true`, same description, no replacement plan modifier.

### Client and API Mapping

Add `Protection proxmoxOptionalBool` to `QemuVMConfig` and `qemuVMConfigKnown` with JSON key `protection`.

Add `Protection *bool` to `qemuVMConfigRequest`, include it in `UpdateQemuVMRequest.IsEmpty`, and encode it with `setOptionalBool(form, "protection", req.Protection)`.

In `qemuVMStateFromAPI`, map `config.Protection.Ptr()` to a Terraform boolean and normalize an omitted/null API value to `false`, because Proxmox documents `protection` default `0`. This means imported/refreshed resources expose the effective Proxmox value even when Proxmox omits the default, and a user-configured `protection = false` does not churn back to null. This intentional behavior is scoped to `protection`; existing `onboot`/`template` null-on-omission behavior is out of scope.

In `qemuVMConfigRequestFromModel`, map `model.Protection` through `boolPointerValue`. If Terraform carries a known planned value from prior state on an update where the user did not explicitly configure `protection`, sending that idempotent value is acceptable; it preserves the effective typed state and avoids raw fallback logic.

Add `"protection"` to the `knownKeys` set in `decodeQemuVMConfig` so it is decoded only as the typed field and is not duplicated into `ExtraConfig`.

### Raw Conflict Rule

Extend raw conflict detection so `raw.extra_config["protection"]` is rejected even when the typed `protection` attribute is omitted. Implement this by adding `"protection"` as an unconditional reserved typed key in `qemuVMTypedConfigKeys` or a small reserved-key list consumed by `validateQemuVMRawConflicts`; do not depend on `model.Protection` being known/non-null. This intentionally reserves the key once the provider covers it, matching the raw escape hatch contract for keys not typed by this provider version. Existing scalar keys that lack raw conflict coverage are out of scope for this change.

### Tests

Update focused unit tests instead of adding broad acceptance coverage:

- QEMU config decode/client tests cover `protection` JSON boolean/integer/string decoding and form encoding.
- QEMU mapping tests cover explicit state readback, omitted API value readback as `false`, create/update request generation for `true` and `false`, and raw conflict detection for `raw.extra_config["protection"]` even when the typed attribute is omitted.
- Provider/resource schema tests continue passing with the new attribute.

### Documentation

Run `make generate` after schema/example changes so `docs/resources/qemu_vm.md`, `docs/data-sources/qemu_vm.md`, and provider docs stay aligned. Update `examples/resources/proxmox_qemu_vm/resource.tf` to include `protection = true` near other lifecycle/config scalar fields.

## Acceptance Criteria

- `proxmox_qemu_vm` resource accepts `protection = true|false` and sends `protection=1|0` to Proxmox when Terraform has a known planned value.
- `proxmox_qemu_vm` data source and resource refresh read Proxmox `protection` config values into the typed boolean attribute; if Proxmox omits the default value, state reads `protection = false`.
- A configured `protection = false` is sent as `protection=0` and does not refresh back to null when Proxmox omits the default value.
- `raw.extra_config["protection"]` is rejected even when the typed `protection` attribute is omitted; users must use the typed attribute for this now-covered Proxmox key.
- Unsupported/other raw config keys continue to round-trip unchanged.
- Generated docs include the new resource/data-source attribute and the resource example uses it.
- `docs/roadmap.md` records the completed `protection` typed field and the next Proxmox-alignment follow-up.
- `go test ./...`, `make generate`, `git diff --check`, and a final no-diff check after generation succeed.
- Destroying a protected VM is not auto-worked-around by disabling protection; the provider surfaces the Proxmox delete error with its context.

## Safety and Rollback

The change adds a Terraform schema field and reserves a previously raw key that is now typed. Rollback is removing the typed attribute and docs/example changes; after rollback, users could again manage `protection` through `raw.extra_config`. This design intentionally does not mask Proxmox delete failures for protected VMs.
