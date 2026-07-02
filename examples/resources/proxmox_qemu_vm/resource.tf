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
  startup     = "order=1"
  bios        = "ovmf"
  machine     = "q35"
  agent       = "enabled=1"
  cores       = 2
  sockets     = 1
  memory      = 2048
  cpu         = "host"
  ostype      = "l26"
  boot        = "order=scsi0;net0"

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
  }

  raw = {
    extra_config = {
      serial0 = "socket"
    }
  }
}
