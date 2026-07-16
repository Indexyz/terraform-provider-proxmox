# Proxmox E2E CI Tools

These scripts support the GitHub Actions Proxmox e2e job. They prepare and boot a single-node Proxmox VE guest, then the provider runs one read-only smoke test against the real API.

The smoke test reads `proxmox_version`, `proxmox_nodes`, and the built-in `pam` realm, and requires the API to report Proxmox VE 9. It does not create resources or validate multi-node, ZFS, or external realm behavior.

## Pinned inputs and host requirements

- Proxmox VE ISO: `9.2-1`
- ISO URL: `https://enterprise.proxmox.com/iso/proxmox-ve_9.2-1.iso`
- ISO SHA256: `4e88fe416df9b527624a175f24c9aa07c714d3332afb1ee3dbf3879573ef2c6c`
- Auto-install assistant: `proxmox-auto-install-assistant_9.2.7_amd64.deb`
- Auto-install assistant SHA256: `92d34cd218bcabea83b72dd45f6e92ab571a221a8cbe1b548076685bc9234f15`
- Default guest resources: 2 vCPUs, 6144 MiB RAM, and a 32 GiB sparse qcow2 disk

Use a Linux x86_64 host with enough memory and disk space. The pinned assistant requires `libc6` >= 2.39 and `libssl3t64`; CI uses Ubuntu 24.04, and local runs should use Ubuntu 24.04 or Debian 13. Read/write access to `/dev/kvm` is strongly preferred. The scripts fall back to QEMU TCG when KVM is unavailable, but installation or boot can exceed the default timeouts.

## Local setup

Install the host packages on Debian or Ubuntu:

```bash
sudo apt-get update
sudo apt-get install -y qemu-system-x86 qemu-utils curl jq xorriso ca-certificates
```

Download, verify, and install the pinned auto-install assistant used by CI:

```bash
assistant_deb='proxmox-auto-install-assistant_9.2.7_amd64.deb'
curl --fail --location --retry 3 --output "$assistant_deb" \
  "http://download.proxmox.com/debian/pve/dists/trixie/pve-no-subscription/binary-amd64/$assistant_deb"
printf '%s  %s\n' \
  '92d34cd218bcabea83b72dd45f6e92ab571a221a8cbe1b548076685bc9234f15' \
  "$assistant_deb" | sha256sum --check
sudo apt-get install -y "./$assistant_deb"
```

Ensure the current user can read and write `/dev/kvm` if the device exists. The appropriate group or permission setup depends on the host distribution.

## Run the smoke test

From the repository root:

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

The preparation script reuses any non-empty `.e2e/proxmox/proxmox-e2e.qcow2`. The start script runs QEMU in the background, forwards `127.0.0.1:8006` to the guest API, records the process ID in `.e2e/proxmox/qemu.pid`, and accepts HTTP 200 or 401 as proof that the API endpoint is ready. The acceptance test then verifies authentication and the read-only version, nodes, and built-in realm API reads.

Use the exact `-run` selector above for this read-only CI smoke test. `make testacc` runs every acceptance test and requires an explicitly chosen Proxmox environment and credentials.

## Stop or rebuild the guest

Stop the background QEMU process after a local run:

```bash
pid_file='.e2e/proxmox/qemu.pid'
if test -s "$pid_file" && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
  kill "$(cat "$pid_file")"
fi
rm -f "$pid_file"
```

Local disk reuse does not implement the GitHub Actions cache key. Stop QEMU and remove the qcow2 file when changing pinned installer inputs, the root password, or installation behavior:

```bash
rm -f .e2e/proxmox/proxmox-e2e.qcow2
```

The next `prepare-proxmox-e2e-image.sh` run will download the ISO and rebuild the guest.

## Diagnostics

Logs are written under `${PROXMOX_E2E_WORK_DIR:-.e2e/proxmox}`. Collect the same diagnostics used by CI:

```bash
ls -l /dev/kvm || true
qemu-system-x86_64 --version
ps aux | grep '[q]emu-system-x86_64' || true
qemu-img info .e2e/proxmox/proxmox-e2e.qcow2 || true
tail -200 .e2e/proxmox/api-poll.log || true
tail -200 .e2e/proxmox/install.log || true
tail -200 .e2e/proxmox/boot.log || true
```

Common failure points:

- **Missing or inaccessible KVM:** confirm `/dev/kvm` permissions. TCG fallback is substantially slower.
- **ISO or assistant checksum failure:** remove the partial download, confirm the pinned version and checksum, and retry on a trusted network.
- **Installer timeout:** inspect `install.log`; the default limit is 4500 seconds.
- **Boot or API readiness timeout:** inspect `boot.log` and `api-poll.log`; the default limit is 1500 seconds with a 10-second poll interval.
- **Port 8006 already in use:** stop the conflicting process or set `PROXMOX_E2E_HOST_PORT` for the start script and use the same port in `PROXMOX_VE_ENDPOINT`.
- **Existing but invalid disk:** stop QEMU, remove the local qcow2 file, and rerun preparation.

GitHub Actions additionally supplies a clean runner, controls job-level timeouts, and caches the qcow2 with a key derived from pinned inputs and script hashes. Local runs do not reproduce those cache invalidation or runner lifecycle guarantees.

## Validate the CI tooling

The `tools` directory is a separate Go module. Run its script and workflow contract tests from that module:

```bash
(
  cd tools
  go test ./ci
)

bash -n tools/ci/prepare-proxmox-e2e-image.sh
bash -n tools/ci/start-proxmox-e2e.sh
```
