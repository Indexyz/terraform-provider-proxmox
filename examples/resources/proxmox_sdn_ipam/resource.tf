# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# IPAMs stay staged until proxmox_sdn_apply commits the SDN configuration.
# Provider credentials need SDN.Allocate on /sdn/ipams.

# The built-in PVE IPAM needs no connection data.
resource "proxmox_sdn_ipam" "local" {
  ipam = "pve"
  type = "pve"
}

# An external phpIPAM instance. `token` is stored in /etc/pve/sdn/ipams.cfg.
resource "proxmox_sdn_ipam" "phpipam" {
  ipam        = "ipam1"
  type        = "phpipam"
  url         = "https://phpipam.example.com/api"
  token       = var.phpipam_token
  section     = 3
  fingerprint = var.phpipam_fingerprint
}
