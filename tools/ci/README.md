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
