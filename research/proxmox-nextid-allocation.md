# Research: `GET /cluster/nextid` for VM create and clone ID allocation

## Question

Can the provider call the Proxmox `nextid` API when creating a VM or a clone, and control the value the allocation starts from?

## Answer

Yes for allocation, with one semantic correction on "start value": `/cluster/nextid?vmid=N` is an availability **assert** for a specific ID, not a floor. It returns `N` only when `N` is free and fails with HTTP 400 `VM N already exists` otherwise; the server's no-parameter scan always starts from the datacenter `next-id.lower` (default 100), not from a caller-supplied ID. A per-resource "next free ID >= N" therefore needs a small client-side propose-and-assert loop, while a cluster-wide start range can be set server-side via the datacenter `next-id` option.

## Verified PVE API contract

From `PVE/API2/Cluster.pm` (pve-manager master) and `datacenter.cfg(5)`:

```
GET /cluster/nextid[?vmid=N]        permissions: user => 'all'
returns: integer (older PVE versions returned a JSON string)
```

- `permissions: user => 'all'` — any authenticated user or API token may call it; no extra privilege is needed beyond what create/clone already requires.
- With `vmid=N`: return `N` if `N` is not in the cluster vmlist; otherwise raise a parameter exception (`VM N already exists`, HTTP 400). It never returns a different ID.
- Without `vmid`: scan `lower..upper` from `datacenter.cfg` `next-id` (`lower` default 100 inclusive, `upper` default 1000000 exclusive) and return the lowest unused VMID cluster-wide (vmlist covers qemu + lxc + templates); fail when the range is exhausted.
- The call is a pure read. It does **not** reserve anything; the check-then-create window is inherently racy.
- Deleted IDs are reused (lowest-free semantics).
- The datacenter `next-id` option (`PUT /cluster/options`) is the supported server-side way to bound the auto-selection pool.

## Current provider state

- No `/cluster/nextid` client method exists anywhere (`rg nextid` finds nothing).
- `proxmox_qemu_vm`: `vm_id` is `Required` + `RequiresReplace`; both create paths read `plan.VMID.ValueInt64()`:
  - regular create: `CreateQemuVM` posts `vmid=<VMID>` to `/nodes/{node}/qemu`;
  - clone mode: `qemuVMCloneRequestFromModel` maps `NewID` from the same `plan.VMID`, so **one allocation point in `Create` covers both create and clone**.
- `proxmox_lxc_container` has the identical shape (`vm_id` required; `CloneLXCContainerRequest.NewID`; create posts `vmid`), so the same change applies there if wanted.
- `Client.do` already unwraps the `{"data": ...}` envelope and supports GET query parameters (`ClusterResources` precedent), so the new client method is small.

## Recommended design

### 1. Client method (new)

```go
func (c *Client) GetNextVMID(ctx context.Context, assertID *int64) (int64, error) {
	query := url.Values{}
	if assertID != nil {
		query.Set("vmid", strconv.FormatInt(*assertID, 10))
	}

	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/cluster/nextid", query, nil, &raw); err != nil {
		return 0, err
	}

	// Older PVE versions return the ID as a JSON string, current ones as a number.
	var number int64
	if err := json.Unmarshal(raw, &number); err == nil {
		return number, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, fmt.Errorf("unable to decode cluster nextid response %s: %w", raw, err)
	}
	id, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("unable to parse cluster nextid response %q: %w", text, err)
	}
	return id, nil
}
```

The taken-ID error surfaces through the existing `APIError` (StatusCode 400, body contains `already exists`).

### 2. Schema: `vm_id` becomes Optional + Computed

```go
"vm_id": schema.Int64Attribute{
	Optional: true,
	Computed: true,
	MarkdownDescription: "Numeric VMID of the QEMU virtual machine. When omitted, the provider allocates the next free cluster VMID via `/cluster/nextid`.",
	PlanModifiers: []planmodifier.Int64{
		int64planmodifier.UseStateForUnknown(),
		int64planmodifier.RequiresReplace(),
	},
},
```

This mirrors bpg/terraform-provider-proxmox (`id`: Optional + Computed + `UseStateForUnknown` + `RequiresReplace`). Plan semantics: omitted at create → unknown until apply; allocated value persists in state; user-set value changes still force replace; import unchanged.

### 3. One allocation point in `Create`

```go
if plan.VMID.IsNull() {
	nextID, err := r.client.GetNextVMID(ctx, nil)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Allocate Proxmox VMID", err.Error())
		return
	}
	plan.VMID = types.Int64Value(nextID)
}
```

Runs before both the clone branch (`CloneQemuVM`) and the regular create branch (`CreateQemuVM`). After creation the state write already reads back `vm_id` from the model, so the allocated ID lands in state without further changes.

### 4. "Start value" options

| Approach | Mechanism | Scope | Effort |
| --- | --- | --- | --- |
| A. Server-side range (recommended first step) | Set datacenter `next-id` (`lower`/`upper`) in the PVE GUI or via a future cluster-options resource; auto-allocation automatically respects it | Cluster-wide, affects UI/scripts too | Zero provider change |
| B. Resource-level `vm_id_start` attribute | When `vm_id` is omitted and `vm_id_start` is set, loop: `GetNextVMID(ctx, &candidate)`, on HTTP 400 `already exists` increment `candidate` and retry (bounded, e.g. up to `upper`), then create with the asserted ID | Per resource, exact "next free >= start" | Small: one attribute + a bounded loop |

Option B's loop is the correct client-side translation of "start from N", because `?vmid=` cannot express a floor.

## Known risks

- **TOCTOU race**: `nextid` is read-only; another client (UI, second `terraform apply`, scripts) can take the ID between allocation and create. PVE then fails the create task with an `already exists` error, which today surfaces as a failed apply. Mitigations, in increasing complexity: surface a clear retryable error (minimal); retry create once with a re-allocated ID (what bpg effectively does via its retry loop); random-range allocation to reduce parallel-apply collisions (bpg's `random_vm_ids`).
- **Parallel creates get the same ID**: `for_each` with N VMs created in parallel all call `nextid` before any create lands. Without conflict retry, parallel applies of auto-allocated VMs are expected to fail intermittently. This is the main reason bpg wraps allocation in lock + sequence-file + retry; for this provider a create-on-conflict retry is the proportionate response.
- **ID reuse after deletion**: lowest-free semantics mean a recreated resource can receive an ID previously used by a deleted guest. Explicit `vm_id` in config is unaffected.
- **Unknown-at-plan**: with `vm_id` omitted, `vm_id` and `id` (`node/vm_id`) are unknown during plan; downstream references resolve at apply time. Standard Terraform behavior, but worth documenting in the resource description.
- Upstream is experimenting with `unique-next-id` (`used_vmids.list`) to stop ID reuse server-side; not needed for this feature and not assumed.

## Effort estimate

Slice 1 (allocation): client method + tests, schema flip, allocation point, docs/examples regeneration — small, no new lifecycle machinery. Slice 2 (optional): `vm_id_start` propose-assert loop or a datacenter `next-id` options resource.
