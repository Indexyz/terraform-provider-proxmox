# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_storage_file_download" "debian_iso" {
  node                = "pve-1"
  storage             = "local"
  content             = "iso"
  filename            = "debian-13.0.0-amd64-netinst.iso"
  url                 = "https://cdimage.debian.org/debian-cd/current/amd64/iso-cd/debian-13.0.0-amd64-netinst.iso"
  checksum            = "replace-with-sha256"
  checksum_algorithm  = "sha256"
  verify_certificates = true
}
