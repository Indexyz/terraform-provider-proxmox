resource "proxmox_cluster_ceph_health_mute" "near_full" {
  code = "OSD_NEAR_FULL"
  ttl  = "1h"
}
