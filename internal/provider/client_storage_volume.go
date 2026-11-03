// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

type StorageVolumeMetadata struct {
	Format    string              `json:"format"`
	Notes     string              `json:"notes"`
	Path      string              `json:"path"`
	Protected proxmoxOptionalBool `json:"protected"`
	Size      int64               `json:"size"`
	Used      int64               `json:"used"`
}

func storageVolumePath(node, storage, volume string) string {
	return "/nodes/" + url.PathEscape(node) + "/storage/" + url.PathEscape(storage) + "/content/" + url.PathEscape(volume)
}

func (c *Client) StorageVolume(ctx context.Context, node, storage, volume string) (*StorageVolumeMetadata, error) {
	var metadata StorageVolumeMetadata
	if err := c.do(ctx, http.MethodGet, storageVolumePath(node, storage, volume), nil, nil, &metadata); err != nil {
		return nil, err
	}
	return &metadata, nil
}

func (c *Client) SetStorageVolumeMetadata(ctx context.Context, node, storage, volume, notes string, protected bool) error {
	form := url.Values{}
	_ = form
	if notes != "" && strings.Contains(volume, ":backup/") {
		form.Set("notes", notes)
	}
	if protected {
		form.Set("protected", "1")
	} else {
		form.Set("protected", "0")
	}
	return c.do(ctx, http.MethodPut, storageVolumePath(node, storage, volume), nil, form, nil)
}

func (c *Client) DeleteStorageVolume(ctx context.Context, node, storage, volume string) error {
	return c.do(ctx, http.MethodDelete, storageVolumePath(node, storage, volume), nil, nil, nil)
}

// TransferStorageVolume copies or moves a volume and returns the worker UPID.
func (c *Client) TransferStorageVolume(ctx context.Context, node, storage, volume, targetVolume, targetNode string) (string, error) {
	form := url.Values{"target": {targetVolume}}
	if targetNode != "" {
		form.Set("target_node", targetNode)
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, storageVolumePath(node, storage, volume), nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "volume transfer"); err != nil {
		return "", err
	}
	return upid, nil
}
