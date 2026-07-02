# QEMU VM `vga` Typed Block Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a typed `vga` nested block to the `proxmox_qemu_vm` resource and data source, modeling the Proxmox QEMU `/config` `vga` compound value `type[,memory=N][,clipboard=vnc]`, keeping typed and `raw.extra_config` a single source of truth.

**Architecture:** Mirror the existing single-nested-block precedents `efi_disk`/`tpm_state` exactly. `vga` flows through `config.ExtraConfig` (no `client_qemu.go` changes). A new `qemuVMVGAModel` plus parse/encode/state-value/conflict-key/empty helpers are added to `qemu_vm_mapping.go`; the schema block is added to `qemu_vm_schema.go`; tests mirror the `efi_disk`/`tpm_state` test patterns.

**Tech Stack:** Go, Terraform Plugin Framework (`resource/schema`, `datasourceschema`, `types`), existing helpers `splitQemuConfigSegments`, `splitQemuConfigKeyValue`, `appendInt64Config`, `appendStringConfig`.

## Global Constraints

- QEMU VM typed fields and `raw.extra_config` must remain a single source of truth: when a typed `vga` block is configured, `raw.extra_config["vga"]` must be rejected by `ValidateConfig`.
- No `client_qemu.go` changes: `vga` continues to flow through `ExtraConfig`, exactly like `efidisk0`/`tpmstate0`. It is NOT added to the client known-key allowlist.
- No enum validation in this increment: `type` and `clipboard` accept and round-trip the Proxmox value verbatim.
- No legacy fallback, no dual-source compatibility shim, no unrelated refactors.
- TDD: write the failing test first, watch it fail, implement minimal code to pass.
- After implementation run `go build ./...`, `go vet ./...`, `gofmt -l internal/provider/`, and `go test ./internal/provider/`.
- The Terraform CLI needed for `make generate` is not installed locally; generated docs (`docs/resources/qemu_vm.md`, `docs/data-sources/qemu_vm.md`) are synced by hand following the existing attribute ordering, and CI's generate job verifies.

---

### Task 1: Schema — `vga` model, attr types, and resource/data-source attributes

**Files:**
- Modify: `internal/provider/qemu_vm_schema.go`
  - Add `qemuVMVGAModel` struct (after `qemuVMTPMStateModel`, ~line 126).
  - Add `VGA types.Object \`tfsdk:"vga"\`` field to `qemuVMModel` (after the `TPMState` field, ~line 35).
  - Add `qemuVMVGAAttrTypes()`, `qemuVMVGAResourceAttribute()`, `qemuVMVGADataSourceAttribute()` (near `qemuVMTPMState...` helpers, ~line 219).
  - Add `"vga": qemuVMVGAResourceAttribute(),` to `qemuVMResourceAttributes()` (placed alphabetically after `"tpm_state"` and before `"raw"`, ~line 514).
  - Add `"vga": qemuVMVGADataSourceAttribute(),` to `qemuVMDataSourceAttributes()` (after `"tpm_state"`, before `"raw"`, ~line 578).

**Interfaces:**
- Produces: `qemuVMVGAModel{ Type, Memory, Clipboard }`, `qemuVMVGAAttrTypes() map[string]attr.Type`, schema attributes named `vga` with nested attributes `type` (String), `memory` (Int64), `clipboard` (String). Later tasks reference these by these exact names.

- [ ] **Step 1: Add the model struct**

In `internal/provider/qemu_vm_schema.go`, add after the `qemuVMTPMStateModel` struct definition:

```go
type qemuVMVGAModel struct {
	Type      types.String `tfsdk:"type"`
	Memory    types.Int64  `tfsdk:"memory"`
	Clipboard types.String `tfsdk:"clipboard"`
}
```

- [ ] **Step 2: Add the `VGA` field to `qemuVMModel`**

In the `qemuVMModel` struct, add after the `TPMState` field (before `Raw`):

```go
	TPMState    types.Object `tfsdk:"tpm_state"`
	VGA         types.Object `tfsdk:"vga"`
	Raw         types.Object `tfsdk:"raw"`
```

- [ ] **Step 3: Add attr-types and schema-attribute helpers**

