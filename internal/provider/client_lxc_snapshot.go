// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type LXCSnapshot = snapshotListEntry

type CreateLXCSnapshotRequest struct {
	Node        string
	VMID        int64
	Name        string
	Description *string
}

func (c *Client) CreateLXCSnapshot(ctx context.Context, req CreateLXCSnapshotRequest) error {
	return c.createSnapshot(ctx, snapshotKindLXC, req.Node, req.VMID, req.Name, req.Description)
}

func (c *Client) GetLXCSnapshot(ctx context.Context, node string, vmID int64, name string) (LXCSnapshot, error) {
	return c.getSnapshot(ctx, snapshotKindLXC, node, vmID, name)
}

func (c *Client) UpdateLXCSnapshot(ctx context.Context, node string, vmID int64, name string, description string) error {
	return c.updateSnapshot(ctx, snapshotKindLXC, node, vmID, name, description)
}

func (c *Client) DeleteLXCSnapshot(ctx context.Context, node string, vmID int64, name string) error {
	return c.deleteSnapshot(ctx, snapshotKindLXC, node, vmID, name)
}

func lxcSnapshotID(node string, vmID int64, name string) string {
	return fmt.Sprintf("%s/%d/%s", node, vmID, name)
}

func parseLXCSnapshotImportID(id string) (string, int64, string, error) {
	parts := strings.Split(id, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", 0, "", fmt.Errorf("expected import identifier in node/vm_id/name form")
	}
	vmID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, "", fmt.Errorf("invalid vmid %q: %w", parts[1], err)
	}
	return parts[0], vmID, parts[2], nil
}
