resource "proxmox_cluster_join" "cluster" {
  hostname    = "10.0.0.2"
  password    = var.peer_root_password
  fingerprint = var.peer_certificate_fingerprint
  triggers    = { gen = "1" }
}
