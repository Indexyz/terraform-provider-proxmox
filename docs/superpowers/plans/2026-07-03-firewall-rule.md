# `proxmox_firewall_rule` Implementation Plan

> **For agentic workers:** Implement this plan task-by-task using TDD. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Add `proxmox_firewall_rule` resource for cluster-level Proxmox firewall rules (`/cluster/firewall/rules`).

**Architecture:** Content-based identity (11 identity fields), `pos` is computed and re-resolved on every op. Client does CRUD. No digest (GET returns none). No import.

## Global Constraints (from spec)

- **Identity fields** (all `RequiresReplaceIfConfigured`): `type` (Required), `action` (Required), `source`, `dest`, `proto`, `dport`, `sport`, `icmp_type`, `iface`, `macro`, `log` (all Optional+Computed).
- **Mutable fields** (Optional+Computed, no replace): `enable` (Int64), `comment` (String).
- **Computed-only**: `pos` (Int64).
- **Content normalization**: null and empty-string both normalize to `""` for matching.
- **No digest** on PUT/DELETE (GET does not return one).
- **No import** support.
- **Update PUT** sends the **full resolved payload** (all identity + enable + comment).
- API key mapping: `icmp_type` → `icmp-type`; all others same name.

---

### Task 1: Client methods + tests

**Files:**
- Create: `internal/provider/client_firewall_rule.go`
- Test: `internal/provider/client_firewall_rule_test.go`

**Produces:** `FirewallRule` struct, `FirewallRuleRequest`, `GetFirewallRules`, `CreateFirewallRule`, `UpdateFirewallRule`, `DeleteFirewallRule`.

- [ ] **Step 1: Write failing client test**

```go
func TestClientFirewallRuleMethods(t *testing.T) {
    // Test server handling:
    // GET /cluster/firewall/rules → list with 2 rules (pos 0 and 1)
    // POST /cluster/firewall/rules → assert form has type=in, action=ACCEPT, source=10.0.0.0/8, dport=443, proto=tcp, enable=1
    // PUT /cluster/firewall/rules/0 → assert form has full payload (type+action+source+dport+proto+enable+comment)
    // DELETE /cluster/firewall/rules/0 → assert path
```

**Important casing note**: Proxmox uses lowercase `type` values (`in`/`out`/`forward`/`group`). All tests, examples, and the implementation must use lowercase consistently. No uppercasing.
}
```

- [ ] **Step 2: Run test, verify it fails (undefined methods)**

Run: `go test ./internal/provider/ -run FirewallRule`
Expected: compile failure (undefined: `GetFirewallRules` etc.)

- [ ] **Step 3: Implement client**

```go
type FirewallRule struct {
    Pos      int
    Type     string
    Action   string
    Enable   proxmoxOptionalInt64
    Comment  string
    Source   string
    Dest     string
    Proto    string
    DPort    string
    SPort    string
    ICMPType string
    Iface    string
    Macro    string
    Log      string
}

