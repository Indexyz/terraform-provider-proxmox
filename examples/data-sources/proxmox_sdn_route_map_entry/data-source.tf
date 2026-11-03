data "proxmox_sdn_route_map_entry" "inbound" {
  route_map_id = "inbound"
  order        = 10
}
