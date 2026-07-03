# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_qemu_snapshot" "pre_deploy" {
  node        = "pve-1"
  vm_id       = 101
  name        = "pre-deploy"
  description = "Snapshot before deploying changes"
}
