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
	Tag                     proxmoxOptionalInt64       `json:"tag"`
	VLANProtocol            string                     `json:"vlan-protocol"`
	Peers                   string                     `json:"peers"`
	Fabric                  string                     `json:"fabric"`
	VXLANPort               proxmoxOptionalInt64       `json:"vxlan-port"`
	VRFVXLAN                proxmoxOptionalInt64       `json:"vrf-vxlan"`
	Controller              string                     `json:"controller"`
	SecondaryControllers    []string                   `json:"secondary-controllers"`
	MAC                     string                     `json:"mac"`
	ExitNodes               string                     `json:"exitnodes"`
	ExitNodesLocalRouting   proxmoxOptionalBool        `json:"exitnodes-local-routing"`
	ExitNodesPrimary        string                     `json:"exitnodes-primary"`
	AdvertiseSubnets        proxmoxOptionalBool        `json:"advertise-subnets"`
	DisableARPNDSuppression proxmoxOptionalBool        `json:"disable-arp-nd-suppression"`
	RTImport                string                     `json:"rt-import"`
	DPID                    proxmoxOptionalInt64       `json:"dp-id"`
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
	Tag                     *int64
	VLANProtocol            *string
	Peers                   []string
	Fabric                  *string
	VXLANPort               *int64
	VRFVXLAN                *int64
	Controller              *string
	SecondaryControllers    []string
	MAC                     *string
	ExitNodes               []string
	ExitNodesLocalRouting   *bool
	ExitNodesPrimary        *string
	AdvertiseSubnets        *bool
	DisableARPNDSuppression *bool
	RTImport                *string
	DPID                    *int64
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
	setOptionalInt64(form, "tag", req.Tag)
	setOptionalString(form, "vlan-protocol", req.VLANProtocol)
	if req.Peers != nil {
		form.Set("peers", strings.Join(sortedStrings(req.Peers), ","))
	}
	setOptionalString(form, "fabric", req.Fabric)
	setOptionalInt64(form, "vxlan-port", req.VXLANPort)
	setOptionalInt64(form, "vrf-vxlan", req.VRFVXLAN)
	setOptionalString(form, "controller", req.Controller)
	for _, controller := range sortedStrings(req.SecondaryControllers) {
		form.Add("secondary-controllers", controller)
	}
	setOptionalString(form, "mac", req.MAC)
	if req.ExitNodes != nil {
		form.Set("exitnodes", strings.Join(sortedStrings(req.ExitNodes), ","))
	}
	setOptionalBool(form, "exitnodes-local-routing", req.ExitNodesLocalRouting)
	setOptionalString(form, "exitnodes-primary", req.ExitNodesPrimary)
	setOptionalBool(form, "advertise-subnets", req.AdvertiseSubnets)
	setOptionalBool(form, "disable-arp-nd-suppression", req.DisableARPNDSuppression)
	setOptionalString(form, "rt-import", req.RTImport)
	setOptionalInt64(form, "dp-id", req.DPID)
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

// SDNIPAM is one IPAM entry of the staged SDN configuration.
type SDNIPAM struct {
	IPAM        string                     `json:"ipam"`
	Type        string                     `json:"type"`
	URL         string                     `json:"url"`
	Token       string                     `json:"token"`
	Section     proxmoxOptionalInt64       `json:"section"`
	Fingerprint string                     `json:"fingerprint"`
	State       string                     `json:"state"`
	Pending     map[string]json.RawMessage `json:"pending"`
	Digest      string                     `json:"digest"`
}

type SDNIPAMRequest struct {
	Type        string
	URL         *string
	Token       *string
	Section     *int64
	Fingerprint *string
	LockToken   string
	Digest      *string
	Delete      []string
}

// SDNDNS is one DNS provider entry of the staged SDN configuration.
type SDNDNS struct {
	DNS           string                     `json:"dns"`
	Type          string                     `json:"type"`
	URL           string                     `json:"url"`
	Key           string                     `json:"key"`
	TTL           proxmoxOptionalInt64       `json:"ttl"`
	ReverseMaskV6 proxmoxOptionalInt64       `json:"reversemaskv6"`
	Fingerprint   string                     `json:"fingerprint"`
	State         string                     `json:"state"`
	Pending       map[string]json.RawMessage `json:"pending"`
	Digest        string                     `json:"digest"`
}

