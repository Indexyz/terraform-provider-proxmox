data "proxmox_query_oci_repo_tags" "t" {
  node      = "pve-1"
  reference = "docker.io/library/alpine"
}
