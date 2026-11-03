resource "proxmox_node_apt_repository" "enterprise" {
  node    = "pve-1"
  path    = "/etc/apt/sources.list.d/pve-enterprise.sources"
  index   = 0
  enabled = false
}