Add near the `qemuVMTPMStateAttrTypes`/`qemuVMTPMStateResourceAttribute` helpers:

```go
func qemuVMVGAAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"type":      types.StringType,
		"memory":    types.Int64Type,
		"clipboard": types.StringType,
	}
}

func qemuVMVGAResourceAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		Computed:            true,
		MarkdownDescription: "Typed VGA hardware configuration managed through `/config`. Unsupported grammar remains available through `raw.extra_config[\"vga\"]`.",
		Attributes: map[string]schema.Attribute{
			"type":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "VGA hardware type such as `std`, `virtio`, `qxl`, or `serial0`. The primary positional part of the Proxmox `vga` value; the block is emitted only when `type` is set."},
			"memory":    schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "VGA memory in MiB managed through `/config`."},
			"clipboard": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Clipboard selection such as `vnc` managed through `/config`."},
		},
	}
}

func qemuVMVGADataSourceAttribute() datasourceschema.SingleNestedAttribute {
	return datasourceschema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Typed VGA hardware configuration from `/config`. Unsupported grammar remains available through `raw.extra_config[\"vga\"]`.",
		Attributes: map[string]datasourceschema.Attribute{
			"type":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "VGA hardware type such as `std`, `virtio`, `qxl`, or `serial0` from `/config`."},
			"memory":    datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "VGA memory in MiB from `/config`."},
			"clipboard": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Clipboard selection such as `vnc` from `/config`."},
		},
	}
}
```

- [ ] **Step 4: Register the attribute in both schema maps**

In `qemuVMResourceAttributes()`, add (alphabetically: after `"tpm_state"` and before `"raw"`):

```go
		"tpm_state":   qemuVMTPMStateResourceAttribute(),
		"vga":         qemuVMVGAResourceAttribute(),
		"raw":         qemuVMRawResourceAttribute(),
```

In `qemuVMDataSourceAttributes()`, add (after `"tpm_state"` and before `"raw"`):

```go
		"tpm_state":   qemuVMTPMStateDataSourceAttribute(),
		"vga":         qemuVMVGADataSourceAttribute(),
		"raw":         qemuVMRawDataSourceAttribute(),
```

- [ ] **Step 5: Verify it builds**

Run: `go build ./...`
Expected: builds clean (the new `VGA` field on `qemuVMModel` is not yet read/written by mapping, which is fine for compilation; the `types.Object` zero value is null).

- [ ] **Step 6: Commit**

```bash
git add internal/provider/qemu_vm_schema.go
git commit -m "feat(qemu): add vga schema block"
```

---

### Task 2: Mapping — parse, encode, state-value, empty, expand, conflict-key, request wiring

**Files:**
- Modify: `internal/provider/qemu_vm_mapping.go`
  - Add `parseQemuVMVGA`, `encodeQemuVMVGA`, `qemuVMVGAModelIsEmpty`, `expandQemuVMVGAModel`, `qemuVMVGAStateValue`.
  - Wire `vga` into `qemuVMStateFromAPI` (extract from `extraConfigRaw`).
  - Wire `vga` into `qemuVMConfigRequestFromModel` (set `extraConfig["vga"]` when non-empty).
  - Wire `vga` into `qemuVMTypedConfigKeys` (append `"vga"` when non-empty).

**Interfaces:**
- Consumes: `qemuVMVGAModel`, `qemuVMVGAAttrTypes()` from Task 1; existing helpers `splitQemuConfigSegments`, `splitQemuConfigKeyValue`, `appendInt64Config`, `appendStringConfig`, `types.ObjectValueFrom`, `types.ObjectNull`, `basetypes.ObjectAsOptions`.
- Produces: `qemuVMVGAStateValue(ctx, base) (types.Object, map[string]string, diag.Diagnostics)`, `encodeQemuVMVGA(qemuVMVGAModel) string`, `qemuVMVGAModelIsEmpty(qemuVMVGAModel) bool`.

- [ ] **Step 1: Write the failing parse/encode/state/request/conflict tests**

Append to `internal/provider/qemu_vm_mapping_test.go`. These mirror the `efi_disk`/`tpm_state` tests (`TestParseAndEncodeQemuVMNetworkTrunks`, `TestQemuVMStateFromAPI`, `TestValidateQemuVMRawConflicts...`).

```go
func TestParseAndEncodeQemuVMVGA(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		raw  string
		parsed qemuVMVGAModel
	}{
		{name: "type only", raw: "std", parsed: qemuVMVGAModel{Type: types.StringValue("std")}},
		{name: "type and memory", raw: "std,memory=128", parsed: qemuVMVGAModel{Type: types.StringValue("std"), Memory: types.Int64Value(128)}},
		{name: "type memory clipboard", raw: "qxl,memory=256,clipboard=vnc", parsed: qemuVMVGAModel{Type: types.StringValue("qxl"), Memory: types.Int64Value(256), Clipboard: types.StringValue("vnc")}},
		{name: "serial terminal", raw: "serial0", parsed: qemuVMVGAModel{Type: types.StringValue("serial0")}},
		{name: "type and clipboard", raw: "virtio,clipboard=vnc", parsed: qemuVMVGAModel{Type: types.StringValue("virtio"), Clipboard: types.StringValue("vnc")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseQemuVMVGA(tc.raw)
			if !ok {
				t.Fatalf("parseQemuVMVGA(%q) expected ok, got false", tc.raw)
			}
			if got.Type != tc.parsed.Type || got.Memory != tc.parsed.Memory || got.Clipboard != tc.parsed.Clipboard {
				t.Fatalf("parseQemuVMVGA(%q) = %#v, want %#v", tc.raw, got, tc.parsed)
			}
			encoded := encodeQemuVMVGA(got)
			if encoded != tc.raw {
				t.Fatalf("encodeQemuVMVGA round-trip = %q, want %q", encoded, tc.raw)
			}
		})
	}
}

