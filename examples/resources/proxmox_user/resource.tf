# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_user" "deploy" {
  user_id   = "deploy@pve"
  firstname = "Terraform"
  lastname  = "Bot"
  email     = "bot@example.com"
  password  = "change-me-please"
  groups    = proxmox_group.admins.id
}
