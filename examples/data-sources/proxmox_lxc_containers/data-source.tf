# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Find an LXC template by name and clone from it. The `template` flag comes
# from `/cluster/resources`, so a template created within the last minute may
# not appear until Proxmox publishes its RRD stats.
data "proxmox_lxc_containers" "debian_template" {
  template = true
  name     = "debian-13-standard"
}

resource "proxmox_lxc_container" "app" {
  node  = one(data.proxmox_lxc_containers.debian_template.containers[*].node)
  vm_id = 999

  clone = {
    source_vmid = one(data.proxmox_lxc_containers.debian_template.containers[*].vm_id)
    full        = true
  }
}
