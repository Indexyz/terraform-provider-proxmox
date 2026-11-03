// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SDNZoneResource{}
var _ resource.ResourceWithImportState = &SDNZoneResource{}
var _ resource.ResourceWithValidateConfig = &SDNZoneResource{}

const sdnZoneManagedFieldsKey = "sdn-zone-managed-fields"

const (
	sdnZoneTypeSimple = "simple"
	sdnZoneTypeVLAN   = "vlan"
	sdnZoneTypeQinQ   = "qinq"
	sdnZoneTypeVXLAN  = "vxlan"
	sdnZoneTypeEVPN   = "evpn"
	sdnZoneTypeFaucet = "faucet"
)

var (
	sdnZoneIDPattern       = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]*[a-zA-Z0-9]$`)
	sdnMACPattern          = regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)
	sdnControllerIDPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*[a-zA-Z0-9]$`)
	sdnZoneTypes           = []string{sdnZoneTypeSimple, sdnZoneTypeVLAN, sdnZoneTypeQinQ, sdnZoneTypeVXLAN, sdnZoneTypeEVPN, sdnZoneTypeFaucet}
)

type sdnZoneField struct {
	name       string
	types      []string
	configured func(sdnZoneModel) bool
}

type SDNZoneResource struct {
	client *Client
}

type sdnZoneModel struct {
	ID                       types.String `tfsdk:"id"`
	Zone                     types.String `tfsdk:"zone"`
	Type                     types.String `tfsdk:"type"`
	Nodes                    types.Set    `tfsdk:"nodes"`
	MTU                      types.Int64  `tfsdk:"mtu"`
	DNS                      types.String `tfsdk:"dns"`
	ReverseDNS               types.String `tfsdk:"reversedns"`
	DNSZone                  types.String `tfsdk:"dnszone"`
	IPAM                     types.String `tfsdk:"ipam"`
	DHCP                     types.String `tfsdk:"dhcp"`
	Bridge                   types.String `tfsdk:"bridge"`
	BridgeDisableMACLearning types.Bool   `tfsdk:"bridge_disable_mac_learning"`
	Tag                      types.Int64  `tfsdk:"tag"`
	VLANProtocol             types.String `tfsdk:"vlan_protocol"`
	Peers                    types.Set    `tfsdk:"peers"`
	Fabric                   types.String `tfsdk:"fabric"`
	VXLANPort                types.Int64  `tfsdk:"vxlan_port"`
	VRFVXLAN                 types.Int64  `tfsdk:"vrf_vxlan"`
	Controller               types.String `tfsdk:"controller"`
	SecondaryControllers     types.Set    `tfsdk:"secondary_controllers"`
	MAC                      types.String `tfsdk:"mac"`
	ExitNodes                types.Set    `tfsdk:"exitnodes"`
	ExitNodesLocalRouting    types.Bool   `tfsdk:"exitnodes_local_routing"`
	ExitNodesPrimary         types.String `tfsdk:"exitnodes_primary"`
	AdvertiseSubnets         types.Bool   `tfsdk:"advertise_subnets"`
	DisableARPNDSuppression  types.Bool   `tfsdk:"disable_arp_nd_suppression"`
	RTImport                 types.String `tfsdk:"rt_import"`
	DPID                     types.Int64  `tfsdk:"dp_id"`
	State                    types.String `tfsdk:"state"`
	Pending                  types.Map    `tfsdk:"pending"`
}

func NewSDNZoneResource() resource.Resource {
	return &SDNZoneResource{}
}

func (r *SDNZoneResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_zone"
}

