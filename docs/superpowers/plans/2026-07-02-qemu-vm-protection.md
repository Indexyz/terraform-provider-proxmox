# QEMU VM Protection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a typed `protection` boolean to `proxmox_qemu_vm` resource and data source, aligned with Proxmox VE's QEMU VM protection flag.

**Architecture:** Extend the existing scalar QEMU config pipeline end-to-end: Terraform schema/model, Proxmox client decode/encode structs, API-to-state mapping, request mapping, raw conflict validation, generated docs, example, and roadmap. `protection` is intentionally normalized to `false` when Proxmox omits it because Proxmox documents default `0`, and `raw.extra_config["protection"]` is reserved unconditionally once the provider types the key.

**Tech Stack:** Go 1.26.4, Terraform Plugin Framework, existing `httptest` client tests, existing mapping tests, `tfplugindocs` through `make generate`.

## Global Constraints

- No legacy fallback.
- Keep typed QEMU fields and `raw.extra_config` as a single source of truth.
- Preserve underlying error context; do not mask Proxmox delete failures for protected VMs.
- Do not add destroy-time logic that disables protection before deleting.
- Do not add typed fields for `tablet`, `scsihw`, `vga`, serial devices, RNG, audio, USB, or virtiofs.
- Do not change tag separators, power-state management, clone semantics, or disk/network parsing.
- Update `docs/roadmap.md` after code changes.
- Do not commit until the final SDD review/verification gate.

---

## File Structure

- Modify `internal/provider/qemu_vm_schema.go`: add the Terraform-facing `Protection` model field plus data source/resource attributes.
- Modify `internal/provider/client_qemu.go`: add Proxmox API decode/request fields, known-key classification, `IsEmpty`, and form encoding.
- Modify `internal/provider/qemu_vm_mapping.go`: map API config to Terraform state, map Terraform model to request, and reserve `raw.extra_config["protection"]` unconditionally.
- Modify `internal/provider/client_qemu_test.go`: cover client decode and form encoding for `protection`.
- Modify `internal/provider/qemu_vm_mapping_test.go`: cover state mapping, omitted default normalization, request mapping, and unconditional raw conflict.
- Modify `internal/provider/data_source_qemu_vm_test.go`: require schema exposure in resource and data source attribute maps.
- Modify `examples/resources/proxmox_qemu_vm/resource.tf`: show `protection = true` with the other top-level VM config fields.
- Regenerate `docs/index.md`, `docs/resources/qemu_vm.md`, and `docs/data-sources/qemu_vm.md` with `make generate`.
- Modify `docs/roadmap.md`: record the completed typed protection field and the next Proxmox-alignment follow-up.

---

### Task 1: Add failing unit tests for typed QEMU VM protection

**Files:**
- Modify: `internal/provider/client_qemu_test.go`
- Modify: `internal/provider/qemu_vm_mapping_test.go`
- Modify: `internal/provider/data_source_qemu_vm_test.go`

**Interfaces:**
- Consumes: existing unexported functions `decodeQemuVMConfig`, `qemuVMStateFromAPI`, `qemuVMCreateRequestFromModel`, `qemuVMUpdateRequestFromModel`, `validateQemuVMRawConflicts`.
- Produces: failing tests that define `Protection` fields and expected behavior for later implementation.

- [ ] **Step 1: Extend client method test expectations**

In `internal/provider/client_qemu_test.go`, add `"protection": true` to the GET config response in `TestClientQemuVMMethods`:

```go
writeEnvelope(t, w, map[string]any{
    "name":        "api-vm",
    "description": "Managed by Terraform",
    "tags":        "prod,terraform",
    "template":    0,
    "pool":        "platform",
    "onboot":      1,
    "protection":  true,
    "startup":     "order=2",
    "bios":        "ovmf",
    "machine":     "q35",
    "agent":       "enabled=1",
    "cores":       "4",
    "sockets":     2,
    "memory":      8192,
    "cpu":         "host",
    "ostype":      "l26",
    "boot":        "order=scsi0;net0",
})
```

Add `"protection": {"1"}` to the create form assertion and `"protection": {"0"}` to the update form assertion:

```go
assertFormValues(t, r, url.Values{
    "vmid":        {"101"},
    "name":        {"api-vm"},
    "description": {"Managed by Terraform"},
    "tags":        {"prod,terraform"},
    "pool":        {"platform"},
    "onboot":      {"1"},
    "protection":  {"1"},
    "startup":     {"order=2"},
    "bios":        {"ovmf"},
    "machine":     {"q35"},
    "agent":       {"enabled=1"},
    "cores":       {"4"},
    "sockets":     {"2"},
    "memory":      {"8192"},
    "cpu":         {"host"},
    "ostype":      {"l26"},
    "boot":        {"order=scsi0;net0"},
})
```