type SDNDNSRequest struct {
	Type          string
	URL           *string
	Key           *string
	TTL           *int64
	ReverseMaskV6 *int64
	Fingerprint   *string
	LockToken     string
	Digest        *string
	Delete        []string
}

// SDNController is one SDN controller entry of the staged SDN configuration.
type SDNController struct {
	Controller       string                     `json:"controller"`
	Type             string                     `json:"type"`
	ASN              proxmoxOptionalInt64       `json:"asn"`
	Node             string                     `json:"node"`
	Peers            string                     `json:"peers"`
	Nodes            string                     `json:"nodes"`
	Fabric           string                     `json:"fabric"`
	PeerGroupName    string                     `json:"peer-group-name"`
	BGPMode          string                     `json:"bgp-mode"`
	EBGP             proxmoxOptionalBool        `json:"ebgp"`
	EBGPMultihop     proxmoxOptionalInt64       `json:"ebgp-multihop"`
	MultipathASRelax proxmoxOptionalBool        `json:"bgp-multipath-as-path-relax"`
	Loopback         string                     `json:"loopback"`
	ISISDomain       string                     `json:"isis-domain"`
	ISISIfaces       string                     `json:"isis-ifaces"`
	ISISNet          string                     `json:"isis-net"`
	RouteMapIn       string                     `json:"route-map-in"`
	RouteMapOut      string                     `json:"route-map-out"`
	State            string                     `json:"state"`
	Pending          map[string]json.RawMessage `json:"pending"`
	Digest           string                     `json:"digest"`
}

type SDNControllerRequest struct {
	Type             string
	ASN              *int64
	Node             *string
	Peers            []string
	Nodes            []string
	Fabric           *string
	PeerGroupName    *string
	BGPMode          *string
	EBGP             *bool
	EBGPMultihop     *int64
	MultipathASRelax *bool
	Loopback         *string
	ISISDomain       *string
	ISISIfaces       []string
	ISISNet          *string
	RouteMapIn       *string
	RouteMapOut      *string
	LockToken        string
	Digest           *string
	Delete           []string
}

func (c *Client) sdnSectionList(ctx context.Context, apiPath string) ([]json.RawMessage, error) {
	var objects []json.RawMessage
	query := url.Values{"pending": {"1"}}
	if err := c.do(ctx, http.MethodGet, apiPath, query, nil, &objects); err != nil {
		return nil, err
	}
	return objects, nil
}

func (c *Client) createSDNObject(ctx context.Context, apiPath, idParam, id, objectType, lockToken string, form url.Values) error {
	form.Set(idParam, id)
	form.Set("type", objectType)
	form.Set("lock-token", lockToken)
	return c.do(ctx, http.MethodPost, apiPath, nil, form, nil)
}

func (c *Client) updateSDNObject(ctx context.Context, apiPath, idParam, id, objectType, lockToken string, digest *string, deleteKeys []string, form url.Values) error {
	form.Set(idParam, id)
	form.Set("type", objectType)
	form.Set("lock-token", lockToken)
	setOptionalString(form, "digest", digest)
	if len(deleteKeys) > 0 {
		form.Set("delete", strings.Join(sortedStrings(deleteKeys), ","))
	}
	return c.do(ctx, http.MethodPut, apiPath+"/"+url.PathEscape(id), nil, form, nil)
}

func (c *Client) deleteSDNObject(ctx context.Context, apiPath, id, lockToken string) error {
	form := url.Values{"lock-token": {lockToken}}
	return c.do(ctx, http.MethodDelete, apiPath+"/"+url.PathEscape(id), nil, form, nil)
}