func (r *SDNZoneResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox VE SDN zone through `/cluster/sdn/zones`. Supports the `simple`, `vlan`, `qinq`, `vxlan`, `evpn`, and `faucet` zone types with typed fields per type. Writes stay staged until `proxmox_sdn_apply` commits the whole SDN configuration; Proxmox reloads every node's network on commit. Destroy only removes the staged zone, and Proxmox refuses to delete a zone that a VNet still uses.",
		Attributes: map[string]schema.Attribute{
			"id":    schema.StringAttribute{Computed: true, MarkdownDescription: "Zone identifier (same as `zone`).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"zone":  schema.StringAttribute{Required: true, MarkdownDescription: "SDN zone identifier: starts with a letter, only letters and digits, 2 to 8 characters. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"type":  schema.StringAttribute{Required: true, MarkdownDescription: "Zone type: `simple`, `vlan`, `qinq`, `vxlan`, `evpn`, or `faucet`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"nodes": schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Cluster nodes the zone is created on. Proxmox stores them comma separated in sorted order."},
			"mtu":   schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "MTU for the bridges created for this zone's VNets."},
			"dns":   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "ID of the DNS server used by this zone."},
			"reversedns": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "ID of the reverse DNS server used by this zone.",
			},
			"dnszone": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Domain name of the zone. Requires `dns`."},
			"ipam":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "ID of the IPAM used by this zone. Proxmox refuses to change it while the zone already has subnets."},
			"dhcp":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "DHCP backend for a `simple` zone; Proxmox currently only knows `dnsmasq`."},
			"bridge":  schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Bridge whose VLANs the zone manages. Required for `vlan` and `qinq`, not allowed elsewhere."},
			"bridge_disable_mac_learning": schema.BoolAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Disable MAC learning on the `vlan`/`qinq` zone bridge.",
			},
			"tag":           schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Outer service VLAN tag of a `qinq` zone, between 2 and 4094."},
			"vlan_protocol": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "VLAN protocol of a `qinq` zone: `802.1q` (the Proxmox default) or `802.1ad`."},
			"peers":         schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Peer IP addresses of a `vxlan` zone; Proxmox stores them comma separated in sorted order."},
			"fabric":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "SDN fabric used as the underlay of a `vxlan` zone."},
			"vxlan_port":    schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "VXLAN UDP port, the Proxmox default is 4789."},
			"vrf_vxlan":     schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "VNI of the `evpn` zone VRF, between 1 and 16777215."},
			"controller":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "SDN controller of an `evpn` or `faucet` zone."},
			"secondary_controllers": schema.SetAttribute{
				Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Additional `evpn` controllers.",
			},
			"mac":                     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Anycast logical router MAC address of an `evpn` zone."},
			"exitnodes":               schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "`evpn` exit nodes."},
			"exitnodes_local_routing": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Let `evpn` exit nodes connect to EVPN guests."},
			"exitnodes_primary":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "`evpn` exit node that traffic must pass first."},
			"advertise_subnets":       schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Advertise IP prefixes (Type-5 routes) instead of MAC/IP pairs (Type-2)."},
			"disable_arp_nd_suppression": schema.BoolAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Disable ARP/ND suppression for the `evpn` zone.",
			},
			"rt_import": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Route targets imported into the `evpn` zone VRF."},
			"dp_id":     schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Faucet dataplane id of a `faucet` zone."},
			"state":     schema.StringAttribute{Computed: true, MarkdownDescription: "Proxmox staging state: `new` or `changed` while the zone is not committed yet."},
			"pending":   schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Properties Proxmox currently reports as staged but not committed, with `deleted` for removals."},
		},
	}
}

func (r *SDNZoneResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", err.Error())
		return
	}
	r.client = client
}

func (r *SDNZoneResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config sdnZoneModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateSDNZoneConfig(config)...)
}

