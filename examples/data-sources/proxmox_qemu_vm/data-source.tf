# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

data "proxmox_qemu_vm" "example" {
  node  = "pve-1"
  vm_id = 101
}

# Use proxmox_cluster_resources for bulk inventory. This data source is the
# single-VM inspection surface for typed config read-back, raw long-tail keys,
# and observed-only fields such as status and uptime.
