# QEMU VM `scsihw` Typed Field Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a typed top-level `scsihw` field for `proxmox_qemu_vm` while keeping `raw.extra_config` and typed fields as a single source of truth.

**Architecture:** Mirror the existing top-level `protection` plumbing, but use string/null semantics like `bios` and `startup`. `scsihw` is decoded from Proxmox `/config`, encoded into create/update form bodies, mapped to Terraform state/request models, reserved from `raw.extra_config`, shown in examples, and regenerated into Terraform docs.

**Tech Stack:** Go, Terraform Plugin Framework, Proxmox QEMU `/config`, `go test`, `gofmt`, `make generate`.

## Global Constraints

- No legacy fallback.
- Keep typed QEMU fields and `raw.extra_config` as a single source of truth.
- Lyre audio topology is server relay only. Do not add peer mesh audio mode, peer-to-peer audio negotiation, or mesh compatibility fallbacks.
- Preserve lower-level error cause/context chains.
- Avoid over-engineering; make only directly required changes.
- Update `docs/roadmap.md` after code updates.
- Do not add `tablet` or `vga` in this increment.
- When Proxmox omits `scsihw`, Terraform state leaves `scsihw` null rather than guessing the Proxmox default.
- `raw.extra_config["scsihw"]` is rejected unconditionally in this provider version.

---

## File Structure

- Modify `internal/provider/qemu_vm_schema.go`: add `qemuVMModel.SCSIHW` and resource/data source schema attributes.
- Modify `internal/provider/client_qemu.go`: add `SCSIHW` to API config structs, request struct, update emptiness, decode known-key list, and form encoding.
- Modify `internal/provider/qemu_vm_mapping.go`: map API state, Terraform request, and unconditional raw conflict key.
- Modify `internal/provider/client_qemu_test.go`: cover GET decode and create/update form encode for `scsihw`; add focused decode test for extra_config exclusion.
- Modify `internal/provider/qemu_vm_mapping_test.go`: cover state mapping, omitted null state, request mapping, and raw conflict reservation.
- Modify `internal/provider/data_source_qemu_vm_test.go`: include `scsihw` in schema smoke key lists.
- Modify `examples/resources/proxmox_qemu_vm/resource.tf`: show `scsihw` as a typed attribute.
- Modify generated docs under `docs/` by running `make generate`.
- Modify `docs/roadmap.md`: record completion and leave `tablet` as the next candidate.

### Task 1: Add typed `scsihw` end-to-end

**Files:**
- Modify: `internal/provider/qemu_vm_schema.go`
- Modify: `internal/provider/client_qemu.go`
- Modify: `internal/provider/qemu_vm_mapping.go`
- Modify: `internal/provider/client_qemu_test.go`
- Modify: `internal/provider/qemu_vm_mapping_test.go`
- Modify: `internal/provider/data_source_qemu_vm_test.go`
- Modify: `examples/resources/proxmox_qemu_vm/resource.tf`
- Modify: generated files under `docs/`
- Modify: `docs/roadmap.md`

**Interfaces:**
- Consumes: existing `qemuVMModel`, `QemuVMConfig`, `qemuVMConfigKnown`, `qemuVMConfigRequest`, `qemuVMStateFromAPI`, `qemuVMConfigRequestFromModel`, `validateQemuVMRawConflicts`, `qemuVMDataSourceAttributes`, `qemuVMResourceAttributes`.
- Produces: `qemuVMModel.SCSIHW types.String`, `QemuVMConfig.SCSIHW string`, `qemuVMConfigKnown.SCSIHW string`, `qemuVMConfigRequest.SCSIHW *string`, top-level Terraform attribute `scsihw`, Proxmox form key `scsihw`, and unconditional raw conflict reservation for `raw.extra_config["scsihw"]`.

- [ ] **Step 1: Write focused failing tests**

In `internal/provider/client_qemu_test.go`:

1. In `TestClientQemuVMMethods`, add `"scsihw": "virtio-scsi-pci"` to the GET config response, `"scsihw": {"virtio-scsi-pci"}` to the create form, and `SCSIHW: stringPtr("virtio-scsi-pci")` to the create request literal. Add `"scsihw": {"megasas"}` to the update form and `SCSIHW: stringPtr("megasas")` to the update request literal. After the protection assertion, add:

```go
	if config.SCSIHW != "virtio-scsi-pci" {
		t.Fatalf("expected scsihw=virtio-scsi-pci, got %q", config.SCSIHW)
	}
```

2. Add this focused test after `TestDecodeQemuVMConfigProtectionBoolVariants`:

```go
func TestDecodeQemuVMConfigSCSIHWIsTyped(t *testing.T) {
	t.Parallel()

	config, err := decodeQemuVMConfig(map[string]json.RawMessage{
		"scsihw":   json.RawMessage(`"virtio-scsi-single"`),
		"hostpci0": json.RawMessage(`"0000:00:1f.0"`),
	})
	if err != nil {
		t.Fatalf("decodeQemuVMConfig() unexpected error: %v", err)
	}
	if config.SCSIHW != "virtio-scsi-single" {
		t.Fatalf("expected typed scsihw, got %q", config.SCSIHW)
	}
	if _, ok := config.ExtraConfig["scsihw"]; ok {
		t.Fatalf("expected scsihw to be decoded as typed field, got extra config %#v", config.ExtraConfig)
	}
	if got := config.ExtraConfig["hostpci0"]; got != "0000:00:1f.0" {
		t.Fatalf("expected unrelated raw key to remain raw, got %#v", config.ExtraConfig)
	}
}
```

In `internal/provider/qemu_vm_mapping_test.go`:

1. In `TestQemuVMStateFromAPI`, add `SCSIHW: "virtio-scsi-pci",` to the `QemuVMConfig` literal. After the integer mapping assertion, add:

```go
	if state.SCSIHW.ValueString() != "virtio-scsi-pci" {
		t.Fatalf("expected scsihw state, got %#v", state.SCSIHW)
	}
```

2. Add this test after `TestQemuVMStateFromAPIDefaultsOmittedProtectionToFalse`:

```go
func TestQemuVMStateFromAPIOmittedSCSIHWIsNull(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{Name: "api-vm"}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if !state.SCSIHW.IsNull() || state.SCSIHW.IsUnknown() {
		t.Fatalf("expected omitted scsihw to read as null, got %#v", state.SCSIHW)
	}
}
```

3. In `TestQemuVMRequestFromModel`, add `SCSIHW: types.StringValue("virtio-scsi-pci"),` to the `qemuVMModel` literal. After the protection request assertion, add:

```go
	if createReq.SCSIHW == nil || *createReq.SCSIHW != "virtio-scsi-pci" {
		t.Fatalf("expected scsihw in create request, got %#v", createReq.SCSIHW)
	}
```

Extend the update request assertion to require `updateReq.SCSIHW != nil && *updateReq.SCSIHW == "virtio-scsi-pci"`.

4. Add this test after `TestValidateQemuVMRawConflictsReservesProtection`:

```go
func TestValidateQemuVMRawConflictsReservesSCSIHW(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		Raw: mustQemuVMRawValue(t, qemuVMRawModel{
			ExtraConfig: mustStringMapValue(t, map[string]string{
				"scsihw": "virtio-scsi-pci",
			}),
		}),
	}

	diags := validateQemuVMRawConflicts(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected raw-vs-typed conflict diagnostics for scsihw")
	}
	if got := diags[0].Summary(); got != "Conflicting raw.extra_config entry" {
		t.Fatalf("unexpected diagnostic summary: %q", got)
	}
}
```

In `internal/provider/data_source_qemu_vm_test.go`, add `"scsihw"` to both hardcoded key lists in `TestQemuVMDataSourceSchemaAttributes` and `TestQemuVMResourceSchemaAttributes`.

- [ ] **Step 2: Run focused tests and confirm they fail before implementation**

Run:

```bash
go test ./internal/provider -run 'Test(ClientQemuVMMethods|DecodeQemuVMConfigSCSIHWIsTyped|QemuVMStateFromAPI|QemuVMStateFromAPIOmittedSCSIHWIsNull|QemuVMRequestFromModel|ValidateQemuVMRawConflictsReservesSCSIHW|QemuVM(DataSource|Resource)SchemaAttributes)' -count=1
```