var sdnZoneFieldTypes = []sdnZoneField{
	{name: "bridge", types: []string{sdnZoneTypeVLAN, sdnZoneTypeQinQ}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.Bridge) }},
	{name: "bridge_disable_mac_learning", types: []string{sdnZoneTypeVLAN, sdnZoneTypeQinQ}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.BridgeDisableMACLearning) }},
	{name: "dhcp", types: []string{sdnZoneTypeSimple}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.DHCP) }},
	{name: "tag", types: []string{sdnZoneTypeQinQ}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.Tag) }},
	{name: "vlan_protocol", types: []string{sdnZoneTypeQinQ}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.VLANProtocol) }},
	{name: "peers", types: []string{sdnZoneTypeVXLAN}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.Peers) }},
	{name: "fabric", types: []string{sdnZoneTypeVXLAN}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.Fabric) }},
	{name: "vxlan_port", types: []string{sdnZoneTypeVXLAN, sdnZoneTypeEVPN}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.VXLANPort) }},
	{name: "vrf_vxlan", types: []string{sdnZoneTypeEVPN}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.VRFVXLAN) }},
	{name: "controller", types: []string{sdnZoneTypeEVPN, sdnZoneTypeFaucet}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.Controller) }},
	{name: "secondary_controllers", types: []string{sdnZoneTypeEVPN}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.SecondaryControllers) }},
	{name: "mac", types: []string{sdnZoneTypeEVPN}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.MAC) }},
	{name: "exitnodes", types: []string{sdnZoneTypeEVPN}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.ExitNodes) }},
	{name: "exitnodes_local_routing", types: []string{sdnZoneTypeEVPN}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.ExitNodesLocalRouting) }},
	{name: "exitnodes_primary", types: []string{sdnZoneTypeEVPN}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.ExitNodesPrimary) }},
	{name: "advertise_subnets", types: []string{sdnZoneTypeEVPN}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.AdvertiseSubnets) }},
	{name: "disable_arp_nd_suppression", types: []string{sdnZoneTypeEVPN}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.DisableARPNDSuppression) }},
	{name: "rt_import", types: []string{sdnZoneTypeEVPN}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.RTImport) }},
	{name: "dp_id", types: []string{sdnZoneTypeFaucet}, configured: func(m sdnZoneModel) bool { return isConfiguredValue(m.DPID) }},
}

// sdnZoneRequiredFields lists the fields Proxmox marks required per zone type.
var sdnZoneRequiredFields = map[string][]string{
	sdnZoneTypeVLAN:   {"bridge"},
	sdnZoneTypeQinQ:   {"bridge", "tag"},
	sdnZoneTypeEVPN:   {"vrf_vxlan", "controller"},
	sdnZoneTypeFaucet: {"controller", "dp_id"},
}

