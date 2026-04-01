# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

data "proxmox_group" "developers" {
  group_id = "developers"
}
