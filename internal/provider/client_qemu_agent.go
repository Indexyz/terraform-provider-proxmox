// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type AgentCommand struct {
	Name string `json:"name"`
}

// AgentCommands lists the guest agent commands supported by the running agent.
func (c *Client) AgentCommands(ctx context.Context, node string, vmid int64) ([]string, error) {
	var commands []AgentCommand
	if err := c.do(ctx, http.MethodGet, agentPath(node, vmid, ""), nil, nil, &commands); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(commands))
	for _, command := range commands {
		names = append(names, command.Name)
	}
	return names, nil
}

type AgentOSInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	PrettyName    string `json:"pretty-name"`
	Version       string `json:"version"`
	VersionID     string `json:"version-id"`
	Machine       string `json:"machine"`
	KernelRelease string `json:"kernel-release"`
	KernelVersion string `json:"kernel-version"`
}

func (c *Client) AgentOSInfo(ctx context.Context, node string, vmid int64) (*AgentOSInfo, error) {
	var result struct {
		Result AgentOSInfo `json:"result"`
	}
	if err := c.do(ctx, http.MethodGet, agentPath(node, vmid, "get-osinfo"), nil, nil, &result); err != nil {
		return nil, err
	}
	return &result.Result, nil
}

func (c *Client) AgentHostName(ctx context.Context, node string, vmid int64) (string, error) {
	var result struct {
		Result struct {
			HostName string `json:"host-name"`
		} `json:"result"`
	}
	if err := c.do(ctx, http.MethodGet, agentPath(node, vmid, "get-host-name"), nil, nil, &result); err != nil {
		return "", err
	}
	return result.Result.HostName, nil
}

type AgentTimezone struct {
	Zone   string `json:"zone"`
	Offset int64  `json:"offset"`
}

func (c *Client) AgentTimezone(ctx context.Context, node string, vmid int64) (*AgentTimezone, error) {
	var result struct {
		Result AgentTimezone `json:"result"`
	}
	if err := c.do(ctx, http.MethodGet, agentPath(node, vmid, "get-timezone"), nil, nil, &result); err != nil {
		return nil, err
	}
	return &result.Result, nil
}

type AgentDisk struct {
	Dev     string `json:"dev"`
	Serial  string `json:"serial"`
	BusType string `json:"bus-type"`
	Target  int64  `json:"target"`
	Unit    int64  `json:"unit"`
	Bus     int64  `json:"bus"`
}

type AgentFilesystem struct {
	Name           string      `json:"name"`
	Mountpoint     string      `json:"mountpoint"`
	Type           string      `json:"type"`
	TotalBytes     int64       `json:"total-bytes"`
	UsedBytes      int64       `json:"used-bytes"`
	TotalBytesPriv int64       `json:"total-bytes-privileged"`
	UsedBytesPriv  int64       `json:"used-bytes-privileged"`
	Disks          []AgentDisk `json:"disk"`
}

func (c *Client) AgentFilesystems(ctx context.Context, node string, vmid int64) ([]AgentFilesystem, error) {
	var result struct {
		Result []AgentFilesystem `json:"result"`
	}
	if err := c.do(ctx, http.MethodGet, agentPath(node, vmid, "get-fsinfo"), nil, nil, &result); err != nil {
		return nil, err
	}
	return result.Result, nil
}

type AgentIPAddress struct {
	Address string `json:"ip-address"`
	Type    string `json:"ip-address-type"`
	Prefix  int64  `json:"prefix"`
}

type AgentInterface struct {
	Name       string           `json:"name"`
	MAC        string           `json:"hardware-address"`
	Addresses  []AgentIPAddress `json:"ip-addresses"`
	Statistics map[string]int64 `json:"statistics"`
}

func (c *Client) AgentInterfaces(ctx context.Context, node string, vmid int64) ([]AgentInterface, error) {
	var result struct {
		Result []AgentInterface `json:"result"`
	}
	if err := c.do(ctx, http.MethodGet, agentPath(node, vmid, "network-get-interfaces"), nil, nil, &result); err != nil {
		return nil, err
	}
	return result.Result, nil
}

type AgentExecStatus struct {
	Exited       proxmoxOptionalBool `json:"exited"`
	ExitCode     int64               `json:"exitcode"`
	OutData      string              `json:"out-data"`
	ErrData      string              `json:"err-data"`
	OutTruncated proxmoxOptionalBool `json:"out-truncated"`
	ErrTruncated proxmoxOptionalBool `json:"err-truncated"`
	Signal       int64               `json:"signal"`
}

// AgentExec starts a guest agent command and returns the process id.
func (c *Client) AgentExec(ctx context.Context, node string, vmid int64, command []string, inputData string) (int64, error) {
	form := url.Values{}
	for _, part := range command {
		form.Add("command", part)
	}
	if inputData != "" {
		form.Set("input-data", inputData)
	}
	var result struct {
		PID int64 `json:"pid"`
	}
	if err := c.do(ctx, http.MethodPost, agentPath(node, vmid, "exec"), nil, form, &result); err != nil {
		return 0, err
	}
	return result.PID, nil
}

func (c *Client) AgentExecStatus(ctx context.Context, node string, vmid, pid int64) (*AgentExecStatus, error) {
	query := url.Values{"pid": {fmt.Sprintf("%d", pid)}}
	var status AgentExecStatus
	if err := c.do(ctx, http.MethodGet, agentPath(node, vmid, "exec-status"), query, nil, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// WaitAgentExec polls the process until it exits or the timeout elapses.
func (c *Client) WaitAgentExec(ctx context.Context, node string, vmid, pid int64, timeout time.Duration) (*AgentExecStatus, error) {
	deadline := time.Now().Add(timeout)
	for {
		status, err := c.AgentExecStatus(ctx, node, vmid, pid)
		if err != nil {
			return nil, err
		}
		if status.Exited.Ptr() != nil && *status.Exited.Ptr() {
			return status, nil
		}
		if time.Now().After(deadline) {
			return status, fmt.Errorf("guest agent command %d did not exit within %s", pid, timeout)
		}
		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func agentPath(node string, vmid int64, command string) string {
	result := fmt.Sprintf("/nodes/%s/qemu/%d/agent", url.PathEscape(node), vmid)
	if command != "" {
		result += "/" + strings.TrimPrefix(command, "/")
	}
	return result
}

var _ = json.RawMessage{}
