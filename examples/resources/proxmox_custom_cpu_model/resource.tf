resource "proxmox_custom_cpu_model" "m" {
  cputype        = "custom-x"
  reported_model = "qemu64"
}
