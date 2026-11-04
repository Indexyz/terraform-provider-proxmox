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
)

type NodeTask struct {
	UPID      string `json:"upid"`
	Node      string `json:"node"`
	Type      string `json:"type"`
	ID        string `json:"id"`
	Status    string `json:"status"`
	User      string `json:"user"`
	StartTime int64  `json:"starttime"`
	EndTime   int64  `json:"endtime"`
	PID       int64  `json:"pid"`
	PStart    int64  `json:"pstart"`
}

func (c *Client) NodeTasks(ctx context.Context, node string, limit int64) ([]NodeTask, error) {
	query := url.Values{}
	if limit > 0 {
		query.Set("limit", strconv.FormatInt(limit, 10))
	}
	var tasks []NodeTask
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/tasks", query, nil, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (c *Client) NodeCapabilities(ctx context.Context, node string) ([]string, error) {
	var entries []struct {
		Name string `json:"name"`
	}
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/capabilities", nil, nil, &entries); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	return names, nil
}

type NodeNetstatEntry struct {
	Dev  string     `json:"dev"`
	In   string     `json:"in"`
	Out  string     `json:"out"`
	VMID flexString `json:"vmid"`
}

func (c *Client) NodeNetstat(ctx context.Context, node string) ([]NodeNetstatEntry, error) {
	var entries []NodeNetstatEntry
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/netstat", nil, nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

type NodeHosts struct {
	Data   string `json:"data"`
	Digest string `json:"digest"`
}

func (c *Client) NodeHosts(ctx context.Context, node string) (*NodeHosts, error) {
	var hosts NodeHosts
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/hosts", nil, nil, &hosts); err != nil {
		return nil, err
	}
	return &hosts, nil
}

func (c *Client) SetNodeHosts(ctx context.Context, node, data, digest string) error {
	form := url.Values{"data": {data}}
	if digest != "" {
		form.Set("digest", digest)
	}
	return c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/hosts", nil, form, nil)
}

func (c *Client) NodeReport(ctx context.Context, node string) (string, error) {
	var report string
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/report", nil, nil, &report); err != nil {
		return "", err
	}
	return report, nil
}

type APLTemplate struct {
	Package     string `json:"package"`
	Template    string `json:"template"`
	OS          string `json:"os"`
	Version     string `json:"version"`
	Arch        string `json:"architecture"`
	Section     string `json:"section"`
	Description string `json:"description"`
	Headline    string `json:"headline"`
	Location    string `json:"location"`
	InfoPage    string `json:"infopage"`
	Maintainer  string `json:"maintainer"`
	Source      string `json:"source"`
}

func (c *Client) APLTemplates(ctx context.Context, node string) ([]APLTemplate, error) {
	var templates []APLTemplate
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/aplinfo", nil, nil, &templates); err != nil {
		return nil, err
	}
	return templates, nil
}

func (c *Client) DownloadAPLTemplate(ctx context.Context, node, storage, template string) (string, error) {
	form := url.Values{"storage": {storage}, "template": {template}}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/aplinfo", nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "apl template download"); err != nil {
		return "", err
	}
	return upid, nil
}

func (c *Client) NodeSDN(ctx context.Context, node string) ([]string, error) {
	var entries []struct {
		Name string `json:"name"`
	}
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/sdn", nil, nil, &entries); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	return names, nil
}

func (c *Client) NodeRRDImage(ctx context.Context, node, timeframe, cf, ds string) (string, error) {
	query := url.Values{"timeframe": {timeframe}, "cf": {cf}, "ds": {ds}}
	var image struct {
		Image string `json:"image"`
	}
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/rrd", query, nil, &image); err != nil {
		return "", err
	}
	return image.Image, nil
}

type PermissionEntry struct {
	Path        string          `json:"-"`
	Permissions map[string]bool `json:"-"`
}

func (c *Client) AccessPermissions(ctx context.Context, path, userid string) ([]PermissionEntry, error) {
	query := url.Values{}
	if path != "" {
		query.Set("path", path)
	}
	if userid != "" {
		query.Set("userid", userid)
	}
	var result map[string]map[string]int64
	if err := c.do(ctx, http.MethodGet, "/access/permissions", query, nil, &result); err != nil {
		return nil, err
	}
	entries := make([]PermissionEntry, 0, len(result))
	for entryPath, privileges := range result {
		converted := make(map[string]bool, len(privileges))
		for name, value := range privileges {
			converted[name] = value != 0
		}
		entries = append(entries, PermissionEntry{Path: entryPath, Permissions: converted})
	}
	return entries, nil
}

type NotificationTarget struct {
	Name    string              `json:"name"`
	Type    string              `json:"type"`
	Comment string              `json:"comment"`
	Origin  string              `json:"origin"`
	Disable proxmoxOptionalBool `json:"disable"`
}

func (c *Client) NotificationTargets(ctx context.Context) ([]NotificationTarget, error) {
	var targets []NotificationTarget
	if err := c.do(ctx, http.MethodGet, "/cluster/notifications/targets", nil, nil, &targets); err != nil {
		return nil, err
	}
	return targets, nil
}

type LXCInterface struct {
	Name  string `json:"name"`
	MAC   string `json:"hwaddr"`
	Inet  string `json:"inet"`
	Inet6 string `json:"inet6"`
}

func (c *Client) LXCInterfaces(ctx context.Context, node string, vmid int64) ([]LXCInterface, error) {
	var interfaces []LXCInterface
	apiPath := fmt.Sprintf("/nodes/%s/lxc/%d/interfaces", url.PathEscape(node), vmid)
	if err := c.do(ctx, http.MethodGet, apiPath, nil, nil, &interfaces); err != nil {
		return nil, err
	}
	return interfaces, nil
}

