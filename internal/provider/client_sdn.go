// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// SDNZone is one zone entry of the pending-or-running SDN configuration.
// Proxmox keeps the running snapshot apart from the staged config and reports
// the difference in State/Pending.
type SDNZone struct {
	Zone                    string                     `json:"zone"`
	Type                    string                     `json:"type"`
	Nodes                   string                     `json:"nodes"`
	MTU                     proxmoxOptionalInt64       `json:"mtu"`
	DNS                     string                     `json:"dns"`
	ReverseDNS              string                     `json:"reversedns"`
	DNSZone                 string                     `json:"dnszone"`
	IPAM                    string                     `json:"ipam"`
	DHCP                    string                     `json:"dhcp"`
	Bridge                  string                     `json:"bridge"`
	BridgeDisableMACLearned proxmoxOptionalBool        `json:"bridge-disable-mac-learning"`
	State                   string                     `json:"state"`
	Pending                 map[string]json.RawMessage `json:"pending"`
	Digest                  string                     `json:"digest"`
}

// SDNZoneRequest carries explicitly configured, already validated wire values.
type SDNZoneRequest struct {
	Type                    string
	Nodes                   []string
	MTU                     *int64
	DNS                     *string
	ReverseDNS              *string
	DNSZone                 *string
	IPAM                    *string
	DHCP                    *string
	Bridge                  *string
	BridgeDisableMACLearned *bool
	LockToken               string
	Digest                  *string
	Delete                  []string
}

// LockSDN acquires the global SDN configuration lock, allowing existing pending
// changes to stay staged.
func (c *Client) LockSDN(ctx context.Context) (string, error) {
	var token string
	if err := c.do(ctx, http.MethodPost, "/cluster/sdn/lock", nil, url.Values{"allow-pending": {"1"}}, &token); err != nil {
		return "", err
	}
	return token, nil
}

func (c *Client) UnlockSDN(ctx context.Context, token string) error {
	return c.do(ctx, http.MethodDelete, "/cluster/sdn/lock", nil, url.Values{"lock-token": {token}}, nil)
}

// SDNZones lists zones of the staged configuration, including the pending diff
// against the running snapshot.
func (c *Client) SDNZones(ctx context.Context) ([]SDNZone, error) {
	var zones []SDNZone
	query := url.Values{"pending": {"1"}}
	if err := c.do(ctx, http.MethodGet, "/cluster/sdn/zones", query, nil, &zones); err != nil {
		return nil, err
	}
	return zones, nil
}

func (c *Client) GetSDNZone(ctx context.Context, zone string) (SDNZone, error) {
	zones, err := c.SDNZones(ctx)
	if err != nil {
		return SDNZone{}, err
	}
	for _, entry := range zones {
		if entry.Zone == zone {
			return entry, nil
		}
	}
	return SDNZone{}, errNotFound
}

func (c *Client) CreateSDNZone(ctx context.Context, zone string, req SDNZoneRequest) error {
	form := sdnZoneForm(req)
	form.Set("zone", zone)
	form.Set("type", req.Type)
	form.Set("lock-token", req.LockToken)
	return c.do(ctx, http.MethodPost, "/cluster/sdn/zones", nil, form, nil)
}

func (c *Client) UpdateSDNZone(ctx context.Context, zone string, req SDNZoneRequest) error {
	form := sdnZoneForm(req)
	// `zone` and `type` stay required body properties of the update schema.
	form.Set("zone", zone)
	form.Set("type", req.Type)
	form.Set("lock-token", req.LockToken)
	setOptionalString(form, "digest", req.Digest)
	if len(req.Delete) > 0 {
		form.Set("delete", strings.Join(sortedStrings(req.Delete), ","))
	}
	return c.do(ctx, http.MethodPut, "/cluster/sdn/zones/"+url.PathEscape(zone), nil, form, nil)
}

func (c *Client) DeleteSDNZone(ctx context.Context, zone, lockToken string) error {
	form := url.Values{"lock-token": {lockToken}}
	return c.do(ctx, http.MethodDelete, "/cluster/sdn/zones/"+url.PathEscape(zone), nil, form, nil)
}

// ApplySDN commits the staged SDN configuration and reloads every node. It
// holds the lock for the commit and releases it on success; a failed commit is
// unlocked on a best-effort basis and reported.
func (c *Client) ApplySDN(ctx context.Context) (string, error) {
	token, err := c.LockSDN(ctx)
	if err != nil {
		return "", err
	}
	form := url.Values{"lock-token": {token}, "release-lock": {"1"}}
	var upid string
	if err := c.do(ctx, http.MethodPut, "/cluster/sdn", nil, form, &upid); err != nil {
		_ = c.UnlockSDN(ctx, token)
		return "", err
	}
	if err := validateQemuTaskAck(upid, "SDN reload"); err != nil {
		return "", err
	}
	return upid, nil
}

