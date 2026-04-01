# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

data "proxmox_node_dns" "pve01" {
  node = "pve01"
}
