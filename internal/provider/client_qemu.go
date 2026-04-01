// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type proxmoxOptionalBool struct {
	value *bool
}

func (b *proxmoxOptionalBool) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "null" {
		b.value = nil
		return nil
	}

	var boolValue bool
	if err := json.Unmarshal(data, &boolValue); err == nil {
		b.value = &boolValue
		return nil
	}

	var intValue int
	if err := json.Unmarshal(data, &intValue); err == nil {
		v := intValue != 0
		b.value = &v
		return nil
	}

	var stringValue string
	if err := json.Unmarshal(data, &stringValue); err == nil {
		switch strings.ToLower(strings.TrimSpace(stringValue)) {
		case "1", "on", "yes", "true":
			v := true
			b.value = &v
			return nil
		case "0", "", "off", "no", "false":
			v := false
			b.value = &v
			return nil
		}
	}

	return fmt.Errorf("unable to decode Proxmox boolean value %q", trimmed)
}

func (b proxmoxOptionalBool) Ptr() *bool {
	return b.value
}

type proxmoxOptionalInt64 struct {
	value *int64
}

func (i *proxmoxOptionalInt64) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "null" {
		i.value = nil
		return nil
	}

	var intValue int64
	if err := json.Unmarshal(data, &intValue); err == nil {
		i.value = &intValue
		return nil
	}

	var stringValue string
	if err := json.Unmarshal(data, &stringValue); err == nil {
		if strings.TrimSpace(stringValue) == "" {
			i.value = nil
			return nil
		}

		parsed, err := strconv.ParseInt(strings.TrimSpace(stringValue), 10, 64)
		if err != nil {
			return fmt.Errorf("unable to decode Proxmox integer value %q: %w", strings.TrimSpace(stringValue), err)
		}

		i.value = &parsed
		return nil
	}

	return fmt.Errorf("unable to decode Proxmox integer value %q", trimmed)
}

func (i proxmoxOptionalInt64) Ptr() *int64 {
	return i.value
}

type QemuVMConfig struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Tags        string               `json:"tags"`
	Template    proxmoxOptionalBool  `json:"template"`
	Pool        string               `json:"pool"`
	OnBoot      proxmoxOptionalBool  `json:"onboot"`
	Startup     string               `json:"startup"`
	Bios        string               `json:"bios"`
	Machine     string               `json:"machine"`
	Agent       string               `json:"agent"`
	Cores       proxmoxOptionalInt64 `json:"cores"`
	Sockets     proxmoxOptionalInt64 `json:"sockets"`
	Memory      proxmoxOptionalInt64 `json:"memory"`
	CPU         string               `json:"cpu"`
	OSType      string               `json:"ostype"`
	Boot        string               `json:"boot"`
}

type QemuVMStatus struct {
	Status string               `json:"status"`
	Uptime proxmoxOptionalInt64 `json:"uptime"`
}

type CreateQemuVMRequest struct {
	VMID        int64
	Name        *string
	Description *string
	Tags        *string
	Pool        *string
	OnBoot      *bool
	Startup     *string
	Bios        *string
	Machine     *string
	Agent       *string
	Cores       *int64
	Sockets     *int64
	Memory      *int64
	CPU         *string
	OSType      *string
	Boot        *string
}

type UpdateQemuVMRequest struct {
	Name        *string
	Description *string
	Tags        *string
	Pool        *string
	OnBoot      *bool
	Startup     *string
	Bios        *string
	Machine     *string
	Agent       *string
	Cores       *int64
	Sockets     *int64
	Memory      *int64
	CPU         *string
	OSType      *string
	Boot        *string
}

func (c *Client) GetQemuVMConfig(ctx context.Context, node string, vmID int64) (QemuVMConfig, error) {
	var config QemuVMConfig
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/qemu/%d/config", url.PathEscape(node), vmID), nil, nil, &config)
	return config, err
}

func (c *Client) GetQemuVMStatus(ctx context.Context, node string, vmID int64) (QemuVMStatus, error) {
	var status QemuVMStatus
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/qemu/%d/status/current", url.PathEscape(node), vmID), nil, nil, &status)
	return status, err
}

func (c *Client) CreateQemuVM(ctx context.Context, node string, req CreateQemuVMRequest) error {
	form := url.Values{}
	form.Set("vmid", strconv.FormatInt(req.VMID, 10))
	encodeQemuVMFields(form, req.Name, req.Description, req.Tags, req.Pool, req.OnBoot, req.Startup, req.Bios, req.Machine, req.Agent, req.Cores, req.Sockets, req.Memory, req.CPU, req.OSType, req.Boot)
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/nodes/%s/qemu", url.PathEscape(node)), nil, form, nil)
}

func (c *Client) UpdateQemuVM(ctx context.Context, node string, vmID int64, req UpdateQemuVMRequest) error {
	form := url.Values{}
	encodeQemuVMFields(form, req.Name, req.Description, req.Tags, req.Pool, req.OnBoot, req.Startup, req.Bios, req.Machine, req.Agent, req.Cores, req.Sockets, req.Memory, req.CPU, req.OSType, req.Boot)
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/nodes/%s/qemu/%d/config", url.PathEscape(node), vmID), nil, form, nil)
}

func (c *Client) DeleteQemuVM(ctx context.Context, node string, vmID int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/nodes/%s/qemu/%d", url.PathEscape(node), vmID), nil, nil, nil)
}

func encodeQemuVMFields(
	form url.Values,
	name *string,
	description *string,
	tags *string,
	pool *string,
	onBoot *bool,
	startup *string,
	bios *string,
	machine *string,
	agent *string,
	cores *int64,
	sockets *int64,
	memory *int64,
	cpu *string,
	osType *string,
	boot *string,
) {
	setOptionalString(form, "name", name)
	setOptionalString(form, "description", description)
	setOptionalString(form, "tags", tags)
	setOptionalString(form, "pool", pool)
	setOptionalBool(form, "onboot", onBoot)
	setOptionalString(form, "startup", startup)
	setOptionalString(form, "bios", bios)
	setOptionalString(form, "machine", machine)
	setOptionalString(form, "agent", agent)
	setOptionalInt64(form, "cores", cores)
	setOptionalInt64(form, "sockets", sockets)
	setOptionalInt64(form, "memory", memory)
	setOptionalString(form, "cpu", cpu)
	setOptionalString(form, "ostype", osType)
	setOptionalString(form, "boot", boot)
}

func setOptionalString(form url.Values, key string, value *string) {
	if value != nil {
		form.Set(key, *value)
	}
}

func setOptionalBool(form url.Values, key string, value *bool) {
	if value != nil {
		if *value {
			form.Set(key, "1")
		} else {
			form.Set(key, "0")
		}
	}
}

func setOptionalInt64(form url.Values, key string, value *int64) {
	if value != nil {
		form.Set(key, strconv.FormatInt(*value, 10))
	}
}
