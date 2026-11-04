// Copyright IBM Corp. 2021, 2026
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
	"time"
)

// CephStatus returns the raw cluster status JSON.
func (c *Client) CephStatus(ctx context.Context) (string, error) {
	var status map[string]any
	if err := c.do(ctx, http.MethodGet, "/cluster/ceph/status", nil, nil, &status); err != nil {
		return "", err
	}
	data, err := json.Marshal(status)
	if err != nil {
		return "", fmt.Errorf("encode ceph status: %w", err)
	}
	return string(data), nil
}

type CephFlag struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Value       proxmoxOptionalBool `json:"value"`
}

func (c *Client) CephFlags(ctx context.Context) ([]CephFlag, error) {
	var flags []CephFlag
	if err := c.do(ctx, http.MethodGet, "/cluster/ceph/flags", nil, nil, &flags); err != nil {
		return nil, err
	}
	return flags, nil
}

func (c *Client) SetCephFlag(ctx context.Context, flag string, value bool) error {
	form := url.Values{"flag": {flag}, "value": {map[bool]string{true: "1", false: "0"}[value]}}
	return c.do(ctx, http.MethodPut, "/cluster/ceph/flags/"+url.PathEscape(flag), nil, form, nil)
}

// CephOSDTree returns the raw OSD tree JSON.
func (c *Client) CephOSDTree(ctx context.Context, node string) (string, error) {
	var tree map[string]any
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/ceph/osd", nil, nil, &tree); err != nil {
		return "", err
	}
	data, err := json.Marshal(tree)
	if err != nil {
		return "", fmt.Errorf("encode ceph osd tree: %w", err)
	}
	return string(data), nil
}

type CephMonitor struct {
	Name        string              `json:"name"`
	Addr        string              `json:"addr"`
	Host        string              `json:"host"`
	Rank        int64               `json:"rank"`
	Quorum      proxmoxOptionalBool `json:"quorum"`
	State       string              `json:"state"`
	CephVersion string              `json:"ceph_version"`
}

func (c *Client) CephMonitors(ctx context.Context, node string) ([]CephMonitor, error) {
	var monitors []CephMonitor
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/ceph/mon", nil, nil, &monitors); err != nil {
		return nil, err
	}
	return monitors, nil
}

func (c *Client) CreateCephMonitor(ctx context.Context, node, monid, address string) (string, error) {
	form := url.Values{}
	if address != "" {
		form.Set("mon-address", address)
	}
	var upid string
	apiPath := "/nodes/" + url.PathEscape(node) + "/ceph/mon/" + url.PathEscape(monid)
	if err := c.do(ctx, http.MethodPost, apiPath, nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "ceph mon create"); err != nil {
		return "", err
	}
	return upid, nil
}

func (c *Client) DeleteCephMonitor(ctx context.Context, node, monid string) (string, error) {
	apiPath := "/nodes/" + url.PathEscape(node) + "/ceph/mon/" + url.PathEscape(monid)
	var upid string
	if err := c.do(ctx, http.MethodDelete, apiPath, nil, nil, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "ceph mon destroy"); err != nil {
		return "", err
	}
	return upid, nil
}

type CephPool struct {
	Name            string  `json:"pool_name"`
	PoolID          int64   `json:"pool"`
	Size            int64   `json:"size"`
	MinSize         int64   `json:"min_size"`
	PGNum           int64   `json:"pg_num"`
	PGNumMin        int64   `json:"pg_num_min"`
	PGAutoscaleMode string  `json:"pg_autoscale_mode"`
	CrushRule       int64   `json:"crush_rule"`
	CrushRuleName   string  `json:"crush_rule_name"`
	BytesUsed       int64   `json:"bytes_used"`
	PercentUsed     float64 `json:"percent_used"`
	Type            string  `json:"type"`
}

