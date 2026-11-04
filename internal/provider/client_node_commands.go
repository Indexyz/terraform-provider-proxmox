// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

func (c *Client) WakeOnLAN(ctx context.Context, node string) (string, error) {
	var mac string
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/wakeonlan", nil, url.Values{}, &mac); err != nil {
		return "", err
	}
	return mac, nil
}

func (c *Client) StartAll(ctx context.Context, node string, vmids []int64, force bool, maxWorkers int64) (string, error) {
	form := url.Values{}
	if len(vmids) > 0 {
		form.Set("vms", joinInt64(vmids))
	}
	if force {
		form.Set("force", "1")
	}
	if maxWorkers > 0 {
		form.Set("max-workers", strconv.FormatInt(maxWorkers, 10))
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/startall", nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "startall"); err != nil {
		return "", err
	}
	return upid, nil
}

func (c *Client) MigrateAll(ctx context.Context, node, target string, vmids []int64, withLocalDisks bool, maxWorkers int64) (string, error) {
	form := url.Values{"target": {target}}
	if len(vmids) > 0 {
		form.Set("vms", joinInt64(vmids))
	}
	if withLocalDisks {
		form.Set("with-local-disks", "1")
	}
	if maxWorkers > 0 {
		form.Set("max-workers", strconv.FormatInt(maxWorkers, 10))
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/migrateall", nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "migrateall"); err != nil {
		return "", err
	}
	return upid, nil
}

func (c *Client) OCIPull(ctx context.Context, node, storage, reference, filename string) (string, error) {
	form := url.Values{"reference": {reference}}
	if filename != "" {
		form.Set("filename", filename)
	}
	var upid string
	apiPath := fmt.Sprintf("/nodes/%s/storage/%s/oci-registry-pull", url.PathEscape(node), url.PathEscape(storage))
	if err := c.do(ctx, http.MethodPost, apiPath, nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "oci pull"); err != nil {
		return "", err
	}
	return upid, nil
}

func (c *Client) SendKey(ctx context.Context, node string, vmid int64, key string) error {
	form := url.Values{"key": {key}}
	apiPath := fmt.Sprintf("/nodes/%s/qemu/%d/sendkey", url.PathEscape(node), vmid)
	return c.do(ctx, http.MethodPut, apiPath, nil, form, nil)
}

func (c *Client) QemuMonitor(ctx context.Context, node string, vmid int64, command string) (string, error) {
	form := url.Values{"command": {command}}
	apiPath := fmt.Sprintf("/nodes/%s/qemu/%d/monitor", url.PathEscape(node), vmid)
	var result string
	if err := c.do(ctx, http.MethodPost, apiPath, nil, form, &result); err != nil {
		return "", err
	}
	return result, nil
}

func (c *Client) UpdateCloudInitPending(ctx context.Context, node string, vmid int64, values map[string]string, deleteKeys []string) error {
	form := url.Values{}
	for key, value := range values {
		form.Set(key, value)
	}
	for _, key := range deleteKeys {
		form.Add("delete", key)
	}
	apiPath := fmt.Sprintf("/nodes/%s/qemu/%d/cloudinit/pending", url.PathEscape(node), vmid)
	return c.do(ctx, http.MethodPut, apiPath, nil, form, nil)
}

func (c *Client) AgentFSFreeze(ctx context.Context, node string, vmid int64, action string) (string, error) {
	var result struct {
		Result any `json:"result"`
	}
	apiPath := fmt.Sprintf("/nodes/%s/qemu/%d/agent/fsfreeze-%s", url.PathEscape(node), vmid, action)
	if err := c.do(ctx, http.MethodPost, apiPath, nil, url.Values{}, &result); err != nil {
		return "", err
	}
	if result.Result == nil {
		return "", nil
	}
	return fmt.Sprint(result.Result), nil
}

func (c *Client) AgentFstrim(ctx context.Context, node string, vmid int64) error {
	apiPath := fmt.Sprintf("/nodes/%s/qemu/%d/agent/fstrim", url.PathEscape(node), vmid)
	return c.do(ctx, http.MethodPost, apiPath, nil, url.Values{}, nil)
}

type QemuMoveDisk struct {
	Disk         string
	Storage      string
	TargetDisk   string
	TargetVMID   int64
	Delete       bool
	Format       string
	BWLimit      int64
	Digest       string
	TargetDigest string
}

