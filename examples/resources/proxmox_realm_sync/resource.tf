# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# A sync is a command, not stored configuration: changing an attribute or a
# trigger re-runs it, and destroying the resource does nothing.
# Credentials need Realm.AllocateUser on the realm and User.Modify on
# /access/groups.
resource "proxmox_realm_sync" "corp" {
  realm = proxmox_realm.ldap.realm
  scope = "both"

  # Destructive: also delete users, groups, properties and ACLs that the
  # directory no longer returns. `none` (the default) never deletes anything.
  remove_vanished = "acl;properties;entry"
  enable_new      = true

  triggers = {
    revision = "2026-11-03"
  }
}
