data "proxmox_backup_jobs" "all" {}

data "proxmox_backup_job_included_volumes" "volumes" {
  id = data.proxmox_backup_jobs.all.jobs[0].id
}
