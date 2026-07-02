# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_lxc_container" "example" {
  node       = "pve-1"
  vm_id      = 201
  hostname   = "terraform-lxc"
  ostemplate = "local:vztmpl/debian-12-standard_12.2-1_amd64.tar.zst"
  rootfs     = "local-lvm:8"
  memory     = 512
  swap       = 512
  cores      = 2
  onboot     = true
  protection = true

  network = {
    net0 = "name=eth0,bridge=vmbr0,ip=dhcp,type=veth"
  }

  raw = {
    extra_config = {
      "lxc.apparmor.profile" = "unconfined"
    }
  }
}