func (c *Client) SDNIPAMs(ctx context.Context) ([]SDNIPAM, error) {
	objects, err := c.sdnSectionList(ctx, "/cluster/sdn/ipams")
	if err != nil {
		return nil, err
	}
	var ipams []SDNIPAM
	for _, raw := range objects {
		var ipam SDNIPAM
		if err := json.Unmarshal(raw, &ipam); err != nil {
			return nil, fmt.Errorf("unable to decode SDN IPAM: %w", err)
		}
		ipams = append(ipams, ipam)
	}
	return ipams, nil
}

func (c *Client) GetSDNIPAM(ctx context.Context, ipam string) (SDNIPAM, error) {
	ipams, err := c.SDNIPAMs(ctx)
	if err != nil {
		return SDNIPAM{}, err
	}
	for _, entry := range ipams {
		if entry.IPAM == ipam {
			return entry, nil
		}
	}
	return SDNIPAM{}, errNotFound
}

func (c *Client) CreateSDNIPAM(ctx context.Context, ipam string, req SDNIPAMRequest) error {
	return c.createSDNObject(ctx, "/cluster/sdn/ipams", "ipam", ipam, req.Type, req.LockToken, sdnIPAMForm(req))
}

func (c *Client) UpdateSDNIPAM(ctx context.Context, ipam string, req SDNIPAMRequest) error {
	return c.updateSDNObject(ctx, "/cluster/sdn/ipams", "ipam", ipam, req.Type, req.LockToken, req.Digest, req.Delete, sdnIPAMForm(req))
}

func (c *Client) DeleteSDNIPAM(ctx context.Context, ipam, lockToken string) error {
	return c.deleteSDNObject(ctx, "/cluster/sdn/ipams", ipam, lockToken)
}

func sdnIPAMForm(req SDNIPAMRequest) url.Values {
	form := url.Values{}
	setOptionalString(form, "url", req.URL)
	setOptionalString(form, "token", req.Token)
	setOptionalInt64(form, "section", req.Section)
	setOptionalString(form, "fingerprint", req.Fingerprint)
	return form
}

func (c *Client) SDNDNSs(ctx context.Context) ([]SDNDNS, error) {
	objects, err := c.sdnSectionList(ctx, "/cluster/sdn/dns")
	if err != nil {
		return nil, err
	}
	var servers []SDNDNS
	for _, raw := range objects {
		var server SDNDNS
		if err := json.Unmarshal(raw, &server); err != nil {
			return nil, fmt.Errorf("unable to decode SDN DNS provider: %w", err)
		}
		servers = append(servers, server)
	}
	return servers, nil
}

func (c *Client) GetSDNDNS(ctx context.Context, dns string) (SDNDNS, error) {
	servers, err := c.SDNDNSs(ctx)
	if err != nil {
		return SDNDNS{}, err
	}
	for _, entry := range servers {
		if entry.DNS == dns {
			return entry, nil
		}
	}
	return SDNDNS{}, errNotFound
}

func (c *Client) CreateSDNDNS(ctx context.Context, dns string, req SDNDNSRequest) error {
	return c.createSDNObject(ctx, "/cluster/sdn/dns", "dns", dns, req.Type, req.LockToken, sdnDNSForm(req))
}

func (c *Client) UpdateSDNDNS(ctx context.Context, dns string, req SDNDNSRequest) error {
	return c.updateSDNObject(ctx, "/cluster/sdn/dns", "dns", dns, req.Type, req.LockToken, req.Digest, req.Delete, sdnDNSForm(req))
}

func (c *Client) DeleteSDNDNS(ctx context.Context, dns, lockToken string) error {
	return c.deleteSDNObject(ctx, "/cluster/sdn/dns", dns, lockToken)
}

func sdnDNSForm(req SDNDNSRequest) url.Values {
	form := url.Values{}
	setOptionalString(form, "url", req.URL)
	setOptionalString(form, "key", req.Key)
	setOptionalInt64(form, "ttl", req.TTL)
	setOptionalInt64(form, "reversemaskv6", req.ReverseMaskV6)
	setOptionalString(form, "fingerprint", req.Fingerprint)
	return form
}

