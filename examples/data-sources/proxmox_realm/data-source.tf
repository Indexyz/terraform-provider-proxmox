# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

data "proxmox_realm" "corporate_sso" {
  realm = "corp-sso"
}
