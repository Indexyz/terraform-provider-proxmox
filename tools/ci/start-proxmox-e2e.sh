#!/usr/bin/env bash
# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

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
API_RESPONSE_PATH="$WORK_DIR/api-response.json"
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
  rm -f "$API_RESPONSE_PATH"
  http_code="$(curl --insecure --silent --show-error --max-time 10 --output "$API_RESPONSE_PATH" --write-out '%{http_code}' "$API_URL" || true)"
  if [[ "$http_code" == "200" || "$http_code" == "401" ]]; then
    if [[ -s "$API_RESPONSE_PATH" ]]; then
      cat "$API_RESPONSE_PATH" >> "$POLL_LOG"
      printf '\n' >> "$POLL_LOG"
    fi
    log "Proxmox API is ready (HTTP $http_code)"
    exit 0
  fi
  printf '[%(%Y-%m-%dT%H:%M:%SZ)T] API not ready yet (HTTP %s)\n' -1 "$http_code" | tee -a "$POLL_LOG" >/dev/null
  sleep "$POLL_INTERVAL_SECONDS"
done

log "Proxmox API did not become ready within ${BOOT_TIMEOUT_SECONDS}s"
if [[ -f "$PID_FILE" ]]; then
  ps -fp "$(cat "$PID_FILE")" || true
fi
tail -200 "$BOOT_LOG" || true
tail -200 "$POLL_LOG" || true
exit 1