func validateSDNZoneConfig(config sdnZoneModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.Zone) {
		zone := config.Zone.ValueString()
		if len(zone) < 2 || len(zone) > 8 || !sdnZoneIDPattern.MatchString(zone) {
			diags.AddAttributeError(path.Root("zone"), "Invalid SDN zone identifier", "zone must start with a letter, contain only letters and digits, and be 2 to 8 characters long.")
		}
	}
	if isConfiguredValue(config.MTU) && (config.MTU.ValueInt64() < 1 || config.MTU.ValueInt64() > 65535) {
		diags.AddAttributeError(path.Root("mtu"), "Invalid zone MTU", "mtu must be between 1 and 65535.")
	}
	if isConfiguredValue(config.DHCP) && config.DHCP.ValueString() != "dnsmasq" {
		diags.AddAttributeError(path.Root("dhcp"), "Invalid DHCP backend", "dhcp must be `dnsmasq`.")
	}
	if isConfiguredValue(config.DNSZone) && !isConfiguredValue(config.DNS) {
		diags.AddAttributeError(path.Root("dnszone"), "Missing DNS server", "dnszone requires `dns`.")
	}
	if isConfiguredValue(config.Tag) && (config.Tag.ValueInt64() < 2 || config.Tag.ValueInt64() > 4094) {
		diags.AddAttributeError(path.Root("tag"), "Invalid QinQ service VLAN tag", "tag must be between 2 and 4094.")
	}
	if isConfiguredValue(config.VRFVXLAN) && (config.VRFVXLAN.ValueInt64() < 1 || config.VRFVXLAN.ValueInt64() > 16777215) {
		diags.AddAttributeError(path.Root("vrf_vxlan"), "Invalid VRF VNI", "vrf_vxlan must be between 1 and 16777215.")
	}
	if isConfiguredValue(config.VXLANPort) && (config.VXLANPort.ValueInt64() < 1 || config.VXLANPort.ValueInt64() > 65535) {
		diags.AddAttributeError(path.Root("vxlan_port"), "Invalid VXLAN port", "vxlan_port must be between 1 and 65535.")
	}
	if isConfiguredValue(config.VLANProtocol) && !slices.Contains([]string{"802.1q", "802.1ad"}, config.VLANProtocol.ValueString()) {
		diags.AddAttributeError(path.Root("vlan_protocol"), "Invalid VLAN protocol", "vlan_protocol must be 802.1q or 802.1ad.")
	}
	if isConfiguredValue(config.MAC) && !sdnMACPattern.MatchString(config.MAC.ValueString()) {
		diags.AddAttributeError(path.Root("mac"), "Invalid MAC address", "mac must be a MAC address such as AA:BB:CC:DD:EE:FF.")
	}
	for _, node := range sdnZoneNodes(config.Nodes) {
		if !proxmoxNodeNamePattern.MatchString(node) {
			diags.AddAttributeError(path.Root("nodes"), "Invalid node name", fmt.Sprintf("%q is not a valid cluster node name.", node))
		}
	}
	for _, node := range sdnZoneNodes(config.ExitNodes) {
		if !proxmoxNodeNamePattern.MatchString(node) {
			diags.AddAttributeError(path.Root("exitnodes"), "Invalid exit node name", fmt.Sprintf("%q is not a valid cluster node name.", node))
		}
	}
	for _, peer := range sdnZoneSetValues(config.Peers) {
		if net.ParseIP(peer) == nil {
			diags.AddAttributeError(path.Root("peers"), "Invalid VXLAN peer", fmt.Sprintf("%q is not an IP address.", peer))
		}
	}
	for _, controller := range sdnZoneSetValues(config.SecondaryControllers) {
		if !sdnControllerIDPattern.MatchString(controller) {
			diags.AddAttributeError(path.Root("secondary_controllers"), "Invalid controller id", fmt.Sprintf("%q is not a valid controller id.", controller))
		}
	}

	if !isConfiguredValue(config.Type) {
		return diags
	}
	zoneType := config.Type.ValueString()
	if !slices.Contains(sdnZoneTypes, zoneType) {
		diags.AddAttributeError(path.Root("type"), "Invalid SDN zone type", "type must be simple, vlan, qinq, vxlan, evpn, or faucet.")
		return diags
	}
	for _, field := range sdnZoneFieldTypes {
		if !field.configured(config) || slices.Contains(field.types, zoneType) {
			continue
		}
		diags.AddAttributeError(path.Root(field.name), "Unsupported field for zone type", fmt.Sprintf("%s is only valid for %s zones.", field.name, strings.Join(field.types, ", ")))
	}
	for _, name := range sdnZoneRequiredFields[zoneType] {
		for _, field := range sdnZoneFieldTypes {
			if field.name == name && !field.configured(config) {
				diags.AddAttributeError(path.Root(name), "Missing zone field", fmt.Sprintf("%s is required for %s zones.", name, zoneType))
			}
		}
	}
	return diags
}

func sdnZoneNodes(value types.Set) []string {
	if !isConfiguredValue(value) {
		return nil
	}
	var nodes []string
	for _, element := range value.Elements() {
		if element.IsNull() || element.IsUnknown() {
			continue
		}
		nodeValue, ok := element.(types.String)
		if !ok {
			continue
		}
		nodes = append(nodes, nodeValue.ValueString())
	}
	return nodes
}

