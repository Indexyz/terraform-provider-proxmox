# Code Context

## Files Retrieved
1. `internal/provider/qemu_vm_schema.go` (lines 17-128, 240-658) - QEMU Terraform model, attr type maps, data source/resource schema for typed fields, clone, raw escape hatch, status.
2. `internal/provider/qemu_vm_mapping.go` (lines 15-247, 286-521, 582-616, 716-1184) - API↔state mapping, raw conflict validation, typed key discovery, parse/encode logic, slot classification.
3. `internal/provider/client_qemu.go` (lines 80-211, 260-431) - Proxmox QEMU API structs, known config fields, CRUD/clone endpoints, `/config` decode/encode and extra config preservation.
4. `internal/provider/resource_qemu_vm.go` (lines 17-238) - resource lifecycle, ValidateConfig, clone/create/update/read/delete/import flows.
5. `internal/provider/data_source_qemu_vm.go` (lines 15-89) - data source read flow using shared QEMU state mapper.
6. `internal/provider/qemu_vm_mapping_test.go` (lines 15-220; names through 700 from grep) - main mapping/round-trip/raw fallback test patterns.
7. `internal/provider/client_qemu_test.go` (lines 15-165) - HTTP client endpoint/form encoding tests.
8. `internal/provider/data_source_qemu_vm_test.go` (lines 13-44) - schema presence tests for data source/resource.
9. `docs/resources/qemu_vm.md` (lines 1-180) - generated resource docs and example schema.
10. `examples/resources/proxmox_qemu_vm/resource.tf` (lines 1-66) - maintained example config.
11. `README.md` (lines 88-105) - QEMU workflow constraints and extension rules.

## Key Code

Supported top-level typed QEMU fields are in `qemuVMModel` / `qemuVMConfig` / schema: `name`, `description`, `tags`, read-only `template`, `pool`, `onboot`, `startup`, `bios`, `machine`, `agent`, `cores`, `sockets`, `memory`, `cpu`, `ostype`, `boot`, observed-only `status`, `uptime` (`internal/provider/qemu_vm_schema.go:17-45`, `internal/provider/client_qemu.go:80-137`, `internal/provider/qemu_vm_schema.go:592-658`).

Typed nested domains:
- `common.hotplug` (`qemu_vm_schema.go:47-49`, `240-263`).
- `cloud_init`: `cicustom`, sensitive `cipassword`, `citype`, `ciupgrade`, `ciuser`, `sshkeys`, and slot-keyed `ipconfig` with `ipv4/gateway/ipv6/gateway6` (`qemu_vm_schema.go:51-64`, `265-320`).
- `network` map keyed by `netN`: `model`, `bridge`, `macaddr`, `tag`, `trunks`, `firewall`, `link_down`, `mtu`, `queues`, `rate` (`qemu_vm_schema.go:66-78`, `322-366`).
- `disk` map keyed by `ideN/sataN/scsiN/virtioN`: `storage`, `volume`, `size`, `media`, `cache`, `discard`, `iothread`, `ssd`, `replicate`, `backup`, `shared`, `snapshot`, `serial`, IOPS and MBPS limit fields (`qemu_vm_schema.go:80-108`, `368-446`).
- `efi_disk` maps only `efidisk0`: `storage`, `volume`, `size`, `efitype`, `format`, `ms_cert`, `pre_enrolled_keys` (`qemu_vm_schema.go:110-118`, `448-481`).
- `tpm_state` maps only `tpmstate0`: `storage`, `volume`, `size`, `format`, `version` (`qemu_vm_schema.go:120-126`, `483-512`).
- `clone` create-time-only: `source_node`, required `source_vmid`, `full`, `snapshot_name`, `storage`, `format`, `bwlimit` (`qemu_vm_schema.go:134-144`, `529-553`).

Raw escape hatch:
- Schema: `raw.extra_config` is `map(string)`, optional+computed on resource, computed on data source (`qemu_vm_schema.go:514-527`).
- Decode: unknown `/config` keys go to `ExtraConfig`; slot keys are first classified as IP/network/disk by prefix+decimal suffix (`client_qemu.go:316-377`, `qemu_vm_mapping.go:1159-1184`).
- Unsupported network/disk grammar remains raw: `qemuVMNetworkStateValue` / `qemuVMDiskStateValue` return parsed typed maps plus `unsupported`; `qemuVMRawStateValue` merges base extras + unsupported slots (`qemu_vm_mapping.go:304-366`).
- Unsupported `efidisk0` / `tpmstate0` grammar remains `raw.extra_config["efidisk0"]` / `["tpmstate0"]`; parsed forms are removed from raw and surfaced typed (`qemu_vm_mapping.go:376-447`).
- Plan validation forbids `raw.extra_config` keys that overlap typed fields/slots currently configured in the same plan (`validateQemuVMRawConflicts`, `qemuVMTypedConfigKeys`: `qemu_vm_mapping.go:138-170`, `450-521`).

Critical mapping seams:
- Add top-level typed config in all of: `qemuVMModel`, `qemuVMConfig`, `qemuVMConfigKnown`, `qemuVMConfigRequest`, `UpdateQemuVMRequest.IsEmpty`, `decodeQemuVMConfig.knownKeys`, `encodeQemuVMFields`, `qemuVMStateFromAPI`, `qemuVMConfigRequestFromModel`, data source/resource schema (`qemu_vm_schema.go`, `client_qemu.go`, `qemu_vm_mapping.go`).
- Add nested field in all of: model struct, attr type map, data source/resource schema attrs, parser, encoder, typed-key conflict discovery if it controls a raw Proxmox key, tests/docs.
- Add a new slot-keyed family by extending client classification (`isQemuVM...Key`) and mapping state/request encode paths; preserve unsupported grammar in raw like network/disk.

