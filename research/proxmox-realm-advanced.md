# Research: realm advanced follow-up (list, LDAP sync fields, client certs, TFA)

## Question

`proxmox_realm` manages LDAP/AD/OpenID realms with the basic connection fields, and
`proxmox_realm` (data source) reads one realm. What is still missing for real LDAP/AD
directory integration, and what is the safe contract for it?

Remaining bucket from the roadmap: realm **list** data source, LDAP/AD group & sync
configuration fields, LDAP client certificates, and realm-level TFA.

## Answer

Four small, independent slices on already-verified endpoints. Two reuse the existing
`Realm.Allocate` item surface, one is the world-readable login index, and TFA is a
property string with a fixed two-variant grammar.

## Verified PVE contract

Pinned to `pve-access-control` `bbc4ed6` (`PVE/API2/Domains.pm`, `PVE/Auth/LDAP.pm`,
`PVE/Auth/AD.pm`, `PVE/Auth/OpenId.pm`, `PVE/Auth/Plugin.pm`).

`GET /access/domains` is `user => 'world'` (needed by the login box, no ACL) and returns
only `realm`, `type`, optional `comment`, `default`, and the **TFA type** (`yubico` /
`oath`) parsed out of the stored TFA config. It never exposes secrets or the full TFA
parameters.

`POST`/`PUT` build their schema from the plugin `properties()`; `GET /access/domains/{realm}`
returns the raw section of `/etc/pve/domains.cfg`, including `password`, `certkey` and
the raw `tfa` property string. Permission is `Realm.Allocate` or `Sys.Audit` on
`/access/realm`.

Wrongly scoped fields fail server-side, so the provider keeps enforcing the variant
matrix locally:

| Field | ldap | ad | openid | Notes |
| --- | --- | --- | --- | --- |
| `cert`, `certkey` | yes | yes | no | Paths on the cluster (`pve-ssl.pem`-style), not secrets. |
| `filter`, `sync_attributes`, `user_classes`, `group_dn`, `group_name_attr`, `group_filter`, `group_classes`, `sync-defaults-options`, `case-sensitive` | yes | yes | no | LDAP/AD sync configuration. |
| `groups-claim`, `groups-autocreate`, `groups-overwrite` | no | no | yes | OpenID group claim mapping. |
| `tfa` | yes | yes | no | Property string `type=oath\|yubico[,id=..,key=..,url=..,digits=6..8,step>=10]`. |

Defaults that matter for reading: `user_classes` (`inetorgperson, posixaccount, person, user`),
`group_classes` (`groupOfNames, group, univentionGroup, ipausergroup`), `case-sensitive`
(`1`). `sync-defaults-options` is passed through as a flat string because
`realm-sync-options` is its own opaque sub-format.

## Slice 1: `proxmox_realms` list data source

Computed-only list over the world-readable index: `realm`, `type`, `comment`, `default`,
`tfa` (TFA type only). Sorted by `realm` for stable plans, no filters, no secrets, no
digest, no item GET per entry. `default` is returned by the implementation although the
published `returns` schema omits it; treat it as a hint, not as an authority.

## Slice 2: LDAP/AD resource fields and client certificates

All fields are Optional + Computed with private managed-field ownership and `delete`
keys, exactly like the existing realm fields. `cert`/`certkey` are plain strings because
Proxmox stores paths, not PEM content.

The existing single-realm `proxmox_realm` data source exposes the same public fields as
computed attributes (including the parsed `tfa` block) so lookup and module wiring stay
symmetric; bind passwords and client keys remain excluded there.

Validation only extends the existing variant matrix (LDAP/AD-only, OpenID-only for the
group claim fields). Format checks stay light: `sync_attributes` must look like
`key=value[,key=value]`, `user_classes`/`group_classes` must be comma-separated
non-empty attribute tokens, `filter`/`group_filter` are capped at 2048 characters,
`group_dn`/`group_name_attr` at 256.

## Slice 3: typed `tfa` block

Single optional nested block mapped to the property string:

- `type` required, enum `oath`/`yubico`;
- `id`, `key`, `url` for `yubico`;
- `digits` (6–8) and `step` (≥10) for `oath`.

Encoding writes `type` first, joins with commas, omits unset keys, and keeps the
configured order otherwise. Decoding parses `key=value` pairs and rejects unknown keys
instead of dropping them silently. The block is Optional + Computed with managed-field
deletion so removing it sends `delete=tfa`.

`key` (Yubico API key) is stored in `/etc/pve/domains.cfg` in plaintext and returned by
`GET`, so it stays a normal (non-write-only) string, documented as such.

## Non-goals

- `/access/domains/{realm}/sync` stays a separate imperative concern; nothing in this
  change triggers a sync run.
- no TFA *user* management (`/access/tfa`), only the realm-level provider restriction;
- no `secure`/`sslversion` (deprecated in favour of `mode`);
- no client-certificate file upload, only path fields;
- no change to import, identity, digest or default-realm handling.

## Validation plan

- index data source: exact `GET /access/domains`, sorted output, `default`/`tfa` mapping,
  empty index, API error context, no item GET, registration;
- resource: exact create/update forms for every new field (including the `tfa` string),
  variant rejection matrix, `sync_attributes` format, `tfa` variant requirements and
  ranges, unknown-value deferral, managed-field deletion for `tfa`, state mapping with
  absent keys, malformed `tfa` rejection;
- examples, generated docs, README/codebase/roadmap, and a `pam`-safe read in the smoke
  test.
