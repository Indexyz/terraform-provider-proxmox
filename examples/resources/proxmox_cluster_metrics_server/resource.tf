# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "proxmox_cluster_metrics_server" "influx" {
  server_id          = "influx-main"
  type               = "influxdb"
  server             = "influx.example.com"
  port               = 8086
  influxdb_protocol  = "https"
  organization       = "infrastructure"
  bucket             = "proxmox"
  token              = var.influxdb_token
  verify_certificate = true
}
