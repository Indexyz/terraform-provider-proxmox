// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// NodeNetworkInterface is one entry of the staged node network configuration.
// Proxmox reports the applied file merged with the staged shadow file, so this
// reflects pending changes until the node network is applied.
type NodeNetworkInterface struct {
	Iface              string               `json:"iface"`
	Type               string               `json:"type"`
	Autostart          proxmoxOptionalBool  `json:"autostart"`
	MTU                proxmoxOptionalInt64 `json:"mtu"`
	Comments           string               `json:"comments"`
	Comments6          string               `json:"comments6"`
	Address            string               `json:"address"`
	Netmask            string               `json:"netmask"`
	Gateway            string               `json:"gateway"`
	Address6           string               `json:"address6"`
	Netmask6           proxmoxOptionalInt64 `json:"netmask6"`
	Gateway6           string               `json:"gateway6"`
	BridgePorts        string               `json:"bridge_ports"`
	BridgeVLANAware    proxmoxOptionalBool  `json:"bridge_vlan_aware"`
	BridgeVIDs         string               `json:"bridge_vids"`
	Slaves             string               `json:"slaves"`
	BondMode           string               `json:"bond_mode"`
	BondXmitHashPolicy string               `json:"bond_xmit_hash_policy"`
	BondPrimary        string               `json:"bond-primary"`
	VLANID             proxmoxOptionalInt64 `json:"vlan-id"`
	VLANRawDevice      string               `json:"vlan-raw-device"`
}

// NodeNetworkInterfaceRequest carries explicitly configured, already validated
// wire values. Port lists are space separated as Proxmox stores them.
type NodeNetworkInterfaceRequest struct {
	Type               string
	Autostart          *bool
	MTU                *int64
	Comments           *string
	Comments6          *string
	Address            *string
	Netmask            *string
	Gateway            *string
	Address6           *string
	Netmask6           *int64
	Gateway6           *string
	BridgePorts        []string
	BridgeVLANAware    *bool
	BridgeVIDs         *string
	Slaves             []string
	BondMode           *string
	BondXmitHashPolicy *string
	BondPrimary        *string
	VLANID             *int64
	VLANRawDevice      *string
	Delete             []string
}

// NodeNetworkInterfaces lists the staged interface view and the pending change
// list. Proxmox exposes the latter as a response result attribute, not in data.
func (c *Client) NodeNetworkInterfaces(ctx context.Context, node string) ([]NodeNetworkInterface, []string, error) {
	var ifaces []NodeNetworkInterface
	var result apiResult
	if err := c.doResult(ctx, http.MethodGet, nodeNetworkPath(node), nil, &ifaces, &result); err != nil {
		return nil, nil, err
	}
	return ifaces, result.Changes, nil
}

func (c *Client) GetNodeNetworkInterface(ctx context.Context, node, iface string) (NodeNetworkInterface, error) {
	ifaces, _, err := c.NodeNetworkInterfaces(ctx, node)
	if err != nil {
		return NodeNetworkInterface{}, err
	}
	for _, entry := range ifaces {
		if entry.Iface == iface {
			return entry, nil
		}
	}
	return NodeNetworkInterface{}, errNotFound
}

func (c *Client) CreateNodeNetworkInterface(ctx context.Context, node, iface string, req NodeNetworkInterfaceRequest) error {
	form := nodeNetworkInterfaceForm(req)
	form.Set("iface", iface)
	form.Set("type", req.Type)
	return c.do(ctx, http.MethodPost, nodeNetworkPath(node), nil, form, nil)
}

func (c *Client) UpdateNodeNetworkInterface(ctx context.Context, node, iface string, req NodeNetworkInterfaceRequest) error {
	form := nodeNetworkInterfaceForm(req)
	form.Set("type", req.Type)
	if len(req.Delete) > 0 {
		form.Set("delete", strings.Join(sortedStrings(req.Delete), ","))
	}
	return c.do(ctx, http.MethodPut, nodeNetworkPath(node)+"/"+url.PathEscape(iface), nil, form, nil)
}

func (c *Client) DeleteNodeNetworkInterface(ctx context.Context, node, iface string) error {
	if _, err := c.GetNodeNetworkInterface(ctx, node, iface); err != nil {
		if errors.Is(err, errNotFound) {
			return nil
		}
		return err
	}
	return c.do(ctx, http.MethodDelete, nodeNetworkPath(node)+"/"+url.PathEscape(iface), nil, nil, nil)
}

// ApplyNodeNetwork replaces the applied configuration with the staged one and
// reloads the node network. It returns the reload worker UPID.
func (c *Client) ApplyNodeNetwork(ctx context.Context, node string, regenerateFRR *bool) (string, error) {
	form := url.Values{}
	setOptionalBool(form, "regenerate-frr", regenerateFRR)
	var upid string
	if err := c.do(ctx, http.MethodPut, nodeNetworkPath(node), nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "network reload"); err != nil {
		return "", err
	}
	return upid, nil
}

func nodeNetworkPath(node string) string {
	return fmt.Sprintf("/nodes/%s/network", url.PathEscape(node))
}

func nodeNetworkInterfaceForm(req NodeNetworkInterfaceRequest) url.Values {
	form := url.Values{}
	setOptionalBool(form, "autostart", req.Autostart)
	setOptionalInt64(form, "mtu", req.MTU)
	setOptionalString(form, "comments", req.Comments)
	setOptionalString(form, "comments6", req.Comments6)
	setOptionalString(form, "address", req.Address)
	setOptionalString(form, "netmask", req.Netmask)
	setOptionalString(form, "gateway", req.Gateway)
	setOptionalString(form, "address6", req.Address6)
	setOptionalInt64(form, "netmask6", req.Netmask6)
	setOptionalString(form, "gateway6", req.Gateway6)
	if req.BridgePorts != nil {
		form.Set("bridge_ports", strings.Join(sortedStrings(req.BridgePorts), " "))
	}
	setOptionalBool(form, "bridge_vlan_aware", req.BridgeVLANAware)
	setOptionalString(form, "bridge_vids", req.BridgeVIDs)
	if req.Slaves != nil {
		form.Set("slaves", strings.Join(sortedStrings(req.Slaves), " "))
	}
	setOptionalString(form, "bond_mode", req.BondMode)
	setOptionalString(form, "bond_xmit_hash_policy", req.BondXmitHashPolicy)
	setOptionalString(form, "bond-primary", req.BondPrimary)
	setOptionalInt64(form, "vlan-id", req.VLANID)
	setOptionalString(form, "vlan-raw-device", req.VLANRawDevice)
	return form
}
