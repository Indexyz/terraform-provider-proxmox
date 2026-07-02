# LXC Container Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add first-class `proxmox_lxc_container` resource and data source support.

**Architecture:** Add a separate LXC client/schema/mapping/resource/data-source surface mirroring QEMU style where appropriate, while keeping LXC-specific task wait and delete-parameter behavior local to LXC code. Expose common scalar fields, raw string maps for `net[n]` and `mp[n]`, and `raw.extra_config` for unsupported keys with unconditional conflict validation.

**Tech Stack:** Go, Terraform Plugin Framework, Proxmox LXC API, `go test`, `gofmt`, `tfplugindocs` generation.

## Global Constraints

- No legacy fallback.
- Keep typed fields and `raw.extra_config` as a single source of truth.
- Lyre audio topology is server relay only. Do not add peer mesh audio mode, peer-to-peer audio negotiation, or mesh compatibility fallbacks.
- Preserve lower-level error cause/context chains.
- Avoid over-engineering; make only directly required changes.
- Update `docs/roadmap.md` after code updates.
- Do not change QEMU behavior in this increment.
- LXC mutating calls wait for returned Proxmox UPID tasks using the approved spec contract.
- LXC update requests use comma-joined bare-key `delete` form parameters for removals.
- `ostemplate`, `rootfs`, `arch`, and `unprivileged` are replacement attributes and are not sent in update/delete parameters.

---

## File Structure

- Create `internal/provider/client_lxc.go`: LXC API structs, endpoints, decode/classification, encode, task wait.
- Create `internal/provider/lxc_container_schema.go`: Terraform model, raw nested model, resource/data source schema attributes.
- Create `internal/provider/lxc_container_mapping.go`: ID parsing, API-state mapping, request mapping, delete-parameter diffing, raw conflict validation.
- Create `internal/provider/resource_lxc_container.go`: resource CRUD/import/validation.
- Create `internal/provider/data_source_lxc_container.go`: data source read.
- Create tests: `client_lxc_test.go`, `lxc_container_mapping_test.go`, `data_source_lxc_container_test.go`, `resource_lxc_container_test.go`.
- Modify `internal/provider/provider.go` and `internal/provider/provider_unit_test.go`: register the new resource/data source.
- Create examples under `examples/resources/proxmox_lxc_container/` and `examples/data-sources/proxmox_lxc_container/`.
- Update generated docs under `docs/` and `docs/roadmap.md`.

### Task 1: LXC client and task wait

**Files:**
- Create: `internal/provider/client_lxc.go`
- Create: `internal/provider/client_lxc_test.go`

**Interfaces:**
- Produces: `LXCContainerConfig`, `LXCContainerStatus`, `CreateLXCContainerRequest`, `UpdateLXCContainerRequest`, `GetLXCContainerConfig`, `GetLXCContainerStatus`, `CreateLXCContainer`, `UpdateLXCContainer`, `DeleteLXCContainer`, `decodeLXCContainerConfig`, `encodeLXCContainerFields`.
- Consumes: existing `Client.do`, `proxmoxOptionalBool`, `proxmoxOptionalInt64`, `decodeQemuConfigStringValue`, `setOptionalString`, `setOptionalBool`, `setOptionalInt64`, `setSortedStringMap`, `errNotFound`.

- [ ] **Step 1: Write client tests**

Create `internal/provider/client_lxc_test.go` with tests:

- `TestClientLXCContainerMethods`: httptest server covers:
  - `GET /api2/json/nodes/pve-1/lxc/101/config` returns hostname, description, tags, arch, cores, memory, swap, onboot, protection, startup, unprivileged, features, ostype, rootfs, nameserver, searchdomain, timezone, `net0`, `mp0`, and `lxc.apparmor.profile`.
  - `GET /api2/json/nodes/pve-1/lxc/101/status/current` returns `status=running`, `uptime="300"`.
  - `POST /api2/json/nodes/pve-1/lxc` expects form values `vmid=101`, `ostemplate=local:vztmpl/debian-12.tar.zst`, typed scalar values, `net0`, `mp0`, and raw key `lxc.apparmor.profile`; returns data `UPID:pve-1:0001:create:101:`.
  - `PUT /api2/json/nodes/pve-1/lxc/101/config` expects changed form values including `hostname=ct-updated`, `memory=1024`, `net0=...`, `delete=tags,mp0`; returns data `UPID:pve-1:0002:update:101:`.
  - task status endpoints for both UPIDs return `status=stopped`, `exitstatus=OK`.
  - `DELETE /api2/json/nodes/pve-1/lxc/101` returns data `UPID:pve-1:0003:destroy:101:` and its task status returns OK.
  Assert config fields, status fields, and no unexpected requests.

