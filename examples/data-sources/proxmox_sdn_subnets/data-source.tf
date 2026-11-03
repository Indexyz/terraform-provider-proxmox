data "proxmox_sdn_subnets" "vnet1" {
  vnet = proxmox_sdn_vnet.app.vnet
}
