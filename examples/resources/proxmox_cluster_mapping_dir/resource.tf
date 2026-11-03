resource "proxmox_cluster_mapping_dir" "share" {
  id  = "shared-srv"
  map = [{ node = "pve-1", path = "/srv/share" }]
}
