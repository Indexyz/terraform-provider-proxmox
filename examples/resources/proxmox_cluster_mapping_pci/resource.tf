resource "proxmox_cluster_mapping_pci" "gpu" {
  id          = "gpu-node1"
  description = "passthrough GPU"

  map = [{
    node         = "pve-1"
    path         = "0000:01:00.0"
    id           = "10de:1eb8"
    subsystem_id = "10de:12a2"
  }]
}