func TestParseQemuVMVGARejectsKeyedFirstSegment(t *testing.T) {
	t.Parallel()
	if _, ok := parseQemuVMVGA("memory=128"); ok {
		t.Fatal("expected keyed-only vga value (no positional type) to be unparseable so it stays raw")
	}
}

func TestParseQemuVMVGARejectsBadMemory(t *testing.T) {
	t.Parallel()
	if _, ok := parseQemuVMVGA("std,memory=big"); ok {
		t.Fatal("expected non-integer memory to be unparseable so it stays raw")
	}
}

func TestParseQemuVMVGARejectsUnknownKey(t *testing.T) {
	t.Parallel()
	if _, ok := parseQemuVMVGA("std,resolution=1024"); ok {
		t.Fatal("expected unknown vga key to be unparseable so it stays raw")
	}
}

func TestEncodeQemuVMVGAEmitRequiresType(t *testing.T) {
	t.Parallel()
	// memory/clipboard only, no type: treated as empty (no output), preserving parse/encode symmetry.
	encoded := encodeQemuVMVGA(qemuVMVGAModel{Memory: types.Int64Value(128)})
	if encoded != "" {
		t.Fatalf("expected empty vga encoding when type is null, got %q", encoded)
	}
}
```

Also add state-mapping, request-mapping, and conflict tests:

```go
func TestQemuVMStateFromAPIParsesVGA(t *testing.T) {
	t.Parallel()
	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{
		Name: "api-vm",
		ExtraConfig: map[string]string{
			"vga":     "std,memory=128,clipboard=vnc",
			"hostpci0": "0000:00:1f.0",
		},
	}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	vga := decodeQemuVMVGA(t, state.VGA)
	if vga.Type.ValueString() != "std" || vga.Memory.ValueInt64() != 128 || vga.Clipboard.ValueString() != "vnc" {
		t.Fatalf("unexpected vga state: %#v", vga)
	}
	raw := decodeQemuVMRaw(t, state.Raw)
	gotRaw := decodeStringMap(t, raw.ExtraConfig)
	if _, ok := gotRaw["vga"]; ok {
		t.Fatalf("expected typed vga to be removed from raw.extra_config, got %#v", gotRaw)
	}
	if gotRaw["hostpci0"] != "0000:00:1f.0" {
		t.Fatalf("expected unrelated raw key preserved, got %#v", gotRaw)
	}
}

