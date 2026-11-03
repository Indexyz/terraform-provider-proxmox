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
	"sort"
	"strings"
)

type NodeDisk struct {
	DevPath string              `json:"devpath"`
	GPT     proxmoxOptionalBool `json:"gpt"`
	Health  string              `json:"health"`
	Model   string              `json:"model"`
	Mounted proxmoxOptionalBool `json:"mounted"`
	OSDID   flexString          `json:"osdid"`
	Parent  string              `json:"parent"`
	Serial  string              `json:"serial"`
	Size    int64               `json:"size"`
	Used    string              `json:"used"`
	Vendor  string              `json:"vendor"`
	WWN     string              `json:"wwn"`
}

type ZFSPool struct {
	Name   string     `json:"name"`
	Health string     `json:"health"`
	Size   int64      `json:"size"`
	Free   int64      `json:"free"`
	Alloc  int64      `json:"alloc"`
	Frag   int64      `json:"frag"`
	Dedup  flexString `json:"dedup"`
}

type LVMPool struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Free   int64  `json:"free"`
	Used   int64  `json:"used"`
	Status string `json:"status"`
}

type LVMThinPool struct {
	Name        string  `json:"lv"`
	VolumeGroup string  `json:"vg"`
	Size        int64   `json:"lv_size"`
	Used        float64 `json:"used"`
}

type DirectoryDisk struct {
	Path       string `json:"path"`
	Device     string `json:"device"`
	Filesystem string `json:"filesystem"`
	Size       int64  `json:"size"`
	Units      string `json:"units"`
}

type SmartResult struct {
	Health     string                     `json:"health"`
	Type       string                     `json:"type"`
	Text       string                     `json:"text"`
	Attributes map[string]json.RawMessage `json:"attributes"`
}

func nodeDiskPath(node, kind string) string {
	return fmt.Sprintf("/nodes/%s/disks/%s", url.PathEscape(node), kind)
}

func nodeDiskItemPath(node, kind, name string) string {
	return nodeDiskPath(node, kind) + "/" + url.PathEscape(name)
}

func (c *Client) NodeDisks(ctx context.Context, node string) ([]NodeDisk, error) {
	var disks []NodeDisk
	if err := c.do(ctx, http.MethodGet, nodeDiskPath(node, "list"), nil, nil, &disks); err != nil {
		return nil, err
	}
	return disks, nil
}

func (c *Client) NodeDiskSmart(ctx context.Context, node, disk string) (SmartResult, error) {
	query := url.Values{"disk": {disk}}
	var result SmartResult
	if err := c.do(ctx, http.MethodGet, nodeDiskPath(node, "smart"), query, nil, &result); err != nil {
		return SmartResult{}, err
	}
	return result, nil
}

func (c *Client) CreateNodeZFS(ctx context.Context, node, name string, devices []string, raidLevel string, params map[string]string) (string, error) {
	form := url.Values{"name": {name}, "raidlevel": {raidLevel}}
	form.Set("devices", strings.Join(devices, ","))
	for key, value := range params {
		form.Set(key, value)
	}
	return c.nodeDiskCreate(ctx, nodeDiskPath(node, "zfs"), form, "ZFS pool")
}

func (c *Client) CreateNodeLVM(ctx context.Context, node, name, device string, addStorage bool) (string, error) {
	form := url.Values{"name": {name}, "device": {device}}
	setOptionalBool(form, "add_storage", &addStorage)
	return c.nodeDiskCreate(ctx, nodeDiskPath(node, "lvm"), form, "LVM volume group")
}

func (c *Client) CreateNodeLVMThin(ctx context.Context, node, name, device string, addStorage bool) (string, error) {
	form := url.Values{"name": {name}, "device": {device}}
	setOptionalBool(form, "add_storage", &addStorage)
	return c.nodeDiskCreate(ctx, nodeDiskPath(node, "lvmthin"), form, "LVM thin pool")
}

func (c *Client) CreateNodeDirectory(ctx context.Context, node, name, device, filesystem string, addStorage bool) (string, error) {
	form := url.Values{"name": {name}, "device": {device}}
	if filesystem != "" {
		form.Set("filesystem", filesystem)
	}
	setOptionalBool(form, "add_storage", &addStorage)
	return c.nodeDiskCreate(ctx, nodeDiskPath(node, "directory"), form, "directory")
}

func (c *Client) nodeDiskCreate(ctx context.Context, apiPath string, form url.Values, label string) (string, error) {
	var upid string
	if err := c.do(ctx, http.MethodPost, apiPath, nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, label+" creation"); err != nil {
		return "", err
	}
	return upid, nil
}

func (c *Client) DeleteNodeDisk(ctx context.Context, node, kind, name, volumeGroup string, addStorage bool) (string, error) {
	form := url.Values{"cleanup-config": {"1"}, "cleanup-disks": {"1"}}
	if addStorage {
		form.Set("cleanup-config", "1")
	}
	if volumeGroup != "" {
		form.Set("volume-group", volumeGroup)
	}
	var upid string
	if err := c.do(ctx, http.MethodDelete, nodeDiskItemPath(node, kind, name), nil, form, &upid); err != nil {
		if errors.Is(err, errNotFound) {
			return "", nil
		}
		return "", err
	}
	return upid, nil
}

func (c *Client) InitNodeGPT(ctx context.Context, node, disk, uuid string) (string, error) {
	form := url.Values{"disk": {disk}}
	if uuid != "" {
		form.Set("uuid", uuid)
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, nodeDiskPath(node, "initgpt"), nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "GPT initialization"); err != nil {
		return "", err
	}
	return upid, nil
}

func (c *Client) WipeNodeDisk(ctx context.Context, node, disk string) (string, error) {
	form := url.Values{"disk": {disk}}
	var upid string
	if err := c.do(ctx, http.MethodPut, nodeDiskPath(node, "wipedisk"), nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "disk wipe"); err != nil {
		return "", err
	}
	return upid, nil
}

func (c *Client) NodeZFSPools(ctx context.Context, node string) ([]ZFSPool, error) {
	var pools []ZFSPool
	if err := c.do(ctx, http.MethodGet, nodeDiskPath(node, "zfs"), nil, nil, &pools); err != nil {
		return nil, err
	}
	return pools, nil
}

func (c *Client) NodeLVMPools(ctx context.Context, node string) ([]LVMPool, error) {
	var wrapper struct {
		Children []LVMPool `json:"children"`
	}
	if err := c.do(ctx, http.MethodGet, nodeDiskPath(node, "lvm"), nil, nil, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Children, nil
}

func (c *Client) NodeLVMThinPools(ctx context.Context, node string) ([]LVMThinPool, error) {
	var pools []LVMThinPool
	if err := c.do(ctx, http.MethodGet, nodeDiskPath(node, "lvmthin"), nil, nil, &pools); err != nil {
		return nil, err
	}
	return pools, nil
}

func (c *Client) NodeDirectories(ctx context.Context, node string) ([]DirectoryDisk, error) {
	var entries []DirectoryDisk
	if err := c.do(ctx, http.MethodGet, nodeDiskPath(node, "directory"), nil, nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func sortDisks(disks []NodeDisk) {
	sort.Slice(disks, func(i, j int) bool { return disks[i].DevPath < disks[j].DevPath })
}

// flexString accepts numbers and strings, as some Proxmox fields switch type.
type flexString string

func (value *flexString) UnmarshalJSON(raw []byte) error {
	if len(raw) > 0 && raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		*value = flexString(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return err
	}
	*value = flexString(number.String())
	return nil
}
