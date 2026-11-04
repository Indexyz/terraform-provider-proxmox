resource "proxmox_node_apl_download" "tpl" {
  node     = "pve-1"
  storage  = "local"
  template = "alpine-3.24-default_20260803_amd64.tar.xz"
  triggers = { gen = "1" }
}