Expected: FAIL with compile errors for missing `SCSIHW` fields and missing `scsihw` schema/model plumbing.

- [ ] **Step 3: Implement schema, client, and mapping**

In `internal/provider/qemu_vm_schema.go`:

- Add `SCSIHW types.String ` + "`tfsdk:\"scsihw\"`" + ` after `Protection` in `qemuVMModel`.
- Add data source attribute:

```go
		"scsihw":     datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "SCSI controller hardware type from `/config`."},
```

- Add resource attribute:

```go
		"scsihw":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "SCSI controller hardware type managed through `/config`."},
```

In `internal/provider/client_qemu.go`:

- Add `SCSIHW string` after `Protection` in `QemuVMConfig`.
- Add `SCSIHW string ` + "`json:\"scsihw\"`" + ` after `Protection` in `qemuVMConfigKnown`.
- Add `SCSIHW *string` after `Protection` in `qemuVMConfigRequest`.
- Add `r.SCSIHW == nil &&` after `r.Protection == nil &&` in `UpdateQemuVMRequest.IsEmpty()`.
- In `decodeQemuVMConfig`, set `SCSIHW: known.SCSIHW,` after `Protection: known.Protection,`.
- Add `"scsihw": {}` to the known-key allowlist next to `"protection": {}`.
- In `encodeQemuVMFields`, add `setOptionalString(form, "scsihw", req.SCSIHW)` after `setOptionalBool(form, "protection", req.Protection)`.

In `internal/provider/qemu_vm_mapping.go`:

- In `qemuVMStateFromAPI`, set `SCSIHW: stringOrNull(config.SCSIHW),` after `Protection`.
- In `qemuVMConfigRequestFromModel`, set `SCSIHW: stringPointerValue(model.SCSIHW),` after `Protection`.
- In `qemuVMTypedConfigKeys`, change `keys := []string{"protection"}` to `keys := []string{"protection", "scsihw"}`.

- [ ] **Step 4: Run focused tests and gofmt**

Run:

```bash
gofmt -w internal/provider/qemu_vm_schema.go internal/provider/client_qemu.go internal/provider/qemu_vm_mapping.go internal/provider/client_qemu_test.go internal/provider/qemu_vm_mapping_test.go internal/provider/data_source_qemu_vm_test.go
go test ./internal/provider -run 'Test(ClientQemuVMMethods|DecodeQemuVMConfigSCSIHWIsTyped|QemuVMStateFromAPI|QemuVMStateFromAPIOmittedSCSIHWIsNull|QemuVMRequestFromModel|ValidateQemuVMRawConflictsReservesSCSIHW|QemuVM(DataSource|Resource)SchemaAttributes)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Update example, generated docs, and roadmap**

In `examples/resources/proxmox_qemu_vm/resource.tf`, add:

```hcl
  scsihw      = "virtio-scsi-pci"
```

near the other top-level QEMU settings, and do not add `scsihw` to `raw.extra_config`.

Run:

```bash
make generate
```

Update `docs/roadmap.md`:

- Add a completed bullet stating that `proxmox_qemu_vm` resource/data source now support typed Proxmox QEMU `scsihw`, covering schema, client decode/encode, state/request mapping, raw conflict validation, tests, example, and generated docs; `raw.extra_config["scsihw"]` must move to the typed field.
- Replace the current next-step bullet naming `scsihw` or `tablet` with one that leaves `tablet` as the next small typed field candidate.

- [ ] **Step 6: Final verification for this task**

Run:

```bash
go test ./internal/provider -run 'Test(ClientQemuVMMethods|DecodeQemuVMConfigSCSIHWIsTyped|QemuVMStateFromAPI|QemuVMStateFromAPIOmittedSCSIHWIsNull|QemuVMRequestFromModel|ValidateQemuVMRawConflictsReservesSCSIHW|QemuVM(DataSource|Resource)SchemaAttributes)' -count=1
go test ./internal/provider -count=1
```

Expected: PASS for both commands.

- [ ] **Step 7: Commit**

Do not include unrelated pre-existing `.gitignore` changes. Commit only the spec/plan and intended implementation/docs files after review gates pass.
