// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// RRDData serializes the raw rrddata array of a node or guest.
func (c *Client) RRDData(ctx context.Context, apiPath string, query url.Values) (string, error) {
	var points []map[string]any
	if err := c.do(ctx, http.MethodGet, apiPath, query, nil, &points); err != nil {
		return "", err
	}
	data, err := json.Marshal(points)
	if err != nil {
		return "", fmt.Errorf("encode rrd data: %w", err)
	}
	return string(data), nil
}

func (c *Client) NodeRRDData(ctx context.Context, node, timeframe, cf string) (string, error) {
	query := url.Values{"timeframe": {timeframe}}
	if cf != "" {
		query.Set("cf", cf)
	}
	return c.RRDData(ctx, "/nodes/"+url.PathEscape(node)+"/rrddata", query)
}

func (c *Client) QemuRRDData(ctx context.Context, node string, vmid int64, timeframe string) (string, error) {
	query := url.Values{"timeframe": {timeframe}}
	path := fmt.Sprintf("/nodes/%s/qemu/%d/rrddata", url.PathEscape(node), vmid)
	return c.RRDData(ctx, path, query)
}
