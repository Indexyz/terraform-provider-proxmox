data "proxmox_node_ceph_cmd_safety" "stop" {
  node    = "pve"
  service = "osd"
  action  = "stop"
  name    = "0"
}