type FirewallRuleRequest struct {
    Type, Action            string
    Enable                  *int64
    Comment                 *string
    Source, Dest, Proto     *string
    DPort, SPort            *string
    ICMPType                *string
    Iface, Macro, Log       *string
}
```

`GetFirewallRules`: GET `/cluster/firewall/rules`, decode list, map `icmp-type`→ICMPType, `dport`→DPort, `sport`→SPort.
`CreateFirewallRule`: POST `/cluster/firewall/rules` with form (type, action, enable, comment, source, dest, proto, dport, sport, icmp-type, iface, macro, log).
`UpdateFirewallRule(ctx, pos int, req)`: PUT `/cluster/firewall/rules/{pos}` with **full resolved payload**.
`DeleteFirewallRule(ctx, pos int)`: DELETE `/cluster/firewall/rules/{pos}`. **No digest.**

- [ ] **Step 4: Run test, verify pass**
- [ ] **Step 5: Commit**

```bash
git add internal/provider/client_firewall_rule.go internal/provider/client_firewall_rule_test.go
git commit -m "feat(firewall): add firewall rule client methods"
```

---

### Task 2: Resource + schema + lifecycle + tests

**Files:**
- Create: `internal/provider/resource_firewall_rule.go`
- Modify: `internal/provider/provider.go` (register `NewFirewallRuleResource`)
- Modify: `internal/provider/provider_unit_test.go` (add `proxmox_firewall_rule` to exports list)

**Produces:** `firewallRuleModel`, schema, content-matching helper, Create/Read/Update/Delete, duplicate pre-check.

- [ ] **Step 1: Write failing resource tests**

Tests to write:
- [ ] Schema attribute presence test (`type`, `action`, `enable`, `comment`, `source`, `dest`, `proto`, `dport`, `sport`, `icmp_type`, `iface`, `macro`, `log`, `pos` all present).
- [ ] Content-matching helper test: matches by identity fields, normalizes null/empty to `""`.
- [ ] Duplicate pre-check test: if GetFirewallRules returns ≥1 match, Create returns diagnostic error.
- [ ] Ambiguous match test: if ≥2 matches, Read returns diagnostic error.
- [ ] Missing remote rule test: 0 matches → Read removes from state.
- [ ] **Update full-payload test**: verify the resource GETs the rule list, resolves the matching rule by content, and PUTs the full resolved payload (all identity fields + enable + comment) at the matched `pos`.
- [ ] **Create pos-resolution test**: verify after POST, the resource re-GETs, matches by identity, and sets the resolved `pos` in state.

- [ ] **Step 2: Run tests, verify fail**

- [ ] **Step 3: Implement schema**

`type`/`action`: Required + `RequiresReplaceIfConfigured`.
`source`/`dest`/`proto`/`dport`/`sport`/`icmp_type`/`iface`/`macro`/`log`: Optional+Computed + `RequiresReplaceIfConfigured`.
`enable`: Int64, Optional+Computed.
`comment`: String, Optional+Computed.
`pos`: Int64, Computed only.

- [ ] **Step 4: Implement content-matching helper**

`matchFirewallRule(rules []FirewallRule, model firewallRuleModel) ([]FirewallRule, error)`:
- Normalize: `stringValue(types.String)` returns `""` for null/unknown.
- Match identity fields: type, action, source, dest, proto, dport, sport, icmp_type, iface, macro, log.
- Return matches slice. Caller checks: 0 → remove/no-op; 1 → use; ≥2 → error.

- [ ] **Step 5: Implement lifecycle**

**Create**: pre-check (GET + match → error on ≥1 match), then POST. **After POST**: re-GET the rule list, match by identity, require exactly 1 match, and set state including the resolved `pos` and all Computed fields.
**Read**: GET + match → 0: remove from state; 1: set state with pos; ≥2: diagnostic error.
**Update**: GET + match → PUT full payload at matched pos.
**Delete**: GET + match → DELETE at matched pos; 0: no-op success.
**No ImportState method** (documented limitation).

- [ ] **Step 6: Register resource, update exports test**
- [ ] **Step 7: Run all tests, verify pass**
- [ ] **Step 8: Commit**

```bash
git add internal/provider/resource_firewall_rule.go internal/provider/provider.go internal/provider/provider_unit_test.go
git commit -m "feat(firewall): add proxmox_firewall_rule resource"
```

---

### Task 3: Docs, example, roadmap

**Files:**
- Create: `examples/resources/proxmox_firewall_rule/resource.tf`
- Create: `docs/resources/firewall_rule.md`
- Modify: `docs/roadmap.md`

- [ ] **Step 1: Create example**

```hcl
resource "proxmox_firewall_rule" "allow_https" {
  type   = "in"
  action = "ACCEPT"
  source = "10.0.0.0/8"
  proto  = "tcp"
  dport  = "443"
}
```

- [ ] **Step 2: Create docs/resources/firewall_rule.md** (schema reference with all fields)
- [ ] **Step 3: Update roadmap** (add to 已完成)
- [ ] **Step 4: Final verification**

Run: `go build ./... && go vet ./... && gofmt -l internal/provider/ && go test ./...`
Expected: all clean, all tests pass.

- [ ] **Step 5: Commit + push**
