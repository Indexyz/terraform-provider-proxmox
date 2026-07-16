# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_realm" "sso" {
  realm      = "corp-sso"
  type       = "openid"
  issuer_url = "https://id.example.com"
  client_id  = "proxmox"

  client_key         = var.proxmox_oidc_client_key
  client_key_version = 1

  username_claim = "email"
  scopes         = "openid email profile"
  audiences      = "proxmox"
  autocreate     = true
  comment        = "Corporate OpenID Connect realm"
}
