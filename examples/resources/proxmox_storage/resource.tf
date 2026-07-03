# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_storage" "local_dir" {
  storage = "local-dir"
  type    = "dir"
  content = "images,iso,vztmpl,snippets"
  path    = "/mnt/data"
  nodes   = "pve-1"
}