func (c *Client) CephPools(ctx context.Context, node string) ([]CephPool, error) {
	var pools []CephPool
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/ceph/pool", nil, nil, &pools); err != nil {
		return nil, err
	}
	return pools, nil
}

// CreateCephPool creates a pool and returns the worker UPID.
func (c *Client) CreateCephPool(ctx context.Context, node string, pool CephPool) (string, error) {
	form := url.Values{"name": {pool.Name}}
	if pool.Size > 0 {
		form.Set("size", strconv.FormatInt(pool.Size, 10))
	}
	if pool.MinSize > 0 {
		form.Set("min_size", strconv.FormatInt(pool.MinSize, 10))
	}
	if pool.PGNum > 0 {
		form.Set("pg_num", strconv.FormatInt(pool.PGNum, 10))
	}
	if pool.PGNumMin > 0 {
		form.Set("pg_num_min", strconv.FormatInt(pool.PGNumMin, 10))
	}
	if pool.PGAutoscaleMode != "" {
		form.Set("pg_autoscale_mode", pool.PGAutoscaleMode)
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/ceph/pool", nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "ceph pool create"); err != nil {
		return "", err
	}
	return upid, nil
}

// UpdateCephPool applies mutable pool settings.
func (c *Client) UpdateCephPool(ctx context.Context, node, name string, pool CephPool) error {
	form := url.Values{}
	if pool.Size > 0 {
		form.Set("size", strconv.FormatInt(pool.Size, 10))
	}
	if pool.MinSize > 0 {
		form.Set("min_size", strconv.FormatInt(pool.MinSize, 10))
	}
	if pool.PGNum > 0 {
		form.Set("pg_num", strconv.FormatInt(pool.PGNum, 10))
	}
	if pool.PGNumMin > 0 {
		form.Set("pg_num_min", strconv.FormatInt(pool.PGNumMin, 10))
	}
	if pool.PGAutoscaleMode != "" {
		form.Set("pg_autoscale_mode", pool.PGAutoscaleMode)
	}
	return c.do(ctx, http.MethodPut, "/nodes/"+url.PathEscape(node)+"/ceph/pool/"+url.PathEscape(name), nil, form, nil)
}

func (c *Client) DeleteCephPool(ctx context.Context, node, name string) (string, error) {
	var upid string
	apiPath := "/nodes/" + url.PathEscape(node) + "/ceph/pool/" + url.PathEscape(name)
	if err := c.do(ctx, http.MethodDelete, apiPath, nil, nil, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "ceph pool destroy"); err != nil {
		return "", err
	}
	return upid, nil
}

// CreateCephOSD creates an OSD and returns the worker UPID.
func (c *Client) CreateCephOSD(ctx context.Context, node, dev, dbDev, walDev, deviceClass string, osdsPerDevice int64, encrypted bool) (string, error) {
	form := url.Values{"dev": {dev}}
	if osdsPerDevice > 0 {
		form.Set("osds-per-device", strconv.FormatInt(osdsPerDevice, 10))
	}
	if dbDev != "" {
		form.Set("db_dev", dbDev)
	}
	if walDev != "" {
		form.Set("wal_dev", walDev)
	}
	if deviceClass != "" {
		form.Set("crush-device-class", deviceClass)
	}
	if encrypted {
		form.Set("encrypted", "1")
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/ceph/osd", nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "ceph osd create"); err != nil {
		return "", err
	}
	return upid, nil
}

func (c *Client) DeleteCephOSD(ctx context.Context, node string, osdid int64, cleanup bool) (string, error) {
	form := url.Values{}
	if cleanup {
		form.Set("cleanup", "1")
	}
	apiPath := fmt.Sprintf("/nodes/%s/ceph/osd/%d", url.PathEscape(node), osdid)
	var upid string
	if err := c.do(ctx, http.MethodDelete, apiPath, nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "ceph osd destroy"); err != nil {
		return "", err
	}
	return upid, nil
}

