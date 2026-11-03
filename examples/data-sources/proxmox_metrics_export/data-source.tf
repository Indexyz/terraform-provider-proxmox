data "proxmox_metrics_export" "cluster" {
  node_list = "pve-1,pve-2"
}
