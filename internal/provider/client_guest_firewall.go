// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GuestFirewallOptions models per-VM or per-container firewall options.
// Both QEMU and LXC share the same field set at .../firewall/options.
type GuestFirewallOptions struct {
	Enable      proxmoxOptionalBool
	DHCP        proxmoxOptionalBool
	IPFilter    proxmoxOptionalBool
	MACFilter   proxmoxOptionalBool
	LogLevelIn  string
	LogLevelOut string
	PolicyIn    string
	PolicyOut   string
	NDP         proxmoxOptionalBool
	RADV        proxmoxOptionalBool
}

type guestFirewallOptionsKnown struct {
	Enable      proxmoxOptionalBool `json:"enable"`
	DHCP        proxmoxOptionalBool `json:"dhcp"`
	IPFilter    proxmoxOptionalBool `json:"ipfilter"`
	MACFilter   proxmoxOptionalBool `json:"macfilter"`
	LogLevelIn  string              `json:"log_level_in"`
	LogLevelOut string              `json:"log_level_out"`
	PolicyIn    string              `json:"policy_in"`
	PolicyOut   string              `json:"policy_out"`
	NDP         proxmoxOptionalBool `json:"ndp"`
	RADV        proxmoxOptionalBool `json:"radv"`
}

type GuestFirewallOptionsRequest struct {
	Enable      *bool
	DHCP        *bool
	IPFilter    *bool
	MACFilter   *bool
	LogLevelIn  *string
	LogLevelOut *string
	PolicyIn    *string
	PolicyOut   *string
	NDP         *bool
	RADV        *bool
	Delete      []string
}

func (c *Client) GetGuestFirewallOptions(ctx context.Context, kind, node string, vmID int64) (GuestFirewallOptions, error) {
	var raw map[string]json.RawMessage
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/%s/%d/firewall/options", url.PathEscape(node), kind, vmID), nil, nil, &raw); err != nil {
		return GuestFirewallOptions{}, err
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return GuestFirewallOptions{}, fmt.Errorf("unable to marshal raw guest firewall options: %w", err)
	}
	var known guestFirewallOptionsKnown
	if err := json.Unmarshal(payload, &known); err != nil {
		return GuestFirewallOptions{}, fmt.Errorf("unable to decode guest firewall options: %w", err)
	}
	return GuestFirewallOptions{
		Enable:      known.Enable,
		DHCP:        known.DHCP,
		IPFilter:    known.IPFilter,
		MACFilter:   known.MACFilter,
		LogLevelIn:  known.LogLevelIn,
		LogLevelOut: known.LogLevelOut,
		PolicyIn:    known.PolicyIn,
		PolicyOut:   known.PolicyOut,
		NDP:         known.NDP,
		RADV:        known.RADV,
	}, nil
}

func (c *Client) UpdateGuestFirewallOptions(ctx context.Context, kind, node string, vmID int64, req GuestFirewallOptionsRequest) error {
	form := url.Values{}
	setOptionalBool(form, "enable", req.Enable)
	setOptionalBool(form, "dhcp", req.DHCP)
	setOptionalBool(form, "ipfilter", req.IPFilter)
	setOptionalBool(form, "macfilter", req.MACFilter)
	setOptionalString(form, "log_level_in", req.LogLevelIn)
	setOptionalString(form, "log_level_out", req.LogLevelOut)
	setOptionalString(form, "policy_in", req.PolicyIn)
	setOptionalString(form, "policy_out", req.PolicyOut)
	setOptionalBool(form, "ndp", req.NDP)
	setOptionalBool(form, "radv", req.RADV)
	if len(req.Delete) > 0 {
		form.Set("delete", strings.Join(sortedStrings(req.Delete), ","))
	}
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/nodes/%s/%s/%d/firewall/options", url.PathEscape(node), kind, vmID), nil, form, nil)
}

func (c *Client) ResetGuestFirewallOptions(ctx context.Context, kind, node string, vmID int64) error {
	req := GuestFirewallOptionsRequest{
		Delete: []string{"enable", "dhcp", "ipfilter", "macfilter", "log_level_in", "log_level_out", "policy_in", "policy_out", "ndp", "radv"},
	}
	err := c.UpdateGuestFirewallOptions(ctx, kind, node, vmID, req)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}
