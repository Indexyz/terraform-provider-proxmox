// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

type HARule struct {
	Rule      string               `json:"rule"`
	Type      string               `json:"type"`
	Resources string               `json:"resources"`
	Nodes     string               `json:"nodes"`
	Affinity  string               `json:"affinity"`
	Strict    proxmoxOptionalBool  `json:"strict"`
	Disable   proxmoxOptionalBool  `json:"disable"`
	Comment   string               `json:"comment"`
	Order     proxmoxOptionalInt64 `json:"order"`
	Errors    map[string]string    `json:"errors"`
	Digest    string               `json:"digest"`
}

type HANode struct {
	Node     string
	Priority int64
}

type HARuleRequest struct {
	Rule      string
	Type      string
	Resources []string
	Nodes     []HANode
	Affinity  *string
	Strict    *bool
	Disable   *bool
	Comment   *string
	Digest    *string
	Delete    []string
}

func (c *Client) HARules(ctx context.Context) ([]HARule, error) {
	var rules []HARule
	if err := c.do(ctx, http.MethodGet, "/cluster/ha/rules", nil, nil, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func (c *Client) GetHARule(ctx context.Context, rule string) (HARule, error) {
	rules, err := c.HARules(ctx)
	if err != nil {
		return HARule{}, err
	}
	for _, entry := range rules {
		if entry.Rule == rule {
			return entry, nil
		}
	}
	return HARule{}, errNotFound
}

func (c *Client) CreateHARule(ctx context.Context, rule string, req HARuleRequest) error {
	form := haRuleForm(req)
	form.Set("rule", rule)
	if req.Type != "" {
		form.Set("type", req.Type)
	}
	return c.do(ctx, http.MethodPost, "/cluster/ha/rules", nil, form, nil)
}

func (c *Client) UpdateHARule(ctx context.Context, rule string, req HARuleRequest) error {
	form := haRuleForm(req)
	// `rule` stays a required body property of the update schema; `type` is not part of it.
	form.Set("rule", rule)
	setOptionalString(form, "digest", req.Digest)
	if len(req.Delete) > 0 {
		form.Set("delete", strings.Join(sortedStrings(req.Delete), ","))
	}
	return c.do(ctx, http.MethodPut, "/cluster/ha/rules/"+url.PathEscape(rule), nil, form, nil)
}

func (c *Client) DeleteHARule(ctx context.Context, rule string) error {
	// Proxmox does not check existence on DELETE, so a missing rule is already success.
	return c.do(ctx, http.MethodDelete, "/cluster/ha/rules/"+url.PathEscape(rule), nil, nil, nil)
}

func haRuleForm(req HARuleRequest) url.Values {
	form := url.Values{}
	if len(req.Resources) > 0 {
		form.Set("resources", strings.Join(sortedStrings(req.Resources), ","))
	}
	if len(req.Nodes) > 0 {
		nodes := slices.Clone(req.Nodes)
		slices.SortFunc(nodes, func(a, b HANode) int { return strings.Compare(a.Node, b.Node) })
		entries := make([]string, 0, len(nodes))
		for _, node := range nodes {
			if node.Priority == 0 {
				entries = append(entries, node.Node)
				continue
			}
			entries = append(entries, fmt.Sprintf("%s:%d", node.Node, node.Priority))
		}
		form.Set("nodes", strings.Join(entries, ","))
	}
	setOptionalString(form, "affinity", req.Affinity)
	setOptionalBool(form, "disable", req.Disable)
	setOptionalString(form, "comment", req.Comment)
	setOptionalBool(form, "strict", req.Strict)
	return form
}
