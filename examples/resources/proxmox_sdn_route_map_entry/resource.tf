# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Route map entries stay staged until proxmox_sdn_apply. The route map itself
# exists implicitly through its entries.
resource "proxmox_sdn_route_map_entry" "accept_underlay" {
  route_map_id = "rm-underlay"
  order        = 10
  action       = "permit"
  match        = ["ip-address=10.10.0.0/16"]
  set          = ["local-preference=200"]
  exit_action  = "on-match-next"
}
