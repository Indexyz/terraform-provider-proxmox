// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	lxcContainerTaskPollInterval = 2 * time.Second
	lxcContainerTaskTimeoutCap   = 10 * time.Minute
)

type LXCContainerConfig struct {
	Hostname     string
	Description  string
	Tags         string
	Arch         string
	Startup      string
	Features     string
	OSType       string
	RootFS       string
	Nameserver   string
	Searchdomain string
	Timezone     string
	OnBoot       proxmoxOptionalBool
	Protection   proxmoxOptionalBool
	Unprivileged proxmoxOptionalBool
	Console      proxmoxOptionalBool
	TTY          proxmoxOptionalInt64
	CMode        string
	Hookscript   string
	Cores        proxmoxOptionalInt64
	CPULimit     proxmoxOptionalFloat64
	CPUUnits     proxmoxOptionalInt64
	Memory       proxmoxOptionalInt64
	Swap         proxmoxOptionalInt64
	Network      map[string]string
	MountPoint   map[string]string
	ExtraConfig  map[string]string
}

type lxcContainerConfigKnown struct {
	Hostname     string                 `json:"hostname"`
	Description  string                 `json:"description"`
	Tags         string                 `json:"tags"`
	Arch         string                 `json:"arch"`
	Startup      string                 `json:"startup"`
	Features     string                 `json:"features"`
	OSType       string                 `json:"ostype"`
	RootFS       string                 `json:"rootfs"`
	Nameserver   string                 `json:"nameserver"`
	Searchdomain string                 `json:"searchdomain"`
	Timezone     string                 `json:"timezone"`
	OnBoot       proxmoxOptionalBool    `json:"onboot"`
	Protection   proxmoxOptionalBool    `json:"protection"`
	Unprivileged proxmoxOptionalBool    `json:"unprivileged"`
	Console      proxmoxOptionalBool    `json:"console"`
	TTY          proxmoxOptionalInt64   `json:"tty"`
	CMode        string                 `json:"cmode"`
	Hookscript   string                 `json:"hookscript"`
	Cores        proxmoxOptionalInt64   `json:"cores"`
	CPULimit     proxmoxOptionalFloat64 `json:"cpulimit"`
	CPUUnits     proxmoxOptionalInt64   `json:"cpuunits"`
	Memory       proxmoxOptionalInt64   `json:"memory"`
	Swap         proxmoxOptionalInt64   `json:"swap"`
}

type LXCContainerStatus struct {
	Status string               `json:"status"`
	Uptime proxmoxOptionalInt64 `json:"uptime"`
}

type lxcContainerConfigRequest struct {
	Hostname     *string
	Description  *string
	Tags         *string
	Arch         *string
	Startup      *string
	Features     *string
	OSType       *string
	RootFS       *string
	Nameserver   *string
	Searchdomain *string
	Timezone     *string
	OnBoot       *bool
	Protection   *bool
	Unprivileged *bool
	Console      *bool
	TTY          *int64
	CMode        *string
	Hookscript   *string
	Cores        *int64
	CPULimit     *float64
	CPUUnits     *int64
	Memory       *int64
	Swap         *int64
	Network      map[string]string
	MountPoint   map[string]string
	ExtraConfig  map[string]string
	Delete       []string
}

type CreateLXCContainerRequest struct {
	VMID       int64
	OSTemplate *string
	lxcContainerConfigRequest
}

type UpdateLXCContainerRequest struct {
	lxcContainerConfigRequest
}

type lxcContainerTaskStatus struct {
	Status     string `json:"status"`
	ExitStatus string `json:"exitstatus"`
}

func (c *Client) GetLXCContainerConfig(ctx context.Context, node string, vmID int64) (LXCContainerConfig, error) {
	var raw map[string]json.RawMessage
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/lxc/%d/config", url.PathEscape(node), vmID), nil, nil, &raw); err != nil {
		return LXCContainerConfig{}, err
	}

	return decodeLXCContainerConfig(raw)
}

