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

// snapshotKind selects the Proxmox guest kind for shared snapshot operations.
type snapshotKind string

const (
	snapshotKindQEMU snapshotKind = "qemu"
	snapshotKindLXC  snapshotKind = "lxc"
)

func (k snapshotKind) basePath(node string, vmID int64) string {
	return fmt.Sprintf("/nodes/%s/%s/%d", url.PathEscape(node), string(k), vmID)
}

type snapshotListEntry struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Parent      string               `json:"parent"`
	Snaptime    proxmoxOptionalInt64 `json:"snaptime"`
}

func (c *Client) createSnapshot(ctx context.Context, kind snapshotKind, node string, vmID int64, name string, description *string) error {
	form := url.Values{}
	form.Set("snapname", name)
	setOptionalString(form, "description", description)
	var upid string
	if err := c.do(ctx, http.MethodPost, kind.basePath(node, vmID)+"/snapshot", nil, form, &upid); err != nil {
		return err
	}
	return c.waitSnapshotTask(ctx, kind, node, upid)
}

func (c *Client) getSnapshot(ctx context.Context, kind snapshotKind, node string, vmID int64, name string) (snapshotListEntry, error) {
	var entries []snapshotListEntry
	if err := c.do(ctx, http.MethodGet, kind.basePath(node, vmID)+"/snapshot", nil, nil, &entries); err != nil {
		return snapshotListEntry{}, err
	}
	for _, entry := range entries {
		if entry.Name == name {
			return entry, nil
		}
	}
	return snapshotListEntry{}, errNotFound
}

func (c *Client) updateSnapshot(ctx context.Context, kind snapshotKind, node string, vmID int64, name string, description string) error {
	form := url.Values{}
	form.Set("description", description)
	var upid string
	endpoint := fmt.Sprintf("%s/snapshot/%s/config", kind.basePath(node, vmID), url.PathEscape(name))
	if err := c.do(ctx, http.MethodPut, endpoint, nil, form, &upid); err != nil {
		return err
	}
	return c.waitSnapshotTask(ctx, kind, node, upid)
}

func (c *Client) deleteSnapshot(ctx context.Context, kind snapshotKind, node string, vmID int64, name string) error {
	var upid string
	endpoint := fmt.Sprintf("%s/snapshot/%s", kind.basePath(node, vmID), url.PathEscape(name))
	if err := c.do(ctx, http.MethodDelete, endpoint, nil, nil, &upid); err != nil {
		if errors.Is(err, errNotFound) {
			return nil
		}
		return err
	}
	return c.waitSnapshotTask(ctx, kind, node, upid)
}

// waitSnapshotTask polls a snapshot task to completion. It reuses the LXC
// container task polling because snapshot endpoints return UPIDs scoped to
// the same node task tracker.
func (c *Client) waitSnapshotTask(ctx context.Context, kind snapshotKind, node, upid string) error {
	if upid == "" {
		return nil
	}
	return c.waitForLXCContainerTask(ctx, node, upid)
}
