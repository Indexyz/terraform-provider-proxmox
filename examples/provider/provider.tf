# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

provider "proxmox" {
  endpoint         = "https://pve.example.com:8006"
  api_token_id     = "terraform@pve!provider"
  api_token_secret = "replace-me"
}