func (c *Client) GetLXCContainerStatus(ctx context.Context, node string, vmID int64) (LXCContainerStatus, error) {
	var status LXCContainerStatus
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/lxc/%d/status/current", url.PathEscape(node), vmID), nil, nil, &status)
	return status, err
}

func (c *Client) CreateLXCContainer(ctx context.Context, node string, req CreateLXCContainerRequest) error {
	form := url.Values{}
	form.Set("vmid", strconv.FormatInt(req.VMID, 10))
	setOptionalString(form, "ostemplate", req.OSTemplate)
	encodeLXCContainerCreateFields(form, req.lxcContainerConfigRequest)

	var upid string
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/nodes/%s/lxc", url.PathEscape(node)), nil, form, &upid); err != nil {
		return err
	}
	return c.waitForLXCContainerTask(ctx, node, upid)
}

func (c *Client) UpdateLXCContainer(ctx context.Context, node string, vmID int64, req UpdateLXCContainerRequest) error {
	form := url.Values{}
	encodeLXCContainerUpdateFields(form, req.lxcContainerConfigRequest)

	var upid string
	if err := c.do(ctx, http.MethodPut, fmt.Sprintf("/nodes/%s/lxc/%d/config", url.PathEscape(node), vmID), nil, form, &upid); err != nil {
		return err
	}
	return c.waitForLXCContainerTask(ctx, node, upid)
}

func (c *Client) DeleteLXCContainer(ctx context.Context, node string, vmID int64) error {
	var upid string
	if err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/nodes/%s/lxc/%d", url.PathEscape(node), vmID), nil, nil, &upid); err != nil {
		if errors.Is(err, errNotFound) {
			return nil
		}
		return err
	}
	return c.waitForLXCContainerTask(ctx, node, upid)
}

func decodeLXCContainerConfig(raw map[string]json.RawMessage) (LXCContainerConfig, error) {
	payload, err := json.Marshal(raw)
	if err != nil {
		return LXCContainerConfig{}, fmt.Errorf("unable to marshal raw LXC config: %w", err)
	}

	var known lxcContainerConfigKnown
	if err := json.Unmarshal(payload, &known); err != nil {
		return LXCContainerConfig{}, fmt.Errorf("unable to decode LXC config: %w", err)
	}

	config := LXCContainerConfig{
		Hostname:     known.Hostname,
		Description:  known.Description,
		Tags:         known.Tags,
		Arch:         known.Arch,
		Startup:      known.Startup,
		Features:     known.Features,
		OSType:       known.OSType,
		RootFS:       known.RootFS,
		Nameserver:   known.Nameserver,
		Searchdomain: known.Searchdomain,
		Timezone:     known.Timezone,
		OnBoot:       known.OnBoot,
		Protection:   known.Protection,
		Unprivileged: known.Unprivileged,
		Console:      known.Console,
		TTY:          known.TTY,
		CMode:        known.CMode,
		Hookscript:   known.Hookscript,
		Cores:        known.Cores,
		CPULimit:     known.CPULimit,
		CPUUnits:     known.CPUUnits,
		Memory:       known.Memory,
		Swap:         known.Swap,
		Network:      map[string]string{},
		MountPoint:   map[string]string{},
		ExtraConfig:  map[string]string{},
	}

	knownKeys := map[string]struct{}{
		"hostname": {}, "description": {}, "tags": {}, "arch": {}, "startup": {}, "features": {}, "ostype": {}, "rootfs": {},
		"nameserver": {}, "searchdomain": {}, "timezone": {}, "onboot": {}, "protection": {}, "unprivileged": {}, "console": {}, "tty": {}, "cmode": {}, "hookscript": {},
		"cores": {}, "cpulimit": {}, "cpuunits": {}, "memory": {}, "swap": {},
	}

	for key, value := range raw {
		if _, ok := knownKeys[key]; ok {
			continue
		}

		decoded, ok := decodeQemuConfigStringValue(value)
		if !ok || decoded == "" {
			continue
		}

		switch {
		case isLXCContainerNetworkKey(key):
			config.Network[key] = decoded
		case isLXCContainerMountPointKey(key):
			config.MountPoint[key] = decoded
		default:
			config.ExtraConfig[key] = decoded
		}
	}

	if len(config.Network) == 0 {
		config.Network = nil
	}
	if len(config.MountPoint) == 0 {
		config.MountPoint = nil
	}
	if len(config.ExtraConfig) == 0 {
		config.ExtraConfig = nil
	}

	return config, nil
}

