# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

data "proxmox_sdn_vnets" "app" {
  zone = "vz1"
}

output "app_vnets" {
  value = [for vnet in data.proxmox_sdn_vnets.app.vnets : vnet.vnet]
}
