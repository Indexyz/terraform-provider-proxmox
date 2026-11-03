# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Rules constrain guests that are already HA configured, so they reference
# proxmox_ha_resource. Provider credentials need Sys.Audit and Sys.Console on /.

# Keep the web guests on the fast nodes, with a relative priority.
resource "proxmox_ha_rule" "web_on_fast_nodes" {
  rule      = "web-fast-nodes"
  type      = "node-affinity"
  resources = [proxmox_ha_resource.web.resource_id]
  nodes = {
    node1 = 2
    node2 = 1
  }
  comment = "Prefer the fast nodes for web guests"
}

# Keep the primary and standby database guests apart. A negative rule needs at
# least as many cluster nodes as resources and pve-ha-manager 5.2.5 or later.
resource "proxmox_ha_rule" "database_apart" {
  rule      = "database-apart"
  type      = "resource-affinity"
  resources = [proxmox_ha_resource.database_primary.resource_id, proxmox_ha_resource.database_standby.resource_id]
  affinity  = "negative"
}

# Negative node affinity avoids the listed nodes instead of enumerating the
# allowed ones. Node priorities must stay 0. Disabled rules are still stored but
# no longer constrain scheduling.
resource "proxmox_ha_rule" "batch_off_backup_node" {
  rule      = "batch-off-backup"
  type      = "node-affinity"
  resources = [proxmox_ha_resource.batch.resource_id]
  nodes     = { backup1 = 0 }
  affinity  = "negative"
  strict    = true
  disable   = true
  comment   = "Keep batch guests off the backup node"
}
