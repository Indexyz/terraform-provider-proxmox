# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_nocloud_iso" "runner_seed" {
  node     = "pve-1"
  storage  = "local"
  filename = "runner-seed-20260214000000.iso"

  # Rendered by templatefile() in the caller. The instance-id must be unique
  # per generation so cloud-init re-runs first boot only on real replacement.
  meta_data = yamlencode({
    instance-id    = "iid-runner-20260214000000"
    local-hostname = "runner"
  })

  user_data = file("${path.module}/files/user-data")

  # The seed ISO carries its own network configuration; DHCP is supplied here
  # as generic cloud-init input.
  network_config = yamlencode({
    version = 2
    ethernets = {
      nic0 = {
        match = {
          name = "en*"
        }
        dhcp4 = true
      }
    }
  })
}
