// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type LXCSnapshot struct {
	Name        string
	Description string
	Parent      string
	Snaptime    proxmoxOptionalInt64
}

type lxcSnapshotListEntry struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Parent      string               `json:"parent"`
	Snaptime    proxmoxOptionalInt64 `json:"snaptime"`
}

type CreateLXCSnapshotRequest struct {
	Node        string
	VMID        int64
	Name        string
	Description *string
}

func (c *Client) CreateLXCSnapshot(ctx context.Context, req CreateLXCSnapshotRequest) error {
	form := url.Values{}
	form.Set("snapname", req.Name)
	setOptionalString(form, "description", req.Description)

	var upid string
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/nodes/%s/lxc/%d/snapshot", url.PathEscape(req.Node), req.VMID), nil, form, &upid); err != nil {
		return err
	}
	return c.waitForLXCContainerTask(ctx, req.Node, upid)
}

func (c *Client) GetLXCSnapshot(ctx context.Context, node string, vmID int64, name string) (LXCSnapshot, error) {
	var entries []lxcSnapshotListEntry
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/lxc/%d/snapshot", url.PathEscape(node), vmID), nil, nil, &entries); err != nil {
		return LXCSnapshot{}, err
	}
	for _, entry := range entries {
		if entry.Name == name {
			return LXCSnapshot{
				Name:        entry.Name,
				Description: entry.Description,
				Parent:      entry.Parent,
				Snaptime:    entry.Snaptime,
			}, nil
		}
	}
	return LXCSnapshot{}, errNotFound
}

func (c *Client) UpdateLXCSnapshot(ctx context.Context, node string, vmID int64, name string, description string) error {
	form := url.Values{}
	form.Set("description", description)

	var upid string
	if err := c.do(ctx, http.MethodPut, fmt.Sprintf("/nodes/%s/lxc/%d/snapshot/%s/config", url.PathEscape(node), vmID, url.PathEscape(name)), nil, form, &upid); err != nil {
		return err
	}
	if upid == "" {
		return nil
	}
	return c.waitForLXCContainerTask(ctx, node, upid)
}

func (c *Client) DeleteLXCSnapshot(ctx context.Context, node string, vmID int64, name string) error {
	var upid string
	if err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/nodes/%s/lxc/%d/snapshot/%s", url.PathEscape(node), vmID, url.PathEscape(name)), nil, nil, &upid); err != nil {
		if errors.Is(err, errNotFound) {
			return nil
		}
		return err
	}
	if upid == "" {
		return nil
	}
	return c.waitForLXCContainerTask(ctx, node, upid)
}

// lxcSnapshotTaskTimeoutCap mirrors the LXC container task timeout; snapshots
// also return UPIDs that the caller must poll to completion.
var lxcSnapshotTaskTimeoutCap = 10 * time.Minute

func lxcSnapshotID(node string, vmID int64, name string) string {
	return fmt.Sprintf("%s/%d/%s", node, vmID, name)
}

func parseLXCSnapshotImportID(id string) (string, int64, string, error) {
	parts := splitPathSegments(id)
	if len(parts) != 3 || anyEmpty(parts[0], parts[1], parts[2]) {
		return "", 0, "", fmt.Errorf("expected import identifier in node/vm_id/name form")
	}
	vmID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, "", fmt.Errorf("invalid vmid %q: %w", parts[1], err)
	}
	return parts[0], vmID, parts[2], nil
}

func splitPathSegments(id string) []string {
	var segments []string
	current := ""
	for _, r := range id {
		if r == '/' {
			segments = append(segments, current)
			current = ""
			continue
		}
		current += string(r)
	}
	return append(segments, current)
}

func anyEmpty(values ...string) bool {
	for _, v := range values {
		if v == "" {
			return true
		}
	}
	return false
}
