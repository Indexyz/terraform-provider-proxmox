resource "proxmox_node_acme_certificate" "pve1" {
  node    = "pve-1"
  account = proxmox_acme_account.le.name
  domains = ["pve1.example.com"]
  domain_plugins = {
    "pve1.example.com" = proxmox_acme_plugin.cloudflare.plugin
  }

  triggers = { generation = "1" }
}
