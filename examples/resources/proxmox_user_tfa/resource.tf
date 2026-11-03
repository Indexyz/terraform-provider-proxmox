# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Proxmox rejects API tokens on TFA endpoints, so the provider must use ticket
# authentication. A non-root@pam identity additionally needs `password`.
resource "proxmox_user_tfa" "root_totp" {
  user_id     = "root@pam"
  type        = "totp"
  totp        = "otpauth://totp/root%40pam?secret=BASE32SECRET&issuer=Proxmox"
  description = "Laptop authenticator"
}

# Generated recovery codes are only known right after creation and are stored
# in state (sensitive).
resource "proxmox_user_tfa" "root_recovery" {
  user_id = "root@pam"
  type    = "recovery"
}

output "recovery_codes" {
  value     = proxmox_user_tfa.root_recovery.recovery_codes
  sensitive = true
}