func sdnZoneRequestFromModels(ctx context.Context, config, plan sdnZoneModel) (SDNZoneRequest, diag.Diagnostics) {
	request := SDNZoneRequest{Type: plan.Type.ValueString()}
	var diags diag.Diagnostics
	if isConfiguredValue(config.Nodes) {
		nodes, nodeDiags := expandStringSet(ctx, plan.Nodes)
		diags.Append(nodeDiags...)
		request.Nodes = append([]string{}, nodes...)
	}
	setString := func(configured types.String, planned **string, value types.String) {
		if isConfiguredValue(configured) {
			*planned = stringPointer(value)
		}
	}
	setString(config.DNS, &request.DNS, plan.DNS)
	setString(config.ReverseDNS, &request.ReverseDNS, plan.ReverseDNS)
	setString(config.DNSZone, &request.DNSZone, plan.DNSZone)
	setString(config.IPAM, &request.IPAM, plan.IPAM)
	setString(config.DHCP, &request.DHCP, plan.DHCP)
	setString(config.Bridge, &request.Bridge, plan.Bridge)
	if isConfiguredValue(config.MTU) {
		request.MTU = int64PointerValue(plan.MTU)
	}
	if isConfiguredValue(config.BridgeDisableMACLearning) {
		request.BridgeDisableMACLearned = boolPointerValue(plan.BridgeDisableMACLearning)
	}
	setString(config.VLANProtocol, &request.VLANProtocol, plan.VLANProtocol)
	setString(config.Fabric, &request.Fabric, plan.Fabric)
	setString(config.Controller, &request.Controller, plan.Controller)
	setString(config.MAC, &request.MAC, plan.MAC)
	setString(config.ExitNodesPrimary, &request.ExitNodesPrimary, plan.ExitNodesPrimary)
	setString(config.RTImport, &request.RTImport, plan.RTImport)
	setInt64 := func(configured types.Int64, planned **int64, value types.Int64) {
		if isConfiguredValue(configured) {
			*planned = int64PointerValue(value)
		}
	}
	setInt64(config.Tag, &request.Tag, plan.Tag)
	setInt64(config.VXLANPort, &request.VXLANPort, plan.VXLANPort)
	setInt64(config.VRFVXLAN, &request.VRFVXLAN, plan.VRFVXLAN)
	setInt64(config.DPID, &request.DPID, plan.DPID)
	setBool := func(configured types.Bool, planned **bool, value types.Bool) {
		if isConfiguredValue(configured) {
			*planned = boolPointerValue(value)
		}
	}
	setBool(config.ExitNodesLocalRouting, &request.ExitNodesLocalRouting, plan.ExitNodesLocalRouting)
	setBool(config.AdvertiseSubnets, &request.AdvertiseSubnets, plan.AdvertiseSubnets)
	setBool(config.DisableARPNDSuppression, &request.DisableARPNDSuppression, plan.DisableARPNDSuppression)
	if isConfiguredValue(config.Peers) {
		peers, peerDiags := expandStringSet(ctx, plan.Peers)
		diags.Append(peerDiags...)
		request.Peers = append([]string{}, peers...)
	}
	if isConfiguredValue(config.ExitNodes) {
		exitNodes, exitDiags := expandStringSet(ctx, plan.ExitNodes)
		diags.Append(exitDiags...)
		request.ExitNodes = append([]string{}, exitNodes...)
	}
	if isConfiguredValue(config.SecondaryControllers) {
		controllers, controllerDiags := expandStringSet(ctx, plan.SecondaryControllers)
		diags.Append(controllerDiags...)
		request.SecondaryControllers = append([]string{}, controllers...)
	}
	return request, diags
}