func TestQemuVMStateFromAPIUnparseableVGASaysRaw(t *testing.T) {
	t.Parallel()
	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{
		Name: "api-vm",
		ExtraConfig: map[string]string{"vga": "std,resolution=1024"},
	}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if !state.VGA.IsNull() {
		t.Fatalf("expected vga block null for unparseable value, got %#v", state.VGA)
	}
	raw := decodeQemuVMRaw(t, state.Raw)
	gotRaw := decodeStringMap(t, raw.ExtraConfig)
	if gotRaw["vga"] != "std,resolution=1024" {
		t.Fatalf("expected unparseable vga preserved in raw, got %#v", gotRaw)
	}
}

func TestQemuVMStateFromAPIAbsentVGANull(t *testing.T) {
	t.Parallel()
	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{Name: "api-vm"}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if !state.VGA.IsNull() {
		t.Fatalf("expected absent vga block null, got %#v", state.VGA)
	}
}

func TestQemuVMRequestFromModelEncodesVGA(t *testing.T) {
	t.Parallel()
	model := qemuVMModel{
		VMID: types.Int64Value(101),
		VGA: mustQemuVMVGAValue(t, qemuVMVGAModel{Type: types.StringValue("virtio"), Memory: types.Int64Value(256)}),
	}
	createReq, diags := qemuVMCreateRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if got := createReq.ExtraConfig["vga"]; got != "virtio,memory=256" {
		t.Fatalf("expected vga encoded into extra_config, got %#v", createReq.ExtraConfig)
	}
}

func TestQemuVMRequestFromModelOmitsVGAWhenEmpty(t *testing.T) {
	t.Parallel()
	// type null but memory set is empty for encoding purposes.
	model := qemuVMModel{
		VMID: types.Int64Value(101),
		VGA: mustQemuVMVGAValue(t, qemuVMVGAModel{Memory: types.Int64Value(128)}),
	}
	createReq, diags := qemuVMCreateRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if _, ok := createReq.ExtraConfig["vga"]; ok {
		t.Fatalf("expected no vga in extra_config when type null, got %#v", createReq.ExtraConfig)
	}
}

func TestValidateQemuVMRawConflictsReservesVGA(t *testing.T) {
	t.Parallel()
	model := qemuVMModel{
		VGA: mustQemuVMVGAValue(t, qemuVMVGAModel{Type: types.StringValue("std")}),
		Raw: mustQemuVMRawValue(t, qemuVMRawModel{
			ExtraConfig: mustStringMapValue(t, map[string]string{"vga": "std"}),
		}),
	}
	diags := validateQemuVMRawConflicts(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected raw-vs-typed conflict diagnostics for vga")
	}
	if got := diags[0].Summary(); got != "Conflicting raw.extra_config entry" {
		t.Fatalf("unexpected diagnostic summary: %q", got)
	}
}
```

Add the test helper `decodeQemuVMVGA` and the value-constructor `mustQemuVMVGAValue` (mirror the existing `decodeQemuVMEFIDisk`/`mustQemuVMEFIDiskValue`):

```go
func decodeQemuVMVGA(t *testing.T, value types.Object) qemuVMVGAModel {
	t.Helper()
	if value.IsNull() {
		t.Fatalf("expected non-null vga block")
	}
	var model qemuVMVGAModel
	diags := value.As(context.Background(), &model, qemuObjectAsOptions())
	if diags.HasError() {
		t.Fatalf("unable to decode vga object: %v", diags)
	}
	return model
}

