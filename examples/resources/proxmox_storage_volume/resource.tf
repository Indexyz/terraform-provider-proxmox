resource "proxmox_storage_volume" "backup" {
  node      = "pve-1"
  storage   = "backup"
  volume    = "backup:backup/vzdump-qemu-300-2026_01_01-00_00_00.vma.zst"
  notes     = "keep for audit"
  protected = true
}
