data "proxmox_query_url_metadata" "m" {
  node = "pve-1"
  url  = "https://example.com/image.iso"
}
