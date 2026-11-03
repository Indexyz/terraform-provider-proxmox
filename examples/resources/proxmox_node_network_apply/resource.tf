# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Applies the staged interface configuration of one node in a single reload and
# waits for the reload task. Bump a trigger whenever the staged configuration
# changed; a robust trigger is a hash over the interfaces, for example
#   interfaces = sha1(jsonencode([for i in proxmox_node_network_interface.node1 : [i.id, i.address, i.bridge_ports]]))
# Destroying this resource does not undo the applied configuration.
# Provider credentials need Sys.Modify on /nodes/{node}.
resource "proxmox_node_network_apply" "node1" {
  node = "node1"

  triggers = {
    revision = "2026-11-03"
  }
}
