// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
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
	Name        string
	Description string
	Tags        string
	Template    proxmoxOptionalBool
	Pool        string
	OnBoot      proxmoxOptionalBool
	Protection  proxmoxOptionalBool
	SCSIHW      string
	Tablet      proxmoxOptionalBool
	Startup     string
	Bios        string
	Machine     string
	Agent       string
	Cores       proxmoxOptionalInt64
	Sockets     proxmoxOptionalInt64
	Memory      proxmoxOptionalInt64
	CPU         string
	OSType      string
	Boot        string
	Hotplug     string
	CICustom    string
	CIPassword  string
	CIType      string
	CIUpgrade   proxmoxOptionalBool
	CIUser      string
	SSHKeys     string
	IPConfig    map[string]string
	Network     map[string]string
	Disk        map[string]string
	ExtraConfig map[string]string
}

type qemuVMConfigKnown struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Tags        string               `json:"tags"`
	Template    proxmoxOptionalBool  `json:"template"`
	Pool        string               `json:"pool"`
	OnBoot      proxmoxOptionalBool  `json:"onboot"`
	Protection  proxmoxOptionalBool  `json:"protection"`
	SCSIHW      string               `json:"scsihw"`
	Tablet      proxmoxOptionalBool  `json:"tablet"`
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
	Hotplug     string               `json:"hotplug"`
	CICustom    string               `json:"cicustom"`
	CIPassword  string               `json:"cipassword"`
	CIType      string               `json:"citype"`
	CIUpgrade   proxmoxOptionalBool  `json:"ciupgrade"`
	CIUser      string               `json:"ciuser"`
	SSHKeys     string               `json:"sshkeys"`
}

type QemuVMStatus struct {
	Status string               `json:"status"`
	Uptime proxmoxOptionalInt64 `json:"uptime"`
}

type qemuVMConfigRequest struct {
	Name        *string
	Description *string
	Tags        *string
	Pool        *string
	OnBoot      *bool
	Protection  *bool
	SCSIHW      *string
	Tablet      *bool
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
	Hotplug     *string
	CICustom    *string
	CIPassword  *string
	CIType      *string
	CIUpgrade   *bool
	CIUser      *string
	SSHKeys     *string
	IPConfig    map[string]string
	Network     map[string]string
	Disk        map[string]string
	ExtraConfig map[string]string
}

type CreateQemuVMRequest struct {
	VMID int64
	qemuVMConfigRequest
}

type UpdateQemuVMRequest struct {
	qemuVMConfigRequest
}

type CloneQemuVMRequest struct {
	SourceNode   string
	SourceVMID   int64
	TargetNode   string
	NewID        int64
	Name         *string
	Description  *string
	Pool         *string
	Full         *bool
	SnapshotName *string
	Storage      *string
	Format       *string
	BWLimit      *int64
}

func (r UpdateQemuVMRequest) IsEmpty() bool {
	return r.Name == nil &&
		r.Description == nil &&
		r.Tags == nil &&
		r.Pool == nil &&
		r.OnBoot == nil &&
		r.Protection == nil &&
		r.SCSIHW == nil &&
		r.Tablet == nil &&
		r.Startup == nil &&
		r.Bios == nil &&
		r.Machine == nil &&
		r.Agent == nil &&
		r.Cores == nil &&
		r.Sockets == nil &&
		r.Memory == nil &&
		r.CPU == nil &&
		r.OSType == nil &&
		r.Boot == nil &&
		r.Hotplug == nil &&
		r.CICustom == nil &&
		r.CIPassword == nil &&
		r.CIType == nil &&
		r.CIUpgrade == nil &&
		r.CIUser == nil &&
		r.SSHKeys == nil &&
		len(r.IPConfig) == 0 &&
		len(r.Network) == 0 &&
		len(r.Disk) == 0 &&
		len(r.ExtraConfig) == 0
}

