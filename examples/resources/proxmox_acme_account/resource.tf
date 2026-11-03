resource "proxmox_acme_account" "le" {
  name    = "letsencrypt"
  contact = var.acme_contact_email
  tos_url = "https://letsencrypt.org/documents/LE-SA-v1.5-February-24-2025.pdf"
}
