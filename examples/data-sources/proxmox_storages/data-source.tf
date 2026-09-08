# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

data "proxmox_storages" "all" {}

# Pick the largest zfs storage for image placement. Storages without
# reported capacity are excluded, and ties are all returned — assert
# uniqueness with one().
data "proxmox_storages" "largest_zfs" {
  type    = "zfs"
  largest = true
}

output "largest_zfs_storage" {
  value = one(data.proxmox_storages.largest_zfs.storages[*].storage)
}
