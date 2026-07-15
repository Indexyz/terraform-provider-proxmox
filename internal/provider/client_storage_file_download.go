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

type StorageFile struct {
	Format    string               `json:"format"`
	Notes     string               `json:"notes"`
	Path      string               `json:"path"`
	Protected proxmoxOptionalBool  `json:"protected"`
	Size      proxmoxOptionalInt64 `json:"size"`
	Used      proxmoxOptionalInt64 `json:"used"`
}

type DownloadStorageFileRequest struct {
	Node               string
	Storage            string
	Content            string
	Filename           string
	URL                string
	Checksum           *string
	ChecksumAlgorithm  *string
	Compression        *string
	VerifyCertificates *bool
}

func (c *Client) DownloadStorageFile(ctx context.Context, req DownloadStorageFileRequest) (string, error) {
	form := url.Values{
		"content":  {req.Content},
		"filename": {req.Filename},
		"url":      {req.URL},
	}
	setOptionalString(form, "checksum", req.Checksum)
	setOptionalString(form, "checksum-algorithm", req.ChecksumAlgorithm)
	setOptionalString(form, "compression", req.Compression)
	setOptionalBool(form, "verify-certificates", req.VerifyCertificates)

	var upid string
	apiPath := fmt.Sprintf("/nodes/%s/storage/%s/download-url", url.PathEscape(req.Node), url.PathEscape(req.Storage))
	if err := c.do(ctx, http.MethodPost, apiPath, nil, form, &upid); err != nil {
		return "", err
	}
	if err := c.waitForNodeTask(ctx, req.Node, upid); err != nil {
		return "", fmt.Errorf("unable to download storage file %q: %w", req.Filename, err)
	}
	return storageFileVolumeID(req.Storage, req.Content, req.Filename), nil
}

func (c *Client) GetStorageFile(ctx context.Context, node, storage, volume string) (StorageFile, error) {
	var file StorageFile
	apiPath := fmt.Sprintf("/nodes/%s/storage/%s/content/%s", url.PathEscape(node), url.PathEscape(storage), url.PathEscape(volume))
	if err := c.do(ctx, http.MethodGet, apiPath, nil, nil, &file); err != nil {
		return StorageFile{}, err
	}
	return file, nil
}

func (c *Client) DeleteStorageFile(ctx context.Context, node, storage, volume string) error {
	var upid string
	apiPath := fmt.Sprintf("/nodes/%s/storage/%s/content/%s", url.PathEscape(node), url.PathEscape(storage), url.PathEscape(volume))
	if err := c.do(ctx, http.MethodDelete, apiPath, nil, nil, &upid); err != nil {
		if errors.Is(err, errNotFound) {
			return nil
		}
		return err
	}
	if err := c.waitForNodeTask(ctx, node, upid); err != nil {
		return fmt.Errorf("unable to delete storage file %q: %w", volume, err)
	}
	return nil
}

func storageFileVolumeID(storage, content, filename string) string {
	return fmt.Sprintf("%s:%s/%s", storage, content, filename)
}