func sdnZoneManagedFields(config sdnZoneModel) []string {
	var fields []string
	addString := func(key string, value types.String) {
		if isConfiguredValue(value) {
			fields = append(fields, key)
		}
	}
	addString("dns", config.DNS)
	addString("reversedns", config.ReverseDNS)
	addString("dnszone", config.DNSZone)
	addString("ipam", config.IPAM)
	addString("dhcp", config.DHCP)
	addString("bridge", config.Bridge)
	if isConfiguredValue(config.Nodes) {
		fields = append(fields, "nodes")
	}
	if isConfiguredValue(config.MTU) {
		fields = append(fields, "mtu")
	}
	if isConfiguredValue(config.BridgeDisableMACLearning) {
		fields = append(fields, "bridge-disable-mac-learning")
	}
	addString("vlan-protocol", config.VLANProtocol)
	addString("fabric", config.Fabric)
	addString("controller", config.Controller)
	addString("mac", config.MAC)
	addString("exitnodes-primary", config.ExitNodesPrimary)
	addString("rt-import", config.RTImport)
	if isConfiguredValue(config.Tag) {
		fields = append(fields, "tag")
	}
	if isConfiguredValue(config.VXLANPort) {
		fields = append(fields, "vxlan-port")
	}
	if isConfiguredValue(config.VRFVXLAN) {
		fields = append(fields, "vrf-vxlan")
	}
	if isConfiguredValue(config.DPID) {
		fields = append(fields, "dp-id")
	}
	if isConfiguredValue(config.ExitNodesLocalRouting) {
		fields = append(fields, "exitnodes-local-routing")
	}
	if isConfiguredValue(config.AdvertiseSubnets) {
		fields = append(fields, "advertise-subnets")
	}
	if isConfiguredValue(config.DisableARPNDSuppression) {
		fields = append(fields, "disable-arp-nd-suppression")
	}
	if isConfiguredValue(config.Peers) {
		fields = append(fields, "peers")
	}
	if isConfiguredValue(config.ExitNodes) {
		fields = append(fields, "exitnodes")
	}
	if isConfiguredValue(config.SecondaryControllers) {
		fields = append(fields, "secondary-controllers")
	}
	slices.Sort(fields)
	return fields
}

func sdnZoneDeleteKeys(config sdnZoneModel, previouslyManaged []string) []string {
	current := sdnZoneManagedFields(config)
	var deleted []string
	for _, key := range previouslyManaged {
		if !slices.Contains(current, key) {
			deleted = append(deleted, key)
		}
	}
	slices.Sort(deleted)
	return deleted
}

// withSDNLock runs one mutation under a fresh global SDN lock and releases the
// lock again, so staged changes survive while nothing keeps the lock.
func withSDNLock(ctx context.Context, client *Client, diags *diag.Diagnostics, operation func(lockToken string) error) bool {
	token, err := client.LockSDN(ctx)
	if err != nil {
		diags.AddError("Unable to Lock Proxmox SDN Configuration", err.Error())
		return false
	}
	operationErr := operation(token)
	if unlockErr := client.UnlockSDN(ctx, token); unlockErr != nil {
		diags.AddWarning("Unable to Release Proxmox SDN Lock", fmt.Sprintf("the SDN configuration lock may be stale, release it with `DELETE /cluster/sdn/lock?force=1`: %s", unlockErr))
	}
	if operationErr != nil {
		diags.AddError("Unable to Write Proxmox SDN Zone", operationErr.Error())
		return false
	}
	return true
}

func (r *SDNZoneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config sdnZoneModel
	var plan sdnZoneModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	createReq, diags := sdnZoneRequestFromModels(ctx, config, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	zone := plan.Zone.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		createReq.LockToken = token
		return r.client.CreateSDNZone(ctx, zone, createReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, zone)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNZoneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sdnZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.Zone.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if refreshed.ID.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *SDNZoneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config sdnZoneModel
	var plan sdnZoneModel
	var prior sdnZoneModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManaged, diags := readSDNZoneManagedFields(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	// PVE reports a vanished zone as a parameter exception, so fail fast here.
	current, err := r.client.GetSDNZone(ctx, prior.Zone.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN Zone", err.Error())
		return
	}
	updateReq, requestDiags := sdnZoneRequestFromModels(ctx, config, plan)
	resp.Diagnostics.Append(requestDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateReq.Delete = sdnZoneDeleteKeys(config, previouslyManaged)
	zone := prior.Zone.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		updateReq.LockToken = token
		if current.Digest != "" {
			updateReq.Digest = &current.Digest
		}
		return r.client.UpdateSDNZone(ctx, zone, updateReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, zone)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNZoneResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sdnZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.GetSDNZone(ctx, state.Zone.ValueString()); err != nil {
		if errors.Is(err, errNotFound) {
			return
		}
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN Zone", err.Error())
		return
	}
	zone := state.Zone.ValueString()
	withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		return r.client.DeleteSDNZone(ctx, zone, token)
	})
}

func (r *SDNZoneResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if len(req.ID) < 2 || len(req.ID) > 8 || !sdnZoneIDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected an SDN zone identifier: a letter, letters and digits only, 2 to 8 characters.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone"), req.ID)...)
}

func (r *SDNZoneResource) readState(ctx context.Context, zone string) (sdnZoneModel, diag.Diagnostics) {
	entry, err := r.client.GetSDNZone(ctx, zone)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return sdnZoneModel{ID: types.StringNull()}, nil
		}
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox SDN Zone", fmt.Sprintf("Unable to read SDN zone %q: %s", zone, err))
		return sdnZoneModel{}, diags
	}
	if entry.State == "deleted" {
		return sdnZoneModel{ID: types.StringNull()}, nil
	}
	return sdnZoneStateFromAPI(ctx, entry)
}

