// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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

// taskOwnerNode returns the node that runs a task, parsed from the UPID
// (`UPID:<node>:<pid>:<pstart>:<starttime>:<dtype>:<id>:<user>`). Endpoints
// such as storage upload are not proxied to the requested node, so the task
// must be polled on its actual owner.
func taskOwnerNode(upid string) (string, error) {
	parts := strings.Split(upid, ":")
	if len(parts) < 2 || parts[0] != "UPID" || parts[1] == "" {
		return "", fmt.Errorf("malformed Proxmox task UPID %q", upid)
	}
	return parts[1], nil
}

// validateQemuTaskAck rejects empty or malformed task acknowledgements before
// any polling: an async Proxmox endpoint must acknowledge with a structurally
// valid UPID naming its owner node, so garbage or ownerless acknowledgements
// are errors, never silent success.
func validateQemuTaskAck(upid, what string) error {
	if upid == "" {
		return fmt.Errorf("%s returned no UPID", what)
	}
	if _, err := taskOwnerNode(upid); err != nil {
		return fmt.Errorf("%s returned an invalid UPID: %w", what, err)
	}
	return nil
}

// nodeTaskFinished classifies a task status reply against the only two states
// Proxmox reports (running/stopped). Unknown or missing status values are
// errors so a retained task is never silently treated as finished or awaited
// forever.
func nodeTaskFinished(status nodeTaskStatus, upid string) (bool, error) {
	switch status.Status {
	case "stopped":
		return true, nil
	case "running":
		return false, nil
	case "":
		return false, fmt.Errorf("task %q status reply is missing a status", upid)
	default:
		return false, fmt.Errorf("task %q status reply has unknown status %q", upid, status.Status)
	}
}

func (c *Client) GetNodeTaskStatus(ctx context.Context, node string, upid string) (nodeTaskStatus, error) {
	var status nodeTaskStatus
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/tasks/%s/status", url.PathEscape(node), url.PathEscape(upid)), nil, nil, &status); err != nil {
		return nodeTaskStatus{}, fmt.Errorf("unable to poll task %q status: %w", upid, err)
	}
	return status, nil
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
		status, err := c.GetNodeTaskStatus(waitCtx, node, upid)
		if err != nil {
			return err
		}

		finished, err := nodeTaskFinished(status, upid)
		if err != nil {
			return err
		}
		if finished {
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
