# QEMU VM `scsihw` Typed Field Design

## Context

The provider already maps several Proxmox QEMU `/config` keys into typed Terraform attributes while preserving `raw.extra_config` for unsupported keys. The roadmap asks to continue API alignment by evaluating `scsihw` or `tablet` and to keep typed fields and `raw.extra_config` as a single source of truth.

## Field Selection

Add `scsihw` before `tablet`.

- `scsihw` is a small top-level Proxmox QEMU config field with values such as `lsi`, `lsi53c810`, `megasas`, `pvscsi`, `virtio-scsi-pci`, and `virtio-scsi-single`; the provider can expose it as a simple string without adding custom enum validation in this increment.
- `tablet` is also small, but its documented default depends on `vga qxl`, while `vga` is still only available through `raw.extra_config`. Adding `tablet` now would force a conditional default decision that is less isolated.

## Requirements

- Add a top-level `scsihw` attribute to `proxmox_qemu_vm` resource and data source.
- Resource schema: `scsihw` is Optional + Computed, with a description that it manages the SCSI controller hardware type through `/config`.
- Data source schema: `scsihw` is Computed with the same Proxmox meaning.
- Client decode: `/config` key `scsihw` populates `QemuVMConfig.SCSIHW`, is included in the known-key allowlist, and is not retained in `ExtraConfig`.
- Client encode: create and update requests send typed `scsihw` through the form key `scsihw` when the Terraform typed field is known.
- Update emptiness: `UpdateQemuVMRequest.IsEmpty()` includes `SCSIHW == nil` so no-op update detection remains correct.
- State mapping: API `scsihw` reads into top-level Terraform state; when Proxmox omits `scsihw`, Terraform state leaves `scsihw` null rather than guessing the Proxmox default.
- Request mapping: Terraform `scsihw` maps to a new `qemuVMConfigRequest.SCSIHW *string` field, which create and update requests encode with `setOptionalString(form, "scsihw", req.SCSIHW)`.
- Raw conflict rule: `raw.extra_config["scsihw"]` is rejected unconditionally for this provider version, matching `TestValidateQemuVMRawConflictsReservesProtection` and the existing top-level `protection` single-source-of-truth behavior.
- Do not add `tablet`, `vga`, peer audio/mesh behavior, fallback compatibility paths, or unrelated refactors.

## Acceptance Criteria

- Focused unit tests cover client decode/encode, state mapping, request mapping, raw conflict rejection, and resource/data source schema attributes for `scsihw`; the hardcoded schema-attribute key lists in `data_source_qemu_vm_test.go` include `scsihw` for both resource and data source.
- Generated Terraform docs include the new resource/data source attribute.
- Example `proxmox_qemu_vm` resource shows `scsihw` as a typed attribute and does not duplicate it in `raw.extra_config`.
- `docs/roadmap.md` records `scsihw` as completed and edits the existing next-step bullet so `scsihw` no longer appears as a pending candidate; `tablet` remains the next small typed-field candidate.
- Verification includes the targeted provider tests and documentation generation.

## Safety and Rollback

This change adds a typed schema attribute and intentionally makes `raw.extra_config["scsihw"]` invalid in this provider version to enforce one source of truth. Existing users who configured `scsihw` through `raw.extra_config` must move that value to the new top-level `scsihw` attribute during upgrade; no migration shim, legacy fallback, or dual-source compatibility path is added. Rollback is a normal git revert of the implementation commit.
