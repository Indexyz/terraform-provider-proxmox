data "proxmox_storage_file_restore_download" "d" {
  node     = "pve-1"
  storage  = "backup"
  volume   = "backup:backup/vzdump-qemu-300-2026_01_01-00_00_00.vma.zst"
  filepath = "/etc/hostname"
}
