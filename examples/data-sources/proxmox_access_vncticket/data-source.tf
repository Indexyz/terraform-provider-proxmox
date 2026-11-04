data "proxmox_access_vncticket" "t" {
  userid    = "root@pam"
  path      = "/vms/300"
  vncticket = "PVEVNC:..."
  privs     = "VM.Console"
}
