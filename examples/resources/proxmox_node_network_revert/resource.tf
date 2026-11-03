# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Discards every staged change of the node. Proxmox reverts per node, so this
# drops all pending interface edits, not just the ones of one resource.
resource "proxmox_node_network_revert" "node1_staged" {
  node = "node1"

  triggers = {
    staged = sha1(jsonencode(proxmox_node_network_interface.vmbr7))
  }

  # Revert before the apply so the reload below picks up the reverted state.
  depends_on = [proxmox_node_network_interface.vmbr7]
}
