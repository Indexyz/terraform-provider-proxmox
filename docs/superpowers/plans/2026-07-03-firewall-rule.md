# `proxmox_firewall_rule` Implementation Plan

> **For agentic workers:** Implement this plan task-by-task using TDD. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Add `proxmox_firewall_rule` resource for cluster-level Proxmox firewall rules.

**Architecture:** Content-based identity (type+action+fields), `pos` is computed. Client does CRUD against `/cluster/firewall/rules`, resolving `pos` via list GET + content matching.

## Task 1: Client + tests

**Files:** Create `internal/provider/client_firewall_rule.go` (append to existing client_firewall.go or new file), test in `internal/provider/client_firewall_rule_test.go`.

- [ ] Write client struct + CRUD methods (TDD: test first, then implement)
- [ ] `FirewallRule` struct, `CreateFirewallRule`, `GetFirewallRules` (list), `UpdateFirewallRule`, `DeleteFirewallRule`
- [ ] Test: create POST form, GET list parse, update PUT form, delete
- [ ] Run tests, verify pass

## Task 2: Resource + schema + mapping

**Files:** Create `internal/provider/resource_firewall_rule.go`.

- [ ] `firewallRuleModel`, schema (type/action required+replace, others optional+computed, pos computed)
- [ ] Content-matching helper for read/update/delete
- [ ] Create/Read/Update/Delete lifecycle
- [ ] Register in provider.go
- [ ] Update provider exports test

## Task 3: Docs, example, roadmap

- [ ] Example `examples/resources/proxmox_firewall_rule/resource.tf`
- [ ] `docs/resources/firewall_rule.md`
- [ ] Roadmap update