func (c *Client) SDNControllers(ctx context.Context) ([]SDNController, error) {
	objects, err := c.sdnSectionList(ctx, "/cluster/sdn/controllers")
	if err != nil {
		return nil, err
	}
	var controllers []SDNController
	for _, raw := range objects {
		var controller SDNController
		if err := json.Unmarshal(raw, &controller); err != nil {
			return nil, fmt.Errorf("unable to decode SDN controller: %w", err)
		}
		controllers = append(controllers, controller)
	}
	return controllers, nil
}

func (c *Client) GetSDNController(ctx context.Context, controller string) (SDNController, error) {
	controllers, err := c.SDNControllers(ctx)
	if err != nil {
		return SDNController{}, err
	}
	for _, entry := range controllers {
		if entry.Controller == controller {
			return entry, nil
		}
	}
	return SDNController{}, errNotFound
}

func (c *Client) CreateSDNController(ctx context.Context, controller string, req SDNControllerRequest) error {
	return c.createSDNObject(ctx, "/cluster/sdn/controllers", "controller", controller, req.Type, req.LockToken, sdnControllerForm(req))
}

func (c *Client) UpdateSDNController(ctx context.Context, controller string, req SDNControllerRequest) error {
	return c.updateSDNObject(ctx, "/cluster/sdn/controllers", "controller", controller, req.Type, req.LockToken, req.Digest, req.Delete, sdnControllerForm(req))
}

func (c *Client) DeleteSDNController(ctx context.Context, controller, lockToken string) error {
	return c.deleteSDNObject(ctx, "/cluster/sdn/controllers", controller, lockToken)
}

func sdnControllerForm(req SDNControllerRequest) url.Values {
	form := url.Values{}
	setOptionalInt64(form, "asn", req.ASN)
	setOptionalString(form, "node", req.Node)
	if req.Peers != nil {
		form.Set("peers", strings.Join(sortedStrings(req.Peers), ","))
	}
	if req.Nodes != nil {
		form.Set("nodes", strings.Join(sortedStrings(req.Nodes), ","))
	}
	setOptionalString(form, "fabric", req.Fabric)
	setOptionalString(form, "peer-group-name", req.PeerGroupName)
	setOptionalString(form, "bgp-mode", req.BGPMode)
	setOptionalBool(form, "ebgp", req.EBGP)
	setOptionalInt64(form, "ebgp-multihop", req.EBGPMultihop)
	setOptionalBool(form, "bgp-multipath-as-path-relax", req.MultipathASRelax)
	setOptionalString(form, "loopback", req.Loopback)
	setOptionalString(form, "isis-domain", req.ISISDomain)
	if req.ISISIfaces != nil {
		form.Set("isis-ifaces", strings.Join(sortedStrings(req.ISISIfaces), ","))
	}
	setOptionalString(form, "isis-net", req.ISISNet)
	setOptionalString(form, "route-map-in", req.RouteMapIn)
	setOptionalString(form, "route-map-out", req.RouteMapOut)
	return form
}

// SDNVnetFirewallOptions are the firewall options of one SDN VNet.
type SDNVnetFirewallOptions struct {
	Enable          proxmoxOptionalBool `json:"enable"`
	PolicyForward   string              `json:"policy_forward"`
	LogLevelForward string              `json:"log_level_forward"`
	Digest          string              `json:"digest"`
}

type SDNVnetFirewallOptionsRequest struct {
	Enable          *bool
	PolicyForward   *string
	LogLevelForward *string
	Digest          *string
	Delete          []string
}

func sdnVnetFirewallOptionsPath(vnet string) string {
	return "/cluster/sdn/vnets/" + url.PathEscape(vnet) + "/firewall/options"
}

func (c *Client) GetSDNVnetFirewallOptions(ctx context.Context, vnet string) (SDNVnetFirewallOptions, error) {
	var options SDNVnetFirewallOptions
	if err := c.do(ctx, http.MethodGet, sdnVnetFirewallOptionsPath(vnet), nil, nil, &options); err != nil {
		return SDNVnetFirewallOptions{}, err
	}
	return options, nil
}

