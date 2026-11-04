data "proxmox_openid_auth_url" "u" {
  realm        = "oidc"
  redirect_url = "https://pve-1:8006"
}
