data "proxmox_sdn_vnet_firewall_options" "app" {
  vnet = proxmox_sdn_vnet.app.vnet
}
