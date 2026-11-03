# Research: cluster hardware mappings

Pinned to the PVE 9.2 API viewer schema (`/cluster/mapping/*`, pve-manager
`PVE/API2/Cluster/Mapping/{PCI,USB,Dir}.pm`, commit 21297b5 of pve-cluster for
the node-name format).

## Contract

- `GET /cluster/mapping/{pci,usb,dir}` lists entries; `GET .../{id}` reads one.
- `POST` (create) takes `id` (`pve-configid`), `description`, `map` (array of
  property strings) and, for PCI only, `live-migration-capable`.
- `PUT .../{id}` additionally takes `digest` and a `delete` key for clearing
  optional attributes; `DELETE .../{id}` accepts **no** parameters besides the
  path id (a digest in the form is rejected with
  `digest: property is not defined in schema`).
- Map entries are property strings:
  - PCI: `node=<node>,path=<bus:dev.func[;alternatives]>,id=<VVVV:DDDD>` plus
    optional `subsystem-id`, `iommugroup`, `description`.
  - USB: `node=<node>` plus `path=<bus-port>` and/or `id=<VVVV:PPPP>`.
  - Dir: `node=<node>,path=<absolute path>`.
- `vendor:device` ids are uppercase patterns; paths are lowercase hex.

Permission is `Mapping.Modify` on `/mapping/{kind}`; listing additionally
honours `Mapping.Use`/`Mapping.Audit` per entry. `check-node` can add `checks`
to list responses, which is not needed for CRUD.

## Provider decisions

Three resources plus one generic data source (`kind` + `id`), because the entry
shape differs per kind. Identity is the mapping id, so the id requires
replacement; `map` is a set keyed by node and validated locally (nodes, PCI
paths/ids, USB path or id, absolute directories). Deletes carry no digest, so
removal is last-write-wins per Proxmox semantics. Description is tracked as a
managed field so unsetting it clears the server value instead of echoing.
