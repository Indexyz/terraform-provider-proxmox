# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

data "proxmox_qemu_vm" "example" {
  node  = "pve-1"
  vm_id = 101
}
