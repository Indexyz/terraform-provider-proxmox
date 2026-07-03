// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type QemuSnapshot = snapshotListEntry

type CreateQemuSnapshotRequest struct {
	Node        string
	VMID        int64
	Name        string
	Description *string
}

func (c *Client) CreateQemuSnapshot(ctx context.Context, req CreateQemuSnapshotRequest) error {
	return c.createSnapshot(ctx, snapshotKindQEMU, req.Node, req.VMID, req.Name, req.Description)
}

func (c *Client) GetQemuSnapshot(ctx context.Context, node string, vmID int64, name string) (QemuSnapshot, error) {
	return c.getSnapshot(ctx, snapshotKindQEMU, node, vmID, name)
}

func (c *Client) UpdateQemuSnapshot(ctx context.Context, node string, vmID int64, name string, description string) error {
	return c.updateSnapshot(ctx, snapshotKindQEMU, node, vmID, name, description)
}

func (c *Client) DeleteQemuSnapshot(ctx context.Context, node string, vmID int64, name string) error {
	return c.deleteSnapshot(ctx, snapshotKindQEMU, node, vmID, name)
}

func qemuSnapshotID(node string, vmID int64, name string) string {
	return fmt.Sprintf("%s/%d/%s", node, vmID, name)
}

func parseQemuSnapshotImportID(id string) (string, int64, string, error) {
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
