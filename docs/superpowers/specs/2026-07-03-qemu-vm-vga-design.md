# QEMU VM `vga` Typed Block Design

## Context

The provider maps supported Proxmox QEMU `/config` keys into typed Terraform attributes and preserves unsupported keys in `raw.extra_config`. Recent increments added the small top-level typed scalars `protection`, `scsihw`, and `tablet`. The roadmap's next QEMU candidate is `serial*`/`vga`. `vga` is chosen first because it is a single, self-contained `/config` key, whereas `serial*` is a slot family requiring separate slot-key parsing decisions.

## Field Selection

Add `vga` as the next typed surface.

- `vga` is a single Proxmox QEMU `/config` key (not a slot family). Its grammar is a comma-separated value with one positional/default-key part and two optional keyed parts, modeled exactly like the existing `efi_disk`/`tpm_state` single nested blocks.
- Proxmox API viewer (`pve-docs/api-viewer/apidata.js`, `vga` entry) documents the format as: positional `type` (enum, default `std`, default key), optional `memory` (integer 4–512, VGA memory in MiB), optional `clipboard` (enum, currently only `vnc`).

Proxmox `/config` examples this must round-trip:

- `vga: "std"` → `{ type = "std" }`
- `vga: "virtio"` → `{ type = "virtio" }`
- `vga: "std,memory=128"` → `{ type = "std", memory = 128 }`
- `vga: "qxl,clipboard=vnc"` → `{ type = "qxl", clipboard = "vnc" }`
- `vga: "serial0"` → `{ type = "serial0" }` (serial-as-terminal; `memory` is documented as having no effect here and is simply not present)

## Requirements

- Add a top-level `vga` attribute to `proxmox_qemu_vm` resource and data source as a `SingleNestedAttribute` block, mirroring `efi_disk`/`tpm_state`.
- Resource schema: `vga` is Optional + Computed, with a description that it configures VGA hardware through `/config` and that unsupported grammar remains available through `raw.extra_config["vga"]`.
- Data source schema: `vga` is Computed with the same Proxmox meaning.
- Block attributes:
  - `type` — String, Optional + Computed on resource, Computed on data source. The VGA hardware type (e.g. `std`, `virtio`, `qxl`, `serial0`). `type` is the primary/positional field of the Proxmox `vga` value; a typed `vga` block is only emitted on write when `type` is set. The provider does not add enum validation in this increment; it accepts and round-trips the Proxmox value verbatim.
  - `memory` — Int64, Optional + Computed on resource, Computed on data source. VGA memory in MiB.
  - `clipboard` — String, Optional + Computed on resource, Computed on data source. Clipboard selection (e.g. `vnc`).
- State mapping (read): `vga` is extracted from `config.ExtraConfig["vga"]` by `qemuVMVGAStateValue` before the remaining `extra_config` is exposed, exactly like `qemuVMEFIDiskStateValue`. When Proxmox omits `vga`, the block reads null; when present but unparseable by `parseQemuVMVGA`, the raw value remains in `raw.extra_config["vga"]` and the block is null (no silent loss). The provider does not infer the Proxmox default `std` when the key is absent.
- Parse rule (`parseQemuVMVGA`): split the value with the existing `splitQemuConfigSegments`/`splitQemuConfigKeyValue` helpers. The first segment is positional and becomes `type`. Subsequent segments are `memory=<int>` (parsed into an Int64; unparseable integer → parse fails and the value stays raw) and `clipboard=<string>`. Any unknown key, or a non-positional first segment, fails the parse so the raw value is preserved.
- Encode rule (`encodeQemuVMVGA`): the `vga` block is emitted only when `type` is non-null; emit `<type>` positionally first, then `memory=<n>` via the existing `appendInt64Config` helper and `clipboard=<value>` via `appendStringConfig`. When `type` is null, the block is treated as empty and contributes nothing to `extra_config` — this guarantees parse/encode symmetry (the parser requires a positional `type` as the first segment), and a `memory`/`clipboard`-only block is not a meaningful Proxmox VGA configuration for the typed surface (such an unusual value round-trips unchanged through `raw.extra_config["vga"]`).
- Request mapping (write): in `qemuVMConfigRequestFromModel`, after the raw `extra_config` is expanded, if the typed `vga` block is non-empty per `qemuVMVGAModelIsEmpty`, set `extraConfig["vga"] = encodeQemuVMVGA(vga)`. This is encoded through the existing `setSortedStringMap(form, req.ExtraConfig)` path, identical to `efidisk0`/`tpmstate0`.
- Raw conflict rule: in `qemuVMTypedConfigKeys`, append `"vga"` when the typed `vga` block is non-empty, so `raw.extra_config["vga"]` is rejected when a typed `vga` block is also configured. This matches the `efidisk0`/`tpmstate0` single-source-of-truth behavior.
- Client decode/encode: no `client_qemu.go` changes. `vga` continues to flow through `ExtraConfig`; it is not added to the client known-key allowlist, exactly like `efidisk0`/`tpmstate0`.
- Update emptiness: no change to `UpdateQemuVMRequest.IsEmpty()` is required, because `vga` is encoded into the existing `ExtraConfig` map field that is already part of that check.
- Do not add `serial*`, peer audio/mesh behavior, fallback compatibility paths, enum validation, or unrelated refactors.

## Acceptance Criteria

- Focused unit tests cover: parse round-trips for each example above; encode round-trips; state mapping (present → block populated, absent → null, unparseable → stays raw); request mapping (non-empty block → `extraConfig["vga"]` set, empty block → not set); raw conflict reservation for `vga`; and the `vga` attribute is present in both resource and data source schema attribute lists in `data_source_qemu_vm_test.go`.
- Generated Terraform docs include the new `vga` block for resource and data source (`docs/resources/qemu_vm.md`, `docs/data-sources/qemu_vm.md`).
- Example `proxmox_qemu_vm` resource shows a typed `vga` block and does not duplicate `vga` in `raw.extra_config`.
- `docs/roadmap.md` records `vga` as completed and edits the existing next-step bullet so `vga`/`serial*` no longer list `vga` as pending; `serial*` remains the next small typed candidate.
- Verification includes the targeted provider tests and documentation generation.

## Safety and Rollback

This change adds a typed schema block and intentionally makes `raw.extra_config["vga"]` invalid in this provider version when a typed `vga` block is also configured, enforcing one source of truth. Existing users who configured `vga` through `raw.extra_config` may keep doing so when no typed `vga` block is set (the raw value still round-trips unchanged); users who want the typed surface move their value into the `vga` block. No migration shim, legacy fallback, or dual-source compatibility path is added. Rollback is a normal git revert of the implementation commit.
