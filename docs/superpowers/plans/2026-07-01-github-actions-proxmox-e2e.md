# GitHub Actions Proxmox E2E Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a self-contained GitHub Actions e2e job that boots a real Proxmox VE guest on a hosted runner and runs one read-only provider smoke test against it.

**Architecture:** Keep the existing build/generate/unit-test workflow fast, move `TF_ACC=1` into a dedicated `e2e` job, and add repository-owned scripts under `tools/ci/` to prepare/cache and boot a pinned Proxmox VE qcow2 image. The provider test is a single `TestAccProxmoxE2ESmoke` selected by an exact `-run` filter so future acceptance tests do not run accidentally.

**Tech Stack:** GitHub Actions YAML, Bash, QEMU/KVM, Proxmox VE automated installer, Go 1.26.4, Terraform Plugin Framework, `github.com/hashicorp/terraform-plugin-testing v1.16.0`.

## Global Constraints

- Workflow file: `.github/workflows/test.yml`.
- Provider test package: `internal/provider`.
- Proxmox VE ISO version: `8.4-1`.
- ISO URL: `https://enterprise.proxmox.com/iso/proxmox-ve_8.4-1.iso`.
- ISO SHA256: `d237d70ca48a9f6eb47f95fd4fd337722c3f69f8106393844d027d28c26523d8`.
- ISO checksum URL: `https://enterprise.proxmox.com/iso/proxmox-ve_8.4-1.iso.sha256`.
- Auto-install assistant package URL: `http://download.proxmox.com/debian/pve/dists/bookworm/pve-no-subscription/binary-amd64/proxmox-auto-install-assistant_8.2.5_amd64.deb`.
- Auto-install assistant SHA256: `47028ea31ef4463b6534e18aef3f296a29400ccc75d0d82cb296893864b09f15`.
- QEMU guest resources: 32 GiB qcow2 disk, 2 vCPUs, 6144 MiB RAM.
- QEMU system disk bus: virtio, so the Proxmox installer sees `/dev/vda`.
- Proxmox root password for CI guest: `proxmox-e2e-password`.
- Proxmox provider env in e2e job: `PROXMOX_VE_ENDPOINT=https://127.0.0.1:8006`, `PROXMOX_VE_USERNAME=root@pam`, `PROXMOX_VE_PASSWORD=proxmox-e2e-password`, `PROXMOX_VE_INSECURE=true`, `PROXMOX_VE_TIMEOUT=60`.
- E2E Go test command: `TF_ACC=1 go test -v -cover -timeout 120m -run '^TestAccProxmoxE2ESmoke$' ./internal/provider/`.
- E2E job timeout: `120` minutes.
- Installer phase timeout: `75` minutes.
- Boot/API readiness timeout: `25` minutes.
- API poll interval: `10` seconds.
- The workflow must fail loudly rather than silently skip if Proxmox cannot start.
- The workflow must not add the Proxmox apt repository to the Ubuntu runner.
- The workflow must not silently fall back to a mock Proxmox API.
- The smoke test must not create, update, or delete Proxmox resources.
- Normal unit tests without `TF_ACC=1` must remain usable without a live Proxmox endpoint.

---

## File Structure

- Modify `.github/workflows/test.yml`: rename the current Terraform matrix job to unit tests, remove `TF_ACC=1`, add `actions/cache`, add a new `e2e` job with Proxmox setup, exact smoke-test selector, and `if: always()` diagnostics.
- Modify `go.mod` and `go.sum`: add `github.com/hashicorp/terraform-plugin-testing v1.16.0` and tidy transitive dependencies.
- Modify `internal/provider/provider_test.go`: implement `testAccPreCheck` environment validation.
- Create `internal/provider/e2e_smoke_test.go`: add read-only `TestAccProxmoxE2ESmoke` using `resource.Test`.
- Create `tools/ci/prepare-proxmox-e2e-image.sh`: prepare or reuse the Proxmox e2e qcow2 disk from the pinned ISO.
- Create `tools/ci/start-proxmox-e2e.sh`: boot the prepared disk, expose host port `8006`, poll the Proxmox API, and write diagnostics logs.
- Create `tools/ci/README.md`: document local reproduction, pinned inputs, cache invalidation, and troubleshooting.