```go
assertFormValues(t, r, url.Values{
    "name":       {"api-vm"},
    "onboot":     {"0"},
    "protection": {"0"},
    "memory":     {"4096"},
})
```

After the existing `config.OnBoot` assertion, add:

```go
if config.Protection.Ptr() == nil || !*config.Protection.Ptr() {
    t.Fatalf("expected protection=true, got %#v", config.Protection)
}
```

In the create request literal, add `Protection: boolPtr(true)`. In the update request literal, add `Protection: boolPtr(false)`.

- [ ] **Step 2: Add bool variant decode test**

In `internal/provider/client_qemu_test.go`, add `encoding/json` to the imports and this test after `TestClientQemuVMMethods`:

```go
func TestDecodeQemuVMConfigProtectionBoolVariants(t *testing.T) {
    t.Parallel()

    for _, tc := range []struct {
        name string
        raw  json.RawMessage
        want bool
    }{
        {name: "json bool true", raw: json.RawMessage(`true`), want: true},
        {name: "json integer one", raw: json.RawMessage(`1`), want: true},
        {name: "json string false", raw: json.RawMessage(`"false"`), want: false},
        {name: "json string zero", raw: json.RawMessage(`"0"`), want: false},
    } {
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()

            config, err := decodeQemuVMConfig(map[string]json.RawMessage{
                "protection": tc.raw,
                "hostpci0":   json.RawMessage(`"0000:00:1f.0"`),
            })
            if err != nil {
                t.Fatalf("decodeQemuVMConfig() unexpected error: %v", err)
            }
            if config.Protection.Ptr() == nil || *config.Protection.Ptr() != tc.want {
                t.Fatalf("unexpected protection value: got %#v want %v", config.Protection, tc.want)
            }
            if _, ok := config.ExtraConfig["protection"]; ok {
                t.Fatalf("expected protection to be decoded as typed field, got extra config %#v", config.ExtraConfig)
            }
            if got := config.ExtraConfig["hostpci0"]; got != "0000:00:1f.0" {
                t.Fatalf("expected unrelated raw key to remain raw, got %#v", config.ExtraConfig)
            }
        })
    }
}
```

- [ ] **Step 3: Extend state and request mapping tests**

In `internal/provider/qemu_vm_mapping_test.go`, add `Protection: proxmoxOptionalBool{value: boolPtr(true)}` to the `QemuVMConfig` literal in `TestQemuVMStateFromAPI`. Extend the bool assertion to include protection:

```go
if !state.OnBoot.ValueBool() || !state.Protection.ValueBool() || state.Template.ValueBool() {
    t.Fatalf("unexpected bool mapping: %#v", state)
}
```

In `TestQemuVMRequestFromModel`, add `Protection: types.BoolValue(false)` to the top-level `qemuVMModel` literal. After the existing `createReq.Hotplug` assertion, add:

```go
if createReq.Protection == nil || *createReq.Protection {
    t.Fatalf("expected protection=false in create request, got %#v", createReq.Protection)
}
```

Extend the update request assertion from:

```go
if updateReq.OnBoot == nil || !*updateReq.OnBoot || updateReq.Memory == nil || *updateReq.Memory != 8192 {
    t.Fatalf("unexpected update request: %#v", updateReq)
}
```

to:

```go
if updateReq.OnBoot == nil || !*updateReq.OnBoot || updateReq.Protection == nil || *updateReq.Protection || updateReq.Memory == nil || *updateReq.Memory != 8192 {
    t.Fatalf("unexpected update request: %#v", updateReq)
}
```

Add this focused true-case test after `TestQemuVMRequestFromModel`:

```go
func TestQemuVMRequestFromModelMapsProtectionTrue(t *testing.T) {
    t.Parallel()

    model := qemuVMModel{
        VMID:       types.Int64Value(101),
        Protection: types.BoolValue(true),
    }

    createReq, diags := qemuVMCreateRequestFromModel(context.Background(), model)
    assertNoDiags(t, diags)
    if createReq.Protection == nil || !*createReq.Protection {
        t.Fatalf("expected protection=true in create request, got %#v", createReq.Protection)
    }

    updateReq, diags := qemuVMUpdateRequestFromModel(context.Background(), model)
    assertNoDiags(t, diags)
    if updateReq.Protection == nil || !*updateReq.Protection {
        t.Fatalf("expected protection=true in update request, got %#v", updateReq.Protection)
    }
}
```

- [ ] **Step 4: Add omitted default normalization test**

In `internal/provider/qemu_vm_mapping_test.go`, add this test after `TestQemuVMStateFromAPI`:

