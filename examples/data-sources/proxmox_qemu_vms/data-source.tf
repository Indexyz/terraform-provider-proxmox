# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Find a QEMU template by name and clone from it. The `template` flag comes
# from `/cluster/resources`, so a template created within the last minute may
# not appear until Proxmox publishes its RRD stats.
data "proxmox_qemu_vms" "ubuntu_template" {
  template = true
  name     = "ubuntu-24.04-cloudinit"
}

resource "proxmox_qemu_vm" "app" {
  node  = one(data.proxmox_qemu_vms.ubuntu_template.vms[*].node)
  vm_id = 999

  clone = {
    source_vmid = one(data.proxmox_qemu_vms.ubuntu_template.vms[*].vm_id)
    full        = true
  }
}
