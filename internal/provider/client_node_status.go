// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// NodeStatusCommand runs `reboot` or `shutdown` against a node.
func (c *Client) NodeStatusCommand(ctx context.Context, node, command string) error {
	form := url.Values{"command": {command}}
	return c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/status", nil, form, nil)
}

// NodePowerAction starts `stopall` or `suspendall` and returns the worker UPID.
func (c *Client) NodePowerAction(ctx context.Context, node, action, vms string, forceStop bool, timeout, maxWorkers int64) (string, error) {
	form := url.Values{}
	if vms != "" {
		form.Set("vms", vms)
	}
	if forceStop {
		form.Set("force-stop", "1")
	}
	if timeout > 0 {
		form.Set("timeout", strconv.FormatInt(timeout, 10))
	}
	if maxWorkers > 0 {
		form.Set("max-workers", strconv.FormatInt(maxWorkers, 10))
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/"+action, nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, action); err != nil {
		return "", err
	}
	return upid, nil
}

// WaitNodeOnline polls until the node answers again or the timeout elapses.
func (c *Client) WaitNodeOnline(ctx context.Context, node string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	/* Wait for the node to go down first so an immediate answer does not
	   count as the come-back. */
	for {
		if _, err := c.NodeStatus(ctx, node); err != nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("node %q did not go down within %s", node, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	for {
		if _, err := c.NodeStatus(ctx, node); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("node %q did not come back within %s", node, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

var _ = strings.TrimSpace