```go
func TestQemuVMStateFromAPIDefaultsOmittedProtectionToFalse(t *testing.T) {
    t.Parallel()

    state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{Name: "api-vm"}, QemuVMStatus{}, nil)
    if diags.HasError() {
        t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
    }
    if state.Protection.IsNull() || state.Protection.IsUnknown() || state.Protection.ValueBool() {
        t.Fatalf("expected omitted protection to read as false, got %#v", state.Protection)
    }
}
```

- [ ] **Step 5: Add unconditional raw conflict test**

In `internal/provider/qemu_vm_mapping_test.go`, add this test after the existing raw conflict tests:

```go
func TestValidateQemuVMRawConflictsReservesProtection(t *testing.T) {
    t.Parallel()

    model := qemuVMModel{
        Raw: mustQemuVMRawValue(t, qemuVMRawModel{
            ExtraConfig: mustStringMapValue(t, map[string]string{
                "protection": "1",
            }),
        }),
    }

    diags := validateQemuVMRawConflicts(context.Background(), model)
    if !diags.HasError() {
        t.Fatal("expected raw-vs-typed conflict diagnostics for protection")
    }
    if got := diags[0].Summary(); got != "Conflicting raw.extra_config entry" {
        t.Fatalf("unexpected diagnostic summary: %q", got)
    }
}
```

- [ ] **Step 6: Extend schema attribute tests**

In `internal/provider/data_source_qemu_vm_test.go`, add `"protection"` to both attribute key lists:

```go
for _, key := range []string{"node", "vm_id", "name", "template", "protection", "tpm_state", "status", "uptime"} {
    if _, ok := attrs[key]; !ok {
        t.Fatalf("expected data source attribute %q", key)
    }
}
```

```go
for _, key := range []string{"node", "vm_id", "name", "template", "protection", "tpm_state", "status", "uptime"} {
    if _, ok := attrs[key]; !ok {
        t.Fatalf("expected resource attribute %q", key)
    }
}
```

- [ ] **Step 7: Run targeted tests and verify failure**

Run:

```bash
go test ./internal/provider -run 'Test(ClientQemuVMMethods|DecodeQemuVMConfigProtectionBoolVariants|QemuVMStateFromAPI|QemuVMRequestFromModel|ValidateQemuVMRawConflicts|QemuVM(DataSource|Resource)SchemaAttributes)' -count=1
```

Expected: FAIL because `Protection` fields and schema attributes do not exist yet.

---

### Task 2: Implement typed protection end-to-end

**Files:**
- Modify: `internal/provider/qemu_vm_schema.go`
- Modify: `internal/provider/client_qemu.go`
- Modify: `internal/provider/qemu_vm_mapping.go`

**Interfaces:**
- Consumes: failing tests from Task 1.
- Produces: `qemuVMModel.Protection`, `QemuVMConfig.Protection`, `qemuVMConfigRequest.Protection`, typed schema attribute `protection`, Proxmox form key `protection`, and unconditional raw conflict reservation.

- [ ] **Step 1: Add schema/model field**

In `internal/provider/qemu_vm_schema.go`, add the field immediately after `OnBoot` in `qemuVMModel`:

```go
Protection  types.Bool   `tfsdk:"protection"`
```

Add data-source and resource attributes immediately after `onboot` in their maps:

```go
"protection": datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Proxmox protection is enabled for this VM, disabling remove VM and remove disk operations."},
```

```go
"protection": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether Proxmox protection is enabled for this VM, disabling remove VM and remove disk operations."},
```

- [ ] **Step 2: Add client decode/request fields**

In `internal/provider/client_qemu.go`, add `Protection proxmoxOptionalBool` after `OnBoot` in `QemuVMConfig` and `qemuVMConfigKnown`:

```go
Protection  proxmoxOptionalBool
```

```go
Protection  proxmoxOptionalBool  `json:"protection"`
```

Add `Protection *bool` after `OnBoot` in `qemuVMConfigRequest`:

```go
Protection  *bool
```

Add `r.Protection == nil &&` to `UpdateQemuVMRequest.IsEmpty()` immediately after the `r.OnBoot == nil &&` check.

In `decodeQemuVMConfig`, copy the known field into `config`:

```go
Protection:  known.Protection,
```

Add `"protection": {}` to the `knownKeys` map near `"onboot": {}`.

In `encodeQemuVMFields`, add:

```go
setOptionalBool(form, "protection", req.Protection)
```

immediately after the `onboot` encoding.

- [ ] **Step 3: Add state and request mapping**

In `internal/provider/qemu_vm_mapping.go`, compute the effective protection value before the `qemuVMModel` return in `qemuVMStateFromAPI`:

```go
protection := false
if value := config.Protection.Ptr(); value != nil {
    protection = *value
}
```

Add the state field after `OnBoot`:

```go
Protection:  types.BoolValue(protection),
```

In `qemuVMConfigRequestFromModel`, add:

```go
Protection:  boolPointerValue(model.Protection),
```

