data "proxmox_sdn_subnet" "app" {
  vnet   = proxmox_sdn_vnet.app.vnet
  subnet = "10.0.0.0/24"
}