func (c *Client) GetQemuVMConfig(ctx context.Context, node string, vmID int64) (QemuVMConfig, error) {
	var raw map[string]json.RawMessage
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/qemu/%d/config", url.PathEscape(node), vmID), nil, nil, &raw); err != nil {
		return QemuVMConfig{}, err
	}

	return decodeQemuVMConfig(raw)
}

func (c *Client) GetQemuVMStatus(ctx context.Context, node string, vmID int64) (QemuVMStatus, error) {
	var status QemuVMStatus
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/qemu/%d/status/current", url.PathEscape(node), vmID), nil, nil, &status)
	return status, err
}

func (c *Client) CreateQemuVM(ctx context.Context, node string, req CreateQemuVMRequest) error {
	form := url.Values{}
	form.Set("vmid", strconv.FormatInt(req.VMID, 10))
	encodeQemuVMFields(form, req.qemuVMConfigRequest)
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/nodes/%s/qemu", url.PathEscape(node)), nil, form, nil)
}

func (c *Client) CloneQemuVM(ctx context.Context, req CloneQemuVMRequest) error {
	form := url.Values{}
	form.Set("newid", strconv.FormatInt(req.NewID, 10))
	setOptionalString(form, "node", stringPtrIfNotEmpty(req.TargetNode))
	setOptionalString(form, "name", req.Name)
	setOptionalString(form, "description", req.Description)
	setOptionalString(form, "pool", req.Pool)
	setOptionalBool(form, "full", req.Full)
	setOptionalString(form, "snapname", req.SnapshotName)
	setOptionalString(form, "storage", req.Storage)
	setOptionalString(form, "format", req.Format)
	setOptionalInt64(form, "bwlimit", req.BWLimit)
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/nodes/%s/qemu/%d/clone", url.PathEscape(req.SourceNode), req.SourceVMID), nil, form, nil)
}

func (c *Client) UpdateQemuVM(ctx context.Context, node string, vmID int64, req UpdateQemuVMRequest) error {
	form := url.Values{}
	encodeQemuVMFields(form, req.qemuVMConfigRequest)
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/nodes/%s/qemu/%d/config", url.PathEscape(node), vmID), nil, form, nil)
}

func (c *Client) DeleteQemuVM(ctx context.Context, node string, vmID int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/nodes/%s/qemu/%d", url.PathEscape(node), vmID), nil, nil, nil)
}

func decodeQemuVMConfig(raw map[string]json.RawMessage) (QemuVMConfig, error) {
	payload, err := json.Marshal(raw)
	if err != nil {
		return QemuVMConfig{}, fmt.Errorf("unable to marshal raw QEMU config: %w", err)
	}

	var known qemuVMConfigKnown
	if err := json.Unmarshal(payload, &known); err != nil {
		return QemuVMConfig{}, fmt.Errorf("unable to decode QEMU config: %w", err)
	}

	config := QemuVMConfig{
		Name:        known.Name,
		Description: known.Description,
		Tags:        known.Tags,
		Template:    known.Template,
		Pool:        known.Pool,
		OnBoot:      known.OnBoot,
		Protection:  known.Protection,
		SCSIHW:      known.SCSIHW,
		Tablet:      known.Tablet,
		Startup:     known.Startup,
		Bios:        known.Bios,
		Machine:     known.Machine,
		Agent:       known.Agent,
		Cores:       known.Cores,
		Sockets:     known.Sockets,
		Memory:      known.Memory,
		CPU:         known.CPU,
		OSType:      known.OSType,
		Boot:        known.Boot,
		Hotplug:     known.Hotplug,
		CICustom:    known.CICustom,
		CIPassword:  known.CIPassword,
		CIType:      known.CIType,
		CIUpgrade:   known.CIUpgrade,
		CIUser:      known.CIUser,
		SSHKeys:     known.SSHKeys,
		IPConfig:    map[string]string{},
		Network:     map[string]string{},
		Disk:        map[string]string{},
		ExtraConfig: map[string]string{},
	}

	knownKeys := map[string]struct{}{
		"name": {}, "description": {}, "tags": {}, "template": {}, "pool": {}, "onboot": {}, "protection": {}, "scsihw": {}, "tablet": {}, "startup": {},
		"bios": {}, "machine": {}, "agent": {}, "cores": {}, "sockets": {}, "memory": {}, "cpu": {},
		"ostype": {}, "boot": {}, "hotplug": {}, "cicustom": {}, "cipassword": {}, "citype": {},
		"ciupgrade": {}, "ciuser": {}, "sshkeys": {},
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
		case isQemuVMIPConfigKey(key):
			config.IPConfig[key] = decoded
		case isQemuVMNetworkKey(key):
			config.Network[key] = decoded
		case isQemuVMDiskKey(key):
			config.Disk[key] = decoded
		default:
			config.ExtraConfig[key] = decoded
		}
	}

	if len(config.IPConfig) == 0 {
		config.IPConfig = nil
	}
	if len(config.Network) == 0 {
		config.Network = nil
	}
	if len(config.Disk) == 0 {
		config.Disk = nil
	}
	if len(config.ExtraConfig) == 0 {
		config.ExtraConfig = nil
	}

	return config, nil
}

