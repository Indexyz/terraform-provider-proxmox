// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// MigrateGuest moves a guest to another node and returns the worker UPID.
func (c *Client) MigrateGuest(ctx context.Context, guestType, node string, vmid int64, target string, online, withLocalDisks, restart bool, timeout int64, targetStorage string) (string, error) {
	if guestType != "qemu" && guestType != "lxc" {
		return "", fmt.Errorf("unsupported guest type %q", guestType)
	}
	form := url.Values{"target": {target}}
	if online {
		form.Set("online", "1")
	}
	if guestType == "qemu" {
		if withLocalDisks {
			form.Set("with-local-disks", "1")
		}
		if targetStorage != "" {
			form.Set("targetstorage", targetStorage)
		}
	} else {
		if timeout > 0 {
			form.Set("timeout", strconv.FormatInt(timeout, 10))
		}
		form.Set("restart", map[bool]string{true: "1", false: "0"}[restart])
		if targetStorage != "" {
			form.Set("target-storage", targetStorage)
		}
	}
	apiPath := fmt.Sprintf("/nodes/%s/%s/%d/migrate", url.PathEscape(node), guestType, vmid)
	var upid string
	if err := c.do(ctx, http.MethodPost, apiPath, nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "migrate"); err != nil {
		return "", err
	}
	return upid, nil
}
