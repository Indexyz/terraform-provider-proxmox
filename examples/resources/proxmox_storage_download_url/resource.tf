resource "proxmox_storage_download_url" "debian" {
  node     = "pve"
  storage  = "local"
  url      = "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.qcow2"
  filename = "debian-12-genericcloud-amd64.qcow2"
  content  = "import"
  triggers = { revision = "1" }
}
