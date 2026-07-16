#!/usr/bin/env bash
# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORK_DIR="${PROXMOX_E2E_WORK_DIR:-$ROOT_DIR/.e2e/proxmox}"
ISO_VERSION="${PROXMOX_E2E_ISO_VERSION:-9.2-1}"
ISO_URL="${PROXMOX_E2E_ISO_URL:-https://enterprise.proxmox.com/iso/proxmox-ve_9.2-1.iso}"
ISO_SHA256="${PROXMOX_E2E_ISO_SHA256:-4e88fe416df9b527624a175f24c9aa07c714d3332afb1ee3dbf3879573ef2c6c}"
ISO_CHECKSUM_URL="${PROXMOX_E2E_ISO_CHECKSUM_URL:-https://enterprise.proxmox.com/iso/proxmox-ve_9.2-1.iso.sha256}"
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
keyboard = "en-us"
timezone = "UTC"
root-password = "$ROOT_PASSWORD"
reboot-on-error = false

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
  -no-reboot \
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
