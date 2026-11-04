data "proxmox_node_ceph_config_value" "v" {
  node = "pve-1"
  keys = ["global:osd_pool_default_size"]
}
