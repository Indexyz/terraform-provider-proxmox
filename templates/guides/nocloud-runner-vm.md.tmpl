# Provisioning a cloud-init seeded QEMU VM (NoCloud)

This guide walks through the full provisioning chain that the provider supports with API tokens only: look up a unique cloud-init template, build a dedicated NoCloud seed ISO for one generation, clone the template with an automatically allocated VMID, attach the seed as CD-ROM media, start the guest once, and stop it again before destroy. Rendering stays in your Terraform configuration with `templatefile()` and `yamlencode()`: the provider receives generic cloud-init content and does not know about GitHub Actions runners, JIT registration, or any application above the guest.

## Example configuration

```hcl
# Copyright IBM Corp. 2021, 2026
# SPDX-License-Identifier: MPL-2.0

variable "generation" {
  type    = number
  default = 1
}

variable "api_token_secret" {
  type      = string
  sensitive = true
}

variable "runner_registration_token" {
  type      = string
  sensitive = true
}

locals {
  # Zero-padded so it sorts and reads well in filenames and instance IDs.
  generation = format("%010d", var.generation)
}

provider "proxmox" {
  endpoint         = "https://pve.example.com:8006"
  api_token_id     = "terraform@pve!provider"
  api_token_secret = var.api_token_secret
}

# 1. Token-only template lookup. one() errors when more than one template
#    matches the name; the precondition below turns a missing template into
#    a clear plan failure instead of a null dereference.
data "proxmox_qemu_vms" "template" {
  name     = "ubuntu-nocloud-template"
  template = true
}

locals {
  template = one(data.proxmox_qemu_vms.template.vms)
}

# 2. One dedicated seed ISO per generation. The seed carries its own DHCP
#    network-config: Proxmox ipconfig settings never modify this ISO, so the
#    seed must be self-contained.
resource "proxmox_nocloud_iso" "runner_seed" {
  node     = "pve-1"
  storage  = "local"
  filename = "runner-seed-${local.generation}.iso"

  # Rendered outside the provider. The instance-id is unique per generation
  # so cloud-init re-runs first boot only when the generation really changes.
  meta_data = yamlencode({
    instance-id    = "iid-runner-${local.generation}"
    local-hostname = "runner"
  })

  user_data = templatefile("${path.module}/user-data.tftpl", {
    registration_token = var.runner_registration_token
  })

  network_config = yamlencode({
    version = 2
    ethernets = {
      nic0 = {
        match = {
          name = "en*"
        }
        dhcp4 = true
      }
    }
  })

  lifecycle {
    # On generation replacement, create the new seed and re-point the VM
    # before the old file is removed, so the ISO is never deleted while a
    # VM still references it.
    create_before_destroy = true
  }
}

# 3. Clone with automatic VMID allocation starting at 100 and the seed
#    attached as CD-ROM media in ide2. The marker scopes the strict seed
#    safety checks to this workflow, and replace_triggered_by rebuilds the
#    VM whenever the seed generation is replaced.
resource "proxmox_qemu_vm" "runner" {
  node        = "pve-1"
  vm_id_start = 100
  name        = "tf-runner-${local.generation}"

  clone = {
    source_node = try(local.template.node, null)
    source_vmid = try(local.template.vm_id, null)
    full        = true
  }

  nocloud_cdrom_slot = "ide2"

  disk = {
    ide2 = {
      media  = "cdrom"
      volume = proxmox_nocloud_iso.runner_seed.volume_id
    }
  }

  # The guest starts exactly once, after clone, configuration, and seed
  # attachment succeed. Destroy performs a hard power-off before deleting.
  start_on_create = true
  stop_on_destroy = true

  lifecycle {
    replace_triggered_by = [
      proxmox_nocloud_iso.runner_seed
    ]

    precondition {
      condition     = local.template != null
      error_message = "The template lookup must match exactly one template VM."
    }
  }
}
```

`user-data.tftpl` renders the registration material outside the provider, for example:

```ini
#cloud-config
runcmd:
  - echo configure your GitHub Actions runner here
```

## How the chain fits together

### Template lookup and uniqueness

