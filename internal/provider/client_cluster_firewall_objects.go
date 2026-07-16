// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

type ClusterFirewallAlias struct {
	CIDR    string `json:"cidr"`
	Comment string `json:"comment"`
	Digest  string `json:"digest"`
	Name    string `json:"name"`
}

type ClusterFirewallIPSet struct {
	Comment string `json:"comment"`
	Digest  string `json:"digest"`
	Name    string `json:"name"`
}

type ClusterFirewallIPSetEntry struct {
	CIDR    string              `json:"cidr"`
	Comment string              `json:"comment"`
	Digest  string              `json:"digest"`
	NoMatch proxmoxOptionalBool `json:"nomatch"`
}

type ClusterFirewallSecurityGroup struct {
	Comment string `json:"comment"`
	Digest  string `json:"digest"`
	Name    string `json:"group"`
}

func (c *Client) GetClusterFirewallAlias(ctx context.Context, name string) (ClusterFirewallAlias, error) {
	var alias ClusterFirewallAlias
	if err := c.do(ctx, http.MethodGet, "/cluster/firewall/aliases/"+url.PathEscape(name), nil, nil, &alias); err != nil {
		return ClusterFirewallAlias{}, err
	}
	if alias.Name == "" {
		alias.Name = name
	}
	return alias, nil
}

func (c *Client) CreateClusterFirewallAlias(ctx context.Context, alias ClusterFirewallAlias) error {
	form := url.Values{"cidr": {alias.CIDR}, "name": {alias.Name}}
	if alias.Comment != "" {
		form.Set("comment", alias.Comment)
	}
	return c.do(ctx, http.MethodPost, "/cluster/firewall/aliases", nil, form, nil)
}

func (c *Client) UpdateClusterFirewallAlias(ctx context.Context, alias ClusterFirewallAlias) error {
	form := url.Values{"cidr": {alias.CIDR}, "comment": {alias.Comment}}
	if alias.Digest != "" {
		form.Set("digest", alias.Digest)
	}
	return c.do(ctx, http.MethodPut, "/cluster/firewall/aliases/"+url.PathEscape(alias.Name), nil, form, nil)
}

