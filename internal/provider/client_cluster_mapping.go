// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

type ClusterMapping struct {
	ID            string                     `json:"id"`
	Description   string                     `json:"description"`
	Map           []string                   `json:"map"`
	LiveMigration proxmoxOptionalBool        `json:"live-migration-capable"`
	Digest        string                     `json:"digest"`
	Checks        []ClusterMappingCheck      `json:"checks"`
	Pending       map[string]json.RawMessage `json:"pending"`
}

type ClusterMappingCheck struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

func clusterMappingPath(kind string) string { return "/cluster/mapping/" + kind }

func (c *Client) ClusterMappings(ctx context.Context, kind string) ([]ClusterMapping, error) {
	var entries []ClusterMapping
	if err := c.do(ctx, http.MethodGet, clusterMappingPath(kind), nil, nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (c *Client) GetClusterMapping(ctx context.Context, kind, id string) (ClusterMapping, error) {
	entries, err := c.ClusterMappings(ctx, kind)
	if err != nil {
		return ClusterMapping{}, err
	}
	for _, entry := range entries {
		if entry.ID == id {
			return entry, nil
		}
	}
	return ClusterMapping{}, errNotFound
}

func (c *Client) CreateClusterMapping(ctx context.Context, kind, id string, entry ClusterMapping) error {
	form := url.Values{"id": {id}}
	for _, item := range entry.Map {
		form.Add("map", item)
	}
	setOptionalString(form, "description", optionalClusterMappingString(entry.Description))
	if entry.LiveMigration.Ptr() != nil {
		setOptionalBool(form, "live-migration-capable", entry.LiveMigration.Ptr())
	}
	return c.do(ctx, http.MethodPost, clusterMappingPath(kind), nil, form, nil)
}

func (c *Client) UpdateClusterMapping(ctx context.Context, kind, id string, entry ClusterMapping, deleted []string) error {
	form := url.Values{}
	for _, item := range entry.Map {
		form.Add("map", item)
	}
	setOptionalString(form, "description", optionalClusterMappingString(entry.Description))
	if entry.LiveMigration.Ptr() != nil {
		setOptionalBool(form, "live-migration-capable", entry.LiveMigration.Ptr())
	}
	if entry.Digest != "" {
		form.Set("digest", entry.Digest)
	}
	if len(deleted) > 0 {
		form.Set("delete", strings.Join(sortedStrings(deleted), ","))
	}
	return c.do(ctx, http.MethodPut, clusterMappingPath(kind)+"/"+url.PathEscape(id), nil, form, nil)
}

func (c *Client) DeleteClusterMapping(ctx context.Context, kind, id string) error {
	// The mapping DELETE schema has no digest parameter.
	err := c.do(ctx, http.MethodDelete, clusterMappingPath(kind)+"/"+url.PathEscape(id), nil, nil, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

func optionalClusterMappingString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// encodeClusterMappingEntry renders one map entry as a Proxmox property string.
func encodeClusterMappingEntry(pairs map[string]string) string {
	keys := make([]string, 0, len(pairs))
	for key, value := range pairs {
		if value != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+pairs[key])
	}
	return strings.Join(parts, ",")
}