func encodeQemuVMFields(form url.Values, req qemuVMConfigRequest) {
	setOptionalString(form, "name", req.Name)
	setOptionalString(form, "description", req.Description)
	setOptionalString(form, "tags", req.Tags)
	setOptionalString(form, "pool", req.Pool)
	setOptionalBool(form, "onboot", req.OnBoot)
	setOptionalBool(form, "protection", req.Protection)
	setOptionalString(form, "scsihw", req.SCSIHW)
	setOptionalBool(form, "tablet", req.Tablet)
	setOptionalString(form, "startup", req.Startup)
	setOptionalString(form, "bios", req.Bios)
	setOptionalString(form, "machine", req.Machine)
	setOptionalString(form, "agent", req.Agent)
	setOptionalInt64(form, "cores", req.Cores)
	setOptionalInt64(form, "sockets", req.Sockets)
	setOptionalInt64(form, "memory", req.Memory)
	setOptionalString(form, "cpu", req.CPU)
	setOptionalString(form, "ostype", req.OSType)
	setOptionalString(form, "boot", req.Boot)
	setOptionalString(form, "hotplug", req.Hotplug)
	setOptionalString(form, "cicustom", req.CICustom)
	setOptionalString(form, "cipassword", req.CIPassword)
	setOptionalString(form, "citype", req.CIType)
	setOptionalBool(form, "ciupgrade", req.CIUpgrade)
	setOptionalString(form, "ciuser", req.CIUser)
	setOptionalString(form, "sshkeys", req.SSHKeys)
	setSortedStringMap(form, req.IPConfig)
	setSortedStringMap(form, req.Network)
	setSortedStringMap(form, req.Disk)
	setSortedStringMap(form, req.ExtraConfig)
}

func decodeQemuConfigStringValue(value json.RawMessage) (string, bool) {
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" || trimmed == "null" {
		return "", false
	}

	var stringValue string
	if err := json.Unmarshal(value, &stringValue); err == nil {
		return stringValue, true
	}

	var intValue int64
	if err := json.Unmarshal(value, &intValue); err == nil {
		return strconv.FormatInt(intValue, 10), true
	}

	var floatValue float64
	if err := json.Unmarshal(value, &floatValue); err == nil {
		return strconv.FormatFloat(floatValue, 'f', -1, 64), true
	}

	var boolValue bool
	if err := json.Unmarshal(value, &boolValue); err == nil {
		if boolValue {
			return "1", true
		}
		return "0", true
	}

	return "", false
}

func setSortedStringMap(form url.Values, values map[string]string) {
	if len(values) == 0 {
		return
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		form.Set(key, values[key])
	}
}

func stringPtrIfNotEmpty(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
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