---

### Task 1: Add Read-Only Provider E2E Smoke Test

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `internal/provider/provider_test.go`
- Create: `internal/provider/e2e_smoke_test.go`

**Interfaces:**
- Consumes: existing `testAccProtoV6ProviderFactories` from `internal/provider/provider_test.go`.
- Produces: `func TestAccProxmoxE2ESmoke(t *testing.T)` selected by CI with `-run '^TestAccProxmoxE2ESmoke$'`.
- Produces: `testAccPreCheck(t *testing.T)` that fails only when acceptance tests run without required Proxmox provider env vars.

- [ ] **Step 1: Add the Terraform Plugin Testing dependency**

Run:

```bash
go get github.com/hashicorp/terraform-plugin-testing@v1.16.0
```

Expected: `go.mod` contains `github.com/hashicorp/terraform-plugin-testing v1.16.0` either directly or through tidy-adjusted requirements.

- [ ] **Step 2: Implement `testAccPreCheck` env validation**

Edit `internal/provider/provider_test.go` to import `os` and replace the empty function with:

```go
func testAccPreCheck(t *testing.T) {
	t.Helper()

	if os.Getenv(envEndpoint) == "" {
		t.Fatalf("%s must be set for acceptance tests", envEndpoint)
	}

	passwordAuthConfigured := os.Getenv(envUsername) != "" && os.Getenv(envPassword) != ""
	tokenAuthConfigured := os.Getenv(envAPITokenID) != "" && os.Getenv(envAPITokenSecret) != ""
	if !passwordAuthConfigured && !tokenAuthConfigured {
		t.Fatalf("set either %s/%s or %s/%s for acceptance tests", envUsername, envPassword, envAPITokenID, envAPITokenSecret)
	}
}
```

- [ ] **Step 3: Add the smoke acceptance test**

Create `internal/provider/e2e_smoke_test.go` with:

```go
// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccProxmoxE2ESmoke(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: `
data "proxmox_version" "current" {}