type StorageIdentity struct {
	Type string `json:"type"`
}

func (c *Client) StorageIdentity(ctx context.Context, node, storage string) (string, error) {
	var identity map[string]any
	apiPath := fmt.Sprintf("/nodes/%s/storage/%s/identity", url.PathEscape(node), url.PathEscape(storage))
	if err := c.do(ctx, http.MethodGet, apiPath, nil, nil, &identity); err != nil {
		return "", err
	}
	data, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("encode storage identity: %w", err)
	}
	return string(data), nil
}

func (c *Client) StorageImportMetadata(ctx context.Context, node, storage, volume string) (string, error) {
	var metadata map[string]any
	apiPath := fmt.Sprintf("/nodes/%s/storage/%s/import-metadata", url.PathEscape(node), url.PathEscape(storage))
	if err := c.do(ctx, http.MethodGet, apiPath, url.Values{"volume": {volume}}, nil, &metadata); err != nil {
		return "", err
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("encode import metadata: %w", err)
	}
	return string(data), nil
}

func (c *Client) StorageRRDImage(ctx context.Context, node, storage, timeframe, cf, ds string) (string, error) {
	query := url.Values{"timeframe": {timeframe}, "cf": {cf}, "ds": {ds}}
	var image struct {
		Image string `json:"image"`
	}
	apiPath := fmt.Sprintf("/nodes/%s/storage/%s/rrd", url.PathEscape(node), url.PathEscape(storage))
	if err := c.do(ctx, http.MethodGet, apiPath, query, nil, &image); err != nil {
		return "", err
	}
	return image.Image, nil
}

func (c *Client) StorageRRDData(ctx context.Context, node, storage, timeframe, cf string) (string, error) {
	query := url.Values{"timeframe": {timeframe}}
	if cf != "" {
		query.Set("cf", cf)
	}
	apiPath := fmt.Sprintf("/nodes/%s/storage/%s/rrddata", url.PathEscape(node), url.PathEscape(storage))
	return c.RRDData(ctx, apiPath, query)
}

type FileRestoreEntry struct {
	Filepath string `json:"filepath"`
	Type     string `json:"type"`
	Size     int64  `json:"size"`
	Mtime    int64  `json:"mtime"`
	Text     string `json:"text"`
	Leaf     bool   `json:"leaf"`
}

func (c *Client) FileRestoreList(ctx context.Context, node, storage, volume, filepath string) ([]FileRestoreEntry, error) {
	query := url.Values{"volume": {volume}}
	if filepath != "" {
		query.Set("filepath", filepath)
	}
	apiPath := fmt.Sprintf("/nodes/%s/storage/%s/file-restore/list", url.PathEscape(node), url.PathEscape(storage))
	var entries []FileRestoreEntry
	if err := c.do(ctx, http.MethodGet, apiPath, query, nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (c *Client) FileRestoreDownload(ctx context.Context, node, storage, volume, filepath string, tar bool) (string, error) {
	query := url.Values{"volume": {volume}, "filepath": {filepath}}
	if tar {
		query.Set("tar", "1")
	}
	apiPath := fmt.Sprintf("/nodes/%s/storage/%s/file-restore/download", url.PathEscape(node), url.PathEscape(storage))
	var content string
	if err := c.do(ctx, http.MethodGet, apiPath, query, nil, &content); err != nil {
		return "", err
	}
	return content, nil
}

type URLMetadata struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	Mimetype string `json:"mimetype"`
}

func (c *Client) QueryURLMetadata(ctx context.Context, node, targetURL string, verify bool) (*URLMetadata, error) {
	query := url.Values{"url": {targetURL}}
	if !verify {
		query.Set("verify-certificates", "0")
	}
	var metadata URLMetadata
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/query-url-metadata", query, nil, &metadata); err != nil {
		return nil, err
	}
	return &metadata, nil
}

func (c *Client) QueryOCIRepoTags(ctx context.Context, node, reference string) ([]string, error) {
	var tags []string
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/query-oci-repo-tags", url.Values{"reference": {reference}}, nil, &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

func (c *Client) CloudInitDump(ctx context.Context, node string, vmid int64, kind string) (string, error) {
	apiPath := fmt.Sprintf("/nodes/%s/qemu/%d/cloudinit/dump", url.PathEscape(node), vmid)
	var content string
	if err := c.do(ctx, http.MethodGet, apiPath, url.Values{"type": {kind}}, nil, &content); err != nil {
		return "", err
	}
	return content, nil
}

type AgentFileRead struct {
	Content   string              `json:"content"`
	Truncated proxmoxOptionalBool `json:"truncated"`
}

func (c *Client) AgentFileRead(ctx context.Context, node string, vmid int64, file string, count, offset int64, decode bool) (*AgentFileRead, error) {
	query := url.Values{"file": {file}}
	if count > 0 {
		query.Set("count", strconv.FormatInt(count, 10))
	}
	if offset > 0 {
		query.Set("offset", strconv.FormatInt(offset, 10))
	}
	if decode {
		query.Set("decode", "1")
	}
	var result AgentFileRead
	apiPath := fmt.Sprintf("/nodes/%s/qemu/%d/agent/file-read", url.PathEscape(node), vmid)
	if err := c.do(ctx, http.MethodGet, apiPath, query, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) AgentFileWrite(ctx context.Context, node string, vmid int64, file, content string, encode bool) error {
	form := url.Values{"file": {file}, "content": {content}}
	if encode {
		form.Set("encode", "1")
	}
	apiPath := fmt.Sprintf("/nodes/%s/qemu/%d/agent/file-write", url.PathEscape(node), vmid)
	return c.do(ctx, http.MethodPost, apiPath, nil, form, nil)
}
