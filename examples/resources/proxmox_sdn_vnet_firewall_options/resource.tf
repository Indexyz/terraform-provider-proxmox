# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# VNet firewall options apply immediately, unlike the staged SDN objects.
# Credentials need SDN.Allocate on the VNet.
resource "proxmox_sdn_vnet_firewall_options" "app" {
  vnet              = proxmox_sdn_vnet.app.vnet
  enable            = true
  policy_forward    = "DROP"
  log_level_forward = "info"
}
