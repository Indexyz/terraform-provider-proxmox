# `proxmox_firewall_rule` Resource Design

## Context

The provider manages Proxmox VE firewall options (`proxmox_node_firewall_options`) but cannot manage actual firewall rules. Firewall rules are the core of Proxmox's host-level firewall, making this a high-value addition. The Proxmox firewall rule API uses a positional index (`pos`) as the rule identifier, which changes on insertion/deletion. This resource manages rules by **content** rather than position, resolving the `pos` on each operation.

## Scope

This spec covers **cluster-level** firewall rules only (`/cluster/firewall/rules`). Node-level, VM-level, and container-level rule scopes are out of scope for this increment and remain available through future resources.

## Field Selection

Based on the Proxmox API (`/cluster/firewall/rules` POST/GET):

| Field | Type | TF Schema | Identity? | Notes |
|---|---|---|---|---|
| `type` | string | Required, RequiresReplaceIfConfigured | Yes | Rule direction (in/out/forward/group) |
| `action` | string | Required, RequiresReplaceIfConfigured | Yes | Rule action (ACCEPT/DROP/REJECT) |
| `source` | string | Optional+Computed, RequiresReplaceIfConfigured | Yes | Source CIDR/address |
| `dest` | string | Optional+Computed, RequiresReplaceIfConfigured | Yes | Destination CIDR/address |
| `proto` | string | Optional+Computed, RequiresReplaceIfConfigured | Yes | Protocol (tcp/udp/icmp) |
| `dport` | string | Optional+Computed, RequiresReplaceIfConfigured | Yes | Destination port(s) |
| `sport` | string | Optional+Computed, RequiresReplaceIfConfigured | Yes | Source port(s) |
| `icmp_type` | string | Optional+Computed, RequiresReplaceIfConfigured | Yes | ICMP type |
| `iface` | string | Optional+Computed, RequiresReplaceIfConfigured | Yes | Network interface |
| `macro` | string | Optional+Computed, RequiresReplaceIfConfigured | Yes | Predefined macro name |
| `log` | string | Optional+Computed, RequiresReplaceIfConfigured | Yes | Log level (emerg.../nolog) |
| `enable` | integer | Optional+Computed | No (mutable) | Whether the rule is active (0/1) |
| `comment` | string | Optional+Computed | No (mutable) | Rule comment |
| `pos` | integer | Computed | N/A | Position (read from API, not user-set) |

All identity fields use `RequiresReplaceIfConfigured`: if the user changes any identity field, the old rule is deleted (by old identity match) and a new one created. This avoids the "unfindable rule" problem. The `enable` and `comment` fields are updatable in-place via PUT.

### Normalization for Matching

Absent (null) and empty-string values are normalized to `""` for matching. A field that is `null` in Terraform state but present in the API response matches only if the API value is also empty. This prevents accidental identity drift from API defaults.

## Identity Model

The Terraform resource identity is **content-based**. Fields are split into two categories:

### Identity fields (used for content matching, all RequiresReplaceIfConfigured)
`type`, `action`, `source`, `dest`, `proto`, `dport`, `sport`, `icmp_type`, `iface`, `macro`, `log`

### Updatable fields (not used for matching, Optional+Computed)
`enable`, `comment`

These are refreshed from the API on every Read. If the API reports a different value than state, the state is updated (standard Terraform drift reconciliation for Computed fields).

### Content Matching

A rule in the Proxmox list matches our state if ALL identity fields are equal (empty string in state matches absent-from-API, and vice versa). Updatable fields (`enable`, `comment`) are **not** part of the match.

### pos Resolution Rationale

The Proxmox `pos` changes when any rule is inserted or deleted by any actor. We **never** cache `pos` across operations — we re-resolve it via a fresh GET + content match on every Read/Update/Delete. This makes the resource robust against concurrent rule modifications by other tools.

### Duplicate-Content Handling

