# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

data "proxmox_lxc_container" "example" {
  node  = "pve-1"
  vm_id = 201
}
