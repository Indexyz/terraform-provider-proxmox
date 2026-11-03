data "proxmox_sdn_fabric_node" "node1" {
  fabric_id = proxmox_sdn_fabric.ospf.fabric_id
  node_id   = "node1"
}
