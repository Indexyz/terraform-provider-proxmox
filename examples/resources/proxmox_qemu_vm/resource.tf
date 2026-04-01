# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_qemu_vm" "example" {
  node        = "pve-1"
  vm_id       = 101
  name        = "terraform-qemu-vm"
  description = "Managed by Terraform"
  tags        = "terraform,example"
  onboot      = true
  startup     = "order=1"
  bios        = "ovmf"
  machine     = "q35"
  agent       = "enabled=1"
  cores       = 2
  sockets     = 1
  memory      = 2048
  cpu         = "host"
  ostype      = "l26"
  boot        = "order=scsi0;net0"
}
