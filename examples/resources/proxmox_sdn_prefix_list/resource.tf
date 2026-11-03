# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Prefix lists stay staged until proxmox_sdn_apply. Entries are Proxmox
# property strings.
resource "proxmox_sdn_prefix_list" "underlay" {
  prefix_list_id = "underlay"
  entries = [
    "action=permit,prefix=10.10.0.0/16,le=32",
    "action=deny,prefix=0.0.0.0/0",
  ]
}
