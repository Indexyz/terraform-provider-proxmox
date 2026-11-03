# Research: node certificates and ACME

Pinned to the PVE 9.2 API viewer schema (`/nodes/{node}/certificates/*`,
`/cluster/acme/*`) and live-probed on the e2e node.

## Node certificates

- `GET /nodes/{node}/certificates/info` lists every file under
  `/etc/pve/local` with `filename`, `fingerprint`, `issuer`, `subject`, `san`,
  `notbefore`/`notafter` (epoch or RFC3339, the API has answered both), key
  metadata and the PEM.
- `POST /nodes/{node}/certificates/custom` with `certificates`, `key`,
  optional `force`/`restart` installs a custom chain; the file is
  `pveproxy-ssl.pem`. `DELETE` removes it again. Both are synchronous.
- `san` is either an array or a joined string, `notafter`/`notbefore` are
  numeric in current builds, so the decoder accepts all forms.

## ACME

- Accounts: `POST /cluster/acme/account` and `PUT/DELETE .../{name}` are
  **asynchronous** and answer a UPID. The list only returns `{name}`; the item
  GET returns `directory`, `location`, `tos` and an `account` object with
  `status` and `contact`.
- `contact` is a plain email address (PVE prefixes `mailto:` itself) and the CA
  rejects reserved TLDs such as `.test`/`.local` and forbidden domains like
  `example.com`. Let's Encrypt requires an explicit `tos_url`.
- Plugins: `data` is a base64 property string on create but a plain property
  string on read; `api` values come from the `challenge-schema` enum (for
  example `cf`, not `cloudflare`).
- Certificate orders: `POST/PUT/DELETE /nodes/{node}/certificates/acme/certificate`
  answer UPIDs (`acmenewcert`, revoke). An order requires the node config
  `acme=account=<name>[,domains=...]` and/or `acmedomain[n]=domain=...,plugin=...`;
  setting the same domain in both places is rejected as duplicate. Destroy
  revokes.

## Provider decisions

`proxmox_node_certificate` owns the custom certificate only and exposes the
parsed metadata; destroy removes the custom file. `proxmox_acme_account` and
`proxmox_acme_plugin` are regular resources (account mutations wait for the
worker). `proxmox_node_acme_certificate` is a trigger-driven command that writes
the node ACME config, orders the certificate, and revokes on destroy; the node
config itself stays behind. Contacts are kept from state because the CA may
omit them.
