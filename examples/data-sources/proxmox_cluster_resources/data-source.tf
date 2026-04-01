# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

data "proxmox_cluster_resources" "vms" {
  type = "vm"
}
