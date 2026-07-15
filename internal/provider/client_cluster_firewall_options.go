// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

type ClusterFirewallOptions struct {
	Enable        proxmoxOptionalBool `json:"enable"`
	Ebtables      proxmoxOptionalBool `json:"ebtables"`
	LogRateLimit  string              `json:"log_ratelimit"`
	PolicyForward string              `json:"policy_forward"`
	PolicyIn      string              `json:"policy_in"`
	PolicyOut     string              `json:"policy_out"`
}

type ClusterFirewallOptionsRequest struct {
	Enable        *bool
	Ebtables      *bool
	LogRateLimit  *string
	PolicyForward *string
	PolicyIn      *string
	PolicyOut     *string
	Delete        []string
}

func (c *Client) GetClusterFirewallOptions(ctx context.Context) (ClusterFirewallOptions, error) {
	var options ClusterFirewallOptions
	if err := c.do(ctx, http.MethodGet, "/cluster/firewall/options", nil, nil, &options); err != nil {
		return ClusterFirewallOptions{}, err
	}
	return options, nil
}

func (c *Client) UpdateClusterFirewallOptions(ctx context.Context, req ClusterFirewallOptionsRequest) error {
	form := url.Values{}
	setOptionalBool(form, "enable", req.Enable)
	setOptionalBool(form, "ebtables", req.Ebtables)
	setOptionalString(form, "log_ratelimit", req.LogRateLimit)
	setOptionalString(form, "policy_forward", req.PolicyForward)
	setOptionalString(form, "policy_in", req.PolicyIn)
	setOptionalString(form, "policy_out", req.PolicyOut)
	if len(req.Delete) > 0 {
		form.Set("delete", strings.Join(sortedStrings(req.Delete), ","))
	}
	return c.do(ctx, http.MethodPut, "/cluster/firewall/options", nil, form, nil)
}

func (c *Client) ResetClusterFirewallOptions(ctx context.Context) error {
	err := c.UpdateClusterFirewallOptions(ctx, ClusterFirewallOptionsRequest{Delete: []string{
		"enable", "ebtables", "log_ratelimit", "policy_forward", "policy_in", "policy_out",
	}})
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}
