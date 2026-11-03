# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# DNS providers stay staged until proxmox_sdn_apply commits the SDN
# configuration. Zones reference them through `dns`.
# Provider credentials need SDN.Allocate on /sdn/dns.
resource "proxmox_sdn_dns" "powerdns" {
  dns         = "pdns1"
  type        = "powerdns"
  url         = "https://powerdns.example.com/api"
  key         = var.powerdns_api_key
  ttl         = 300
  fingerprint = var.powerdns_fingerprint
}