func (c *Client) SetSDNVnetFirewallOptions(ctx context.Context, vnet string, req SDNVnetFirewallOptionsRequest) error {
	form := url.Values{}
	setOptionalBool(form, "enable", req.Enable)
	setOptionalString(form, "policy_forward", req.PolicyForward)
	setOptionalString(form, "log_level_forward", req.LogLevelForward)
	setOptionalString(form, "digest", req.Digest)
	if len(req.Delete) > 0 {
		form.Set("delete", strings.Join(sortedStrings(req.Delete), ","))
	}
	return c.do(ctx, http.MethodPut, sdnVnetFirewallOptionsPath(vnet), nil, form, nil)
}

// SDNFabric is one staged SDN fabric. Complex per-protocol lists stay property
// strings, matching how Proxmox encodes them.
type SDNFabric struct {
	ID                  string                     `json:"id"`
	Protocol            string                     `json:"protocol"`
	IPPrefix            string                     `json:"ip_prefix"`
	IP6Prefix           string                     `json:"ip6_prefix"`
	HelloInterval       *float64                   `json:"hello_interval"`
	CSNPInterval        *float64                   `json:"csnp_interval"`
	Area                string                     `json:"area"`
	RouteFilter         string                     `json:"route_filter"`
	PersistentKeepalive proxmoxOptionalInt64       `json:"persistent_keepalive"`
	RouteMapIn          string                     `json:"route_map_in"`
	RouteMapOut         string                     `json:"route_map_out"`
	Redistribute        []string                   `json:"redistribute"`
	State               string                     `json:"state"`
	Pending             map[string]json.RawMessage `json:"pending"`
	Digest              string                     `json:"digest"`
}

type SDNFabricRequest struct {
	Protocol            string
	IPPrefix            *string
	IP6Prefix           *string
	HelloInterval       *float64
	CSNPInterval        *float64
	Area                *string
	RouteFilter         *string
	PersistentKeepalive *int64
	RouteMapIn          *string
	RouteMapOut         *string
	Redistribute        []string
	LockToken           string
	Digest              *string
	Delete              []string
}

// SDNFabricNode is one staged node of an SDN fabric.
type SDNFabricNode struct {
	FabricID   string                     `json:"fabric_id"`
	NodeID     string                     `json:"node_id"`
	Protocol   string                     `json:"protocol"`
	IP         string                     `json:"ip"`
	IP6        string                     `json:"ip6"`
	Interfaces []string                   `json:"interfaces"`
	PublicKey  string                     `json:"public_key"`
	Role       string                     `json:"role"`
	Endpoint   string                     `json:"endpoint"`
	AllowedIPs []string                   `json:"allowed_ips"`
	Peers      []string                   `json:"peers"`
	State      string                     `json:"state"`
	Pending    map[string]json.RawMessage `json:"pending"`
	Digest     string                     `json:"digest"`
}

type SDNFabricNodeRequest struct {
	Protocol   string
	IP         *string
	IP6        *string
	Interfaces []string
	PublicKey  *string
	Role       *string
	Endpoint   *string
	AllowedIPs []string
	Peers      []string
	LockToken  string
	Digest     *string
	Delete     []string
}

func (c *Client) SDNFabrics(ctx context.Context) ([]SDNFabric, error) {
	objects, err := c.sdnSectionList(ctx, "/cluster/sdn/fabrics/fabric")
	if err != nil {
		return nil, err
	}
	var fabrics []SDNFabric
	for _, raw := range objects {
		var fabric SDNFabric
		if err := json.Unmarshal(raw, &fabric); err != nil {
			return nil, fmt.Errorf("unable to decode SDN fabric: %w", err)
		}
		fabrics = append(fabrics, fabric)
	}
	return fabrics, nil
}

func (c *Client) GetSDNFabric(ctx context.Context, id string) (SDNFabric, error) {
	fabrics, err := c.SDNFabrics(ctx)
	if err != nil {
		return SDNFabric{}, err
	}
	for _, entry := range fabrics {
		if entry.ID == id {
			return entry, nil
		}
	}
	return SDNFabric{}, errNotFound
}

