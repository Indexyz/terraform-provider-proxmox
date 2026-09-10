# Research: linked clones for `proxmox_qemu_vm` and `proxmox_lxc_container`

## Question

When cloning a Proxmox template, can the provider create a *linked* clone (`full=0`) instead of a full copy, and what is missing today to call that supported?

## Answer

The wire path already exists and is reachable: `clone.full = false` is preserved by the mapping layer and sent as `full=0` for both QEMU and LXC.

The larger finding is that `full` is **optional and forwarded only when set**, and Proxmox's own default is per-source:

```perl
my $full = $param->{full} // !PVE::QemuConfig->is_template($conf);
```

so the provider **already creates linked clones today** whenever a template is cloned without `full = true` (and fails, without fallback, when linked cloning is not possible). The current schema text ("Whether to request a full clone"), the examples, the guide, and the acceptance test all assume full clones and never document or exercise that default.

The real gaps are therefore not API capability but:

1. documentation of linked-clone semantics and constraints (schema text, resource docs, guide);
2. test coverage for the `full=0` wire path (all existing tests assert `full=1`);
3. missing pre-validation of the one combination PVE rejects (`full=false` with `storage`/`format`), which currently fails mid-apply with a server-side parameter error.

## Verified PVE contract

Pinned to `git.proxmox.com` master fetched 2026-09-10:

- qemu-server `f8c6cf8ff8957452f71b94025b19bfacd77924b8` (`src/PVE/API2/Qemu.pm`, `src/PVE/QemuServer.pm`)
- pve-container `1c0488315a1df22a9bba635a81e7543b5ce664b7` (`src/PVE/API2/LXC.pm`)
- pve-storage `7c6a03839920d4939a8ae725a2b0ef91c0cbc6c9` (`src/PVE/Storage.pm`, `src/PVE/Storage/Plugin.pm`, `LvmThinPlugin.pm`, `RBDPlugin.pm`, `ZFSPoolPlugin.pm`, `BTRFSPlugin.pm`)

### Clone endpoint parameters (QEMU)

`src/PVE/API2/Qemu.pm` (`clone_vm`, `POST /nodes/{node}/qemu/{vmid}/clone`):

- `full` — optional boolean, no schema default; description: "Create a full copy of all disks. This is always done when you clone a normal VM. For VM templates, we try to create a linked clone by default."
- `storage` — "Target storage for full clone." (rejected for linked clones)
- `format` — "Target disk format. Only valid for full clone." `enum raw|qcow2|vmdk`
- `snapname`, `target`, `bwlimit`, `newid`, `name`, `description`, `pool`

Effective default and the linked-clone guard:

```perl
my $full = $param->{full} // !PVE::QemuConfig->is_template($conf);

die "parameter 'storage' not allowed for linked clones\n"
    if defined($storage) && !$full;
die "parameter 'format' not allowed for linked clones\n"
    if defined($format) && !$full;
```

Behavior matrix:

| Request | Source | Result |
| --- | --- | --- |
| `full` omitted | template | `full=0` → linked clone |
| `full` omitted | normal VM | `full=1` → full clone |
| `full=0` | template or snapshot | linked clone |
| `full=0` | normal VM without snapshot | per-drive failure, no fallback |
| `full=0` + `storage`/`format` | any | HTTP parameter error before any task |
| `full=0` + `bwlimit` | any | accepted but ignored (only the full-copy path consumes it) |

There is **no automatic fallback**: `die "Linked $msg\n" if !PVE::Storage::volume_has_feature(..., 'clone', ...)` aborts the clone task; PVE never silently upgrades `full=0` to a full copy.

### Per-drive capability gate (QEMU)

Every non-CD-ROM drive takes either the copy path or the clone path (`src/PVE/API2/Qemu.pm`):

