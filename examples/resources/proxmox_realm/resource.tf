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

# LDAP realm with directory sync, a client certificate and a forced TOTP factor.
resource "proxmox_realm" "ldap" {
  realm     = "corp-ldap"
  type      = "ldap"
  server1   = "ldap.example.com"
  base_dn   = "dc=example,dc=com"
  user_attr = "uid"

  bind_dn               = "cn=terraform,dc=example,dc=com"
  bind_password         = var.proxmox_ldap_bind_password
  bind_password_version = 1

  cert         = "/etc/pve/priv/realm/corp-ldap.crt"
  cert_key     = "/etc/pve/priv/realm/corp-ldap.key"
  filter       = "(objectClass=person)"
  user_classes = "inetorgperson, posixaccount"

  # Group sync keeps the base DN, object classes and name attribute separate.
  group_dn        = "ou=groups,dc=example,dc=com"
  group_name_attr = "cn"
  group_filter    = "(objectClass=groupOfNames)"
  group_classes   = "groupOfNames, group"

  sync_attributes       = "email=mail,firstname=givenName"
  sync_defaults_options = "remove-vanished=entry,scope=users"
  case_sensitive        = true

  tfa = {
    type   = "oath"
    digits = 6
    step   = 30
  }

  comment = "Corporate LDAP directory"
}

# OpenID realm that maps directory groups into Proxmox groups.
resource "proxmox_realm" "sso_groups" {
  realm      = "corp-sso-groups"
  type       = "openid"
  issuer_url = "https://id.example.com"
  client_id  = "proxmox"

  client_key         = var.proxmox_oidc_client_key
  client_key_version = 1

  username_claim    = "email"
  groups_claim      = "groups"
  groups_autocreate = true
  groups_overwrite  = true
}

# Client certificate content instead of a path: the PEM files are uploaded to
# the iso directory of the given storage and the realm points at them. Use a
# shared storage on multi-node clusters so every node can read the files.
resource "proxmox_realm" "ldap_cert_content" {
  realm     = "corp-ldap-cert"
  type      = "ldap"
  server1   = "ldap.example.com"
  base_dn   = "dc=example,dc=com"
  user_attr = "uid"

  bind_dn               = "cn=terraform,dc=example,dc=com"
  bind_password         = var.proxmox_ldap_bind_password
  bind_password_version = 1

  cert_storage     = "local"
  cert_content     = file("realm-client.crt")
  cert_key_content = file("realm-client.key")
}
