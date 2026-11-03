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

type NodeServiceState struct {
	Service     string `json:"service"`
	Name        string `json:"name"`
	Desc        string `json:"desc"`
	State       string `json:"state"`
	ActiveState string `json:"active-state"`
	UnitState   string `json:"unit-state"`
}

type APTRepositoryFile struct {
	Path         string          `json:"path"`
	Digest       []byte          `json:"digest"`
	Product      string          `json:"product"`
	Repositories []APTRepository `json:"repositories"`
}

type APTRepository struct {
	Types      []string            `json:"Types"`
	URIs       []string            `json:"URIs"`
	Suites     []string            `json:"Suites"`
	Components []string            `json:"Components"`
	Enabled    proxmoxOptionalBool `json:"Enabled"`
	Comment    string              `json:"Comment"`
}

type APTRepositories struct {
	Digest string              `json:"digest"`
	Files  []APTRepositoryFile `json:"files"`
	Errors []json.RawMessage   `json:"errors"`
}

func nodeServicesPath(node string) string {
	return "/nodes/" + url.PathEscape(node) + "/services"
}

func (c *Client) NodeServiceState(ctx context.Context, node, service string) (NodeServiceState, error) {
	var state NodeServiceState
	if err := c.do(ctx, http.MethodGet, nodeServicesPath(node)+"/"+url.PathEscape(service)+"/state", nil, nil, &state); err != nil {
		return NodeServiceState{}, err
	}
	return state, nil
}

// NodeServiceAction runs start, stop, restart or reload and returns the UPID.
func (c *Client) NodeServiceAction(ctx context.Context, node, service, action string) (string, error) {
	var upid string
	if err := c.do(ctx, http.MethodPost, nodeServicesPath(node)+"/"+url.PathEscape(service)+"/"+action, nil, url.Values{}, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "service "+action); err != nil {
		return "", err
	}
	return upid, nil
}

func (c *Client) NodeAPTRepositories(ctx context.Context, node string) (APTRepositories, error) {
	var repositories APTRepositories
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/apt/repositories", nil, nil, &repositories); err != nil {
		return APTRepositories{}, err
	}
	return repositories, nil
}

// ToggleAPTRepository enables or disables one repository entry identified by
// its file path and index.
func (c *Client) ToggleAPTRepository(ctx context.Context, node, apiPath string, index int64, enabled bool, digest string) error {
	form := url.Values{"path": {apiPath}, "index": {fmt.Sprintf("%d", index)}, "enabled": {"0"}}
	if enabled {
		form.Set("enabled", "1")
	}
	if digest != "" {
		form.Set("digest", digest)
	}
	return c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/apt/repositories", nil, form, nil)
}

func (c *Client) NodeAPTUpdate(ctx context.Context, node string, quiet, notify bool) (string, error) {
	form := url.Values{}
	if quiet {
		form.Set("quiet", "1")
	}
	if notify {
		form.Set("notify", "1")
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/apt/update", nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "APT update"); err != nil {
		return "", err
	}
	return upid, nil
}