If the GET list contains **zero** matches: Read removes the resource from state (it's gone); Update/Delete treat it as already-deleted (no-op success).

If the GET list contains **multiple** matches (≥2 rules with identical identity fields): the resource returns a diagnostic error on Read/Update/Delete ("ambiguous firewall rule match: N rules found with identical content"), because we cannot determine which `pos` to target. This is a documented limitation of content-based identity. Create checks the list first and fails if a duplicate already exists.

### Field API Key Mapping

| Terraform field | Proxmox API key |
|---|---|
| `icmp_type` | `icmp-type` |
| `dport` | `dport` |
| `sport` | `sport` |
| `log` | `log` |
| All others | same name |

### Go Schema Types

All fields are `types.String` in the Go model, except:
- `enable` → `types.Int64` (Proxmox uses 0/1 integers)
- `pos` → `types.Int64` (Computed)

`type`, `action`, `log` are strings (no enum validation in this increment; values round-trip verbatim).

## Client Method Signatures

The GET response for `/cluster/firewall/rules` returns rule objects with `pos` but **no `digest`** (confirmed via API viewer: props are action/comment/dest/dport/enable/icmp-type/iface/ipversion/log/macro/pos/proto/source/sport/type). The PUT/DELETE `digest` parameter is therefore optional and **omitted** in this implementation (no optimistic locking). This means concurrent rule modifications between our GET and PUT/DELETE could target a shifted `pos`; we accept this risk and document it, since the content-match approach is robust against it (we re-resolve pos on every operation).

```go
func (c *Client) GetFirewallRules(ctx context.Context) ([]FirewallRule, error)
func (c *Client) CreateFirewallRule(ctx context.Context, req FirewallRuleRequest) error
func (c *Client) UpdateFirewallRule(ctx context.Context, pos int, req FirewallRuleRequest) error
func (c *Client) DeleteFirewallRule(ctx context.Context, pos int) error
```

### PUT Request Body Semantics

`UpdateFirewallRule` sends the **full resolved rule payload** (all identity fields + `enable` + `comment`) in the PUT form. This ensures the rule at `pos` is set to exactly the desired state. Identity fields are included because Proxmox PUT replaces the rule at that position.

### Create Pre-Check Behavior

Before POST, the resource calls `GetFirewallRules` and checks for content matches:
- **0 matches**: proceed with POST (normal create).
- **1 match**: return a diagnostic error ("a firewall rule with identical content already exists at pos N; remove the existing rule or modify your config"). This prevents creating duplicates that would make the resource ambiguous.
- **≥2 matches**: return a diagnostic error (same as Read/Update/Delete ambiguous-match error).

## Non-Atomic Operations

Proxmox firewall operations are non-atomic against a shared API. The GET response does not include a digest, so PUT/DELETE are sent without optimistic locking. If another actor modifies rules between our GET and our PUT/DELETE, the operation may target a shifted `pos`. The content-match re-resolution on every operation mitigates this in practice, but cannot fully prevent it. This is a documented limitation.

## Requirements

- Resource `proxmox_firewall_rule` with schema per the field table above.
- `type` and `action` are Required + RequiresReplaceIfConfigured.
- All other identity fields (`source`/`dest`/`proto`/`dport`/`sport`/`icmp_type`/`iface`/`macro`/`log`) are Optional + Computed + RequiresReplaceIfConfigured.
- `enable` and `comment` are Optional + Computed (mutable, no replace).
- `pos` is Computed only (not user-settable).
- Create: pre-check for duplicate content (error on ≥1 match); POST to `/cluster/firewall/rules`.
- Read: GET `/cluster/firewall/rules`, match by identity content, return pos. Error on ≥2 matches. Remove from state on 0 matches.
- Update: find rule by identity, PUT `/cluster/firewall/rules/{pos}` with the full resolved payload (all identity + updatable fields).
- Delete: find rule by identity, DELETE `/cluster/firewall/rules/{pos}` (no digest; see Non-Atomic Operations).
- Import: not supported (content-based identity makes import impractical; documented limitation).
- Client: `CreateFirewallRule`, `GetFirewallRules` (list), `UpdateFirewallRule`, `DeleteFirewallRule`.
- No `digest` is sent on PUT/DELETE (GET does not return one); see Non-Atomic Operations for the concurrency tradeoff.

## Acceptance Criteria

- Focused client tests: create returns correct form, GET list parsing, update by pos (full payload), delete by pos.
- Resource test: schema attribute presence.
- Duplicate-content error path test: if ≥2 rules match identity fields, Read returns a diagnostic error.
- Create pre-check test: if 1 identical rule already exists, Create returns a diagnostic error.
- Missing remote rule: Read removes from state; Update/Delete no-op success.
- `go build`/`go vet`/`go test ./...` all pass.
- Provider exports test updated with `proxmox_firewall_rule`.
- Example, docs, roadmap updated.

## Safety

Firewall rules control network access. Misconfiguration can lock out nodes. This resource only manages rules it created (by content match). Delete removes only the matching rule. No bulk operations.
