# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# The realm index is world-readable and carries only login metadata: no bind
# password, client key, certificate path or full TFA parameters.
data "proxmox_realms" "all" {}

output "realm_types" {
  value = { for realm in data.proxmox_realms.all.realms : realm.realm => realm.type }
}
