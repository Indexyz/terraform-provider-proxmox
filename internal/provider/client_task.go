// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

var (
	nodeTaskPollInterval = 2 * time.Second
	nodeTaskTimeoutCap   = 10 * time.Minute
)

type nodeTaskStatus struct {
	Status     string `json:"status"`
	ExitStatus string `json:"exitstatus"`
}

func (c *Client) waitForNodeTask(ctx context.Context, node string, upid string) error {
	if upid == "" {
		return nil
	}

	waitCtx := ctx
	cancel := func() {}
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > nodeTaskTimeoutCap {
		waitCtx, cancel = context.WithTimeout(ctx, nodeTaskTimeoutCap)
	}
	defer cancel()

	for {
		var status nodeTaskStatus
		if err := c.do(waitCtx, http.MethodGet, fmt.Sprintf("/nodes/%s/tasks/%s/status", url.PathEscape(node), url.PathEscape(upid)), nil, nil, &status); err != nil {
			return fmt.Errorf("unable to poll task %q status: %w", upid, err)
		}

		if status.Status == "stopped" {
			if status.ExitStatus == "OK" {
				return nil
			}
			return fmt.Errorf("task %q failed with exit status %q", upid, status.ExitStatus)
		}

		timer := time.NewTimer(nodeTaskPollInterval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return fmt.Errorf("waiting for task %q: %w", upid, waitCtx.Err())
		case <-timer.C:
		}
	}
}
