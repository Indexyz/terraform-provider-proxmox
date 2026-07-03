# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

data "proxmox_storage" "local_lvm" {
  storage = "local-lvm"
}
