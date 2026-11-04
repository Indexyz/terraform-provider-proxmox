resource "proxmox_guest_remote_migrate" "r" {
  node            = "pve-1"
  vmid            = 300
  target_endpoint = "host=pve-2.example.com,apitoken=PVEAPIToken=root@pam!tf=secret"
  target_storage  = "local"
  target_bridge   = "vmbr0"
  online          = true
  triggers        = { gen = "1" }
}
