// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

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

type firewallRuleRaw struct {
	Pos      json.RawMessage      `json:"pos"`
	Type     string               `json:"type"`
	Action   string               `json:"action"`
	Enable   proxmoxOptionalInt64 `json:"enable"`
	Comment  string               `json:"comment"`
	Source   string               `json:"source"`
	Dest     string               `json:"dest"`
	Proto    string               `json:"proto"`
	DPort    string               `json:"dport"`
	SPort    string               `json:"sport"`
	ICMPType string               `json:"icmp-type"`
	Iface    string               `json:"iface"`
	Macro    string               `json:"macro"`
	Log      string               `json:"log"`
}

type FirewallRuleRequest struct {
	Type     string
	Action   string
	Enable   *int64
	Comment  *string
	Source   *string
	Dest     *string
	Proto    *string
	DPort    *string
	SPort    *string
	ICMPType *string
	Iface    *string
	Macro    *string
	Log      *string
}

func (c *Client) GetFirewallRules(ctx context.Context) ([]FirewallRule, error) {
	var rawRules []firewallRuleRaw
	if err := c.do(ctx, http.MethodGet, "/cluster/firewall/rules", nil, nil, &rawRules); err != nil {
		return nil, err
	}
	rules := make([]FirewallRule, 0, len(rawRules))
	for _, raw := range rawRules {
		pos := 0
		if len(raw.Pos) > 0 {
			var posVal json.Number
			if err := json.Unmarshal(raw.Pos, &posVal); err == nil {
				if p, err := posVal.Int64(); err == nil {
					pos = int(p)
				}
			}
		}
		rules = append(rules, FirewallRule{
			Pos:      pos,
			Type:     raw.Type,
			Action:   raw.Action,
			Enable:   raw.Enable,
			Comment:  raw.Comment,
			Source:   raw.Source,
			Dest:     raw.Dest,
			Proto:    raw.Proto,
			DPort:    raw.DPort,
			SPort:    raw.SPort,
			ICMPType: raw.ICMPType,
			Iface:    raw.Iface,
			Macro:    raw.Macro,
			Log:      raw.Log,
		})
	}
	return rules, nil
}

func (c *Client) CreateFirewallRule(ctx context.Context, req FirewallRuleRequest) error {
	form := url.Values{}
	form.Set("type", req.Type)
	form.Set("action", req.Action)
	setOptionalInt64(form, "enable", req.Enable)
	setOptionalString(form, "comment", req.Comment)
	setOptionalString(form, "source", req.Source)
	setOptionalString(form, "dest", req.Dest)
	setOptionalString(form, "proto", req.Proto)
	setOptionalString(form, "dport", req.DPort)
	setOptionalString(form, "sport", req.SPort)
	setOptionalString(form, "icmp-type", req.ICMPType)
	setOptionalString(form, "iface", req.Iface)
	setOptionalString(form, "macro", req.Macro)
	setOptionalString(form, "log", req.Log)
	return c.do(ctx, http.MethodPost, "/cluster/firewall/rules", nil, form, nil)
}

func (c *Client) UpdateFirewallRule(ctx context.Context, pos int, req FirewallRuleRequest) error {
	form := url.Values{}
	form.Set("type", req.Type)
	form.Set("action", req.Action)
	if req.Enable != nil {
		form.Set("enable", strconv.FormatInt(*req.Enable, 10))
	}
	setOptionalString(form, "comment", req.Comment)
	setOptionalString(form, "source", req.Source)
	setOptionalString(form, "dest", req.Dest)
	setOptionalString(form, "proto", req.Proto)
	setOptionalString(form, "dport", req.DPort)
	setOptionalString(form, "sport", req.SPort)
	setOptionalString(form, "icmp-type", req.ICMPType)
	setOptionalString(form, "iface", req.Iface)
	setOptionalString(form, "macro", req.Macro)
	setOptionalString(form, "log", req.Log)
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/cluster/firewall/rules/%d", pos), nil, form, nil)
}

func (c *Client) DeleteFirewallRule(ctx context.Context, pos int) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/cluster/firewall/rules/%d", pos), nil, nil, nil)
}