`data.proxmox_qemu_vms` searches cluster guests by exact `name` and `template = true` using the same API-token credentials as the resources. `one()` fails the plan when the lookup matches more than one template, and the non-null precondition reports a missing template clearly. Clone provenance is create-time input: refreshing or importing a VM cannot infer it.

### One seed ISO per generation

Every `proxmox_nocloud_iso` creation input requires replacement, so a new generation means a new file. The filename must be unique across states and generations: the PVE upload API silently overwrites an existing destination and has no atomic create-if-absent, so the provider refuses an existing destination instead of adopting it. Give each generation a distinct `filename` and a distinct `instance-id` in `meta_data`, as in the example.

The ISO is generated inside the provider process: the `user-data`, `meta-data`, and `network-config` contents are staged as plaintext files in a private (`0700`) temporary directory - the ISO9660 writer's staging area, located through `TMPDIR` - before the final image is assembled in memory and uploaded through `/nodes/{node}/storage/{storage}/upload`. The staging directory is removed on every normal path and cleanup failures are reported rather than swallowed, but a provider crash or hard kill can leave plaintext residue in the temporary directory: point `TMPDIR` at a private location for stateful runs and treat it with the same care as the Terraform state. The upload task can be owned by a different node than the requested one; the provider polls the task on its actual owner.

### CD-ROM attachment safety

Setting `nocloud_cdrom_slot = "ide2"` (as in the example) marks which typed disk slot carries the NoCloud seed. The strict seed checks apply only to marked VMs, so ordinary multi-ISO guests and native Proxmox cloud-init usage keep working without seed-layout restrictions. On a marked VM the provider checks, before any guest or clone task exists:

- The marked slot plans a real ISO CD-ROM volume with **explicit** `media = "cdrom"`, and the slot is an `ide`, `sata`, or `scsi` slot. A bare `.iso` volume without `media`, or an explicit disk medium, is rejected before any guest or clone task exists: PVE requires explicit `media=cdrom` for ISO images.
- The seed's storage is visible, enabled, active, and supports `iso` content **on the VM's node**, and the exact seed volume exists there, proven by the node's authoritative ISO content listing. A same-named storage on the wrong node without the file fails before the clone; API errors are errors, never treated as absence.
- At most one seed drive is planned: another ISO/cloud-init drive on any other slot - including raw values the typed disk parser does not fully recognize - fails the plan.

After a clone succeeds, the provider inspects the inherited raw disk configuration before applying the CD-ROM:

- The marked slot may only hold the same volume already, an empty bay, or the cloud-init drive Proxmox generated for the clone (`<storage>:vm-<vmid>-cloudinit`, or its file-backed `storage:<vmid>/vm-<vmid>-cloudinit.<format>` form). Replacing that drive in its own slot is the explicit, supported path for templates that were created with Proxmox cloud-init.
- Pseudo values (`none`, a bare `cdrom`) may not clear an inherited hard disk or foreign medium, and any other inherited medium - a hard disk, a foreign ISO, an unrecognizable value - aborts the create with the clone identity retained in state. Nothing is deleted and the guest is not started.
- A second ISO/cloud-init drive remaining in the effective wire configuration is rejected as an ambiguous double seed.