func encodeLXCContainerCreateFields(form url.Values, req lxcContainerConfigRequest) {
	encodeLXCContainerCommonFields(form, req)
	setOptionalString(form, "arch", req.Arch)
	setOptionalBool(form, "unprivileged", req.Unprivileged)
	setOptionalString(form, "rootfs", req.RootFS)
}

func encodeLXCContainerUpdateFields(form url.Values, req lxcContainerConfigRequest) {
	encodeLXCContainerCommonFields(form, req)
	if len(req.Delete) > 0 {
		form.Set("delete", strings.Join(sortedStrings(req.Delete), ","))
	}
}

func encodeLXCContainerCommonFields(form url.Values, req lxcContainerConfigRequest) {
	setOptionalString(form, "hostname", req.Hostname)
	setOptionalString(form, "description", req.Description)
	setOptionalString(form, "tags", req.Tags)
	setOptionalInt64(form, "cores", req.Cores)
	setOptionalFloat64(form, "cpulimit", req.CPULimit)
	setOptionalInt64(form, "cpuunits", req.CPUUnits)
	setOptionalInt64(form, "memory", req.Memory)
	setOptionalInt64(form, "swap", req.Swap)
	setOptionalBool(form, "onboot", req.OnBoot)
	setOptionalBool(form, "protection", req.Protection)
	setOptionalString(form, "startup", req.Startup)
	setOptionalString(form, "features", req.Features)
	setOptionalBool(form, "console", req.Console)
	setOptionalInt64(form, "tty", req.TTY)
	setOptionalString(form, "cmode", req.CMode)
	setOptionalString(form, "hookscript", req.Hookscript)
	setOptionalString(form, "ostype", req.OSType)
	setOptionalString(form, "nameserver", req.Nameserver)
	setOptionalString(form, "searchdomain", req.Searchdomain)
	setOptionalString(form, "timezone", req.Timezone)
	setSortedStringMap(form, req.Network)
	setSortedStringMap(form, req.MountPoint)
	setSortedStringMap(form, req.ExtraConfig)
}

func (c *Client) waitForLXCContainerTask(ctx context.Context, node string, upid string) error {
	if upid == "" {
		return nil
	}

	waitCtx := ctx
	cancel := func() {}
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > lxcContainerTaskTimeoutCap {
		waitCtx, cancel = context.WithTimeout(ctx, lxcContainerTaskTimeoutCap)
	}
	defer cancel()

	for {
		var status lxcContainerTaskStatus
		if err := c.do(waitCtx, http.MethodGet, fmt.Sprintf("/nodes/%s/tasks/%s/status", url.PathEscape(node), url.PathEscape(upid)), nil, nil, &status); err != nil {
			return fmt.Errorf("unable to poll LXC task %q status: %w", upid, err)
		}

		if status.Status == "stopped" {
			if status.ExitStatus == "OK" {
				return nil
			}
			return fmt.Errorf("LXC task %q failed with exit status %q", upid, status.ExitStatus)
		}

		timer := time.NewTimer(lxcContainerTaskPollInterval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return fmt.Errorf("waiting for LXC task %q: %w", upid, waitCtx.Err())
		case <-timer.C:
		}
	}
}

func isLXCContainerNetworkKey(key string) bool {
	return isLXCContainerSlotKey(key, "net")
}

func isLXCContainerMountPointKey(key string) bool {
	return isLXCContainerSlotKey(key, "mp")
}

func isLXCContainerSlotKey(key string, prefix string) bool {
	if !strings.HasPrefix(key, prefix) || len(key) == len(prefix) {
		return false
	}
	for _, r := range key[len(prefix):] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