immediately after `OnBoot`.

- [ ] **Step 4: Reserve raw protection key unconditionally**

In `qemuVMTypedConfigKeys`, initialize `keys` with `protection` instead of an empty slice:

```go
keys := []string{"protection"}
```

Keep the existing conditional append logic for other typed keys and the final `sort.Strings(keys)`.

- [ ] **Step 5: Run targeted tests and verify pass**

Run:

```bash
go test ./internal/provider -run 'Test(ClientQemuVMMethods|DecodeQemuVMConfigProtectionBoolVariants|QemuVMStateFromAPI|QemuVMRequestFromModel|ValidateQemuVMRawConflicts|QemuVM(DataSource|Resource)SchemaAttributes)' -count=1
```

Expected: PASS.

- [ ] **Step 6: Verify delete path was not changed**

Run:

```bash
git diff -- internal/provider/resource_qemu_vm.go
```

Expected: no diff. The provider should not disable protection before delete; existing delete error propagation remains intact.

---

### Task 3: Update example, generated docs, roadmap, and full verification

**Files:**
- Modify: `examples/resources/proxmox_qemu_vm/resource.tf`
- Generated modify: `docs/resources/qemu_vm.md`
- Generated modify: `docs/data-sources/qemu_vm.md`
- Generated modify: `docs/index.md` if `make generate` changes it
- Modify: `docs/roadmap.md`

**Interfaces:**
- Consumes: implemented schema/client/mapping from Task 2.
- Produces: user-facing docs/example plus roadmap update required by `AGENTS.md`.

- [ ] **Step 1: Update QEMU VM resource example**

In `examples/resources/proxmox_qemu_vm/resource.tf`, add `protection = true` near the lifecycle/top-level scalar fields:

```hcl
  pool        = "workloads"
  onboot      = true
  protection  = true
  startup     = "order=1"
  bios        = "ovmf"
```

- [ ] **Step 2: Generate Terraform provider docs**

Run:

```bash
make generate
```

Expected: exits 0. `docs/resources/qemu_vm.md` and `docs/data-sources/qemu_vm.md` include the `protection` attribute; the resource docs example includes `protection = true`.

- [ ] **Step 3: Update roadmap**

Edit `docs/roadmap.md` so `## 已完成` includes a bullet like:

```markdown
- 为 `proxmox_qemu_vm` 资源和数据源新增 Proxmox QEMU `protection` typed boolean，覆盖 schema、client decode/encode、state/request mapping、raw 冲突校验、测试、示例和生成文档；`raw.extra_config["protection"]` 迁移到 typed 字段。
```

Edit `## 接下来` so it includes the next Proxmox-alignment follow-up and does not imply `protection` remains pending:

```markdown
- 继续按 Proxmox QEMU API 对齐下一个小型 typed 字段，优先评估 `scsihw` 或 `tablet`；仍保持 typed 与 `raw.extra_config` 单一 source of truth。
```

Keep the existing QEMU extension guidance bullet about schema/mapping/client/tests/docs.

- [ ] **Step 4: Run full tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Run formatting and whitespace checks**

Run:

```bash
make fmt
git diff --check
```

Expected: both commands exit 0.

- [ ] **Step 6: Ensure generated docs are current**

Run `make generate` one more time, then check for unexpected regenerated diff:

```bash
make generate
git diff -- docs/index.md docs/resources/qemu_vm.md docs/data-sources/qemu_vm.md
```

Expected: the diff only reflects the intended `protection` schema/example changes already in the worktree; a second `make generate` should not introduce additional changes.

- [ ] **Step 7: Review final worktree**

Run:

```bash
git status --short
git diff --stat
git diff -- internal/provider/qemu_vm_schema.go internal/provider/client_qemu.go internal/provider/qemu_vm_mapping.go internal/provider/client_qemu_test.go internal/provider/qemu_vm_mapping_test.go internal/provider/data_source_qemu_vm_test.go examples/resources/proxmox_qemu_vm/resource.tf docs/roadmap.md docs/resources/qemu_vm.md docs/data-sources/qemu_vm.md
```

Expected: only intended files changed. `internal/provider/resource_qemu_vm.go` remains unchanged.

---

## Plan Self-Review

- Spec coverage: Task 1 defines tests for typed state/read/write/default/raw conflict/schema; Task 2 implements schema/client/mapping/conflict behavior; Task 3 covers examples, generated docs, roadmap, and verification. The destroy non-goal is covered by Task 2 Step 6 and final diff review.
- Placeholder scan: no TODO/TBD placeholders remain; all code snippets and commands are concrete.
- Type consistency: all planned names match existing code patterns and the approved spec (`Protection`, `protection`, `proxmoxOptionalBool`, `qemuVMConfigRequest`, `qemuVMTypedConfigKeys`).
