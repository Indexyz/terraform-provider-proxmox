# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_acl" "vm_101" {
  path      = "/vms/101"
  propagate = true
  roles     = [proxmox_role.terraform.role_id]
  users     = [proxmox_user.deploy.user_id]
  groups    = [proxmox_group.admins.id]
}
