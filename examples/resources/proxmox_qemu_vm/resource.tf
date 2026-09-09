# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_qemu_vm" "example" {
  node        = "pve-1"
  vm_id       = 101
  name        = "terraform-qemu-vm"
  description = "Managed by Terraform"
  tags        = "terraform,example"
  pool        = "workloads"
  onboot      = true
  protection  = true
  tablet      = true
  startup     = "order=1"
  bios        = "ovmf"
  machine     = "q35"
  agent       = "enabled=1"
  cores       = 2
  sockets     = 1
  memory      = 2048
  numa        = false
  vcpus       = 2
  cpuunits    = 1024
  cpulimit    = 0
  balloon     = 0
  shares      = 1000
  hugepages   = "any"
  cpu         = "host"
  ostype      = "l26"
  boot        = "order=scsi0;net0"
  scsihw      = "virtio-scsi-pci"

  # Create/destroy-time hooks, never declarative power state. The destroy
  # stop is a hard power-off. See docs/guides/nocloud-runner-vm.md for the
  # full NoCloud template -> seed -> clone -> destroy chain.
  start_on_create = true
  stop_on_destroy = true

  # Declares ide2 as the NoCloud seed slot. This scopes the strict seed
  # checks to this workflow: the seed storage must be visible, active, and
  # support iso content on the VM's node, the exact seed volume must exist
  # there, and no second ISO/cloud-init drive may remain attached. Changing
  # the marker requires replacement.
  nocloud_cdrom_slot = "ide2"

  vga = {
    type   = "std"
    memory = 16
  }

  clone = {
    source_vmid = 9000
    full        = true
    storage     = "local-lvm"
  }

  common = {
    hotplug = "network,disk,usb"
  }

  cloud_init = {
    ciuser    = "ubuntu"
    ciupgrade = true
    sshkeys   = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample terraform@example"
    ipconfig = {
      ipconfig0 = {
        ipv4    = "10.0.10.50/24"
        gateway = "10.0.10.1"
      }
    }
  }

  network = {
    net0 = {
      model    = "virtio"
      bridge   = "vmbr0"
      firewall = true
      tag      = 10
    }
  }

  disk = {
    scsi0 = {
      storage = "local-lvm"
      size    = "32G"
      discard = "on"
      ssd     = true
    }

    # CD-ROM media, for example a proxmox_nocloud_iso seed volume. With
    # nocloud_cdrom_slot set, the attachment is strictly safety-checked
    # against the inherited disks and the VM node's ISO storage; unmarked
    # CD-ROM attachments stay unrestricted.
    ide2 = {
      media  = "cdrom"
      volume = "local:iso/ubuntu-seed.iso"
    }
  }

  serial = {
    serial0 = "socket"
  }

  raw = {
    extra_config = {
      rng0 = "/dev/urandom"
    }
  }
}
