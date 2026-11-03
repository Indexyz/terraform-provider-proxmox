// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

type ClusterOptions struct {
	Description       string               `json:"description"`
	Keyboard          string               `json:"keyboard"`
	Console           string               `json:"console"`
	Language          string               `json:"language"`
	EmailFrom         string               `json:"email_from"`
	Fencing           string               `json:"fencing"`
	Migration         string               `json:"migration"`
	MigrationUnsecure proxmoxOptionalBool  `json:"migration_unsecure"`
	MaxWorkers        proxmoxOptionalInt64 `json:"max_workers"`
	TagStyle          string               `json:"tag-style"`
	HA                string               `json:"ha"`
	HTTPProxy         string               `json:"http_proxy"`
	NextID            string               `json:"next-id"`
	MACPrefix         string               `json:"mac_prefix"`
}

type ClusterConfig struct {
	Totem      map[string]json.RawMessage `json:"totem"`
	APIVersion json.RawMessage            `json:"apiversion"`
	Nodes      []string                   `json:"nodes"`
}

func (c *Client) ClusterOptions(ctx context.Context) (ClusterOptions, error) {
	var options ClusterOptions
	if err := c.do(ctx, http.MethodGet, "/cluster/options", nil, nil, &options); err != nil {
		return ClusterOptions{}, err
	}
	return options, nil
}

func (c *Client) UpdateClusterOptions(ctx context.Context, form url.Values) error {
	return c.do(ctx, http.MethodPut, "/cluster/options", nil, form, nil)
}

func (c *Client) ClusterConfig(ctx context.Context) (ClusterConfig, error) {
	var config ClusterConfig
	if err := c.do(ctx, http.MethodGet, "/cluster/config/totem", nil, nil, &config.Totem); err != nil {
		return ClusterConfig{}, err
	}
	if err := c.do(ctx, http.MethodGet, "/cluster/config/apiversion", nil, nil, &config.APIVersion); err != nil {
		return ClusterConfig{}, err
	}
	var nodes []struct {
		Node string `json:"node"`
	}
	if err := c.do(ctx, http.MethodGet, "/cluster/config/nodes", nil, nil, &nodes); err != nil {
		return ClusterConfig{}, err
	}
	for _, entry := range nodes {
		config.Nodes = append(config.Nodes, entry.Node)
	}
	return config, nil
}

// JoinCluster creates a ticket against the peer cluster and joins this node.
func (c *Client) JoinCluster(ctx context.Context, hostname, password, fingerprint string, nodeID, votes *int64, force bool, links []string) (string, error) {
	form := url.Values{"hostname": {hostname}, "password": {password}, "fingerprint": {fingerprint}}
	setOptionalInt64(form, "nodeid", nodeID)
	setOptionalInt64(form, "votes", votes)
	setOptionalBool(form, "force", boolPointer(force))
	for _, link := range links {
		form.Add("link", link)
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/cluster/config/join", nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "cluster join"); err != nil {
		return "", err
	}
	return upid, nil
}

func (c *Client) RemoveClusterNode(ctx context.Context, node string) error {
	err := c.do(ctx, http.MethodDelete, "/cluster/config/nodes/"+url.PathEscape(node), nil, nil, nil)
	return err
}

func boolPointer(value bool) *bool { return &value }
