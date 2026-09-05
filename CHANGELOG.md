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
