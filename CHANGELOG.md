## 0.4.0 (2026-09-09)

FEATURES:

- Add `proxmox_nocloud_iso` to generate a `CIDATA` ISO containing generic `user-data`, `meta-data`, and `network-config`, upload it with API-token authentication, and manage its exact storage volume through retryable cleanup.
- Add QEMU `start_on_create` and `stop_on_destroy` lifecycle hooks: start only after clone and configuration finish, and await stop before deletion. Refresh and ordinary updates never restart a stopped guest.
- Add `nocloud_cdrom_slot` to opt into seed attachment safety, including exact ISO visibility on the VM node, same-slot replacement of the VM's Proxmox-generated cloud-init drive, and rejection of disk overwrites or multiple seeds. Include a generation-based provisioning guide with VM replacement and ordered ISO cleanup.

FIXES:

- Retain accepted create, clone, upload, and ISO deletion tasks across interrupted waits; reconcile tasks on their actual owner node and reject invalid task acknowledgements or status responses instead of reporting false success.
- Send the correct `target` parameter when cloning QEMU guests across nodes, and preserve stop or delete polling errors so failed operations remain retryable.
- Preserve configured QEMU disk-map keys, including an explicitly empty map, when cloning templates with inherited disks. Send only changed disk and network slots during updates, avoiding unintended writes of inherited attachments and Terraform Core state inconsistencies.

NOTES:

- `stop_on_destroy` performs a hard power-off, not a graceful guest shutdown. Marked seed changes to a different ISO require VM replacement; the guide uses `replace_triggered_by` to preserve dependency ordering.
- Sensitive cloud-init content remains plaintext in Terraform state, the uploaded ISO, and private temporary staging files. Protect these locations and sanitize logs. Guest DHCP, cloud-init activation, Runner operation, and multi-node behavior still require the [real-environment acceptance checklist](https://github.com/Indexyz/terraform-provider-proxmox/blob/v0.4.0/docs/guides/nocloud-runner-vm.md#real-environment-acceptance-checklist).

## 0.3.0 (2026-09-08)

FEATURES:

- Add the `proxmox_qemu_vms` data source to search QEMU guests and templates cluster-wide by exact `name`, `node`, and `template` flag, with numerically sorted results ready for `clone.source_vmid`.
- Extend the `proxmox_storages` data source with `type` and `largest` filters and a computed `capacity_bytes` per storage, aggregating the highest reported capacity across cluster nodes.

FIXES:

- Decode the numeric `template` and `shared` flags returned by `/cluster/resources` as booleans; plain `*bool` decoding failed on any response containing guest or storage entries.

## 0.2.0 (2026-09-06)

FEATURES:

- Add automatic QEMU VMID allocation through `/cluster/nextid` when `vm_id` is omitted, with an optional `vm_id_start` allocation floor and early identity persistence after successful create or clone tasks.

FIXES:

- Wait for QEMU create, clone, and delete tasks to finish before returning, and correctly parse imported guest firewall VMIDs.
- Align PVE 9 user-group, role-privilege, pool-member, and pool/group deletion handling with the live API.
- Treat PVE 9's exact HTTP 500 response for a missing QEMU config as not found while retaining the underlying API error context.

## 0.1.0 (2026-07-20)

BREAKING CHANGES:

- `proxmox_qemu_vm.raw.extra_config` no longer accepts `scsihw`, `tablet`, `numa`, `vcpus`, `cpuunits`, `cpulimit`, `balloon`, `shares`, or `hugepages`. Move those values to their typed top-level attributes.

FEATURES:

- Add first-class LXC container management, including clone workflows, typed network and mount-point blocks, and LXC snapshots.
- Add QEMU snapshots and typed QEMU SCSI controller, tablet, VGA, serial, CPU, NUMA, and memory-balloon configuration.
- Add storage pool management and inventory, plus URL-based storage file downloads.
- Add role, user, ACL, and API token management with matching user and role data sources.
- Add cluster, node, and guest firewall options; cluster firewall aliases, IP sets, security groups, and scoped firewall rules.
- Add backup jobs, replication jobs, cluster metrics servers, and Proxmox VE 9 HA resource enrollment.
- Add Proxmox VE 9 LDAP, Active Directory, and OpenID Connect realm management with write-only secret rotation, plus public realm lookup.
- Add provider configuration and troubleshooting guidance, and upgrade the acceptance smoke environment to Proxmox VE 9.2.

FIXES:

- Resolve CI lint failures and synchronize generated provider reference documentation.
