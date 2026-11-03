# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# `state` and `pending` reveal configuration that is staged but not committed.
data "proxmox_sdn_zones" "vlan" {
  type = "vlan"
}

output "pending_zones" {
  value = { for zone in data.proxmox_sdn_zones.vlan.zones : zone.zone => zone.state }
}
