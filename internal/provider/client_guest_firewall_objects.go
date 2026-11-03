// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

type GuestFirewallAlias struct {
	CIDR    string `json:"cidr"`
	Comment string `json:"comment"`
	Digest  string `json:"digest"`
	Name    string `json:"name"`
}

type GuestFirewallIPSet struct {
	Comment string `json:"comment"`
	Digest  string `json:"digest"`
	Name    string `json:"name"`
}

type GuestFirewallIPSetEntry struct {
	CIDR    string              `json:"cidr"`
	Comment string              `json:"comment"`
	Digest  string              `json:"digest"`
	NoMatch proxmoxOptionalBool `json:"nomatch"`
}

func guestFirewallPath(node, guestType string, vmid int64) string {
	return fmt.Sprintf("/nodes/%s/%s/%d/firewall", url.PathEscape(node), guestType, vmid)
}

func (c *Client) GuestFirewallAliases(ctx context.Context, node, guestType string, vmid int64) ([]GuestFirewallAlias, error) {
	var aliases []GuestFirewallAlias
	err := c.do(ctx, http.MethodGet, guestFirewallPath(node, guestType, vmid)+"/aliases", nil, nil, &aliases)
	return aliases, err
}

func (c *Client) GetGuestFirewallAlias(ctx context.Context, node, guestType string, vmid int64, name string) (GuestFirewallAlias, error) {
	aliases, err := c.GuestFirewallAliases(ctx, node, guestType, vmid)
	if err != nil {
		return GuestFirewallAlias{}, err
	}
	for _, alias := range aliases {
		if alias.Name == name {
			return alias, nil
		}
	}
	return GuestFirewallAlias{}, errNotFound
}

func (c *Client) CreateGuestFirewallAlias(ctx context.Context, node, guestType string, vmid int64, alias GuestFirewallAlias) error {
	form := url.Values{"cidr": {alias.CIDR}, "name": {alias.Name}}
	if alias.Comment != "" {
		form.Set("comment", alias.Comment)
	}
	return c.do(ctx, http.MethodPost, guestFirewallPath(node, guestType, vmid)+"/aliases", nil, form, nil)
}

func (c *Client) UpdateGuestFirewallAlias(ctx context.Context, node, guestType string, vmid int64, alias GuestFirewallAlias) error {
	form := url.Values{"cidr": {alias.CIDR}, "comment": {alias.Comment}}
	if alias.Digest != "" {
		form.Set("digest", alias.Digest)
	}
	return c.do(ctx, http.MethodPut, guestFirewallPath(node, guestType, vmid)+"/aliases/"+url.PathEscape(alias.Name), nil, form, nil)
}

func (c *Client) DeleteGuestFirewallAlias(ctx context.Context, node, guestType string, vmid int64, name, digest string) error {
	form := url.Values{}
	if digest != "" {
		form.Set("digest", digest)
	}
	err := c.do(ctx, http.MethodDelete, guestFirewallPath(node, guestType, vmid)+"/aliases/"+url.PathEscape(name), nil, form, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

func (c *Client) GuestFirewallIPSets(ctx context.Context, node, guestType string, vmid int64) ([]GuestFirewallIPSet, error) {
	var sets []GuestFirewallIPSet
	err := c.do(ctx, http.MethodGet, guestFirewallPath(node, guestType, vmid)+"/ipset", nil, nil, &sets)
	return sets, err
}

func (c *Client) GetGuestFirewallIPSet(ctx context.Context, node, guestType string, vmid int64, name string) (GuestFirewallIPSet, error) {
	sets, err := c.GuestFirewallIPSets(ctx, node, guestType, vmid)
	if err != nil {
		return GuestFirewallIPSet{}, err
	}
	for _, set := range sets {
		if set.Name == name {
			return set, nil
		}
	}
	return GuestFirewallIPSet{}, errNotFound
}

func (c *Client) CreateGuestFirewallIPSet(ctx context.Context, node, guestType string, vmid int64, set GuestFirewallIPSet) error {
	form := url.Values{"name": {set.Name}}
	if set.Comment != "" {
		form.Set("comment", set.Comment)
	}
	return c.do(ctx, http.MethodPost, guestFirewallPath(node, guestType, vmid)+"/ipset", nil, form, nil)
}

func (c *Client) UpdateGuestFirewallIPSet(ctx context.Context, node, guestType string, vmid int64, set GuestFirewallIPSet) error {
	form := url.Values{"comment": {set.Comment}, "name": {set.Name}, "rename": {set.Name}}
	if set.Digest != "" {
		form.Set("digest", set.Digest)
	}
	return c.do(ctx, http.MethodPost, guestFirewallPath(node, guestType, vmid)+"/ipset", nil, form, nil)
}

func (c *Client) DeleteGuestFirewallIPSet(ctx context.Context, node, guestType string, vmid int64, name string) error {
	err := c.do(ctx, http.MethodDelete, guestFirewallPath(node, guestType, vmid)+"/ipset/"+url.PathEscape(name), nil, nil, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

func (c *Client) GuestFirewallIPSetEntries(ctx context.Context, node, guestType string, vmid int64, name string) ([]GuestFirewallIPSetEntry, error) {
	var entries []GuestFirewallIPSetEntry
	err := c.do(ctx, http.MethodGet, guestFirewallPath(node, guestType, vmid)+"/ipset/"+url.PathEscape(name), nil, nil, &entries)
	return entries, err
}

func (c *Client) GetGuestFirewallIPSetEntry(ctx context.Context, node, guestType string, vmid int64, name, cidr string) (GuestFirewallIPSetEntry, error) {
	entries, err := c.GuestFirewallIPSetEntries(ctx, node, guestType, vmid, name)
	if err != nil {
		return GuestFirewallIPSetEntry{}, err
	}
	for _, entry := range entries {
		if entry.CIDR == cidr {
			return entry, nil
		}
	}
	return GuestFirewallIPSetEntry{}, errNotFound
}

func (c *Client) CreateGuestFirewallIPSetEntry(ctx context.Context, node, guestType string, vmid int64, name string, entry GuestFirewallIPSetEntry) error {
	form := url.Values{"cidr": {entry.CIDR}}
	if entry.Comment != "" {
		form.Set("comment", entry.Comment)
	}
	if entry.NoMatch.Ptr() != nil {
		setOptionalBool(form, "nomatch", entry.NoMatch.Ptr())
	}
	return c.do(ctx, http.MethodPost, guestFirewallPath(node, guestType, vmid)+"/ipset/"+url.PathEscape(name), nil, form, nil)
}

func (c *Client) UpdateGuestFirewallIPSetEntry(ctx context.Context, node, guestType string, vmid int64, name string, entry GuestFirewallIPSetEntry) error {
	form := url.Values{"comment": {entry.Comment}}
	if entry.NoMatch.Ptr() != nil {
		setOptionalBool(form, "nomatch", entry.NoMatch.Ptr())
	}
	if entry.Digest != "" {
		form.Set("digest", entry.Digest)
	}
	return c.do(ctx, http.MethodPut, guestFirewallPath(node, guestType, vmid)+"/ipset/"+url.PathEscape(name)+"/"+url.PathEscape(entry.CIDR), nil, form, nil)
}

func (c *Client) DeleteGuestFirewallIPSetEntry(ctx context.Context, node, guestType string, vmid int64, name, cidr, digest string) error {
	form := url.Values{}
	if digest != "" {
		form.Set("digest", digest)
	}
	err := c.do(ctx, http.MethodDelete, guestFirewallPath(node, guestType, vmid)+"/ipset/"+url.PathEscape(name)+"/"+url.PathEscape(cidr), nil, form, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}
