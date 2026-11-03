// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// RunBulkAction starts a cluster-wide action on the given guests and returns the worker UPID.
func (c *Client) RunBulkAction(ctx context.Context, action string, vmids []int64, target string, online, withLocalDisks bool, maxWorkers int64) (string, error) {
	ids := make([]string, 0, len(vmids))
	for _, vmid := range vmids {
		ids = append(ids, strconv.FormatInt(vmid, 10))
	}
	form := url.Values{"vms": {strings.Join(ids, ",")}}
	if target != "" {
		form.Set("target", target)
	}
	if online {
		form.Set("online", "1")
	}
	if withLocalDisks {
		form.Set("with-local-disks", "1")
	}
	if action == "start" || action == "migrate" {
		form.Set("max-workers", strconv.FormatInt(maxWorkers, 10))
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/cluster/bulk-action/guest/"+url.PathEscape(action), nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "bulk "+action); err != nil {
		return "", err
	}
	return upid, nil
}
