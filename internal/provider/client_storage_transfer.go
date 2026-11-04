// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

func (c *Client) DownloadStorageURL(ctx context.Context, node, storage, sourceURL, content, filename, checksum, checksumAlgorithm, compression string, verify bool) (string, error) {
	form := url.Values{
		"url": {sourceURL}, "content": {content}, "filename": {filename},
	}
	if checksum != "" {
		form.Set("checksum", checksum)
	}
	if checksumAlgorithm != "" {
		form.Set("checksum-algorithm", checksumAlgorithm)
	}
	if compression != "" {
		form.Set("compression", compression)
	}
	if !verify {
		form.Set("verify-certificates", "0")
	}
	var upid string
	err := c.do(ctx, http.MethodPost, fmt.Sprintf("/nodes/%s/storage/%s/download-url", url.PathEscape(node), url.PathEscape(storage)), nil, form, &upid)
	if err != nil {
		return "", err
	}
	if upid == "" {
		return "", nil
	}
	return upid, validateQemuTaskAck(upid, "storage download")
}

func storageContentPath(content string) string {
	switch content {
	case "vztmpl":
		return "vztmpl"
	case "import":
		return "import"
	default:
		return "iso"
	}
}

func storageVolumeID(storage, content, filename string) string {
	return storage + ":" + storageContentPath(content) + "/" + filename
}