func mustQemuVMVGAValue(t *testing.T, model qemuVMVGAModel) types.Object {
	t.Helper()
	value, diags := types.ObjectValueFrom(context.Background(), qemuVMVGAAttrTypes(), model)
	if diags.HasError() {
		t.Fatalf("unable to build vga object: %v", diags)
	}
	return value
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/provider/ -run 'VGA|Vga'`
Expected: compile failure (`parseQemuVMVGA`, `encodeQemuVMVGA`, `mustQemuVMVGAValue`, `decodeQemuVMVGA` undefined).

- [ ] **Step 3: Add parse and encode helpers**

Add to `internal/provider/qemu_vm_mapping.go` (near `parseQemuVMEFIDisk`/`encodeQemuVMEFIDisk`):

```go
func parseQemuVMVGA(raw string) (qemuVMVGAModel, bool) {
	if strings.TrimSpace(raw) == "" {
		return qemuVMVGAModel{}, false
	}
	parts := splitQemuConfigSegments(raw)
	item := qemuVMVGAModel{}
	for index, segment := range parts {
		key, value, ok := splitQemuConfigKeyValue(segment)
		if !ok {
			if index != 0 {
				return qemuVMVGAModel{}, false
			}
			trimmed := strings.TrimSpace(segment)
			if trimmed == "" {
				return qemuVMVGAModel{}, false
			}
			item.Type = types.StringValue(trimmed)
			continue
		}
		if strings.TrimSpace(value) == "" {
			return qemuVMVGAModel{}, false
		}
		switch key {
		case "memory":
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return qemuVMVGAModel{}, false
			}
			item.Memory = types.Int64Value(n)
		case "clipboard":
			item.Clipboard = types.StringValue(value)
		default:
			return qemuVMVGAModel{}, false
		}
	}
	return item, !item.Type.IsNull() && !item.Type.IsUnknown()
}

func encodeQemuVMVGA(item qemuVMVGAModel) string {
	if item.Type.IsNull() || item.Type.IsUnknown() {
		return ""
	}
	segments := []string{item.Type.ValueString()}
	appendInt64Config(&segments, "memory", item.Memory)
	appendStringConfig(&segments, "clipboard", item.Clipboard)
	return strings.Join(segments, ",")
}

func qemuVMVGAModelIsEmpty(model qemuVMVGAModel) bool {
	return (model.Type.IsNull() || model.Type.IsUnknown()) && model.Memory.IsNull() && model.Clipboard.IsNull()
}

func expandQemuVMVGAModel(ctx context.Context, value types.Object) (qemuVMVGAModel, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return qemuVMVGAModel{}, nil
	}
	var result qemuVMVGAModel
	diags := value.As(ctx, &result, qemuObjectAsOptions())
	return result, diags
}
```

Note: `strconv` is already imported in `qemu_vm_mapping.go` (used by `parseQemuVMImportID`). Verify the import is present; if not, add `"strconv"`.

- [ ] **Step 4: Add the state-value helper**

Add near `qemuVMTPMStateValue` (mirror it exactly):

```go
func qemuVMVGAStateValue(ctx context.Context, base map[string]string) (types.Object, map[string]string, diag.Diagnostics) {
	if len(base) == 0 {
		return types.ObjectNull(qemuVMVGAAttrTypes()), nil, nil
	}
	extra := make(map[string]string, len(base))
	found := false
	for key, value := range base {
		if key == "vga" {
			found = true
			continue
		}
		extra[key] = value
	}
	if !found {
		return types.ObjectNull(qemuVMVGAAttrTypes()), extra, nil
	}
	raw := base["vga"]
	if len(extra) == 0 {
		extra = nil
	}
	parsed, ok := parseQemuVMVGA(raw)
	if !ok {
		result := types.ObjectNull(qemuVMVGAAttrTypes())
		if extra == nil {
			extra = map[string]string{}
		}
		extra["vga"] = raw
		return result, extra, nil
	}
	value, diags := types.ObjectValueFrom(ctx, qemuVMVGAAttrTypes(), parsed)
	return value, extra, diags
}
```

- [ ] **Step 5: Wire `vga` into `qemuVMStateFromAPI`**

In `qemuVMStateFromAPI`, after the `efiDiskValue, extraConfigRaw, ... := qemuVMEFIDiskStateValue(ctx, extraConfigRaw)` line, add a `vga` extraction that consumes the same `extraConfigRaw`. Then add `VGA: vgaValue,` to the returned `qemuVMModel`. Concretely, change:

```go
	efiDiskValue, extraConfigRaw, efiDiskDiags := qemuVMEFIDiskStateValue(ctx, extraConfigRaw)
	diags.Append(efiDiskDiags...)
	rawValue, rawDiags := qemuVMRawStateValue(ctx, extraConfigRaw, networkRaw, diskRaw)
