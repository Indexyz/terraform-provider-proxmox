resource "proxmox_cluster_mapping_usb" "stick" {
  id = "usb-key"

  map = [{ node = "pve-1", id = "1d6b:0002" }]
}