```perl
if ($full || drive_is_cloudinit($drive) || $opt eq 'tpmstate0') {
    die "Full $msg\n" if !PVE::Storage::volume_has_feature(..., 'copy', ...);
    $fullclone->{$opt} = 1;
} else {
    die "Linked $msg\n" if !PVE::Storage::volume_has_feature(..., 'clone', ...);
}
```

Two consequences that survive into a linked clone:

- cloud-init drives and `tpmstate0` are always fully copied/allocated fresh; only the regular data disks become linked children;
- CD-ROM configuration is copied verbatim.

### Storage capability (pve-storage `volume_has_feature`)

| Plugin | `clone` keys | `template` keys | Linked child name |
| --- | --- | --- | --- |
| dir / NFS / CIFS (generic `Plugin.pm`) | `base` only (`qcow2`, `raw`, `vmdk`); `clone_image` requires a base image and always creates a qcow2 with a relative backing file | `current` | `base-<vmid>-disk-N.qcow2` (in the template's directory) |
| LVM-thin (`LvmThinPlugin.pm`) | `base`, `snap` | `current` | flat `vm-<newvmid>-disk-N` (`lvcreate -prw -kn -s`) |
| RBD / Ceph (`RBDPlugin.pm`) | `base`, `snap` | `current` | `<base>/vm-<newvmid>-disk-N` |
| ZFS (`ZFSPoolPlugin.pm`) | `base` only (`$snap ||= '__base__'`) | `current` | `<base>/vm-<newvmid>-disk-N` (zvol clone) |
| Btrfs (`BTRFSPlugin.pm`) | `base` (`qcow2`, `raw`, `subvol`, `vmdk`), `current` (`raw`), `snap` (`raw`) | `current` | flat `vm-<newvmid>-disk-N` (`btrfs subvolume snapshot`) |
| LVM (thick), iSCSI, passthrough, ... | none | none | linked clone impossible |

So a linked clone requires the source volume to be a **base image** (produced by template conversion) or, on LVM-thin/RBD/Btrfs-raw, a **snapshot**. Btrfs additionally allows a linked clone of a *current* raw volume, and therefore of a non-template guest without any snapshot; `snapshot_name` + `full=0` works on LVM-thin/RBD and on Btrfs raw, and fails on ZFS and file-based storage.

### Btrfs specifics

`BTRFSPlugin.pm` is the most permissive backend, because every raw disk already lives in a Btrfs subvolume:

- `clone => current => raw` means `full=0` on a plain (non-template, non-snapshot) guest works: `clone_image` runs `btrfs subvolume snapshot` on the live subvolume.
- `create_base` renames the volume to `base-...` and flips it read-only (`btrfs property set ... ro true`).
- qcow2/vmdk formats are forwarded to `DirPlugin::clone_image` (relative backing file, base images only).
- **Snapshot defect (pinned revision):** `clone_image` accepts `$snap` but never forwards it to `filesystem_path` (`BTRFSPlugin.pm`, `clone_image` signature versus body), so `full=0` with `snapshot_name` on a Btrfs raw volume silently clones the *live* subvolume while the caller believes it cloned the snapshot. This is reachable through the API (`volume_has_feature` advertises `clone` for the `snap` key on raw volumes) and was reported to pve-devel as `[PATCH storage] btrfs: fix clone_image cloning live data instead of the requested snapshot` (message-id `20260907.btrfsclonesnapfix@cyruspy.gmail.com`, 2026-09-07, no maintainer response and not in master as of the pin). Until it lands, `full = false` with `snapshot_name` on Btrfs must be treated as producing current-state data.

### Base image lifecycle

- Template conversion (`qm template`, `POST /nodes/{node}/qemu/{vmid}/template`) calls `PVE::QemuServer::template_create` → `PVE::Storage::vdisk_create_base`, renaming `vm-<vmid>-disk-N` to `base-<vmid>-disk-N` for storages that advertise the `template` feature.
- `PUT /config -template 1` does **not** convert disks; only the create path (`create_disks`) and the `/template` endpoint call `vdisk_create_base`. A flag-only template cannot be linked-cloned. This provider's `template` attribute is Computed-only (schema text: "Terraform does not manage template conversion") and is never sent, so the provider can neither create flag-only templates nor convert existing guests — templates are always externally converted.
- Destroying a linked clone frees only the clone's own volumes; the base image stays owned by the template.
- Destroying the template while linked clones exist is refused by PVE (`destroy_vm`): `base volume '<volid>' is still in use by linked cloned`.

### Cross-node and LXC parity

- Cross-node linked clone requires shared storage; PVE rejects local-storage sources (`can't clone VM to node '$target' (VM uses local storage)`).
- LXC (`src/PVE/API2/LXC.pm`) uses the identical default (`$full = !is_template`), the same `parameter 'storage' not allowed for linked clones` rejection, and the same per-mountpoint `volume_has_feature('clone', ...)` gate. LXC clone has no `format` parameter at all.

## Current provider state

| Aspect | State |
| --- | --- |
| Request plumbing | `CloneQemuVMRequest.Full` (`client_qemu.go`), `setOptionalBool(form, "full", ...)`; `boolPointerValue` preserves an explicit `false`, so `full=0` is sent. LXC identical (`client_lxc.go`) |
| Implicit default | `full` omitted ⇒ attribute never sent ⇒ PVE default (linked for templates, full otherwise). Undocumented |
| Tests | `client_qemu_test.go`, `client_lxc_test.go`, `resource_guest_lifecycle_test.go` assert `full=1` only; no test exercises `full=0` |
| Validation | none; `full=false` + `storage`/`format` reaches PVE and fails there |
| Docs | `full`: "Whether to request a full clone"; `storage`/`format`: "…for full clones"; example, guide, and e2e all use `full = true` |
| Observability | clone block is echoed from prior state (PVE cannot report provenance); the only linked-clone evidence is the inherited disk value (`<storage>:base-<vmid>-disk-N`) visible in the data source / projected disk map |
| Replacement semantics | `clone` object has `RequiresReplaceIfConfigured()`, so flipping `full` replaces the guest — correct behavior for switching clone type |

## Recommended change set

1. **Schema + generated docs wording** — describe `full` as "full copy vs linked clone", state the PVE default for templates when omitted, and state linked-clone constraints on `storage`, `format`, `bwlimit`, and `snapshot_name` for both resources.
2. **`ValidateConfig` guard** — reject explicit `full = false` combined with `storage` or `format` (QEMU) and `storage` (LXC) before any HTTP call, mirroring the PVE parameter rule. `full` absent stays unvalidated because the effective value depends on the source guest.
3. **Tests** — client-level `full=0` wire assertion for QEMU and LXC, resource-level validation tests, and one mock lifecycle case proving a linked-clone create carries `full=0` and echoes `full=false` in state.
4. **Not recommended** — forcing a provider-side default for `full` (e.g. `Default: true`). That would contradict documented PVE semantics and silently change existing configurations that rely on the template default.

## Known risks and limits

- Linked clones are not independent: the base image must outlive every clone, so the externally managed template cannot be deleted while Terraform-managed clones exist. Terraform destroy order is unaffected because the provider cannot own template conversion.
- Space accounting is shared: base image plus per-clone COW delta; a full clone is often simpler for long-lived, frequently rewritten guests.
- Snapshot-linked clones only work on LVM-thin/RBD and Btrfs raw; ZFS and file-based storage support base-image linked clones only. On Btrfs raw the pinned PVE revision ignores the requested snapshot and clones the live volume (upstream fix pending).
- The real-PVE e2e currently clones a diskless source VM, so it cannot prove linked-clone COW behavior; verifying it needs a template with a base disk on a clone-capable storage (LVM-thin/ZFS/RBD).
- `full=0` on a normal VM without a snapshot fails per drive; the error is surfaced from the clone task, not from Terraform planning.

## Effort estimate

Small: schema text + `ValidateConfig` + tests + `make generate`. No new client methods, no lifecycle machinery.