```

to:

```go
	efiDiskValue, extraConfigRaw, efiDiskDiags := qemuVMEFIDiskStateValue(ctx, extraConfigRaw)
	diags.Append(efiDiskDiags...)
	vgaValue, extraConfigRaw, vgaDiags := qemuVMVGAStateValue(ctx, extraConfigRaw)
	diags.Append(vgaDiags...)
	rawValue, rawDiags := qemuVMRawStateValue(ctx, extraConfigRaw, networkRaw, diskRaw)
```

And in the returned struct add `VGA: vgaValue,` next to `EFIDisk: efiDiskValue,` / `TPMState: tpmStateValue,`.

- [ ] **Step 6: Wire `vga` into `qemuVMConfigRequestFromModel`**

In `qemuVMConfigRequestFromModel`, after expanding `tpmState` and `efiDisk`, also expand `vga`:

```go
	efiDisk, efiDiskDiags := expandQemuVMEFIDiskModel(ctx, model.EFIDisk)
	diags.Append(efiDiskDiags...)
	vga, vgaDiags := expandQemuVMVGAModel(ctx, model.VGA)
	diags.Append(vgaDiags...)
```

Then, alongside the `extraConfig["efidisk0"] = ...` block, add (note: the encode helper returns `""` when type is null, which also guards the "empty when type null" rule, but check explicitly for clarity):

```go
	if encoded := encodeQemuVMVGA(vga); encoded != "" {
		if extraConfig == nil {
			extraConfig = map[string]string{}
		}
		extraConfig["vga"] = encoded
	}
```

- [ ] **Step 7: Wire `vga` into `qemuVMTypedConfigKeys`**

In `qemuVMTypedConfigKeys`, after the `tpmState` block that appends `"tpmstate0"`, add:

```go
	vga, vgaDiags := expandQemuVMVGAModel(ctx, model.VGA)
	diags.Append(vgaDiags...)
	if !qemuVMVGAModelIsEmpty(vga) {
		keys = append(keys, "vga")
	}
```

Note on `qemuVMVGAModelIsEmpty`: a block with only `memory`/`clipboard` (no `type`) is non-empty for the conflict check, so `raw.extra_config["vga"]` is still rejected when the user sets any vga field. This is correct single-source-of-truth behavior; the request encoder separately treats type-null as no output.

- [ ] **Step 8: Run the vga tests and verify they pass**

Run: `go test ./internal/provider/ -run 'VGA|Vga' -v`
Expected: all vga tests PASS.

- [ ] **Step 9: Run the full provider test suite**

Run: `go build ./... && go vet ./... && gofmt -l internal/provider/ && go test ./internal/provider/`
Expected: builds clean, vet clean, fmt clean, all tests PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/provider/qemu_vm_mapping.go internal/provider/qemu_vm_mapping_test.go
git commit -m "feat(qemu): map vga typed block"
```

---

### Task 3: Schema-attribute presence tests + example + generated docs + roadmap

**Files:**
- Modify: `internal/provider/data_source_qemu_vm_test.go` (add `vga` to both attribute lists).
- Modify: `examples/resources/proxmox_qemu_vm/resource.tf` (add a `vga` block).
- Modify: `docs/resources/qemu_vm.md` and `docs/data-sources/qemu_vm.md` (add `vga` attribute + example block by hand, following existing ordering).
- Modify: `docs/roadmap.md` (record `vga` done, update next-step bullet).

**Interfaces:**
- Consumes: the `vga` schema attribute from Task 1.

- [ ] **Step 1: Write the failing schema-presence test update**

In `internal/provider/data_source_qemu_vm_test.go`, add `"vga"` to both attribute key lists:

```go
	for _, key := range []string{"node", "vm_id", "name", "template", "protection", "scsihw", "tablet", "tpm_state", "vga", "status", "uptime"} {
```

Apply the same change to both the data-source and the resource loops.

- [ ] **Step 2: Run test to verify it passes**

Run: `go test ./internal/provider/ -run 'TestQemuVMDataSourceSchemaAttributes|TestQemuVMResourceSchemaAttributes'`
Expected: PASS (the `vga` attribute already exists from Task 1).

- [ ] **Step 3: Update the example**

In `examples/resources/proxmox_qemu_vm/resource.tf`, add a `vga` block (place it among the typed top-level config, e.g. after the `cpu`/`ostype`/`boot` lines or near other typed blocks):