## Architecture

Resource and data source share the same surface definitions and mapper. Terraform schema/model lives in `qemu_vm_schema.go`. Client reads `/nodes/{node}/qemu/{vmid}/config` into `QemuVMConfig`, separating known typed fields, classified slot maps (`IPConfig`, `Network`, `Disk`), and `ExtraConfig`; status comes from `/status/current` (`client_qemu.go:214-377`). `qemuVMStateFromAPI` builds Terraform state, parsing supported nested grammars and shunting unsupported entries into `raw.extra_config` (`qemu_vm_mapping.go:15-74`, `286-366`). Create/update/clone expand Terraform model to `qemuVMConfigRequest`; `encodeQemuVMFields` sends form keys to Proxmox (`qemu_vm_mapping.go:95-247`, `client_qemu.go:260-431`).

Resource lifecycle: `ValidateConfig` checks typed/raw conflict, `Create` either clones then updates or creates directly, `Read` refreshes config+status, `Update` verifies existence then PUTs full request, `Delete` calls DELETE, import ID is `node/vmid` (`resource_qemu_vm.go:45-238`). Clone provenance is not inferred; mapper preserves prior clone block only when state already had it (`qemu_vm_mapping.go:369-374`).

Docs are generated from schema plus examples; README documents extension constraints: status/uptime observed-only, clone create-mode only, slot identities stable, typed/raw single source of truth (`README.md:88-105`).

## Test / validation patterns

- Mapping tests are table/direct unit tests using Terraform framework `types` and helper decoders. Existing coverage asserts state from API, clone preservation/null behavior, slot-keyed domains, efi/tpm typed vs unsupported raw fallback, request encoding including efi/tpm (`qemu_vm_mapping_test.go`, grep-reported tests through line 700).
- Client tests use `httptest.Server`, assert token auth, paths/methods, and exact form values (`client_qemu_test.go:15-165`).
- Schema tests check key presence only (`data_source_qemu_vm_test.go:25-44`).
- Resource tests use fake client/state read and missing-resource paths (`resource_qemu_vm_test.go`, found but not deeply read beyond grep summary).
- Likely verification command for changes: `go test ./internal/provider`.

## Risks / constraints / open questions

- No legacy fallback per `AGENTS.md`; keep typed/raw single source of truth.
- Raw conflict detection only considers typed fields present in config/plan, not all possible typed keys. That matches current behavior but extensions should decide whether a newly typed optional+computed field should conflict only when configured or also when known from state.
- Parsers reject any unknown option in a slot and preserve whole slot raw. Adding one option can move an entire previously-raw slot into typed state; this may be intended but can alter diffs/imports.
- Encoders preserve a specific segment order; tests should assert exact string/form output when adding fields.
- Disk/efi/tpm `storage+size` encoding synthesizes first segment; if `volume` is set, `size` is appended as `size=...` later (`qemu_vm_mapping.go:986-1069`). Be careful matching Proxmox grammar.
- Docs under `docs/` appear generated; update examples/schema docs with the project’s doc generation path rather than hand-editing generated sections unless instructed.
- I did not run `git status` because this scout environment exposes no bash/git tool; only read/grep/find/ls/write were available. This task intentionally modified only the requested research output file.

## Start Here

Open `internal/provider/qemu_vm_schema.go` first to identify the Terraform-facing field/model shape, then `internal/provider/qemu_vm_mapping.go` for parse/encode/raw conflict behavior, and `internal/provider/client_qemu.go` for API key classification and form emission.

## Supervisor coordination

No supervisor decision needed.

```acceptance-report
{
  "criteriaSatisfied": [
    {
      "id": "criterion-1",
      "status": "satisfied",
      "evidence": "Scout-only research written to /home/indexyz/terraform-provider-proxmox/research/local-qemu-surface.md; no provider/source/docs files edited."
    },
    {
      "id": "criterion-2",
      "status": "satisfied",
      "evidence": "Findings include exact file paths/line ranges, supported typed fields, raw behavior, extension seams, tests/docs patterns, and residual risks."
    }
  ],
  "changedFiles": [
    "/home/indexyz/terraform-provider-proxmox/research/local-qemu-surface.md"
  ],
  "testsAddedOrUpdated": [],
  "commandsRun": [
    {
      "command": "ls .",
      "result": "passed",
      "summary": "Listed repository root."
    },
    {
      "command": "find *qemu* and internal Go files; grep proxmox_qemu_vm",
      "result": "passed",
      "summary": "Located QEMU provider source, tests, docs, examples, and README references."
    },
    {
      "command": "read selected QEMU schema/mapping/client/resource/data-source/test/doc/example files",
      "result": "passed",
      "summary": "Collected targeted code context and line ranges."
    }
  ],
  "validationOutput": [
    "Research file written successfully via write tool. No test execution requested; no code changes made."
  ],
  "residualRisks": [
    "Could not verify no staged files with git status because no bash/git tool is available in this scout environment.",
    "Docs are generated; future implementers should use the project doc-generation workflow for schema doc updates."
  ],
  "noStagedFiles": false,
  "diffSummary": "Added/overwrote research/local-qemu-surface.md with scout findings only.",
  "reviewFindings": [
    "no blockers in scoped research; no code review performed"
  ],
  "manualNotes": "Task requested no file modifications except authoritative research output path; complied."
}
```