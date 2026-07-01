# GitHub Actions Proxmox E2E Design

## Goal

Run a real Proxmox VE instance inside a GitHub-hosted Linux runner, then execute a minimal Terraform provider e2e smoke test against its API. The workflow should prove that the provider can authenticate to Proxmox and read basic cluster data without depending on external Proxmox infrastructure.

## Scope

- Add GitHub Actions coverage for Proxmox-backed e2e tests in the existing `Tests` workflow.
- Start a single-node Proxmox VE guest on `ubuntu-latest` with QEMU/KVM.
- Add a minimal acceptance test that reads live Proxmox data through the provider.
- Keep the smoke test read-only to avoid managing VMs, pools, groups, storage, or other persistent resources.

Out of scope:

- Full VM lifecycle acceptance coverage.
- External Proxmox servers or repository secrets for managed infrastructure.
- Release workflow changes.
- Silent fallback to a mock Proxmox API.

## Architecture

The existing `Tests` workflow keeps fast build, lint, generate, and unit-test coverage. The current Terraform-version matrix job must be converted from an acceptance job into a normal unit test matrix by removing `TF_ACC=1` and renaming it accordingly. A new `e2e` job with `needs: build` prepares or restores a Proxmox VE guest disk, starts the guest on the GitHub-hosted runner, waits for the real Proxmox API, then runs the named provider e2e smoke test with `TF_ACC=1`.

Pinned Proxmox inputs:

- Proxmox VE ISO version: `8.4-1`.
- ISO URL: `https://enterprise.proxmox.com/iso/proxmox-ve_8.4-1.iso`.
- ISO SHA256: `d237d70ca48a9f6eb47f95fd4fd337722c3f69f8106393844d027d28c26523d8`.
- ISO checksum URL: `https://enterprise.proxmox.com/iso/proxmox-ve_8.4-1.iso.sha256`.
- Auto-install assistant package URL: `http://download.proxmox.com/debian/pve/dists/bookworm/pve-no-subscription/binary-amd64/proxmox-auto-install-assistant_8.2.5_amd64.deb`.
- Auto-install assistant SHA256: `47028ea31ef4463b6534e18aef3f296a29400ccc75d0d82cb296893864b09f15`.

The assistant package is downloaded directly and installed with `sudo apt-get install ./proxmox-auto-install-assistant_8.2.5_amd64.deb` after verifying the SHA256. The workflow must not add the Proxmox apt repository to the Ubuntu runner. If download, checksum verification, or package installation fails, the job must stop before attempting an interactive installer and print the failing command.

The implementation adds two repository-owned CI scripts under `tools/ci/`:

- `tools/ci/prepare-proxmox-e2e-image.sh`: downloads the pinned Proxmox VE ISO and checksum file, verifies the ISO SHA256, writes an unattended `answer.toml`, validates it with `proxmox-auto-install-assistant`, prepares an automated-install ISO, creates `proxmox-e2e.qcow2`, and runs the installer under QEMU when the cached disk image is missing.
- `tools/ci/start-proxmox-e2e.sh`: starts the installed `proxmox-e2e.qcow2` under QEMU without attaching the installer ISO, forwards host port `8006` to guest port `8006`, writes QEMU output to a log file, and polls the Proxmox `/version` API until it is ready or a timeout expires.

The workflow uses `actions/cache` for `proxmox-e2e.qcow2` keyed by the Proxmox ISO version, ISO SHA256, assistant version, assistant SHA256, `tools/ci/prepare-proxmox-e2e-image.sh`, and `tools/ci/start-proxmox-e2e.sh`. A cache miss performs the unattended install; a cache hit boots the prepared disk directly. Cache use is an optimization only: the job must still be able to build the disk from the pinned ISO without manual steps. The cache payload is expected to be a sparse qcow2 file backed by a 32 GiB virtual disk; compressed cache size is expected to stay below GitHub Actions cache limits, but the e2e job must still tolerate cache eviction by rebuilding.

The e2e job has these phases:

1. Check out the repository, install Go from `go.mod`, and install Terraform `1.15.*` with `hashicorp/setup-terraform`.
2. Install host packages needed by the scripts: `qemu-system-x86`, `qemu-utils`, `curl`, `jq`, `xorriso`, and `ca-certificates`.
3. Restore the cached `proxmox-e2e.qcow2` if present.
4. Download, checksum, and install `proxmox-auto-install-assistant_8.2.5_amd64.deb`.
5. Run `tools/ci/prepare-proxmox-e2e-image.sh` to create the disk image on cache miss.
6. Run `tools/ci/start-proxmox-e2e.sh` to boot the installed disk and wait for `https://127.0.0.1:8006/api2/json/version`.
7. Export provider environment variables:
   - `PROXMOX_VE_ENDPOINT=https://127.0.0.1:8006`
   - `PROXMOX_VE_USERNAME=root@pam`
   - `PROXMOX_VE_PASSWORD=proxmox-e2e-password`
   - `PROXMOX_VE_INSECURE=true`
   - `PROXMOX_VE_TIMEOUT=60`
8. Run `TF_ACC=1 go test -v -cover -timeout 120m -run '^TestAccProxmoxE2ESmoke$' ./internal/provider/`.
9. On failure, print `/dev/kvm` status, QEMU version, QEMU process details, API polling output, and QEMU serial log excerpts from a workflow step guarded with `if: always()`.

The e2e job timeout must be `120` minutes to allow a cache-miss install. The installer phase timeout must be `75` minutes. The boot/API readiness timeout must be `25` minutes with a `10` second poll interval. A cache-hit run is expected to finish much faster, but the same timeouts apply to keep behavior consistent.

## Proxmox Guest Configuration