func sdnZoneForm(req SDNZoneRequest) url.Values {
	form := url.Values{}
	if req.Nodes != nil {
		form.Set("nodes", strings.Join(sortedStrings(req.Nodes), ","))
	}
	setOptionalInt64(form, "mtu", req.MTU)
	setOptionalString(form, "dns", req.DNS)
	setOptionalString(form, "reversedns", req.ReverseDNS)
	setOptionalString(form, "dnszone", req.DNSZone)
	setOptionalString(form, "ipam", req.IPAM)
	setOptionalString(form, "dhcp", req.DHCP)
	setOptionalString(form, "bridge", req.Bridge)
	setOptionalBool(form, "bridge-disable-mac-learning", req.BridgeDisableMACLearned)
	return form
}

// sdnPendingMap flattens the PVE pending diff object (scalar values or the
// literal "deleted") into strings for computed state.
func sdnPendingMap(pending map[string]json.RawMessage) map[string]string {
	if len(pending) == 0 {
		return nil
	}
	result := make(map[string]string, len(pending))
	for key, raw := range pending {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			result[key] = text
			continue
		}
		result[key] = strings.TrimSpace(string(raw))
	}
	return result
}

func sdnZoneNodeList(value string) []string {
	if value == "" {
		return nil
	}
	nodes := strings.Split(value, ",")
	for i := range nodes {
		nodes[i] = strings.TrimSpace(nodes[i])
	}
	return nodes
}

// SDNVnet is one VNet entry of the staged SDN configuration.
type SDNVnet struct {
	Vnet         string                     `json:"vnet"`
	Zone         string                     `json:"zone"`
	Alias        string                     `json:"alias"`
	Tag          proxmoxOptionalInt64       `json:"tag"`
	VLANAware    proxmoxOptionalBool        `json:"vlanaware"`
	IsolatePorts proxmoxOptionalBool        `json:"isolate-ports"`
	State        string                     `json:"state"`
	Pending      map[string]json.RawMessage `json:"pending"`
	Digest       string                     `json:"digest"`
}

type SDNVnetRequest struct {
	Zone         string
	Alias        *string
	Tag          *int64
	VLANAware    *bool
	IsolatePorts *bool
	LockToken    string
	Digest       *string
	Delete       []string
}

func (c *Client) SDNVnets(ctx context.Context) ([]SDNVnet, error) {
	var vnets []SDNVnet
	query := url.Values{"pending": {"1"}}
	if err := c.do(ctx, http.MethodGet, "/cluster/sdn/vnets", query, nil, &vnets); err != nil {
		return nil, err
	}
	return vnets, nil
}

func (c *Client) GetSDNVnet(ctx context.Context, vnet string) (SDNVnet, error) {
	vnets, err := c.SDNVnets(ctx)
	if err != nil {
		return SDNVnet{}, err
	}
	for _, entry := range vnets {
		if entry.Vnet == vnet {
			return entry, nil
		}
	}
	return SDNVnet{}, errNotFound
}

func (c *Client) CreateSDNVnet(ctx context.Context, vnet string, req SDNVnetRequest) error {
	form := sdnVnetForm(req)
	form.Set("vnet", vnet)
	form.Set("type", "vnet")
	form.Set("lock-token", req.LockToken)
	return c.do(ctx, http.MethodPost, "/cluster/sdn/vnets", nil, form, nil)
}

func (c *Client) UpdateSDNVnet(ctx context.Context, vnet string, req SDNVnetRequest) error {
	form := sdnVnetForm(req)
	form.Set("vnet", vnet)
	form.Set("type", "vnet")
	form.Set("lock-token", req.LockToken)
	setOptionalString(form, "digest", req.Digest)
	if len(req.Delete) > 0 {
		form.Set("delete", strings.Join(sortedStrings(req.Delete), ","))
	}
	return c.do(ctx, http.MethodPut, "/cluster/sdn/vnets/"+url.PathEscape(vnet), nil, form, nil)
}

func (c *Client) DeleteSDNVnet(ctx context.Context, vnet, lockToken string) error {
	form := url.Values{"lock-token": {lockToken}}
	return c.do(ctx, http.MethodDelete, "/cluster/sdn/vnets/"+url.PathEscape(vnet), nil, form, nil)
}

func sdnVnetForm(req SDNVnetRequest) url.Values {
	form := url.Values{}
	if req.Zone != "" {
		form.Set("zone", req.Zone)
	}
	setOptionalString(form, "alias", req.Alias)
	setOptionalInt64(form, "tag", req.Tag)
	setOptionalBool(form, "vlanaware", req.VLANAware)
	setOptionalBool(form, "isolate-ports", req.IsolatePorts)
	return form
}