func (c *Client) MoveQemuDisk(ctx context.Context, node string, vmid int64, move QemuMoveDisk) (string, error) {
	form := url.Values{"disk": {move.Disk}, "storage": {move.Storage}}
	if move.TargetDisk != "" {
		form.Set("target-disk", move.TargetDisk)
	}
	if move.TargetVMID > 0 {
		form.Set("target-vmid", strconv.FormatInt(move.TargetVMID, 10))
	}
	if move.Delete {
		form.Set("delete", "1")
	}
	if move.Format != "" {
		form.Set("format", move.Format)
	}
	if move.BWLimit > 0 {
		form.Set("bwlimit", strconv.FormatInt(move.BWLimit, 10))
	}
	if move.Digest != "" {
		form.Set("digest", move.Digest)
	}
	if move.TargetDigest != "" {
		form.Set("target-digest", move.TargetDigest)
	}
	apiPath := fmt.Sprintf("/nodes/%s/qemu/%d/move_disk", url.PathEscape(node), vmid)
	var upid string
	if err := c.do(ctx, http.MethodPost, apiPath, nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "move disk"); err != nil {
		return "", err
	}
	return upid, nil
}

type GuestVolumeMove struct {
	GuestType    string
	Volume       string
	Storage      string
	TargetVol    string
	TargetVMID   int64
	Delete       bool
	BWLimit      int64
	Digest       string
	TargetDigest string
}

func (c *Client) MoveGuestVolume(ctx context.Context, node string, vmid int64, move GuestVolumeMove) (string, error) {
	form := url.Values{"storage": {move.Storage}}
	if move.Volume != "" {
		form.Set("volume", move.Volume)
	}
	if move.TargetVol != "" {
		form.Set("target-volume", move.TargetVol)
	}
	if move.TargetVMID > 0 {
		form.Set("target-vmid", strconv.FormatInt(move.TargetVMID, 10))
	}
	if move.Delete {
		form.Set("delete", "1")
	}
	if move.BWLimit > 0 {
		form.Set("bwlimit", strconv.FormatInt(move.BWLimit, 10))
	}
	if move.Digest != "" {
		form.Set("digest", move.Digest)
	}
	if move.TargetDigest != "" {
		form.Set("target-digest", move.TargetDigest)
	}
	apiPath := fmt.Sprintf("/nodes/%s/%s/%d/move_volume", url.PathEscape(node), move.GuestType, vmid)
	var upid string
	if err := c.do(ctx, http.MethodPost, apiPath, nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "move volume"); err != nil {
		return "", err
	}
	return upid, nil
}

type GuestRemoteMigrate struct {
	GuestType      string
	TargetEndpoint string
	TargetStorage  string
	TargetBridge   string
	TargetVMID     int64
	Online         bool
	Restart        bool
	Delete         bool
	BWLimit        int64
	Timeout        int64
}

func (c *Client) RemoteMigrateGuest(ctx context.Context, node string, vmid int64, move GuestRemoteMigrate) (string, error) {
	form := url.Values{"target-endpoint": {move.TargetEndpoint}}
	if move.TargetStorage != "" {
		form.Set("target-storage", move.TargetStorage)
	}
	if move.TargetBridge != "" {
		form.Set("target-bridge", move.TargetBridge)
	}
	if move.TargetVMID > 0 {
		form.Set("target-vmid", strconv.FormatInt(move.TargetVMID, 10))
	}
	if move.Online {
		form.Set("online", "1")
	}
	if move.Restart {
		form.Set("restart", "1")
	}
	if move.Delete {
		form.Set("delete", "1")
	}
	if move.BWLimit > 0 {
		form.Set("bwlimit", strconv.FormatInt(move.BWLimit, 10))
	}
	if move.Timeout > 0 {
		form.Set("timeout", strconv.FormatInt(move.Timeout, 10))
	}
	apiPath := fmt.Sprintf("/nodes/%s/%s/%d/remote_migrate", url.PathEscape(node), move.GuestType, vmid)
	var upid string
	if err := c.do(ctx, http.MethodPost, apiPath, nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "remote migrate"); err != nil {
		return "", err
	}
	return upid, nil
}

type VNCTicket struct {
	Port   string `json:"port"`
	Ticket string `json:"ticket"`
	UPID   string `json:"upid"`
	User   string `json:"user"`
	Cert   string `json:"cert"`
}