```hcl
  vga = {
    type   = "std"
    memory = 16
  }
```

- [ ] **Step 4: Sync generated resource doc**

In `docs/resources/qemu_vm.md`:
1. In the example HCL block near the top, add the `vga` block to match the example file.
2. In the alphabetically-ordered attribute list, add the nested-attribute line. Nested single attributes appear in their alphabetical position; add after `tpm_state` (or where `vga` sorts) and before `raw`:

```markdown
- `vga` (Attributes) Typed VGA hardware configuration managed through `/config`. Unsupported grammar remains available through `raw.extra_config["vga"]`. (see [below for nested schema](#nestedatt--vga))
```

3. At the end of the file, in the nested-schema section, add the `vga` nested block (mirror the `tpm_state` nested schema format):

```markdown
<a id="nestedatt--vga"></a>
### Nested Schema for `vga`

Optional + Computed:
- `clipboard` (String) Clipboard selection such as `vnc` managed through `/config`.
- `memory` (Number) VGA memory in MiB managed through `/config`.
- `type` (String) VGA hardware type such as `std`, `virtio`, `qxl`, or `serial0`. The primary positional part of the Proxmox `vga` value; the block is emitted only when `type` is set.
```

Match the exact heading format/order used by the existing nested schemas in that file.

- [ ] **Step 5: Sync generated data-source doc**

In `docs/data-sources/qemu_vm.md`, add the `vga` attribute line in alphabetical position (after `tpm_state`, before `raw`):

```markdown
- `vga` (Attributes) Typed VGA hardware configuration from `/config`. Unsupported grammar remains available through `raw.extra_config["vga"]`. (see [below for nested schema](#nestedatt--vga))
```

And add the corresponding nested schema section matching the data-source file's existing format.

- [ ] **Step 6: Update roadmap**

In `docs/roadmap.md`:
1. Add to the "已完成" list a bullet recording the `vga` typed block (mirror the `tablet`/`scsihw` bullet wording).
2. Edit the existing "接下来" bullet that currently mentions `serial*`/`vga` so that it lists `serial*` as the next small typed candidate and drops `vga`.

- [ ] **Step 7: Final verification**

Run: `go build ./... && go vet ./... && gofmt -l internal/provider/ && go test ./...`
Expected: builds clean, vet clean, fmt clean, all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/provider/data_source_qemu_vm_test.go examples/resources/proxmox_qemu_vm/resource.tf docs/resources/qemu_vm.md docs/data-sources/qemu_vm.md docs/roadmap.md
git commit -m "docs(qemu): document vga typed block"
```

---

## Self-Review

**Spec coverage:**
- Resource + data source `vga` SingleNestedAttribute (Optional+Computed / Computed) → Task 1.
- `type`/`memory`/`clipboard` block attributes → Task 1.
- State mapping with parse, unparseable→raw, absent→null → Task 2 (state-value helper + tests).
- Parse rule (positional type first, `memory` int, `clipboard`, unknown/first-keyed fail) → Task 2.
- Encode rule (emit only when type non-null; positional type then `appendInt64Config`/`appendStringConfig`) → Task 2.
- Request mapping (set `extraConfig["vga"]` when non-empty) → Task 2.
- Raw conflict (`qemuVMTypedConfigKeys` appends `"vga"` when non-empty) → Task 2.
- No `client_qemu.go` changes / no client known-key allowlist entry → enforced in Global Constraints.
- Acceptance criteria: parse round-trips, encode, state mapping, request mapping, raw conflict, schema presence lists → Tasks 2 + 3.
- Generated docs + example + roadmap → Task 3.
- Verification: provider tests + doc generation (hand-synced) → Task 3 Step 7.

**Placeholder scan:** none; every step has concrete code or exact commands.

**Type consistency:** `qemuVMVGAModel{Type, Memory, Clipboard}`, `qemuVMVGAAttrTypes()`, `parseQemuVMVGA`, `encodeQemuVMVGA`, `qemuVMVGAModelIsEmpty`, `expandQemuVMVGAModel`, `qemuVMVGAStateValue`, `mustQemuVMVGAValue`, `decodeQemuVMVGA` are referenced consistently across tasks.