func (c *Client) CreateSDNFabric(ctx context.Context, id string, req SDNFabricRequest) error {
	form := sdnFabricForm(req)
	form.Set("id", id)
	form.Set("protocol", req.Protocol)
	form.Set("lock-token", req.LockToken)
	return c.do(ctx, http.MethodPost, "/cluster/sdn/fabrics/fabric", nil, form, nil)
}

func (c *Client) UpdateSDNFabric(ctx context.Context, id string, req SDNFabricRequest) error {
	form := sdnFabricForm(req)
	form.Set("id", id)
	form.Set("protocol", req.Protocol)
	form.Set("lock-token", req.LockToken)
	setOptionalString(form, "digest", req.Digest)
	for _, key := range sortedStrings(req.Delete) {
		form.Add("delete", key)
	}
	return c.do(ctx, http.MethodPut, "/cluster/sdn/fabrics/fabric/"+url.PathEscape(id), nil, form, nil)
}

func (c *Client) DeleteSDNFabric(ctx context.Context, id, lockToken string) error {
	form := url.Values{"lock-token": {lockToken}}
	return c.do(ctx, http.MethodDelete, "/cluster/sdn/fabrics/fabric/"+url.PathEscape(id), nil, form, nil)
}

func sdnFabricForm(req SDNFabricRequest) url.Values {
	form := url.Values{}
	setOptionalString(form, "ip_prefix", req.IPPrefix)
	setOptionalString(form, "ip6_prefix", req.IP6Prefix)
	setOptionalFloat64(form, "hello_interval", req.HelloInterval)
	setOptionalFloat64(form, "csnp_interval", req.CSNPInterval)
	setOptionalString(form, "area", req.Area)
	setOptionalString(form, "route_filter", req.RouteFilter)
	setOptionalInt64(form, "persistent_keepalive", req.PersistentKeepalive)
	setOptionalString(form, "route_map_in", req.RouteMapIn)
	setOptionalString(form, "route_map_out", req.RouteMapOut)
	for _, entry := range sortedStrings(req.Redistribute) {
		form.Add("redistribute", entry)
	}
	return form
}

func sdnFabricNodesPath(fabricID string) string {
	return "/cluster/sdn/fabrics/node/" + url.PathEscape(fabricID)
}

