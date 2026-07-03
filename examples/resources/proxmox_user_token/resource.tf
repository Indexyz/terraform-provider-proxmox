# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_user_token" "ci" {
  user_id  = proxmox_user.deploy.user_id
  token_id = "ci"
  comment  = "CI automation token"
  privsep  = true
}