data "proxmox_nodes" "current" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.proxmox_version.current", "version"),
					resource.TestCheckResourceAttrWith("data.proxmox_nodes.current", "nodes.#", func(value string) error {
						count, err := strconv.Atoi(value)
						if err != nil {
							return fmt.Errorf("nodes.# is not an integer: %w", err)
						}
						if count < 1 {
							return fmt.Errorf("expected at least one Proxmox node, got %d", count)
						}
						return nil
					}),
				),
			},
		},
	})
}
```

- [ ] **Step 4: Tidy modules**

Run:

```bash
go mod tidy
```

Expected: command exits `0`; `go.mod` and `go.sum` are updated consistently.

- [ ] **Step 5: Run unit tests without Proxmox env**

Run:

```bash
go test ./internal/provider/
```

Expected: command exits `0`; `TestAccProxmoxE2ESmoke` is skipped by Terraform Plugin Testing because `TF_ACC` is not set.

- [ ] **Step 6: Verify the acceptance precheck fails loudly without env**

Run:

```bash
TF_ACC=1 go test -run '^TestAccProxmoxE2ESmoke$' ./internal/provider/
```

Expected: command exits non-zero and includes `PROXMOX_VE_ENDPOINT must be set for acceptance tests`.

---

### Task 2: Add Proxmox E2E CI Scripts and Developer Docs

**Files:**
- Create: `tools/ci/prepare-proxmox-e2e-image.sh`
- Create: `tools/ci/start-proxmox-e2e.sh`
- Create: `tools/ci/README.md`

**Interfaces:**
- Produces: `tools/ci/prepare-proxmox-e2e-image.sh` that accepts environment overrides but defaults to pinned spec values.
- Produces: `tools/ci/start-proxmox-e2e.sh` that writes logs under `.e2e/proxmox/`, starts QEMU in the background, and exits only when Proxmox API is reachable or timeout expires.
- Produces: docs consumed by maintainers and referenced from CI diagnostics.

- [ ] **Step 1: Create the prepare script**

Create `tools/ci/prepare-proxmox-e2e-image.sh` with:

```bash
#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORK_DIR="${PROXMOX_E2E_WORK_DIR:-$ROOT_DIR/.e2e/proxmox}"
ISO_VERSION="${PROXMOX_E2E_ISO_VERSION:-8.4-1}"
ISO_URL="${PROXMOX_E2E_ISO_URL:-https://enterprise.proxmox.com/iso/proxmox-ve_8.4-1.iso}"
ISO_SHA256="${PROXMOX_E2E_ISO_SHA256:-d237d70ca48a9f6eb47f95fd4fd337722c3f69f8106393844d027d28c26523d8}"
ISO_CHECKSUM_URL="${PROXMOX_E2E_ISO_CHECKSUM_URL:-https://enterprise.proxmox.com/iso/proxmox-ve_8.4-1.iso.sha256}"
DISK_PATH="${PROXMOX_E2E_DISK_PATH:-$WORK_DIR/proxmox-e2e.qcow2}"
DISK_SIZE="${PROXMOX_E2E_DISK_SIZE:-32G}"
RAM_MB="${PROXMOX_E2E_RAM_MB:-6144}"
CPUS="${PROXMOX_E2E_CPUS:-2}"
INSTALL_TIMEOUT_SECONDS="${PROXMOX_E2E_INSTALL_TIMEOUT_SECONDS:-4500}"
ROOT_PASSWORD="${PROXMOX_E2E_ROOT_PASSWORD:-proxmox-e2e-password}"

ISO_PATH="$WORK_DIR/proxmox-ve_${ISO_VERSION}.iso"
CHECKSUM_PATH="$WORK_DIR/proxmox-ve_${ISO_VERSION}.iso.sha256"
ANSWER_PATH="$WORK_DIR/answer.toml"
PREPARED_ISO_PATH="$WORK_DIR/proxmox-ve_${ISO_VERSION}-auto.iso"
INSTALL_LOG="$WORK_DIR/install.log"