- `TestDecodeLXCContainerConfigClassifiesMapsAndRaw`: call `decodeLXCContainerConfig` with `net0`, `mp0`, `rootfs`, and unknown `lxc.apparmor.profile`; assert net/mount/raw classification.
- `TestClientLXCContainerTaskFailure`: create/update/delete task status returns `exitstatus=ERROR: boom`; assert error contains the UPID and exit status.
- `TestClientLXCContainerNoTaskWhenUPIDMissing`: create/update/delete responses with `data: null`, absent `data`, and empty-string `data` complete without calling any `/tasks/` endpoint.
- `TestClientLXCContainerTaskContextCanceled`: context canceled before task stops; assert error preserves context cancellation.
- `TestClientLXCContainerTaskTimeoutCap`: temporarily set a package-level LXC task wait timeout cap to a few milliseconds, use a context with no deadline and a task that never stops, and assert the timeout error names the UPID without waiting 10 minutes.

- [ ] **Step 2: Run client tests to verify failure**

Run:

```bash
go test ./internal/provider -run 'Test(ClientLXCContainer|DecodeLXCContainerConfig)' -count=1
```

Expected: fail because LXC client types/functions do not exist.

- [ ] **Step 3: Implement client**

Create `client_lxc.go`:

- Define `LXCContainerConfig` with string fields `Hostname`, `Description`, `Tags`, `Arch`, `Startup`, `Features`, `OSType`, `RootFS`, `Nameserver`, `Searchdomain`, `Timezone`; optional bool fields `OnBoot`, `Protection`, `Unprivileged`; optional int fields `Cores`, `Memory`, `Swap`; maps `Network`, `MountPoint`, `ExtraConfig`.
- Define JSON known struct with keys `hostname`, `description`, `tags`, `arch`, `startup`, `features`, `ostype`, `rootfs`, `nameserver`, `searchdomain`, `timezone`, `onboot`, `protection`, `unprivileged`, `cores`, `memory`, `swap`.
- Define `LXCContainerStatus{Status string, Uptime proxmoxOptionalInt64}`.
- Define `lxcContainerConfigRequest` with pointer fields for typed scalars, `Network`, `MountPoint`, `ExtraConfig`, and `Delete []string`; create request adds `VMID int64` and `OSTemplate *string`.
- Implement `GetLXCContainerConfig`, `GetLXCContainerStatus`, `CreateLXCContainer`, `UpdateLXCContainer`, `DeleteLXCContainer` with `/nodes/%s/lxc` endpoints.
- Implement `decodeLXCContainerConfig`: known scalar keys stay typed, `net`+decimal goes to Network, `mp`+decimal goes to MountPoint, unsupported non-empty values go to ExtraConfig.
- Implement `encodeLXCContainerFields`: set typed fields, sorted Network/MountPoint/ExtraConfig, sorted comma-joined `delete` when non-empty.
- Implement response UPID handling so create/update/delete call `waitForLXCContainerTask(ctx,node,upid)` only when a non-empty UPID string is decoded from the Proxmox envelope `data`; `data: null`, absent `data`, and empty-string `data` return without polling.
- Implement package-level variables for task polling interval and timeout cap, defaulting to `2*time.Second` and `10*time.Minute`, so tests can shorten them without sleeping.
- Implement `waitForLXCContainerTask(ctx,node,upid)` with the configurable polling interval, configurable cap when context lacks earlier deadline, URL-path-escaped UPID, `GET /nodes/{node}/tasks/{upid}/status`, stopped+OK success, stopped+non-OK error, context/timeout errors preserving context.

- [ ] **Step 4: Run client verification**

Run:

```bash
gofmt -w internal/provider/client_lxc.go internal/provider/client_lxc_test.go
go test ./internal/provider -run 'Test(ClientLXCContainer|DecodeLXCContainerConfig)' -count=1
```

Expected: pass.

### Task 2: LXC schema and mapping

**Files:**
- Create: `internal/provider/lxc_container_schema.go`
- Create: `internal/provider/lxc_container_mapping.go`
- Create: `internal/provider/lxc_container_mapping_test.go`

**Interfaces:**
- Consumes: Task 1 client types.
- Produces: `lxcContainerModel`, `lxcContainerRawModel`, `lxcContainerResourceAttributes`, `lxcContainerDataSourceAttributes`, `lxcContainerStateFromAPI`, `lxcContainerCreateRequestFromModel`, `lxcContainerUpdateRequestFromModel`, `validateLXCContainerRawConflicts`, `parseLXCContainerImportID`.

- [ ] **Step 1: Write mapping/schema tests**

Create `internal/provider/lxc_container_mapping_test.go` with tests:

- `TestLXCContainerStateFromAPI`: maps API config/status into Terraform model, including prior `OSTemplate` and prior configured `RootFS` preservation, network/mount/raw maps, false defaults for omitted bools, status/uptime.
- `TestLXCContainerStateFromAPIUsesAPIRootFSWithoutPrior`: nil prior uses API `RootFS`.
- `TestLXCContainerRequestFromModel`: create request contains VMID, OSTemplate, rootfs, typed fields, maps, raw; update request excludes `OSTemplate`, `RootFS`, `Arch`, `Unprivileged` but includes updatable fields/maps/raw.
- `TestLXCContainerUpdateRequestDeletesRemovedKeys`: given prior with hostname/tags/network/mount/raw and plan with removals, update request `Delete` contains sorted bare keys such as `hostname`, `tags`, `net0`, `mp0`, `lxc.apparmor.profile`.
- `TestValidateLXCContainerRawConflictsReservesTypedKeys`: raw-only model with `rootfs`, `hostname`, `net0`, and `mp0` conflicts even when typed attributes/maps are omitted.
- `TestParseLXCContainerImportID`: accepts `pve-1/101`; rejects missing slash, empty node, empty VMID, and non-integer VMID.

- [ ] **Step 2: Run mapping tests to verify failure**

Run:

```bash
go test ./internal/provider -run 'Test(LXCContainerStateFromAPI|LXCContainerRequestFromModel|LXCContainerUpdateRequestDeletesRemovedKeys|ValidateLXCContainerRawConflicts|ParseLXCContainerImportID)' -count=1
```

Expected: fail because schema/mapping do not exist.

- [ ] **Step 3: Implement schema and mapping**

Create `lxc_container_schema.go`:

- Model fields: `ID`, `Node`, `VMID`, `OSTemplate`, `Hostname`, `Description`, `Tags`, `Arch`, `Cores`, `Memory`, `Swap`, `OnBoot`, `Protection`, `Startup`, `Unprivileged`, `Features`, `OSType`, `RootFS`, `Nameserver`, `Searchdomain`, `Timezone`, `Network`, `MountPoint`, `Raw`, `Status`, `Uptime`.
- Raw nested model with `ExtraConfig` map and attr types.
- Resource schema: `node` and `vm_id` required/replacement; `ostemplate`, `rootfs`, `arch`, `unprivileged` optional/computed replacement; updatable scalars optional/computed; maps optional/computed string maps; raw optional/computed nested; status/uptime computed.
- Data source schema: `node`, `vm_id` required; all other model fields computed.

Create `lxc_container_mapping.go`:

- `lxcContainerID` and `parseLXCContainerImportID` like QEMU.
- `lxcContainerStateFromAPI(ctx,node,vmID,config,status,prior)`: preserve prior `OSTemplate`; preserve prior `RootFS` when prior known; API rootfs otherwise; bool defaults for onboot/protection/unprivileged false; maps to Terraform map values/null; raw nested object.
- `lxcContainerCreateRequestFromModel(ctx, model)`: expand maps/raw and return create request including replacement fields.
- `lxcContainerUpdateRequestFromModel(ctx, plan, prior)`: return update request excluding replacement fields; include all updatable current known values and maps/raw; compute delete keys from prior-minus-plan.
- `validateLXCContainerRawConflicts`: always reject reserved top-level keys and keys matching `net`+decimal or `mp`+decimal in raw extra_config.

- [ ] **Step 4: Run mapping verification**

Run:

```bash
gofmt -w internal/provider/lxc_container_schema.go internal/provider/lxc_container_mapping.go internal/provider/lxc_container_mapping_test.go
go test ./internal/provider -run 'Test(LXCContainerStateFromAPI|LXCContainerRequestFromModel|LXCContainerUpdateRequestDeletesRemovedKeys|ValidateLXCContainerRawConflicts|ParseLXCContainerImportID)' -count=1
```

Expected: pass.

### Task 3: LXC resource, data source, and provider registration

**Files:**
- Create: `internal/provider/resource_lxc_container.go`
- Create: `internal/provider/data_source_lxc_container.go`
- Create: `internal/provider/resource_lxc_container_test.go`
- Create: `internal/provider/data_source_lxc_container_test.go`
- Modify: `internal/provider/provider.go`
- Modify: `internal/provider/provider_unit_test.go`

**Interfaces:**
- Consumes: Tasks 1-2 client/schema/mapping functions.
- Produces: `NewLXCContainerResource`, `NewLXCContainerDataSource` registered by provider.

- [ ] **Step 1: Write resource/data source/provider tests**

Tests:

- `TestLXCContainerResourceMetadata`: type name `proxmox_lxc_container`.
- `TestLXCContainerResourceReadState`: fake client returns config/status; `readLXCContainerState` returns state.
- `TestLXCContainerResourceReadStateMissing`: fake client 404 returns null ID; direct read removes state behavior can be tested through helper if simpler.
- `TestLXCContainerResourceSchemaAttributes` and `TestLXCContainerDataSourceSchemaAttributes`: required keys present (`node`, `vm_id`, `ostemplate`, `rootfs`, `network`, `mount_point`, `raw`, `status`, `uptime`).
- `TestLXCContainerDataSourceMetadata`: type name.
- Update `TestProviderExportsResourcesAndDataSources` expected lists to include `proxmox_lxc_container` in both resources and data sources.

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/provider -run 'Test(LXCContainer(Resource|DataSource)|ProviderExportsResourcesAndDataSources)' -count=1
```

Expected: fail because resource/data source/provider registration do not exist.

- [ ] **Step 3: Implement resource/data source/provider**

Create resource:

- Metadata/Scheme/Configure like QEMU with LXC names/descriptions.
- ValidateConfig calls `validateLXCContainerRawConflicts`.
- Create decodes plan, validates `ostemplate` and `rootfs` are known non-empty with attribute diagnostics, maps create request, calls `CreateLXCContainer`, then reads state with prior plan.
- Read loads state, calls helper; if refreshed ID null, removes resource.
- Update loads plan+state, checks current config exists, maps update request with prior state, calls update if not empty, reads refreshed state with prior plan/state as appropriate.
- Delete calls `DeleteLXCContainer` and tolerates `errNotFound`.
- Import parses `node/vmid` and sets id/node/vm_id.

Create data source:

- Metadata/Scheme/Configure like QEMU.
- Read gets config/status, maps state with nil prior, not-found diagnostic.

Register both in provider and update provider export tests.

- [ ] **Step 4: Run resource/provider verification**

Run:

```bash
gofmt -w internal/provider/resource_lxc_container.go internal/provider/data_source_lxc_container.go internal/provider/resource_lxc_container_test.go internal/provider/data_source_lxc_container_test.go internal/provider/provider.go internal/provider/provider_unit_test.go
go test ./internal/provider -run 'Test(LXCContainer(Resource|DataSource)|ProviderExportsResourcesAndDataSources)' -count=1
```

Expected: pass.

### Task 4: Examples, docs, roadmap, and final verification

**Files:**
- Create: `examples/resources/proxmox_lxc_container/resource.tf`
- Create: `examples/data-sources/proxmox_lxc_container/data-source.tf`
- Generated: `docs/resources/lxc_container.md`, `docs/data-sources/lxc_container.md`, `docs/index.md`
- Modify: `docs/roadmap.md`

**Interfaces:**
- Consumes: registered schema from Tasks 1-3.
- Produces: user-facing docs and roadmap update.

- [ ] **Step 1: Add examples**

Resource example:

```hcl
resource "proxmox_lxc_container" "example" {
  node       = "pve-1"
  vm_id      = 201
  hostname   = "terraform-lxc"
  ostemplate = "local:vztmpl/debian-12-standard_12.2-1_amd64.tar.zst"
  rootfs     = "local-lvm:8"
  memory     = 512
  swap       = 512
  cores      = 2
  onboot     = true
  protection = true

  network = {
    net0 = "name=eth0,bridge=vmbr0,ip=dhcp,type=veth"
  }

  raw = {
    extra_config = {
      "lxc.apparmor.profile" = "unconfined"
    }
  }
}
```

Data source example:

```hcl
data "proxmox_lxc_container" "example" {
  node  = "pve-1"
  vm_id = 201
}
```

- [ ] **Step 2: Generate docs**

Run:

```bash
make generate
```

If it fails only because `terraform` is not in PATH, run:

```bash
cd tools && go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-dir .. -provider-name proxmox
```

Keep only intended docs changes.

- [ ] **Step 3: Update roadmap**

Add completed bullet for LXC container resource/data source support. Update next steps to mention focused LXC follow-ups such as clone/power/snapshot/structured net/mp parsing, while keeping QEMU typed-field follow-up if still relevant.

- [ ] **Step 4: Final verification**

Run:

```bash
gofmt -w internal/provider/client_lxc.go internal/provider/client_lxc_test.go internal/provider/lxc_container_schema.go internal/provider/lxc_container_mapping.go internal/provider/lxc_container_mapping_test.go internal/provider/resource_lxc_container.go internal/provider/resource_lxc_container_test.go internal/provider/data_source_lxc_container.go internal/provider/data_source_lxc_container_test.go internal/provider/provider.go internal/provider/provider_unit_test.go
go test ./internal/provider -count=1
go build ./...
git diff --check -- . ':(exclude).gitignore'
```

Expected: all pass. `.gitignore` remains an unrelated pre-existing unstaged change and must not be staged.
