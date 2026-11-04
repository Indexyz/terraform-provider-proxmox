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
)

type CephFS struct {
	Name         string `json:"name"`
	DataPool     string `json:"data_pool"`
	MetadataPool string `json:"metadata_pool"`
}

func (c *Client) CephFS(ctx context.Context, node string) ([]CephFS, error) {
	var items []CephFS
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/ceph/fs", nil, nil, &items); err != nil {
		return nil, err
	}
	return items, nil
}
func (c *Client) CreateCephFS(ctx context.Context, node, name string, pgNum int64, addStorage bool) (string, error) {
	form := url.Values{"name": {name}}
	if pgNum > 0 {
		form.Set("pg_num", strconv.FormatInt(pgNum, 10))
	}
	if addStorage {
		form.Set("add-storage", "1")
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/ceph/fs/"+url.PathEscape(name), nil, form, &upid); err != nil {
		return "", err
	}
	return upid, validateQemuTaskAck(upid, "ceph fs create")
}
func (c *Client) DeleteCephFS(ctx context.Context, node, name string) (string, error) {
	var upid string
	if err := c.do(ctx, http.MethodDelete, "/nodes/"+url.PathEscape(node)+"/ceph/fs/"+url.PathEscape(name), nil, nil, &upid); err != nil {
		return "", err
	}
	return upid, validateQemuTaskAck(upid, "ceph fs destroy")
}

type CephMDS struct {
	Name   string `json:"name"`
	Host   string `json:"host"`
	FSName string `json:"fs_name"`
	State  string `json:"state"`
}

func (c *Client) CephMDS(ctx context.Context, node string) ([]CephMDS, error) {
	var items []CephMDS
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/ceph/mds", nil, nil, &items); err != nil {
		return nil, err
	}
	return items, nil
}
func (c *Client) CreateCephMDS(ctx context.Context, node, name string, hotstandby bool) (string, error) {
	form := url.Values{"name": {name}}
	if hotstandby {
		form.Set("hotstandby", "1")
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/ceph/mds/"+url.PathEscape(name), nil, form, &upid); err != nil {
		return "", err
	}
	return upid, validateQemuTaskAck(upid, "ceph mds create")
}
func (c *Client) DeleteCephMDS(ctx context.Context, node, name string) (string, error) {
	var upid string
	if err := c.do(ctx, http.MethodDelete, "/nodes/"+url.PathEscape(node)+"/ceph/mds/"+url.PathEscape(name), nil, nil, &upid); err != nil {
		return "", err
	}
	return upid, validateQemuTaskAck(upid, "ceph mds destroy")
}

type CephMGR struct {
	Name  string `json:"name"`
	Host  string `json:"host"`
	State string `json:"state"`
}

func (c *Client) CephMGRs(ctx context.Context, node string) ([]CephMGR, error) {
	var items []CephMGR
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/ceph/mgr", nil, nil, &items); err != nil {
		return nil, err
	}
	return items, nil
}
func (c *Client) CreateCephMGR(ctx context.Context, node, id string) (string, error) {
	var upid string
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/ceph/mgr/"+url.PathEscape(id), nil, url.Values{}, &upid); err != nil {
		return "", err
	}
	return upid, validateQemuTaskAck(upid, "ceph mgr create")
}
func (c *Client) DeleteCephMGR(ctx context.Context, node, id string) (string, error) {
	var upid string
	if err := c.do(ctx, http.MethodDelete, "/nodes/"+url.PathEscape(node)+"/ceph/mgr/"+url.PathEscape(id), nil, nil, &upid); err != nil {
		return "", err
	}
	return upid, validateQemuTaskAck(upid, "ceph mgr destroy")
}

func (c *Client) CephCrush(ctx context.Context, node string) (string, error) {
	var crush string
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/ceph/crush", nil, nil, &crush); err != nil {
		return "", err
	}
	return crush, nil
}
func (c *Client) CephRules(ctx context.Context, node string) ([]string, error) {
	var items []struct {
		Name string `json:"name"`
	}
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/ceph/rules", nil, nil, &items); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	return names, nil
}

type CephConfigEntry struct {
	Name               string              `json:"name"`
	Section            string              `json:"section"`
	Value              string              `json:"value"`
	Level              string              `json:"level"`
	Mask               string              `json:"mask"`
	CanUpdateAtRuntime proxmoxOptionalBool `json:"can_update_at_runtime"`
}

