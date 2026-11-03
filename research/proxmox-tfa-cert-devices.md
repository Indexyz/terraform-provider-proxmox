# Research: user TFA, realm client certificate content, QEMU compound devices

Pinned sources: `pve-access-control` (TFA.pm, RPCEnvironment reauth), `pve-storage`
(API2/Storage/Status.pm upload), `pve-common` (Network/Interfaces.pm), `qemu-server`
(QemuServer.pm, PCI/USB/RNG/Virtiofs schemas). Commits: `pve-acl` bbc4ed6, `pve-storage`
4250dfd, `pve-common` a00b23b, `qemu-server` fc803fc.

## User TFA (`/access/tfa`)

- `GET /access/tfa/{userid}` and `GET .../{userid}/{id}` require `self` or group
  `User.Modify`/`Sys.Audit`; the item read answers a real 404.
- `POST`/`PUT`/`DELETE` require `self` or group `User.Modify`, and are declared
  `allowtoken => 0`: **API tokens cannot change TFA**. The password applies to every mutation including delete. The provider must use ticket auth;
  a non-`root@pam` identity additionally has to send `password` because
  `reauth_user_for_user_modification` re-authenticates it against its realm.
- `POST` body: `type` (`totp|u2f|webauthn|recovery|yubico`), optional `description`, `totp`
  (otpauth URI), `value` (current TOTP code or Yubico OTP), `challenge`, `password`.
  Response: `{id, challenge?, recovery?}` — `recovery` carries freshly generated codes and
  the id is server-assigned.
- Entry reads expose `id`, `type`, `description`, `created`, `enable`; secrets never come
  back. Deleting the last entry disables TFA for the user.
- Slice: typed resource for the non-interactive types `totp`, `recovery`, `yubico`
  (`webauthn`/`u2f` need a browser ceremony). `description`/`enable` mutable; `totp`,
  `value`, `password` sensitive; `recovery_codes` computed sensitive; import
  `user/entry`.

## Realm client certificate content

Proxmox only stores **paths** for `cert`/`certkey` (`PVE/Auth/LDAP.pm`, `AD.pm`), and the
storage upload endpoint accepts only `iso`, `vztmpl`, `import` content
(`PVE/API2/Storage/Status.pm`), while iso names must end in `.iso|.img`
(`ISO_EXT_RE_0`). There is no generic file-write endpoint and snippets are not uploadable.

Therefore content upload stores the PEM as `.img` files in an iso-capable directory
storage (`<path>/template/iso/terraform-realm-<realm>-{cert,key}.img`) and points the realm
at those paths. Requirements: storage with `path` and `iso` content, enabled and active on
the upload node; the node is the first storage-restricted node, else the first cluster node.
Shared storage is needed on multi-node clusters — the certificate is read by whichever node
handles a login. Files are uploaded before the realm points at them and deleted after the
realm releases them; a vanished file is restored on refresh because a missing cert breaks
every LDAP login.

## QEMU compound devices

Typed slots, whole-slot raw fallback for unknown syntax:

| Family | Slots | Wire (default key first) |
| --- | --- | --- |
| `hostpci` | `hostpci0`–`hostpci15` (`MAX_HOSTPCI_DEVICES`) | `host=<bus:dev.func[;...]>` or `mapping=<id>`; `mdev`, `pcie`, `x-vga`, `rombar`, `romfile`, `legacy-igd`, `vendor-id`/`device-id`/`sub-vendor-id`/`sub-device-id` (`0xhhhh`), `driver=vfio|keep` |
| `usb` | `usb0`–`usb13` (`MAX_USB_DEVICES`) | `host=<bus-port|vendor:product|spice>` or `mapping=<id>`; `usb3` |
| `rng` | `rng0` only | `source=/dev/urandom|/dev/random|/dev/hwrng`, `max_bytes`, `period` |
| `virtiofs` | `virtiofs0`–`virtiofs9` (`MAX_VIRTIOFS`) | `dirid=<configid>`, `cache=auto|always|metadata|never`, `direct-io`, `expose-xattr`, `expose-acl` |

Exactly one of `host`/`mapping` is required for hostpci and usb. `legacy-igd` needs the `pc-i440fx` machine type with VGA `none`, and the `*_id` overrides
only change what the guest sees. Unknown future keys keep the whole slot in
`raw.extra_config`. Passthrough devices usually make a guest non-migratable; virtiofs needs a running
`virtiofsd` in the guest.

Removed slots: QEMU has no per-key delete for typed fields today, so removals are sent
through the `delete` form with the exact slot names, and the outgoing update is narrowed to
new/changed slots exactly like disk/network (Terraform echoes inherited Optional+Computed
slots into the plan).

## Validation plan

- TFA: exact-form create/update/delete, empty-id and 404 handling, recovery capture, secret
  preservation across refresh, type/uri/otp matrix, ticket-auth documentation.
- Cert content: target rejection matrix (no path, no iso, disabled), upload-before-config
  ordering, path derivation, cleanup on failure and destroy, refresh restore.
- Devices: parse/encode round trips, slot ranges, default-key exclusivity, state mapping
  with raw fallback, raw conflict, update delete list.
