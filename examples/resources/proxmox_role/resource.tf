# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_role" "terraform" {
  role_id = "TerraformManage"
  privs   = "VM.Allocate,VM.Audit,VM.Config.Disk,Datastore.AllocateSpace,Datastore.Audit,Pool.Allocate"
}
