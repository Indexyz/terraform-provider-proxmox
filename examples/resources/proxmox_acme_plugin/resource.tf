data "proxmox_acme_challenge_schema" "dns" {
  type = "dns"
}

resource "proxmox_acme_plugin" "cloudflare" {
  plugin = "cloudflare"
  type   = "dns"
  api    = "cf"
  data = {
    CF_API_TOKEN = var.cloudflare_token
  }
}