Use the official Proxmox automated installer format. The generated `answer.toml` must be deterministic and must include:

```toml
[global]
country = "us"
fqdn = "pve-e2e.local"
mailto = "root@localhost"
timezone = "UTC"
root-password = "proxmox-e2e-password"
reboot-mode = "power-off"

[network]
source = "from-dhcp"

[disk-setup]
filesystem = "ext4"
lvm.swapsize = 0
lvm.maxvz = 0
disk-list = ['vda']
```

The QEMU guest uses a 32 GiB qcow2 virtual disk, 2 vCPUs, and 6144 MiB RAM. Both installation and runtime boot must attach the qcow2 system disk with a virtio block device, for example `-drive file="$DISK",format=qcow2,if=virtio`, so the installer sees it as `/dev/vda` and the `disk-list = ['vda']` answer file entry is valid. Installation boots from the prepared automated ISO and exits when Proxmox powers off after install. Runtime boot attaches only the qcow2 disk, not the ISO. Networking uses QEMU user-mode networking with host forwarding from `127.0.0.1:8006` to guest `:8006`. The scripts should prefer KVM acceleration when `/dev/kvm` is available. If KVM is unavailable, the scripts may try TCG software emulation but must keep the same bounded timeout and fail with diagnostics if the guest cannot become ready.

## Acceptance Test

Add one live smoke test in `internal/provider` using `github.com/hashicorp/terraform-plugin-testing/helper/resource`. The test must be named `TestAccProxmoxE2ESmoke` so the e2e job can select only that test with `-run '^TestAccProxmoxE2ESmoke$'`. It must use `testAccProtoV6ProviderFactories` and `testAccPreCheck`. Adding this dependency requires updating `go.mod` and `go.sum`.

The Terraform config must read only stable data sources:

```terraform
data "proxmox_version" "current" {}

data "proxmox_nodes" "current" {}
```

Assertions must verify:

- `data.proxmox_version.current.version` is set.
- `data.proxmox_nodes.current.nodes.#` is at least `1`.

The test must not create, update, or delete Proxmox resources.

Implement the environment validation by editing the existing empty `testAccPreCheck` in `internal/provider/provider_test.go`. `testAccPreCheck` must require these environment variables when acceptance tests run:

- `PROXMOX_VE_ENDPOINT`
- either `PROXMOX_VE_USERNAME` and `PROXMOX_VE_PASSWORD`, or `PROXMOX_VE_API_TOKEN_ID` and `PROXMOX_VE_API_TOKEN_SECRET`

Normal unit tests without `TF_ACC=1` must remain usable without a live Proxmox endpoint. The existing workflow matrix must run those normal tests without `TF_ACC=1`; only the new `e2e` job may set `TF_ACC=1`. The e2e job must use the exact `-run '^TestAccProxmoxE2ESmoke$'` selector so future resource-creating `TestAcc*` cases in `internal/provider` are not accidentally run against this read-only guest.

## Error Handling

The e2e job should fail loudly when Proxmox cannot run on the GitHub-hosted runner. It must not silently skip because a skipped e2e job would give a false signal.

Expected diagnostic checks:

- Show whether `/dev/kvm` exists and is readable/writable.
- Show QEMU version and process state.
- Show Proxmox API polling output.
- Show QEMU serial log excerpts from installation and boot.
- Fail if the API does not become ready within the configured timeout.

## Documentation Impact

Add a short `tools/ci/README.md` covering:

- What the Proxmox e2e scripts do.
- The pinned Proxmox ISO and assistant package versions.
- Local reproduction commands for `prepare-proxmox-e2e-image.sh`, `start-proxmox-e2e.sh`, and the `TF_ACC=1` Go test command.
- Cache invalidation inputs: ISO version/SHA, assistant version/SHA, and CI script hashes.
- Expected diagnostics when KVM, package installation, ISO verification, installation, boot, or API readiness fails.

The provider user-facing README and generated Terraform docs do not need updates because this change affects CI and development workflow only, not provider configuration or schema.

## Testing Strategy

Local verification before committing implementation:

- Run `go test ./internal/provider/` to ensure normal tests still pass without Proxmox.
- Run a targeted acceptance command only if a local Proxmox endpoint is available.
- Run `bash -n tools/ci/prepare-proxmox-e2e-image.sh tools/ci/start-proxmox-e2e.sh`.
- Validate workflow YAML syntax by parsing it with available local tooling.
- Run `go mod tidy` after adding `terraform-plugin-testing`.

CI verification:

- The e2e job itself is the end-to-end verification once pushed to GitHub Actions.
- The smoke test is intentionally read-only to reduce flakiness and cleanup risk.

## Tradeoffs

Running Proxmox inside a hosted runner keeps the workflow self-contained and avoids external secrets, but it is more complex and slower than using a pre-existing Proxmox server. Direct package installation on `ubuntu-latest` is avoided because Proxmox VE is built for a Debian/PVE environment and couples strongly to host system services and kernel details. The first CI run after a cache miss can be much slower because it must create a Proxmox guest disk from the ISO.

## Open Assumptions

- GitHub-hosted Linux runners expose enough virtualization support for QEMU/KVM to boot Proxmox within the workflow timeout.
- Proxmox VE `8.4-1` supports `proxmox-auto-install-assistant prepare-iso --fetch-from iso --answer-file`.
- `proxmox-auto-install-assistant_8.2.5_amd64.deb` installs successfully on the current `ubuntu-latest`; if not, implementation must fail with a clear package-install diagnostic rather than continuing with an interactive installer.
- If KVM is unavailable, software emulation may be too slow; the workflow should fail with diagnostics rather than pretending e2e passed.