func sdnZoneStateFromAPI(ctx context.Context, entry SDNZone) (sdnZoneModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	// The pending view reports the running values with the staged difference
	// nested, so the staged value always wins for state mapping.
	staged := sdnPendingMap(entry.Pending)
	nodesValue, nodeDiags := stringSetValue(ctx, sdnStagedNodes(entry.Nodes, staged))
	diags.Append(nodeDiags...)
	peersValue, peerDiags := stringSetValue(ctx, sdnStagedList(entry.Peers, staged["peers"]))
	diags.Append(peerDiags...)
	exitNodesValue, exitDiags := stringSetValue(ctx, sdnStagedList(entry.ExitNodes, staged["exitnodes"]))
	diags.Append(exitDiags...)
	controllers, controllerDiags := stringSetValue(ctx, sdnStagedList(strings.Join(entry.SecondaryControllers, ","), staged["secondary-controllers"]))
	diags.Append(controllerDiags...)

	state := sdnZoneModel{
		ID:                       types.StringValue(entry.Zone),
		Zone:                     types.StringValue(entry.Zone),
		Type:                     types.StringValue(entry.Type),
		Nodes:                    nodesValue,
		MTU:                      sdnStagedInt64(entry.MTU.Ptr(), staged["mtu"]),
		DNS:                      sdnStagedString(entry.DNS, staged["dns"]),
		ReverseDNS:               sdnStagedString(entry.ReverseDNS, staged["reversedns"]),
		DNSZone:                  sdnStagedString(entry.DNSZone, staged["dnszone"]),
		IPAM:                     sdnStagedString(entry.IPAM, staged["ipam"]),
		DHCP:                     sdnStagedString(entry.DHCP, staged["dhcp"]),
		Bridge:                   sdnStagedString(entry.Bridge, staged["bridge"]),
		BridgeDisableMACLearning: sdnStagedBool(entry.BridgeDisableMACLearned.Ptr(), staged["bridge-disable-mac-learning"]),
		Tag:                      sdnStagedInt64(entry.Tag.Ptr(), staged["tag"]),
		VLANProtocol:             sdnStagedString(entry.VLANProtocol, staged["vlan-protocol"]),
		Peers:                    peersValue,
		Fabric:                   sdnStagedString(entry.Fabric, staged["fabric"]),
		VXLANPort:                sdnStagedInt64(entry.VXLANPort.Ptr(), staged["vxlan-port"]),
		VRFVXLAN:                 sdnStagedInt64(entry.VRFVXLAN.Ptr(), staged["vrf-vxlan"]),
		Controller:               sdnStagedString(entry.Controller, staged["controller"]),
		SecondaryControllers:     controllers,
		MAC:                      sdnStagedString(entry.MAC, staged["mac"]),
		ExitNodes:                exitNodesValue,
		ExitNodesLocalRouting:    sdnStagedBool(entry.ExitNodesLocalRouting.Ptr(), staged["exitnodes-local-routing"]),
		ExitNodesPrimary:         sdnStagedString(entry.ExitNodesPrimary, staged["exitnodes-primary"]),
		AdvertiseSubnets:         sdnStagedBool(entry.AdvertiseSubnets.Ptr(), staged["advertise-subnets"]),
		DisableARPNDSuppression:  sdnStagedBool(entry.DisableARPNDSuppression.Ptr(), staged["disable-arp-nd-suppression"]),
		RTImport:                 sdnStagedString(entry.RTImport, staged["rt-import"]),
		DPID:                     sdnStagedInt64(entry.DPID.Ptr(), staged["dp-id"]),
		State:                    stringOrNull(entry.State),
		Pending:                  types.MapNull(types.StringType),
	}
	if len(staged) > 0 {
		pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		diags.Append(pendingDiags...)
		state.Pending = pending
	}
	return state, diags
}