log() {
  printf '[prepare-proxmox-e2e] %s\n' "$*"
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

accel_args() {
  if [[ -r /dev/kvm && -w /dev/kvm ]]; then
    printf '%s\n' -enable-kvm
  else
    printf '%s\n' -accel tcg
  fi
}

cpu_args() {
  if [[ -r /dev/kvm && -w /dev/kvm ]]; then
    printf '%s\n' '-cpu host'
  else
    printf '%s\n' '-cpu max'
  fi
}

mkdir -p "$WORK_DIR"

for cmd in curl sha256sum qemu-img qemu-system-x86_64 timeout proxmox-auto-install-assistant; do
  require_command "$cmd"
done

if [[ -s "$DISK_PATH" ]]; then
  log "using existing disk image: $DISK_PATH"
  qemu-img info "$DISK_PATH"
  exit 0
fi

log "downloading Proxmox VE ISO checksum from $ISO_CHECKSUM_URL"
curl --fail --location --retry 3 --output "$CHECKSUM_PATH" "$ISO_CHECKSUM_URL"
log "downloading Proxmox VE ISO from $ISO_URL"
curl --fail --location --retry 3 --output "$ISO_PATH" "$ISO_URL"
printf '%s  %s\n' "$ISO_SHA256" "$ISO_PATH" | sha256sum --check --status
log "verified ISO sha256"

cat > "$ANSWER_PATH" <<EOF_ANSWER
[global]
country = "us"
fqdn = "pve-e2e.local"
mailto = "root@localhost"
timezone = "UTC"
root-password = "$ROOT_PASSWORD"
reboot-mode = "power-off"

[network]
source = "from-dhcp"

[disk-setup]
filesystem = "ext4"
lvm.swapsize = 0
lvm.maxvz = 0
disk-list = ['vda']
EOF_ANSWER

proxmox-auto-install-assistant validate-answer "$ANSWER_PATH"
rm -f "$PREPARED_ISO_PATH"
proxmox-auto-install-assistant prepare-iso "$ISO_PATH" --fetch-from iso --answer-file "$ANSWER_PATH" --output "$PREPARED_ISO_PATH"

log "creating $DISK_SIZE qcow2 disk at $DISK_PATH"
qemu-img create -f qcow2 "$DISK_PATH" "$DISK_SIZE"

log "booting unattended installer; log: $INSTALL_LOG"
set +e
timeout "$INSTALL_TIMEOUT_SECONDS" qemu-system-x86_64 \
  $(accel_args) \
  -m "$RAM_MB" \
  -smp "$CPUS" \
  -machine q35 \
  $(cpu_args) \
  -display none \
  -serial file:"$INSTALL_LOG" \
  -drive file="$DISK_PATH",format=qcow2,if=virtio \
  -cdrom "$PREPARED_ISO_PATH" \
  -boot d \
  -netdev user,id=net0 \
  -device virtio-net-pci,netdev=net0
status=$?
set -e

if [[ "$status" -ne 0 ]]; then
  log "installer failed or timed out with status $status"
  tail -200 "$INSTALL_LOG" || true
  exit "$status"
fi

log "installer completed"
qemu-img info "$DISK_PATH"
```

- [ ] **Step 2: Create the start script**

Create `tools/ci/start-proxmox-e2e.sh` with:

```bash
#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORK_DIR="${PROXMOX_E2E_WORK_DIR:-$ROOT_DIR/.e2e/proxmox}"
DISK_PATH="${PROXMOX_E2E_DISK_PATH:-$WORK_DIR/proxmox-e2e.qcow2}"
RAM_MB="${PROXMOX_E2E_RAM_MB:-6144}"
CPUS="${PROXMOX_E2E_CPUS:-2}"
BOOT_TIMEOUT_SECONDS="${PROXMOX_E2E_BOOT_TIMEOUT_SECONDS:-1500}"
POLL_INTERVAL_SECONDS="${PROXMOX_E2E_POLL_INTERVAL_SECONDS:-10}"
HOST_PORT="${PROXMOX_E2E_HOST_PORT:-8006}"
BOOT_LOG="$WORK_DIR/boot.log"
POLL_LOG="$WORK_DIR/api-poll.log"
PID_FILE="$WORK_DIR/qemu.pid"
API_URL="https://127.0.0.1:${HOST_PORT}/api2/json/version"

log() {
  printf '[start-proxmox-e2e] %s\n' "$*"
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

accel_args() {
  if [[ -r /dev/kvm && -w /dev/kvm ]]; then
    printf '%s\n' -enable-kvm
  else
    printf '%s\n' -accel tcg
  fi
}

cpu_args() {
  if [[ -r /dev/kvm && -w /dev/kvm ]]; then
    printf '%s\n' '-cpu host'
  else
    printf '%s\n' '-cpu max'
  fi
}

mkdir -p "$WORK_DIR"
for cmd in curl qemu-img qemu-system-x86_64; do
  require_command "$cmd"
done

if [[ ! -s "$DISK_PATH" ]]; then
  printf 'missing Proxmox e2e disk image: %s\n' "$DISK_PATH" >&2
  exit 1
fi

if [[ -f "$PID_FILE" ]] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
  log "QEMU already running with pid $(cat "$PID_FILE")"
else
  rm -f "$BOOT_LOG" "$POLL_LOG" "$PID_FILE"
  log "starting QEMU; log: $BOOT_LOG"
  qemu-system-x86_64 \
    $(accel_args) \
    -m "$RAM_MB" \
    -smp "$CPUS" \
    -machine q35 \
    $(cpu_args) \
    -display none \
    -serial file:"$BOOT_LOG" \
    -daemonize \
    -pidfile "$PID_FILE" \
    -drive file="$DISK_PATH",format=qcow2,if=virtio \
    -netdev user,id=net0,hostfwd=tcp:127.0.0.1:${HOST_PORT}-:8006 \
    -device virtio-net-pci,netdev=net0
fi

log "waiting for Proxmox API at $API_URL"
deadline=$((SECONDS + BOOT_TIMEOUT_SECONDS))
while (( SECONDS < deadline )); do
  if curl --insecure --silent --show-error --fail --max-time 10 "$API_URL" | tee -a "$POLL_LOG" >/dev/null; then
    log "Proxmox API is ready"
    exit 0
  fi
  printf '[%(%Y-%m-%dT%H:%M:%SZ)T] API not ready yet\n' -1 | tee -a "$POLL_LOG" >/dev/null
  sleep "$POLL_INTERVAL_SECONDS"
done

log "Proxmox API did not become ready within ${BOOT_TIMEOUT_SECONDS}s"
if [[ -f "$PID_FILE" ]]; then
  ps -fp "$(cat "$PID_FILE")" || true
fi
tail -200 "$BOOT_LOG" || true
tail -200 "$POLL_LOG" || true
exit 1
```

- [ ] **Step 3: Mark scripts executable**

Run:

```bash
chmod +x tools/ci/prepare-proxmox-e2e-image.sh tools/ci/start-proxmox-e2e.sh
```

Expected: both files have executable mode in `git diff --summary`.

- [ ] **Step 4: Add CI script documentation**

Create `tools/ci/README.md` with:

```markdown
# Proxmox E2E CI Tools

These scripts support the GitHub Actions Proxmox e2e job. They prepare and boot a single-node Proxmox VE guest on a GitHub-hosted Linux runner, then the workflow runs one read-only provider smoke test against the real Proxmox API.

## Pinned Inputs

- Proxmox VE ISO: `8.4-1`
- ISO URL: `https://enterprise.proxmox.com/iso/proxmox-ve_8.4-1.iso`
- ISO SHA256: `d237d70ca48a9f6eb47f95fd4fd337722c3f69f8106393844d027d28c26523d8`
- Auto-install assistant: `proxmox-auto-install-assistant_8.2.5_amd64.deb`
- Auto-install assistant SHA256: `47028ea31ef4463b6534e18aef3f296a29400ccc75d0d82cb296893864b09f15`

## Local Usage

Install host dependencies first:

```bash
sudo apt-get update
sudo apt-get install -y qemu-system-x86 qemu-utils curl jq xorriso ca-certificates
```

Install the pinned auto-install assistant package after verifying its SHA256, then run:

```bash
tools/ci/prepare-proxmox-e2e-image.sh
tools/ci/start-proxmox-e2e.sh
PROXMOX_VE_ENDPOINT=https://127.0.0.1:8006 \
PROXMOX_VE_USERNAME=root@pam \
PROXMOX_VE_PASSWORD=proxmox-e2e-password \
PROXMOX_VE_INSECURE=true \
PROXMOX_VE_TIMEOUT=60 \
TF_ACC=1 go test -v -cover -timeout 120m -run '^TestAccProxmoxE2ESmoke$' ./internal/provider/
```

## Cache Behavior

GitHub Actions caches `.e2e/proxmox/proxmox-e2e.qcow2`. The cache key includes the ISO version, ISO SHA256, assistant version, assistant SHA256, and CI script hashes. Cache misses rebuild the qcow2 disk from the pinned ISO.

## Diagnostics

The workflow prints `/dev/kvm` status, QEMU version, QEMU process details, API polling output, and QEMU serial log excerpts when the e2e job fails. Common failure points are missing KVM access, package installation failure, ISO checksum mismatch, installer timeout, boot timeout, or Proxmox API readiness timeout.
```

- [ ] **Step 5: Validate shell syntax**

Run:

```bash
bash -n tools/ci/prepare-proxmox-e2e-image.sh tools/ci/start-proxmox-e2e.sh
```

Expected: command exits `0`.

---

### Task 3: Wire GitHub Actions Workflow

**Files:**
- Modify: `.github/workflows/test.yml`

**Interfaces:**
- Consumes: `tools/ci/prepare-proxmox-e2e-image.sh` and `tools/ci/start-proxmox-e2e.sh` from Task 2.
- Consumes: `TestAccProxmoxE2ESmoke` from Task 1.
- Produces: `test` job that runs normal tests without `TF_ACC=1`.
- Produces: `e2e` job that runs only the Proxmox smoke test with `TF_ACC=1`.

- [ ] **Step 1: Rename the current test matrix job and remove `TF_ACC`**

In `.github/workflows/test.yml`, change the current `test` job name from:

```yaml
name: 'Acceptance Tests: Terraform ${{ matrix.terraform }}'
```

to:

```yaml
name: 'Unit Tests: Terraform ${{ matrix.terraform }}'
```

Replace the final test step:

```yaml
      - env:
          TF_ACC: "1"
        run: go test -v -cover ./internal/provider/
        timeout-minutes: 10
```

with:

```yaml
      - run: go test -v -cover ./internal/provider/
        timeout-minutes: 10
```

- [ ] **Step 2: Add the Proxmox e2e job**

Append this job under `jobs:` in `.github/workflows/test.yml`:

```yaml
  e2e:
    name: Proxmox E2E Smoke
    needs: build
    runs-on: ubuntu-latest
    timeout-minutes: 120
    env:
      PROXMOX_E2E_WORK_DIR: .e2e/proxmox
      PROXMOX_E2E_ISO_VERSION: '8.4-1'
      PROXMOX_E2E_ISO_SHA256: d237d70ca48a9f6eb47f95fd4fd337722c3f69f8106393844d027d28c26523d8
      PROXMOX_E2E_ASSISTANT_VERSION: '8.2.5'
      PROXMOX_E2E_ASSISTANT_SHA256: 47028ea31ef4463b6534e18aef3f296a29400ccc75d0d82cb296893864b09f15
      PROXMOX_E2E_ROOT_PASSWORD: proxmox-e2e-password
    steps:
      - uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0
      - uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6.5.0
        with:
          go-version-file: 'go.mod'
          cache: true
      - uses: hashicorp/setup-terraform@dfe3c3f87815947d99a8997f908cb6525fc44e9e # v4.0.1
        with:
          terraform_version: '1.15.*'
          terraform_wrapper: false
      - name: Install QEMU dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y qemu-system-x86 qemu-utils curl jq xorriso ca-certificates
      - name: Install Proxmox auto-install assistant
        run: |
          assistant_deb="proxmox-auto-install-assistant_${PROXMOX_E2E_ASSISTANT_VERSION}_amd64.deb"
          curl --fail --location --retry 3 --output "$assistant_deb" \
            "http://download.proxmox.com/debian/pve/dists/bookworm/pve-no-subscription/binary-amd64/$assistant_deb"
          printf '%s  %s\n' "$PROXMOX_E2E_ASSISTANT_SHA256" "$assistant_deb" | sha256sum --check --status
          sudo apt-get install -y "./$assistant_deb"
      - name: Restore Proxmox disk cache
        uses: actions/cache@0400d5f644dc74513175e3cd8d07132dd4860809 # v4.2.4
        with:
          path: .e2e/proxmox/proxmox-e2e.qcow2
          key: proxmox-e2e-${{ runner.os }}-${{ env.PROXMOX_E2E_ISO_VERSION }}-${{ env.PROXMOX_E2E_ISO_SHA256 }}-${{ env.PROXMOX_E2E_ASSISTANT_VERSION }}-${{ env.PROXMOX_E2E_ASSISTANT_SHA256 }}-${{ hashFiles('tools/ci/prepare-proxmox-e2e-image.sh', 'tools/ci/start-proxmox-e2e.sh') }}
      - name: Prepare Proxmox disk image
        run: tools/ci/prepare-proxmox-e2e-image.sh
        timeout-minutes: 75
      - name: Start Proxmox
        run: tools/ci/start-proxmox-e2e.sh
        timeout-minutes: 25
      - name: Run Proxmox e2e smoke test
        env:
          TF_ACC: "1"
          PROXMOX_VE_ENDPOINT: https://127.0.0.1:8006
          PROXMOX_VE_USERNAME: root@pam
          PROXMOX_VE_PASSWORD: proxmox-e2e-password
          PROXMOX_VE_INSECURE: "true"
          PROXMOX_VE_TIMEOUT: "60"
        run: go test -v -cover -timeout 120m -run '^TestAccProxmoxE2ESmoke$' ./internal/provider/
      - name: Print Proxmox e2e diagnostics
        if: always()
        run: |
          set +e
          echo "== /dev/kvm =="
          ls -l /dev/kvm || true
          test -r /dev/kvm && test -w /dev/kvm && echo "/dev/kvm is readable and writable" || true
          echo "== qemu version =="
          qemu-system-x86_64 --version || true
          echo "== qemu processes =="
          ps aux | grep '[q]emu-system-x86_64' || true
          echo "== disk info =="
          qemu-img info .e2e/proxmox/proxmox-e2e.qcow2 || true
          echo "== api poll log =="
          tail -200 .e2e/proxmox/api-poll.log || true
          echo "== install log =="
          tail -200 .e2e/proxmox/install.log || true
          echo "== boot log =="
          tail -200 .e2e/proxmox/boot.log || true
```

- [ ] **Step 3: Validate workflow YAML parseability**

Run:

```bash
python3 - <<'PY'
from pathlib import Path
import sys
try:
    import yaml
except ImportError:
    print('PyYAML unavailable; falling back to Ruby psych')
    sys.exit(2)
for path in Path('.github/workflows').glob('*.yml'):
    yaml.safe_load(path.read_text())
    print(path)
PY
```

If PyYAML is unavailable, run:

```bash
ruby -e 'require "yaml"; ARGV.each { |path| YAML.load_file(path); puts path }' .github/workflows/*.yml
```

Expected: every workflow YAML file parses without error.

- [ ] **Step 4: Run full local verification available without Proxmox**

Run:

```bash
go test ./internal/provider/
bash -n tools/ci/prepare-proxmox-e2e-image.sh tools/ci/start-proxmox-e2e.sh
TF_ACC=1 go test -run '^TestAccProxmoxE2ESmoke$' ./internal/provider/
```

Expected:

- `go test ./internal/provider/` exits `0`.
- `bash -n tools/ci/prepare-proxmox-e2e-image.sh tools/ci/start-proxmox-e2e.sh` exits `0`.
- The `TF_ACC=1` command exits non-zero with `PROXMOX_VE_ENDPOINT must be set for acceptance tests`, proving the smoke test fails loudly without a Proxmox endpoint.

---

## Plan Self-Review

- Spec coverage: Task 1 covers the live read-only smoke test and `testAccPreCheck`; Task 2 covers QEMU/Proxmox scripts, pinned inputs, resources, timeouts, diagnostics, and docs; Task 3 covers workflow split, cache key, e2e job, exact test selector, and diagnostics step.
- Incomplete-marker scan: no incomplete instructions are intended; all pinned values and commands are explicit.
- Type consistency: test name `TestAccProxmoxE2ESmoke` matches the workflow `-run` selector; disk `vda` matches `if=virtio`; cache key includes ISO version/SHA and assistant version/SHA.
