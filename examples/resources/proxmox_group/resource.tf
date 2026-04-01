# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_group" "developers" {
  group_id = "developers"
  comment  = "Managed by Terraform"
}