// SDNSubnet is one subnet entry of the staged SDN configuration. Subnet is the
// internal Proxmox id (`<zone>-<network>-<mask>`); CIDR is what users write.
type SDNSubnet struct {
	Subnet        string                     `json:"subnet"`
	CIDR          string                     `json:"cidr"`
	Vnet          string                     `json:"vnet"`
	Zone          string                     `json:"zone"`
	Gateway       string                     `json:"gateway"`
	SNAT          proxmoxOptionalBool        `json:"snat"`
	DNSZonePrefix string                     `json:"dnszoneprefix"`
	DHCPRanges    []SDNDHCPRange             `json:"dhcp-range"`
	DHCPDNSServer string                     `json:"dhcp-dns-server"`
	State         string                     `json:"state"`
	Pending       map[string]json.RawMessage `json:"pending"`
	Digest        string                     `json:"digest"`
}

type SDNDHCPRange struct {
	StartAddress string `json:"start-address"`
	EndAddress   string `json:"end-address"`
}

type SDNSubnetRequest struct {
	Vnet          string
	Gateway       *string
	SNAT          *bool
	DNSZonePrefix *string
	DHCPRanges    []SDNDHCPRange
	DHCPDNSServer *string
	LockToken     string
	Digest        *string
	Delete        []string
}

func (c *Client) SDNSubnets(ctx context.Context, vnet string) ([]SDNSubnet, error) {
	var subnets []SDNSubnet
	query := url.Values{"pending": {"1"}}
	if err := c.do(ctx, http.MethodGet, sdnSubnetsPath(vnet), query, nil, &subnets); err != nil {
		return nil, err
	}
	return subnets, nil
}

func (c *Client) GetSDNSubnet(ctx context.Context, vnet, cidr string) (SDNSubnet, error) {
	subnets, err := c.SDNSubnets(ctx, vnet)
	if err != nil {
		return SDNSubnet{}, err
	}
	for _, entry := range subnets {
		if entry.CIDR == cidr {
			return entry, nil
		}
	}
	return SDNSubnet{}, errNotFound
}

// CreateSDNSubnet takes the CIDR because Proxmox derives the internal subnet id
// from the parent zone and the CIDR on create.
func (c *Client) CreateSDNSubnet(ctx context.Context, vnet, cidr string, req SDNSubnetRequest) error {
	form := sdnSubnetForm(req)
	form.Set("vnet", vnet)
	form.Set("subnet", cidr)
	form.Set("type", "subnet")
	form.Set("lock-token", req.LockToken)
	return c.do(ctx, http.MethodPost, sdnSubnetsPath(vnet), nil, form, nil)
}

// UpdateSDNSubnet and DeleteSDNSubnet need the internal subnet id, because the
// handlers look the section up by it.
func (c *Client) UpdateSDNSubnet(ctx context.Context, vnet, subnetID string, req SDNSubnetRequest) error {
	form := sdnSubnetForm(req)
	form.Set("vnet", vnet)
	form.Set("subnet", subnetID)
	form.Set("type", "subnet")
	form.Set("lock-token", req.LockToken)
	setOptionalString(form, "digest", req.Digest)
	if len(req.Delete) > 0 {
		form.Set("delete", strings.Join(sortedStrings(req.Delete), ","))
	}
	return c.do(ctx, http.MethodPut, sdnSubnetsPath(vnet)+"/"+url.PathEscape(subnetID), nil, form, nil)
}

func (c *Client) DeleteSDNSubnet(ctx context.Context, vnet, subnetID, lockToken string) error {
	form := url.Values{"lock-token": {lockToken}}
	return c.do(ctx, http.MethodDelete, sdnSubnetsPath(vnet)+"/"+url.PathEscape(subnetID), nil, form, nil)
}

func sdnSubnetsPath(vnet string) string {
	return "/cluster/sdn/vnets/" + url.PathEscape(vnet) + "/subnets"
}

func sdnSubnetForm(req SDNSubnetRequest) url.Values {
	form := url.Values{}
	setOptionalString(form, "gateway", req.Gateway)
	setOptionalBool(form, "snat", req.SNAT)
	setOptionalString(form, "dnszoneprefix", req.DNSZonePrefix)
	for _, dhcpRange := range req.DHCPRanges {
		form.Add("dhcp-range", fmt.Sprintf("start-address=%s,end-address=%s", dhcpRange.StartAddress, dhcpRange.EndAddress))
	}
	setOptionalString(form, "dhcp-dns-server", req.DHCPDNSServer)
	return form
}