func (c *Client) SDNFabricNodes(ctx context.Context, fabricID string) ([]SDNFabricNode, error) {
	objects, err := c.sdnSectionList(ctx, sdnFabricNodesPath(fabricID))
	if err != nil {
		return nil, err
	}
	var nodes []SDNFabricNode
	for _, raw := range objects {
		var node SDNFabricNode
		if err := json.Unmarshal(raw, &node); err != nil {
			return nil, fmt.Errorf("unable to decode SDN fabric node: %w", err)
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (c *Client) GetSDNFabricNode(ctx context.Context, fabricID, nodeID string) (SDNFabricNode, error) {
	nodes, err := c.SDNFabricNodes(ctx, fabricID)
	if err != nil {
		return SDNFabricNode{}, err
	}
	for _, entry := range nodes {
		if entry.NodeID == nodeID {
			return entry, nil
		}
	}
	return SDNFabricNode{}, errNotFound
}

func (c *Client) CreateSDNFabricNode(ctx context.Context, fabricID, nodeID string, req SDNFabricNodeRequest) error {
	form := sdnFabricNodeForm(req)
	form.Set("node_id", nodeID)
	form.Set("protocol", req.Protocol)
	form.Set("lock-token", req.LockToken)
	return c.do(ctx, http.MethodPost, sdnFabricNodesPath(fabricID), nil, form, nil)
}

func (c *Client) UpdateSDNFabricNode(ctx context.Context, fabricID, nodeID string, req SDNFabricNodeRequest) error {
	form := sdnFabricNodeForm(req)
	form.Set("node_id", nodeID)
	form.Set("protocol", req.Protocol)
	form.Set("lock-token", req.LockToken)
	setOptionalString(form, "digest", req.Digest)
	for _, key := range sortedStrings(req.Delete) {
		form.Add("delete", key)
	}
	return c.do(ctx, http.MethodPut, sdnFabricNodesPath(fabricID)+"/"+url.PathEscape(nodeID), nil, form, nil)
}

func (c *Client) DeleteSDNFabricNode(ctx context.Context, fabricID, nodeID, lockToken string) error {
	form := url.Values{"lock-token": {lockToken}}
	return c.do(ctx, http.MethodDelete, sdnFabricNodesPath(fabricID)+"/"+url.PathEscape(nodeID), nil, form, nil)
}

func sdnFabricNodeForm(req SDNFabricNodeRequest) url.Values {
	form := url.Values{}
	setOptionalString(form, "ip", req.IP)
	setOptionalString(form, "ip6", req.IP6)
	for _, entry := range sortedStrings(req.Interfaces) {
		form.Add("interfaces", entry)
	}
	setOptionalString(form, "public_key", req.PublicKey)
	setOptionalString(form, "role", req.Role)
	setOptionalString(form, "endpoint", req.Endpoint)
	for _, entry := range sortedStrings(req.AllowedIPs) {
		form.Add("allowed_ips", entry)
	}
	for _, entry := range sortedStrings(req.Peers) {
		form.Add("peers", entry)
	}
	return form
}

// SDNPrefixList is one staged SDN prefix list. Entries stay property strings
// (`action=permit,prefix=..[,le=..,ge=..]`) exactly as Proxmox encodes them.
type SDNPrefixList struct {
	ID      string                     `json:"id"`
	Entries []string                   `json:"entries"`
	State   string                     `json:"state"`
	Pending map[string]json.RawMessage `json:"pending"`
	Digest  string                     `json:"digest"`
}

type SDNPrefixListRequest struct {
	Entries   []string
	LockToken string
	Digest    *string
	Delete    []string
}

// SDNRouteMapEntry is one staged route map entry. Route maps exist implicitly
// through their entries.
type SDNRouteMapEntry struct {
	RouteMapID string                     `json:"route-map-id"`
	Order      proxmoxOptionalInt64       `json:"order"`
	Action     string                     `json:"action"`
	Set        []string                   `json:"set"`
	Match      []string                   `json:"match"`
	ExitAction string                     `json:"exit-action"`
	Call       string                     `json:"call"`
	State      string                     `json:"state"`
	Pending    map[string]json.RawMessage `json:"pending"`
	Digest     string                     `json:"digest"`
}

type SDNRouteMapEntryRequest struct {
	RouteMapID string
	Order      int64
	Action     string
	Set        []string
	Match      []string
	ExitAction *string
	Call       *string
	LockToken  string
	Digest     *string
	Delete     []string
}

func (c *Client) SDNPrefixLists(ctx context.Context) ([]SDNPrefixList, error) {
	objects, err := c.sdnSectionList(ctx, "/cluster/sdn/prefix-lists")
	if err != nil {
		return nil, err
	}
	var lists []SDNPrefixList
	for _, raw := range objects {
		var list SDNPrefixList
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, fmt.Errorf("unable to decode SDN prefix list: %w", err)
		}
		lists = append(lists, list)
	}
	return lists, nil
}

func (c *Client) GetSDNPrefixList(ctx context.Context, id string) (SDNPrefixList, error) {
	lists, err := c.SDNPrefixLists(ctx)
	if err != nil {
		return SDNPrefixList{}, err
	}
	for _, entry := range lists {
		if entry.ID == id {
			return entry, nil
		}
	}
	return SDNPrefixList{}, errNotFound
}

func (c *Client) CreateSDNPrefixList(ctx context.Context, id string, req SDNPrefixListRequest) error {
	form := url.Values{}
	form.Set("id", id)
	form.Set("lock-token", req.LockToken)
	setOptionalString(form, "digest", req.Digest)
	for _, entry := range sortedStrings(req.Entries) {
		form.Add("entries", entry)
	}
	return c.do(ctx, http.MethodPost, "/cluster/sdn/prefix-lists", nil, form, nil)
}

func (c *Client) UpdateSDNPrefixList(ctx context.Context, id string, req SDNPrefixListRequest) error {
	form := url.Values{"id": {id}, "lock-token": {req.LockToken}}
	setOptionalString(form, "digest", req.Digest)
	for _, entry := range sortedStrings(req.Entries) {
		form.Add("entries", entry)
	}
	for _, key := range sortedStrings(req.Delete) {
		form.Add("delete", key)
	}
	return c.do(ctx, http.MethodPut, "/cluster/sdn/prefix-lists/"+url.PathEscape(id), nil, form, nil)
}

func (c *Client) DeleteSDNPrefixList(ctx context.Context, id, lockToken string) error {
	form := url.Values{"lock-token": {lockToken}}
	return c.do(ctx, http.MethodDelete, "/cluster/sdn/prefix-lists/"+url.PathEscape(id), nil, form, nil)
}

func sdnRouteMapEntriesPath() string { return "/cluster/sdn/route-maps/entries" }

func (c *Client) SDNRouteMapEntries(ctx context.Context) ([]SDNRouteMapEntry, error) {
	objects, err := c.sdnSectionList(ctx, sdnRouteMapEntriesPath())
	if err != nil {
		return nil, err
	}
	var entries []SDNRouteMapEntry
	for _, raw := range objects {
		var entry SDNRouteMapEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, fmt.Errorf("unable to decode SDN route map entry: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (c *Client) GetSDNRouteMapEntry(ctx context.Context, routeMapID string, order int64) (SDNRouteMapEntry, error) {
	entries, err := c.SDNRouteMapEntries(ctx)
	if err != nil {
		return SDNRouteMapEntry{}, err
	}
	for _, entry := range entries {
		if entry.RouteMapID == routeMapID && entry.Order.Ptr() != nil && *entry.Order.Ptr() == order {
			return entry, nil
		}
	}
	return SDNRouteMapEntry{}, errNotFound
}

func (c *Client) CreateSDNRouteMapEntry(ctx context.Context, req SDNRouteMapEntryRequest) error {
	form := sdnRouteMapEntryForm(req)
	form.Set("lock-token", req.LockToken)
	return c.do(ctx, http.MethodPost, sdnRouteMapEntriesPath(), nil, form, nil)
}

func (c *Client) UpdateSDNRouteMapEntry(ctx context.Context, routeMapID string, order int64, req SDNRouteMapEntryRequest) error {
	form := sdnRouteMapEntryForm(req)
	form.Set("lock-token", req.LockToken)
	setOptionalString(form, "digest", req.Digest)
	for _, key := range sortedStrings(req.Delete) {
		form.Add("delete", key)
	}
	return c.do(ctx, http.MethodPut, fmt.Sprintf("%s/%s/%d", sdnRouteMapEntriesPath(), url.PathEscape(routeMapID), order), nil, form, nil)
}

func (c *Client) DeleteSDNRouteMapEntry(ctx context.Context, routeMapID string, order int64, lockToken string) error {
	form := url.Values{"lock-token": {lockToken}}
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("%s/%s/%d", sdnRouteMapEntriesPath(), url.PathEscape(routeMapID), order), nil, form, nil)
}

func sdnRouteMapEntryForm(req SDNRouteMapEntryRequest) url.Values {
	form := url.Values{
		"route-map-id": {req.RouteMapID},
		"order":        {fmt.Sprintf("%d", req.Order)},
		"action":       {req.Action},
	}
	for _, entry := range sortedStrings(req.Set) {
		form.Add("set", entry)
	}
	for _, entry := range sortedStrings(req.Match) {
		form.Add("match", entry)
	}
	setOptionalString(form, "exit-action", req.ExitAction)
	setOptionalString(form, "call", req.Call)
	return form
}

// RollbackSDN discards every staged SDN change by restoring the running
// snapshot into the pending configuration.
func (c *Client) RollbackSDN(ctx context.Context, lockToken string) error {
	form := url.Values{"lock-token": {lockToken}, "release-lock": {"1"}}
	return c.do(ctx, http.MethodPost, "/cluster/sdn/rollback", nil, form, nil)
}
