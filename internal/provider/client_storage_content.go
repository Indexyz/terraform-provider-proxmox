// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"slices"
)

// StorageContentItem is one entry of a storage content collection listing
// (`GET /nodes/{node}/storage/{storage}/content`).
type StorageContentItem struct {
	Content string               `json:"content"`
	Format  string               `json:"format"`
	Volid   string               `json:"volid"`
	Size    proxmoxOptionalInt64 `json:"size"`
	Used    proxmoxOptionalInt64 `json:"used"`
}

// StorageContentList reads the full content collection of one content type.
// The collection listing is the authoritative existence source for storage
// files: a single-item GET can fail through volume_size_info instead of a
// reliable 404, so absence must be proven by a successful collection read.
func (c *Client) StorageContentList(ctx context.Context, node, storage, content string) ([]StorageContentItem, error) {
	query := url.Values{"content": {content}}

	var items []StorageContentItem
	apiPath := fmt.Sprintf("/nodes/%s/storage/%s/content", url.PathEscape(node), url.PathEscape(storage))
	if err := c.do(ctx, http.MethodGet, apiPath, query, nil, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// isoVolumeExists proves the exact volume_id exists through an authoritative
// storage content collection read. The collection listing is the authoritative
// existence source for storage files: a single-item GET can fail through
// volume_size_info instead of a reliable 404, and API errors (403/500/storage
// failure) are returned, never treated as absence.
func isoVolumeExists(ctx context.Context, client *Client, node, storage, volumeID string) (bool, error) {
	items, err := client.StorageContentList(ctx, node, storage, "iso")
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(items, func(item StorageContentItem) bool { return item.Volid == volumeID }), nil
}

type UploadStorageFileRequest struct {
	Node     string
	Storage  string
	Content  string
	Filename string
	Data     []byte
}

// UploadStorageFile POSTs a file as multipart form data to
// `/nodes/{node}/storage/{storage}/upload` and returns the accepted task UPID
// together with the node that owns the task. The endpoint is not proxied to
// the requested node, so the task owner can differ from the destination node
// and must be polled there. The upload silently overwrites an existing
// destination; callers must refuse existing filenames themselves.
func (c *Client) UploadStorageFile(ctx context.Context, req UploadStorageFileRequest) (string, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("content", req.Content); err != nil {
		return "", "", fmt.Errorf("unable to encode upload form field: %w", err)
	}
	file, err := writer.CreateFormFile("filename", req.Filename)
	if err != nil {
		return "", "", fmt.Errorf("unable to encode upload file part: %w", err)
	}
	if _, err := file.Write(req.Data); err != nil {
		return "", "", fmt.Errorf("unable to write upload payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", "", fmt.Errorf("unable to finalize upload payload: %w", err)
	}

	requestURL, err := c.requestURL(fmt.Sprintf("/nodes/%s/storage/%s/upload", url.PathEscape(req.Node), url.PathEscape(req.Storage)))
	if err != nil {
		return "", "", err
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), &body)
	if err != nil {
		return "", "", err
	}
	httpRequest.Header.Set("Content-Type", writer.FormDataContentType())

	var upid string
	if err := c.execute(httpRequest, &upid); err != nil {
		return "", "", err
	}
	if upid == "" {
		return "", "", fmt.Errorf("upload of %q to storage %q returned no UPID", req.Filename, req.Storage)
	}

	taskNode, err := taskOwnerNode(upid)
	if err != nil {
		return "", "", err
	}
	return upid, taskNode, nil
}

// deleteStorageFile issues the content item DELETE and returns the accepted
// task UPID without waiting, so callers can retain the accepted task identity
// before awaiting completion.
func (c *Client) deleteStorageFile(ctx context.Context, node, storage, volume string) (string, error) {
	var upid string
	apiPath := fmt.Sprintf("/nodes/%s/storage/%s/content/%s", url.PathEscape(node), url.PathEscape(storage), url.PathEscape(volume))
	err := c.do(ctx, http.MethodDelete, apiPath, nil, nil, &upid)
	return upid, err
}
