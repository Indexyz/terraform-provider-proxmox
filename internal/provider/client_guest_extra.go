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

func guestKindPath(kind, node, vmid string) string {
	return fmt.Sprintf("/nodes/%s/%s/%s", url.PathEscape(node), kind, url.PathEscape(vmid))
}

func (c *Client) GuestStatusAction(ctx context.Context, node, kind, vmid, action string, timeout, forceStop, toDisk int64, stateStorage string) (string, error) {
	form := url.Values{}
	switch action {
	case "shutdown", "reboot":
		if timeout > 0 {
			form.Set("timeout", strconv.FormatInt(timeout, 10))
		}
		if action == "shutdown" && forceStop > 0 {
			form.Set("forceStop", strconv.FormatInt(forceStop, 10))
		}
	case "suspend":
		if toDisk > 0 {
			form.Set("todisk", strconv.FormatInt(toDisk, 10))
		}
		if stateStorage != "" {
			form.Set("statestorage", stateStorage)
		}
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, guestKindPath(kind, node, vmid)+"/status/"+action, nil, form, &upid); err != nil {
		return "", err
	}
	if upid == "" {
		return "", nil
	}
	return upid, validateQemuTaskAck(upid, kind+" status "+action)
}

func (c *Client) GuestTemplate(ctx context.Context, node, kind, vmid string, disks []string) error {
	form := url.Values{}
	if kind == "qemu" && len(disks) > 0 {
		form.Set("disk", strings.Join(disks, ","))
	}
	return c.do(ctx, http.MethodPost, guestKindPath(kind, node, vmid)+"/template", nil, form, nil)
}

func (c *Client) GuestCloudinitRegenerate(ctx context.Context, node, vmid string) error {
	return c.do(ctx, http.MethodPut, guestKindPath("qemu", node, vmid)+"/cloudinit", nil, nil, nil)
}

func (c *Client) GuestDBusVMState(ctx context.Context, node, vmid, action string) error {
	return c.do(ctx, http.MethodPost, guestKindPath("qemu", node, vmid)+"/dbus-vmstate", nil, url.Values{"action": {action}}, nil)
}

func (c *Client) GuestUnlink(ctx context.Context, node, vmid string, ids []string, force bool) error {
	form := url.Values{"idlist": {strings.Join(ids, ",")}}
	if force {
		form.Set("force", "1")
	}
	return c.do(ctx, http.MethodPut, guestKindPath("qemu", node, vmid)+"/unlink", nil, form, nil)
}

func (c *Client) LXCResize(ctx context.Context, node, vmid, disk, size, digest string) error {
	form := url.Values{"disk": {disk}, "size": {size}}
	if digest != "" {
		form.Set("digest", digest)
	}
	return c.do(ctx, http.MethodPut, guestKindPath("lxc", node, vmid)+"/resize", nil, form, nil)
}

type GuestFeature struct {
	HasFeature proxmoxOptionalBool `json:"hasFeature"`
	Nodes      []string            `json:"nodes"`
}

func (c *Client) GuestFeature(ctx context.Context, node, kind, vmid, feature, snapname string) (GuestFeature, error) {
	query := url.Values{"feature": {feature}}
	if snapname != "" {
		query.Set("snapname", snapname)
	}
	var result GuestFeature
	err := c.do(ctx, http.MethodGet, guestKindPath(kind, node, vmid)+"/feature", query, nil, &result)
	return result, err
}

type GuestFirewallRef struct {
	Type    string `json:"type"`
	Ref     string `json:"ref"`
	Name    string `json:"name"`
	Comment string `json:"comment"`
	Scope   string `json:"scope"`
}

func (c *Client) GuestFirewallRefs(ctx context.Context, node, kind, vmid, refType string) ([]GuestFirewallRef, error) {
	query := url.Values{}
	if refType != "" {
		query.Set("type", refType)
	}
	var refs []GuestFirewallRef
	err := c.do(ctx, http.MethodGet, guestKindPath(kind, node, vmid)+"/firewall/refs", query, nil, &refs)
	return refs, err
}

func (c *Client) GuestPending(ctx context.Context, node, kind, vmid string) (string, error) {
	var items []map[string]any
	if err := c.do(ctx, http.MethodGet, guestKindPath(kind, node, vmid)+"/pending", nil, nil, &items); err != nil {
		return "", err
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *Client) GuestRRDImage(ctx context.Context, node, kind, vmid, cf, ds, timeframe string) (string, error) {
	query := url.Values{"cf": {cf}, "ds": {ds}, "timeframe": {timeframe}}
	var result struct {
		Image string `json:"image"`
	}
	err := c.do(ctx, http.MethodGet, guestKindPath(kind, node, vmid)+"/rrd", query, nil, &result)
	return result.Image, err
}

func (c *Client) GuestRRDData(ctx context.Context, node, kind, vmid, cf, timeframe string) (string, error) {
	query := url.Values{"cf": {cf}, "timeframe": {timeframe}}
	var items []map[string]any
	if err := c.do(ctx, http.MethodGet, guestKindPath(kind, node, vmid)+"/rrddata", query, nil, &items); err != nil {
		return "", err
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *Client) GuestVNCWebsocketPort(ctx context.Context, node, kind, vmid, vncticket, port string) (string, error) {
	query := url.Values{"vncticket": {vncticket}, "port": {port}}
	var result struct {
		Port string `json:"port"`
	}
	err := c.do(ctx, http.MethodGet, guestKindPath(kind, node, vmid)+"/vncwebsocket", query, nil, &result)
	return result.Port, err
}

func (c *Client) NodeVNCWebsocketPort(ctx context.Context, node, vncticket, port string) (string, error) {
	var result struct {
		Port string `json:"port"`
	}
	err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/vncwebsocket", url.Values{"vncticket": {vncticket}, "port": {port}}, nil, &result)
	return result.Port, err
}
