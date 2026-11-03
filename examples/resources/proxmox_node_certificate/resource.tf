resource "proxmox_node_certificate" "pve1" {
  node         = "pve-1"
  certificates = file("pve1-chain.pem")
  key          = file("pve1-key.pem")
  force        = true
  restart      = true
}
