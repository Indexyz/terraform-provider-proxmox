# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Commits every staged zone/VNet/subnet change in one cluster-wide reload and
# waits for the reload task. Bump a trigger whenever staged SDN configuration
# changed; a robust trigger is a hash over the managed objects.
# Destroying this resource does not undo the commit.
# Provider credentials need SDN.Allocate on /sdn.
resource "proxmox_sdn_apply" "cluster" {
  triggers = {
    revision = sha1(jsonencode([
      for zone in [proxmox_sdn_zone.lab, proxmox_sdn_zone.vlan] : [zone.id, zone.type, zone.bridge, zone.mtu, zone.nodes]
    ]))
  }
}
