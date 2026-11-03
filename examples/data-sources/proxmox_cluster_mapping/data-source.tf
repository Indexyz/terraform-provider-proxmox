data "proxmox_cluster_mapping" "share" {
  kind = "dir"
  id   = proxmox_cluster_mapping_dir.share.id
}