func (c *Client) CephConfigDB(ctx context.Context, node string) ([]CephConfigEntry, error) {
	var items []CephConfigEntry
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/ceph/cfg/db", nil, nil, &items); err != nil {
		return nil, err
	}
	return items, nil
}
func (c *Client) CephConfigRaw(ctx context.Context, node string) (string, error) {
	var raw string
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/ceph/cfg/raw", nil, nil, &raw); err != nil {
		return "", err
	}
	return raw, nil
}
func (c *Client) CephConfigValue(ctx context.Context, node string, keys []string) (string, error) {
	query := url.Values{"config-keys": {strings.Join(keys, ";")}}
	var value map[string]any
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/ceph/cfg/value", query, nil, &value); err != nil {
		return "", err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type CephLogEntry struct {
	Line int64  `json:"n"`
	Text string `json:"t"`
}

func (c *Client) CephLog(ctx context.Context, node string, limit int64) ([]CephLogEntry, error) {
	query := url.Values{}
	if limit > 0 {
		query.Set("limit", strconv.FormatInt(limit, 10))
	}
	var items []CephLogEntry
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/ceph/log", query, nil, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (c *Client) CephInit(ctx context.Context, node string, network, clusterNetwork string, size, minSize, pgBits int64, disableCephx bool) (string, error) {
	form := url.Values{}
	if network != "" {
		form.Set("network", network)
	}
	if clusterNetwork != "" {
		form.Set("cluster-network", clusterNetwork)
	}
	if size > 0 {
		form.Set("size", strconv.FormatInt(size, 10))
	}
	if minSize > 0 {
		form.Set("min_size", strconv.FormatInt(minSize, 10))
	}
	if pgBits > 0 {
		form.Set("pg_bits", strconv.FormatInt(pgBits, 10))
	}
	if disableCephx {
		form.Set("disable_cephx", "1")
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/ceph/init", nil, form, &upid); err != nil {
		return "", err
	}
	return upid, validateQemuTaskAck(upid, "ceph init")
}
func (c *Client) CephRestart(ctx context.Context, node, service string) (string, error) {
	var upid string
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/ceph/restart", nil, url.Values{"service": {service}}, &upid); err != nil {
		return "", err
	}
	return upid, validateQemuTaskAck(upid, "ceph restart")
}
func (c *Client) CephRestartBulk(ctx context.Context, node, serviceType string, force, onlyOutdated, setNoout bool, timeout int64) (string, error) {
	form := url.Values{}
	if serviceType != "" {
		form.Set("service-type", serviceType)
	}
	if force {
		form.Set("force", "1")
	}
	if onlyOutdated {
		form.Set("only-outdated", "1")
	}
	if setNoout {
		form.Set("set-noout", "1")
	}
	if timeout > 0 {
		form.Set("timeout", strconv.FormatInt(timeout, 10))
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/ceph/restart-bulk", nil, form, &upid); err != nil {
		return "", err
	}
	return upid, validateQemuTaskAck(upid, "ceph restart bulk")
}
func (c *Client) ScrubCephOSD(ctx context.Context, node string, osdid int64, deep bool) (string, error) {
	form := url.Values{}
	if deep {
		form.Set("deep", "1")
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/nodes/%s/ceph/osd/%d/scrub", url.PathEscape(node), osdid), nil, form, &upid); err != nil {
		return "", err
	}
	if upid == "" {
		return "", nil
	}
	return upid, validateQemuTaskAck(upid, "ceph osd scrub")
}
func (c *Client) CephOSDLVInfo(ctx context.Context, node string, osdid int64, kind string) (string, error) {
	var info map[string]any
	query := url.Values{"type": {kind}}
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/ceph/osd/%d/lv-info", url.PathEscape(node), osdid), query, nil, &info); err != nil {
		return "", err
	}
	data, err := json.Marshal(info)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
func (c *Client) CephOSDMetadata(ctx context.Context, node string, osdid int64) (string, error) {
	var metadata map[string]any
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/ceph/osd/%d/metadata", url.PathEscape(node), osdid), nil, nil, &metadata); err != nil {
		return "", err
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