type CephOSDTreeEntry struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Type        string              `json:"type"`
	Device      string              `json:"device"`
	Status      string              `json:"status"`
	In          proxmoxOptionalBool `json:"in"`
	DeviceClass string              `json:"device_class"`
	Children    []CephOSDTreeEntry  `json:"children"`
}

// FindCephOSDByDevice returns the OSD id whose CRUSH entry uses dev.
func (c *Client) FindCephOSDByDevice(ctx context.Context, node, dev string) (int64, error) {
	base := dev[strings.LastIndex(dev, "/")+1:]
	deadline := time.Now().Add(45 * time.Second)
	for {
		var tree struct {
			Root CephOSDTreeEntry `json:"root"`
		}
		if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/ceph/osd", nil, nil, &tree); err != nil {
			return 0, err
		}
		for _, entry := range flattenCephTree(tree.Root) {
			if entry.Type != "osd" {
				continue
			}
			osdid, err := strconv.ParseInt(entry.ID, 10, 64)
			if err != nil {
				continue
			}
			devices, err := c.cephOSDDevices(ctx, node, entry.ID)
			if err != nil {
				continue
			}
			for _, device := range devices {
				if device.PhysicalDevice == base || device.DevNode == dev || device.Device == dev {
					return osdid, nil
				}
			}
		}
		if time.Now().After(deadline) {
			return 0, errNotFound
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

type cephOSDDevice struct {
	PhysicalDevice string `json:"physical_device"`
	DevNode        string `json:"dev_node"`
	Device         string `json:"device"`
}

func (c *Client) cephOSDDevices(ctx context.Context, node, osdid string) ([]cephOSDDevice, error) {
	var metadata struct {
		Devices []cephOSDDevice `json:"devices"`
	}
	apiPath := fmt.Sprintf("/nodes/%s/ceph/osd/%s/metadata", url.PathEscape(node), osdid)
	if err := c.do(ctx, http.MethodGet, apiPath, nil, nil, &metadata); err != nil {
		return nil, err
	}
	return metadata.Devices, nil
}

func flattenCephTree(node CephOSDTreeEntry) []CephOSDTreeEntry {
	var out []CephOSDTreeEntry
	var walk func(CephOSDTreeEntry)
	walk = func(current CephOSDTreeEntry) {
		out = append(out, current)
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(node)
	return out
}

// MarkCephOSDOut removes an OSD from the data distribution before destroy.
func (c *Client) MarkCephOSDOut(ctx context.Context, node string, osdid int64) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/nodes/%s/ceph/osd/%d/out", url.PathEscape(node), osdid), nil, url.Values{}, nil)
}

// WaitCephOSDOut polls until the OSD reports in=0.
func (c *Client) WaitCephOSDOut(ctx context.Context, node string, osdid int64) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		var tree struct {
			Root CephOSDTreeEntry `json:"root"`
		}
		if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/ceph/osd", nil, nil, &tree); err != nil {
			return err
		}
		for _, entry := range flattenCephTree(tree.Root) {
			if entry.Type == "osd" && entry.ID == strconv.FormatInt(osdid, 10) && entry.In.Ptr() != nil && !*entry.In.Ptr() {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("osd %d is still in", osdid)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// StopCephOSDService stops the OSD daemon so the volume can be destroyed.
func (c *Client) StopCephOSDService(ctx context.Context, node string, osdid int64) error {
	form := url.Values{"service": {fmt.Sprintf("osd.%d", osdid)}}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/ceph/stop", nil, form, &upid); err != nil {
		return err
	}
	return c.waitForNodeTask(ctx, node, upid)
}

func (c *Client) CephServiceAction(ctx context.Context, node, service, action string) (string, error) {
	var upid string
	err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/ceph/"+action, nil, url.Values{"service": {service}}, &upid)
	if err != nil {
		return "", err
	}
	if upid == "" {
		return "", nil
	}
	return upid, validateQemuTaskAck(upid, "ceph "+action)
}