func (c *Client) DeleteClusterFirewallAlias(ctx context.Context, name, digest string) error {
	form := url.Values{}
	if digest != "" {
		form.Set("digest", digest)
	}
	err := c.do(ctx, http.MethodDelete, "/cluster/firewall/aliases/"+url.PathEscape(name), nil, form, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

func (c *Client) ClusterFirewallIPSets(ctx context.Context) ([]ClusterFirewallIPSet, error) {
	var sets []ClusterFirewallIPSet
	err := c.do(ctx, http.MethodGet, "/cluster/firewall/ipset", nil, nil, &sets)
	return sets, err
}

func (c *Client) GetClusterFirewallIPSet(ctx context.Context, name string) (ClusterFirewallIPSet, error) {
	sets, err := c.ClusterFirewallIPSets(ctx)
	if err != nil {
		return ClusterFirewallIPSet{}, err
	}
	for _, set := range sets {
		if set.Name == name {
			return set, nil
		}
	}
	return ClusterFirewallIPSet{}, errNotFound
}

func (c *Client) CreateClusterFirewallIPSet(ctx context.Context, set ClusterFirewallIPSet) error {
	form := url.Values{"name": {set.Name}}
	if set.Comment != "" {
		form.Set("comment", set.Comment)
	}
	return c.do(ctx, http.MethodPost, "/cluster/firewall/ipset", nil, form, nil)
}

func (c *Client) UpdateClusterFirewallIPSet(ctx context.Context, set ClusterFirewallIPSet) error {
	form := url.Values{"comment": {set.Comment}, "name": {set.Name}, "rename": {set.Name}}
	if set.Digest != "" {
		form.Set("digest", set.Digest)
	}
	return c.do(ctx, http.MethodPost, "/cluster/firewall/ipset", nil, form, nil)
}

func (c *Client) DeleteClusterFirewallIPSet(ctx context.Context, name string) error {
	err := c.do(ctx, http.MethodDelete, "/cluster/firewall/ipset/"+url.PathEscape(name), nil, nil, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

func (c *Client) GetClusterFirewallIPSetEntry(ctx context.Context, name, cidr string) (ClusterFirewallIPSetEntry, error) {
	var entry ClusterFirewallIPSetEntry
	if err := c.do(ctx, http.MethodGet, clusterFirewallIPSetEntryPath(name, cidr), nil, nil, &entry); err != nil {
		return ClusterFirewallIPSetEntry{}, err
	}
	if entry.CIDR == "" {
		entry.CIDR = cidr
	}
	return entry, nil
}

func (c *Client) CreateClusterFirewallIPSetEntry(ctx context.Context, name string, entry ClusterFirewallIPSetEntry) error {
	form := url.Values{"cidr": {entry.CIDR}}
	if entry.Comment != "" {
		form.Set("comment", entry.Comment)
	}
	if entry.NoMatch.Ptr() != nil {
		setOptionalBool(form, "nomatch", entry.NoMatch.Ptr())
	}
	return c.do(ctx, http.MethodPost, "/cluster/firewall/ipset/"+url.PathEscape(name), nil, form, nil)
}

func (c *Client) UpdateClusterFirewallIPSetEntry(ctx context.Context, name string, entry ClusterFirewallIPSetEntry) error {
	form := url.Values{"comment": {entry.Comment}}
	setOptionalBool(form, "nomatch", entry.NoMatch.Ptr())
	if entry.Digest != "" {
		form.Set("digest", entry.Digest)
	}
	return c.do(ctx, http.MethodPut, clusterFirewallIPSetEntryPath(name, entry.CIDR), nil, form, nil)
}

func (c *Client) DeleteClusterFirewallIPSetEntry(ctx context.Context, name, cidr, digest string) error {
	form := url.Values{}
	if digest != "" {
		form.Set("digest", digest)
	}
	err := c.do(ctx, http.MethodDelete, clusterFirewallIPSetEntryPath(name, cidr), nil, form, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

func clusterFirewallIPSetEntryPath(name, cidr string) string {
	return "/cluster/firewall/ipset/" + url.PathEscape(name) + "/" + url.PathEscape(cidr)
}

func (c *Client) ClusterFirewallSecurityGroups(ctx context.Context) ([]ClusterFirewallSecurityGroup, error) {
	var groups []ClusterFirewallSecurityGroup
	err := c.do(ctx, http.MethodGet, "/cluster/firewall/groups", nil, nil, &groups)
	return groups, err
}

func (c *Client) GetClusterFirewallSecurityGroup(ctx context.Context, name string) (ClusterFirewallSecurityGroup, error) {
	groups, err := c.ClusterFirewallSecurityGroups(ctx)
	if err != nil {
		return ClusterFirewallSecurityGroup{}, err
	}
	for _, group := range groups {
		if group.Name == name {
			return group, nil
		}
	}
	return ClusterFirewallSecurityGroup{}, errNotFound
}

func (c *Client) CreateClusterFirewallSecurityGroup(ctx context.Context, group ClusterFirewallSecurityGroup) error {
	form := url.Values{"group": {group.Name}}
	if group.Comment != "" {
		form.Set("comment", group.Comment)
	}
	return c.do(ctx, http.MethodPost, "/cluster/firewall/groups", nil, form, nil)
}

func (c *Client) UpdateClusterFirewallSecurityGroup(ctx context.Context, group ClusterFirewallSecurityGroup) error {
	form := url.Values{"comment": {group.Comment}, "group": {group.Name}, "rename": {group.Name}}
	if group.Digest != "" {
		form.Set("digest", group.Digest)
	}
	return c.do(ctx, http.MethodPost, "/cluster/firewall/groups", nil, form, nil)
}

func (c *Client) DeleteClusterFirewallSecurityGroup(ctx context.Context, name string) error {
	err := c.do(ctx, http.MethodDelete, "/cluster/firewall/groups/"+url.PathEscape(name), nil, nil, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}