func (c *Client) AccessVNCTicket(ctx context.Context, userid, path, vncticket string, privs string) (*VNCTicket, error) {
	form := url.Values{"authid": {userid}, "path": {path}, "vncticket": {vncticket}}
	if privs != "" {
		form.Set("privs", privs)
	}
	var ticket VNCTicket
	if err := c.do(ctx, http.MethodPost, "/access/vncticket", nil, form, &ticket); err != nil {
		return nil, err
	}
	return &ticket, nil
}

func (c *Client) NodeTermProxy(ctx context.Context, node, cmd string, cmdOpts string) (map[string]any, error) {
	return c.nodeProxy(ctx, "/nodes/"+url.PathEscape(node)+"/termproxy", cmd, cmdOpts)
}

func (c *Client) NodeVNCShell(ctx context.Context, node, cmd, cmdOpts string, width, height int64) (map[string]any, error) {
	form := url.Values{"websocket": {"1"}}
	if cmd != "" {
		form.Set("cmd", cmd)
	}
	if cmdOpts != "" {
		form.Set("cmd-opts", cmdOpts)
	}
	if width > 0 {
		form.Set("width", strconv.FormatInt(width, 10))
	}
	if height > 0 {
		form.Set("height", strconv.FormatInt(height, 10))
	}
	var result map[string]any
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/vncshell", nil, form, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) NodeSpiceShell(ctx context.Context, node, cmd, cmdOpts, proxy string) (map[string]any, error) {
	form := url.Values{"proxy": {proxy}}
	if cmd != "" {
		form.Set("cmd", cmd)
	}
	if cmdOpts != "" {
		form.Set("cmd-opts", cmdOpts)
	}
	var result map[string]any
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/spiceshell", nil, form, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) nodeProxy(ctx context.Context, apiPath, cmd, cmdOpts string) (map[string]any, error) {
	form := url.Values{}
	if cmd != "" {
		form.Set("cmd", cmd)
	}
	if cmdOpts != "" {
		form.Set("cmd-opts", cmdOpts)
	}
	var result map[string]any
	if err := c.do(ctx, http.MethodPost, apiPath, nil, form, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) QemuVNCProxy(ctx context.Context, node string, vmid int64, generatePassword bool, websocket bool) (map[string]any, error) {
	form := url.Values{}
	if generatePassword {
		form.Set("generate-password", "1")
	}
	if websocket {
		form.Set("websocket", "1")
	}
	var result map[string]any
	apiPath := fmt.Sprintf("/nodes/%s/qemu/%d/vncproxy", url.PathEscape(node), vmid)
	if err := c.do(ctx, http.MethodPost, apiPath, nil, form, &result); err != nil {
		return nil, err
	}
	return result, nil
}

type GuestProxy struct {
	TypeName         string `json:"-"`
	GuestType        string `json:"-"`
	Node             string `json:"-"`
	VMID             int64  `json:"-"`
	Kind             string `json:"-"`
	GeneratePassword bool   `json:"-"`
	Websocket        bool   `json:"-"`
	Proxy            string `json:"-"`
}

func (c *Client) GuestProxy(ctx context.Context, proxy GuestProxy) (map[string]any, error) {
	form := url.Values{}
	switch proxy.Kind {
	case "vncproxy":
		if proxy.GeneratePassword {
			form.Set("generate-password", "1")
		}
		if proxy.Websocket {
			form.Set("websocket", "1")
		}
	case "spiceproxy":
		if proxy.Proxy != "" {
			form.Set("proxy", proxy.Proxy)
		}
	}
	apiPath := fmt.Sprintf("/nodes/%s/%s/%d/%s", url.PathEscape(proxy.Node), proxy.GuestType, proxy.VMID, proxy.Kind)
	var result map[string]any
	if err := c.do(ctx, http.MethodPost, apiPath, nil, form, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GuestMTunnelWebsocket(ctx context.Context, guestType, node string, vmid int64, socket, ticket string) (map[string]any, error) {
	query := url.Values{"socket": {socket}, "ticket": {ticket}}
	apiPath := fmt.Sprintf("/nodes/%s/%s/%d/mtunnelwebsocket", url.PathEscape(node), guestType, vmid)
	var result map[string]any
	if err := c.do(ctx, http.MethodGet, apiPath, query, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}
