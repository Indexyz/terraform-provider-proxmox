// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

func (c *Client) HAResourceAction(ctx context.Context, sid, action, node string) (string, error) {
	var result map[string]any
	apiPath := "/cluster/ha/resources/" + url.PathEscape(sid) + "/" + action
	if err := c.do(ctx, http.MethodPost, apiPath, nil, url.Values{"node": {node}}, &result); err != nil {
		return "", err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *Client) HAStatusSummary(ctx context.Context) (string, error) {
	var items []map[string]any
	if err := c.do(ctx, http.MethodGet, "/cluster/ha/status", nil, nil, &items); err != nil {
		return "", err
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type CephHealthMute struct {
	Code    string
	Summary string
	TTL     string
	Sticky  proxmoxOptionalBool
}

func (c *Client) CephHealthMutes(ctx context.Context) ([]CephHealthMute, error) {
	var items []CephHealthMute
	if err := c.do(ctx, http.MethodGet, "/cluster/ceph/health-mute", nil, nil, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (c *Client) SetCephHealthMute(ctx context.Context, code string, value bool, ttl string, sticky int64) error {
	form := url.Values{"value": {map[bool]string{true: "1", false: "0"}[value]}}
	if ttl != "" {
		form.Set("ttl", ttl)
	}
	if sticky > 0 {
		form.Set("sticky", strconv.FormatInt(sticky, 10))
	}
	return c.do(ctx, http.MethodPut, "/cluster/ceph/health-mute/"+url.PathEscape(code), nil, form, nil)
}

func (c *Client) CephMetadata(ctx context.Context, scope string) (string, error) {
	var metadata map[string]any
	if err := c.do(ctx, http.MethodGet, "/cluster/ceph/metadata", url.Values{"scope": {scope}}, nil, &metadata); err != nil {
		return "", err
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *Client) CephRestartBulkCluster(ctx context.Context, serviceType string, force, onlyOutdated bool, timeout int64) (string, error) {
	form := url.Values{}
	if serviceType != "" {
		form.Set("service-type", serviceType)
	}
	if force {
		form.Set("force", "1")
	}
	if onlyOutdated {
		form.Set("only-outdated", "1")
	}
	if timeout > 0 {
		form.Set("timeout", strconv.FormatInt(timeout, 10))
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/cluster/ceph/restart-bulk", nil, form, &upid); err != nil {
		return "", err
	}
	if upid == "" {
		return "", nil
	}
	return upid, validateQemuTaskAck(upid, "ceph restart bulk")
}