An in-place update on a marked VM is checked against the same rules before the update request is sent: only the identical volume, an empty bay, or the cloud-init drive Proxmox generated for this VM in its own slot may change under an update. Re-pointing the marked slot to a different real ISO - including the previous generation seed this resource attached in an earlier apply - is refused, because refreshed state observes reality but does not prove this resource attached the current medium: swap seeds by replacing the VM (the example's `replace_triggered_by`), never by overwriting the live medium in place. Unmarked VMs keep their original, unrestricted CD-ROM behavior.

### Lifecycle hooks

`start_on_create` starts the guest once, after clone, `/config` update, and CD-ROM attachment finish, and awaits the start task. It is a create-time hook, not declarative power state: changing the option or refreshing a stopped guest never starts or recreates the VM. A failed start keeps the created guest tracked in state with its stop policy so the run can be recovered.

`stop_on_destroy` stops a running guest before deletion and awaits the stop task. The stop is a **hard power-off** (`qm stop`): the guest gets no chance to shut down gracefully or flush, so unclean guest filesystems stay unclean. A failed, timed-out, or ambiguous stop (including a 404 while polling the stop task) aborts the destroy and leaves the guest tracked; a later retry re-checks the guest with a config read. `onboot` keeps its separate meaning: host-boot autostart.

### Dependency and destroy ordering

The `volume_id` reference from the VM's `disk` entry to `proxmox_nocloud_iso.runner_seed` creates the dependency Terraform uses to order the chain: the ISO is created before the VM, and on destroy the VM is deleted before the seed file. Add `create_before_destroy = true` to the ISO resource as in the example so a generation replacement creates the new seed and re-points the VM before the old file is removed - the ISO is never deleted while a VM still references it.

To make the VM itself follow the generation, the example sets `replace_triggered_by = [proxmox_nocloud_iso.runner_seed]`: when the seed resource is replaced, Terraform forces the VM to be replaced, rebuilding the guest from the template with the new seed instead of updating it in place. This does not guarantee a fresh numeric VMID - with the constant `vm_id_start = 100` the replacement is typically allocated the just-freed ID again - so treat the VMID as a scheduling detail, not an identity. The provider verifies exactly this generation-replacement ordering with Terraform core itself (not hand-invoked resource callbacks): a harness test applies generation 1, replaces it with generation 2, and destroys it against a local mock API, asserting the seed upload, VM replacement, and old-seed cleanup order.

Never attach a seed that Terraform does not manage through this dependency chain: an out-of-band reference gives Terraform no ordering information, and the seed could be deleted while the VM is running.

### Storage and node constraints

The upload node must see the target storage with `iso` content enabled and active, and the VM's node must see it too. Both checks run against the per-node storage index, not against global storage configuration. Shared storages work as long as both nodes report them active.

### Sensitive material handling

`user_data`, `meta_data`, and `network_config` are marked sensitive, but sensitive values are **not encrypted**: they appear in plaintext in the Terraform state, in the uploaded ISO on the Proxmox storage, in the provider's private (`0700`) staging directory while the image is generated, and in any crash logs of the provider process. Registration tokens therefore stay in files rendered by `templatefile()`, in a state backend that you protect accordingly, and in a `TMPDIR` that you control; restrict access to the PVE storage as well. Anyone with state read access can reproduce the seed contents, and the provider deletes only the exact volume it uploaded, never shared or inherited media.

Never publish raw diagnostics when filing issues: `TF_LOG=DEBUG` output, Terraform state and plan files, `user-data`, and Proxmox task logs can all contain the registration token or the API token secret. Sanitize or redact them first.

### Failure recovery

- **Clone or create submission fails with an API error**: the guest was never accepted, no VM identity is tracked, and the winning VM of a candidate race is never touched. `nextid` is queried during Create (apply), not during plan: the provider proposes candidates beginning at `vm_id_start` and keeps the first one the API confirms free, so an ID that was already taken is skipped in favor of the next free candidate. Only an ID claimed in the narrow window between that availability check and the clone/create submission fails the run with the original API error; the next apply allocates a fresh candidate.
- **Upload accepted but the task wait fails or times out**: the resource stays tracked with the retained task UPID in private state; refresh reconciles the task on the node the UPID names before deciding existence, and destroy waits it out before cleanup.
- **Upload lost before the API acknowledged it** (transport failure before the UPID arrived): nothing was written to state and no UPID exists, so refresh cannot discover the file and this resource has no import that could adopt it. Reconcile manually: list the storage content, prove the leftover file is the dedicated upload of this failed run (its unique generation filename) and that no upload task for it is still running, and only then delete that exact volume - never arbitrary or shared media. Reapplying with the same filename before resolving the leftover would rely on the upload API silently overwriting the existing destination.
- **Clone or create submission lost before the response arrived** (transport failure whose outcome is unknown): unlike an API error response, this failure does not prove the guest was never accepted. Terraform tracks nothing, so reconcile manually: check the cluster for the candidate VMID and its task log, and only after proving the guest came from your run either remove it or adopt it explicitly through `terraform import` with its `node/vmid`. Do not reapply blindly while the outcome is unresolved.
- **Create fails after the clone was accepted** (config update, CD-ROM safety check, or start): the clone identity and lifecycle policy are already in state, but because Create returned diagnostics Terraform may taint the instance. Do not expect the next apply to repair and reuse the same guest or to start it: the tracked guest is cleaned up and replaced, and the replacement may legitimately receive the just-freed numeric VMID again.
- **Stop fails during destroy**: the guest stays tracked; fix the cause (locked guest, hung QEMU process) and retry. Because the stop is a hard power-off, a stopped-but-unclean guest still deletes on retry.
- **Seed already deleted out of band**: refresh removes the resource from state; a repeated destroy succeeds idempotently.

## Real-environment acceptance checklist

The provider's test suite verifies the exact HTTP ordering against mock APIs, and a Terraform-core harness applies generation 1, replaces it with generation 2, and destroys it against a local mock API to prove the dependency ordering with the real CLI. The following behaviors **cannot** be verified by API mocks and were **not** verified for this release: guest DHCP lease acquisition, cloud-init activation inside the guest, GitHub Runner registration and job execution, and multi-node storage behavior. Run this checklist on a real PVE host (and the Shaula runner composition) before trusting the chain end to end. No step requires destructive operations beyond the resources Terraform itself manages.

1. **Template present and unique**: upload a cloud-init-ready template image, tag it as template, and confirm `data.proxmox_qemu_vms` with `name` + `template = true` returns exactly one match. A name matching two guests must fail the plan in `one()`.
2. **VMID conflict**: `nextid` is queried during Create on apply, not during plan. Before applying, occupy the VMID that PVE currently reports as next free: the provider's propose-and-assert loop must skip it and allocate a different free ID without touching the winner. The residual race - an ID claimed between the availability check and the clone/create submission - fails the apply with the original API error and also leaves the winner untouched; simulating it requires racing the apply deliberately.
3. **Clone**: apply the example. The clone task must complete, and the new guest must inherit the template layout with the seed visible in `ide2`.
4. **ISO mount**: confirm `qm config <vmid>` shows `ide2: <storage>:iso/runner-seed-<generation>.iso,media=cdrom` and that the PVE-generated `vm-<vmid>-cloudinit` drive of a cloud-init template was replaced in the same slot, not duplicated.
5. **Power start**: with `start_on_create = true`, the guest must be `running` after apply. A deliberately broken start (for example, no KVM on the node) must keep the guest in state and must not delete it.
6. **DHCP lease**: confirm from inside the guest or the router that the NIC obtained a DHCP lease (the seed's `network-config`, not Proxmox `ipconfig`, controls this).
7. **Runner online**: confirm the GitHub runner registers and appears online using the token rendered into `user-data`.
8. **Job run / self-stop**: dispatch one job and confirm the runner picks it up, executes it, and (per your user-data) powers off or deregisters afterwards.
9. **Refresh does not restart**: run `terraform apply` again on an idle, manually stopped guest; the guest must stay stopped, and the plan must be clean or update-only.
10. **VM destroy**: with `stop_on_destroy = true`, destroying the VM must stop the running guest (hard power-off) and then delete it.
11. **ISO cleanup**: destroying the seed must remove exactly `runner-seed-<generation>.iso` from the storage and nothing else. With the VM destroyed first, the storage must be free of the generation's files.
12. **Start failure**: set an invalid start condition, apply, and observe the retained guest, its stop policy, and a clear error. Because Create ended with diagnostics, the instance may be tainted: the next apply cleans up and replaces the tracked guest, and the replacement may reuse the just-freed numeric VMID. Verify the end state (one tracked, healthy guest), not which numeric ID it received.
13. **Interrupted destroy retry**: abort a destroy between the stop and delete tasks (for example, cancel Terraform), then retry; the destroy must complete without orphaning the guest or the seed.

Where a step fails, collect `terraform` logs with `TF_LOG=DEBUG`, the Proxmox task log for the relevant UPID, and `qm config` of the guest. Sanitize all three before filing an issue: debug logs, task logs, and configuration dumps can contain the rendered `user-data` (including the registration token) and the API token secret.
