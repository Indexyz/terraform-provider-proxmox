# Research: notification endpoints and matchers

Pinned to the PVE 9.2 API viewer schema under `/cluster/notifications/*` and the
webhook plugin validation behaviour observed on a live node.

## Contract

- Endpoints live under `/cluster/notifications/endpoints/{smtp,gotify,sendmail,webhook}`;
  `GET` lists, `GET /{name}` reads, `POST` creates, `PUT /{name}` updates
  (`digest`, `delete`), `DELETE /{name}` removes. DELETE takes no parameters
  besides the path name.
- Read hides secrets: SMTP `password` and Gotify `token` are never returned;
  webhook `secret` is echoed as `name=<NAME>` only. Header values are stored as
  base64 (`name=<header>,value=<base64>`), and an update rejects the template
  form `{{ secrets.X }}` that create still accepts, so the provider documents
  base64 values.
- Matchers live under `/cluster/notifications/matchers` with the same method
  set and `target` (array), `mode` (`all`/`any`), `match-field`,
  `match-severity`, `match-calendar`, `invert-match`, `disable`, `comment`.
- Built-in targets (`mail-to-root`, `default-matcher`, ...) are ordinary
  endpoints/matchers with `origin: builtin`; importing them and toggling
  `disable`/`comment` edits them in place.

## Provider decisions

Secrets are sensitive inputs kept from prior state across refreshes, and
webhook secret values are merged back by name. `disable`, `comment` and empty
matcher rule sets are deleted through the `delete` key when cleared. Matchers
require at least one target; severities are validated against the enum.
Endpoints and matchers are importable by name, which is the only way to adopt
the built-in targets.
