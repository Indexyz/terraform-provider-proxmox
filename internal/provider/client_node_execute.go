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

type NodeCommandCall struct {
	Method string            `json:"method"`
	Path   string            `json:"path"`
	Args   map[string]string `json:"args,omitempty"`
}

type NodeCommandResult struct {
	Status  int             `json:"status"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
}

// ExecuteCommands runs several API calls in order on one node.
func (c *Client) ExecuteCommands(ctx context.Context, node string, calls []NodeCommandCall) ([]NodeCommandResult, error) {
	encoded, err := json.Marshal(calls)
	if err != nil {
		return nil, fmt.Errorf("encode command batch: %w", err)
	}
	form := url.Values{"commands": {string(encoded)}}
	var results []NodeCommandResult
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/execute", nil, form, &results); err != nil {
		return nil, err
	}
	for _, result := range results {
		if result.Status >= 400 {
			return results, fmt.Errorf("command %d failed with status %d: %s", len(results), result.Status, result.Message)
		}
	}
	return results, nil
}

type ClusterMetric struct {
	ID        string  `json:"id"`
	Metric    string  `json:"metric"`
	Type      string  `json:"type"`
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

func (c *Client) MetricsExport(ctx context.Context, nodeList string, history bool, startTime int64) ([]ClusterMetric, error) {
	query := url.Values{"node-list": {nodeList}}
	if history {
		query.Set("history", "1")
	}
	if startTime > 0 {
		query.Set("start-time", strconv.FormatInt(startTime, 10))
	}
	var result struct {
		Data []ClusterMetric `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/cluster/metrics/export", query, nil, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}
