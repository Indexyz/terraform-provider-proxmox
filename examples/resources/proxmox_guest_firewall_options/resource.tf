# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_guest_firewall_options" "vm_101" {
  node       = "pve-1"
  vm_id      = 101
  guest_type = "qemu"
  enable     = true
  policy_in  = "DROP"
}