// sdnStagedList decodes a staged list value, which Proxmox reports either as a
// comma separated string, as a JSON array, or as "deleted".
func sdnStagedList(running, staged string) []string {
	switch {
	case staged == "deleted":
		return nil
	case staged == "":
		return sdnZoneNodeList(running)
	case strings.HasPrefix(staged, "["):
		var values []string
		if err := json.Unmarshal([]byte(staged), &values); err == nil {
			return values
		}
		return nil
	default:
		return sdnZoneNodeList(staged)
	}
}

func sdnStagedString(running, staged string) types.String {
	if staged == "deleted" {
		return types.StringNull()
	}
	if staged != "" {
		return types.StringValue(staged)
	}
	return stringOrNull(running)
}

func sdnStagedInt64(running *int64, staged string) types.Int64 {
	if staged == "deleted" {
		return types.Int64Null()
	}
	if staged != "" {
		if parsed, err := strconv.ParseInt(staged, 10, 64); err == nil {
			return types.Int64Value(parsed)
		}
	}
	return int64OrNull(running)
}

func sdnStagedBool(running *bool, staged string) types.Bool {
	if staged == "deleted" {
		return types.BoolNull()
	}
	if staged != "" {
		return types.BoolValue(staged == "1" || strings.EqualFold(staged, "true"))
	}
	return boolOrNull(running)
}

func sdnStagedNodes(running string, staged map[string]string) []string {
	if value, ok := staged["nodes"]; ok {
		if value == "deleted" {
			return nil
		}
		return sdnZoneNodeList(value)
	}
	return sdnZoneNodeList(running)
}

func (r *SDNZoneResource) storeManagedFields(ctx context.Context, config sdnZoneModel, private nodeNetworkPrivateData, diags *diag.Diagnostics) {
	managedFields, err := json.Marshal(sdnZoneManagedFields(config))
	if err != nil {
		diags.AddError("Unable to Store SDN Zone State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, sdnZoneManagedFieldsKey, managedFields)...)
}

func readSDNZoneManagedFields(ctx context.Context, private nodeNetworkPrivateData) ([]string, diag.Diagnostics) {
	value, diags := private.GetKey(ctx, sdnZoneManagedFieldsKey)
	if diags.HasError() || len(value) == 0 {
		return nil, diags
	}
	var fields []string
	if err := json.Unmarshal(value, &fields); err != nil {
		diags.AddError("Unable to Read SDN Zone State", fmt.Sprintf("unable to decode managed fields: %v", err))
	}
	return fields, diags
}

func sdnZoneSetValues(value types.Set) []string {
	if !isConfiguredValue(value) {
		return nil
	}
	var values []string
	for _, element := range value.Elements() {
		if element.IsNull() || element.IsUnknown() {
			continue
		}
		stringValue, ok := element.(types.String)
		if !ok {
			continue
		}
		values = append(values, stringValue.ValueString())
	}
	return values
}

// sdnStagedRawList is sdnStagedList for lists whose items may contain commas
// (property strings such as `action=permit,prefix=..`): the running slice is
// used verbatim and a staged value must be a JSON array.
func sdnStagedRawList(running []string, staged string) []string {
	switch staged {
	case "deleted":
		return nil
	case "":
		return running
	default:
		var values []string
		if err := json.Unmarshal([]byte(staged), &values); err == nil {
			return values
		}
		return []string{staged}
	}
}
