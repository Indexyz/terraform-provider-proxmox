// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &NodeNetworkInterfaceResource{}
var _ resource.ResourceWithImportState = &NodeNetworkInterfaceResource{}
var _ resource.ResourceWithValidateConfig = &NodeNetworkInterfaceResource{}

const nodeNetworkManagedFieldsKey = "node-network-managed-fields"

const (
	nodeNetworkTypeBridge     = "bridge"
	nodeNetworkTypeBond       = "bond"
	nodeNetworkTypeVLAN       = "vlan"
	nodeNetworkTypeEth        = "eth"
	nodeNetworkTypeOVSBridge  = "OVSBridge"
	nodeNetworkTypeOVSPort    = "OVSPort"
	nodeNetworkTypeOVSIntPort = "OVSIntPort"
	nodeNetworkTypeOVSBond    = "OVSBond"
)

var nodeNetworkTypes = []string{nodeNetworkTypeBridge, nodeNetworkTypeBond, nodeNetworkTypeVLAN, nodeNetworkTypeEth, nodeNetworkTypeOVSBridge, nodeNetworkTypeOVSPort, nodeNetworkTypeOVSIntPort, nodeNetworkTypeOVSBond}

type NodeNetworkInterfaceResource struct {
	client *Client
}

type nodeNetworkPrivateData interface {
	GetKey(context.Context, string) ([]byte, diag.Diagnostics)
	SetKey(context.Context, string, []byte) diag.Diagnostics
}

type nodeNetworkModel struct {
	ID                 types.String `tfsdk:"id"`
	Node               types.String `tfsdk:"node"`
	Iface              types.String `tfsdk:"iface"`
	Type               types.String `tfsdk:"type"`
	Autostart          types.Bool   `tfsdk:"autostart"`
	MTU                types.Int64  `tfsdk:"mtu"`
	Comments           types.String `tfsdk:"comments"`
	Comments6          types.String `tfsdk:"comments6"`
	Address            types.String `tfsdk:"address"`
	Netmask            types.String `tfsdk:"netmask"`
	Gateway            types.String `tfsdk:"gateway"`
	Address6           types.String `tfsdk:"address6"`
	Netmask6           types.Int64  `tfsdk:"netmask6"`
	Gateway6           types.String `tfsdk:"gateway6"`
	BridgePorts        types.Set    `tfsdk:"bridge_ports"`
	BridgeVLANAware    types.Bool   `tfsdk:"bridge_vlan_aware"`
	BridgeVIDs         types.String `tfsdk:"bridge_vids"`
	Slaves             types.Set    `tfsdk:"slaves"`
	BondMode           types.String `tfsdk:"bond_mode"`
	BondXmitHashPolicy types.String `tfsdk:"bond_xmit_hash_policy"`
	BondPrimary        types.String `tfsdk:"bond_primary"`
	VLANID             types.Int64  `tfsdk:"vlan_id"`
	VLANRawDevice      types.String `tfsdk:"vlan_raw_device"`
	OVSPorts           types.Set    `tfsdk:"ovs_ports"`
	OVSTag             types.Int64  `tfsdk:"ovs_tag"`
	OVSOptions         types.String `tfsdk:"ovs_options"`
	OVSBridge          types.String `tfsdk:"ovs_bridge"`
	OVSBonds           types.Set    `tfsdk:"ovs_bonds"`
}

func NewNodeNetworkInterfaceResource() resource.Resource {
	return &NodeNetworkInterfaceResource{}
}

func (r *NodeNetworkInterfaceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_network_interface"
}

func (r *NodeNetworkInterfaceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one interface of a Proxmox VE node (Linux and Open vSwitch) through `/nodes/{node}/network`. Writes only stage the change in `/etc/network/interfaces.new`; nothing takes effect until `proxmox_node_network_apply` for the node runs. OVS and SDN-owned interfaces are not supported. Destroy only removes the staged configuration, so a later apply is required to deactivate it.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Interface identifier in `<node>/<iface>` form.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"node":         schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node that owns the interface. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"iface":        schema.StringAttribute{Required: true, MarkdownDescription: "Interface name such as `vmbr1`, `bond1`, `bond0.100`, or `eth1`. Must not already exist; changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"type":         schema.StringAttribute{Required: true, MarkdownDescription: "Interface type: `bridge`, `bond`, `vlan`, `eth`, or an OVS type (`OVSBridge`, `OVSPort`, `OVSIntPort`, `OVSBond`). Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"autostart":    schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Bring the interface up on boot."},
			"mtu":          schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Interface MTU between 1280 and 65520."},
			"comments":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "IPv4 comment stored as `comments`."},
			"comments6":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "IPv6 comment stored as `comments6`."},
			"address":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Static IPv4 address. Must be paired with `netmask`."},
			"netmask":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "IPv4 netmask. Must be paired with `address`."},
			"gateway":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "IPv4 default gateway. Only one interface per node may define it."},
			"address6":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Static IPv6 address. Must be paired with `netmask6`."},
			"netmask6":     schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "IPv6 prefix length between 0 and 128. Must be paired with `address6`."},
			"gateway6":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "IPv6 default gateway. Only one interface per node may define it."},
			"bridge_ports": schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Bridge member interfaces. Proxmox stores them space separated in sorted order."},
			"bridge_vlan_aware": schema.BoolAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Enable VLAN filtering on the bridge.",
			},
			"bridge_vids": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Allowed VLANs such as `2 4 100-200`, only used when the bridge is VLAN aware."},
			"slaves":      schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Bond member interfaces."},
			"bond_mode":   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Bonding mode such as `active-backup` or `802.3ad`."},
			"bond_xmit_hash_policy": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Transmit hash policy for `balance-xor` and `802.3ad` bonds.",
			},
			"bond_primary":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Primary interface for an `active-backup` bond."},
			"vlan_id":         schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "VLAN tag between 1 and 4094."},
			"vlan_raw_device": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Raw interface the VLAN sits on."},
			"ovs_ports":       schema.SetAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Ports Proxmox currently records on an `OVSBridge`. Proxmox maintains the list from the port interfaces themselves, so it is computed-only."},
			"ovs_tag":         schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "VLAN tag of an OVS port, 1 to 4094."},
			"ovs_options":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Raw Open vSwitch options, for example `trunks=100,200`."},
			"ovs_bridge":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Parent `OVSBridge` of an OVS port. Required for `OVSIntPort` and `OVSBond`, which Proxmox cannot place otherwise."},
			"ovs_bonds":       schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Slave interfaces of an `OVSBond`."},
		},
	}
}

func (r *NodeNetworkInterfaceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *NodeNetworkInterfaceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeNetworkModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateNodeNetworkConfig(config)...)
}

func validateNodeNetworkConfig(config nodeNetworkModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if !config.Node.IsNull() && !config.Node.IsUnknown() && !proxmoxNodeNamePattern.MatchString(config.Node.ValueString()) {
		diags.AddAttributeError(path.Root("node"), "Invalid node name", "node must be an existing cluster node name.")
	}
	if !config.Iface.IsNull() && !config.Iface.IsUnknown() && !proxmoxIfacePattern.MatchString(config.Iface.ValueString()) {
		diags.AddAttributeError(path.Root("iface"), "Invalid interface name", "iface must start with a letter, contain only letters, numbers, and underscores, and be 2 to 20 characters long.")
	}

	configuredV4, configuredV6 := isConfiguredValue(config.Address), isConfiguredValue(config.Address6)
	if configuredV4 != isConfiguredValue(config.Netmask) {
		diags.AddAttributeError(path.Root("netmask"), "Incomplete IPv4 configuration", "address and netmask must be configured together.")
	}
	if configuredV6 != isConfiguredValue(config.Netmask6) {
		diags.AddAttributeError(path.Root("netmask6"), "Incomplete IPv6 configuration", "address6 and netmask6 must be configured together.")
	}
	if !config.MTU.IsNull() && !config.MTU.IsUnknown() && (config.MTU.ValueInt64() < 1280 || config.MTU.ValueInt64() > 65520) {
		diags.AddAttributeError(path.Root("mtu"), "Invalid MTU", "mtu must be between 1280 and 65520.")
	}
	if !config.Netmask6.IsNull() && !config.Netmask6.IsUnknown() && (config.Netmask6.ValueInt64() < 0 || config.Netmask6.ValueInt64() > 128) {
		diags.AddAttributeError(path.Root("netmask6"), "Invalid IPv6 prefix length", "netmask6 must be between 0 and 128.")
	}
	if !config.VLANID.IsNull() && !config.VLANID.IsUnknown() && (config.VLANID.ValueInt64() < 1 || config.VLANID.ValueInt64() > 4094) {
		diags.AddAttributeError(path.Root("vlan_id"), "Invalid VLAN tag", "vlan_id must be between 1 and 4094.")
	}
	for _, element := range nodeNetworkPortValues(config.BridgePorts) {
		if !proxmoxIfacePattern.MatchString(element) {
			diags.AddAttributeError(path.Root("bridge_ports"), "Invalid bridge port", fmt.Sprintf("%q is not a valid interface name.", element))
		}
	}
	for _, element := range nodeNetworkPortValues(config.Slaves) {
		if !proxmoxIfacePattern.MatchString(element) {
			diags.AddAttributeError(path.Root("slaves"), "Invalid bond slave", fmt.Sprintf("%q is not a valid interface name.", element))
		}
	}
	for _, element := range nodeNetworkPortValues(config.OVSBonds) {
		if !proxmoxIfacePattern.MatchString(element) {
			diags.AddAttributeError(path.Root("ovs_bonds"), "Invalid OVS bond slave", fmt.Sprintf("%q is not a valid interface name.", element))
		}
	}
	if isConfiguredValue(config.OVSTag) && (config.OVSTag.ValueInt64() < 1 || config.OVSTag.ValueInt64() > 4094) {
		diags.AddAttributeError(path.Root("ovs_tag"), "Invalid OVS VLAN tag", "ovs_tag must be between 1 and 4094.")
	}

	for _, field := range nodeNetworkVariantFields {
		if !field.configured(config) {
			continue
		}
		if config.Type.IsNull() || config.Type.IsUnknown() {
			continue
		}
		if !slices.Contains(field.types, config.Type.ValueString()) {
			diags.AddAttributeError(path.Root(field.name), "Unsupported field for interface type", fmt.Sprintf("%s is only valid for %s interfaces.", field.name, strings.Join(field.types, ", ")))
		}
	}

	if config.Type.IsNull() || config.Type.IsUnknown() {
		return diags
	}
	if !slices.Contains(nodeNetworkTypes, config.Type.ValueString()) {
		diags.AddAttributeError(path.Root("type"), "Invalid interface type", "type must be bridge, bond, vlan, eth, OVSBridge, OVSPort, OVSIntPort, or OVSBond.")
	}
	interfaceType := config.Type.ValueString()
	if isConfiguredValue(config.BondMode) {
		modes := nodeNetworkLinuxBondModes
		if interfaceType == nodeNetworkTypeOVSBond {
			modes = nodeNetworkOVSBondModes
		}
		if slices.Contains([]string{nodeNetworkTypeBond, nodeNetworkTypeOVSBond}, interfaceType) && !slices.Contains(modes, config.BondMode.ValueString()) {
			diags.AddAttributeError(path.Root("bond_mode"), "Invalid bond mode", fmt.Sprintf("bond_mode must be one of %s for %s interfaces.", strings.Join(modes, ", "), interfaceType))
		}
	}
	if slices.Contains([]string{nodeNetworkTypeOVSIntPort, nodeNetworkTypeOVSBond}, interfaceType) && !isConfiguredValue(config.OVSBridge) {
		diags.AddAttributeError(path.Root("ovs_bridge"), "Missing OVS bridge", fmt.Sprintf("ovs_bridge is required for %s interfaces.", interfaceType))
	}
	return diags
}

func isConfiguredValue(value interface {
	IsNull() bool
	IsUnknown() bool
}) bool {
	return !value.IsNull() && !value.IsUnknown()
}

type nodeNetworkVariantField struct {
	name       string
	types      []string
	configured func(nodeNetworkModel) bool
}

// nodeNetworkVariantFields maps every type-specific attribute to the interface
// types PVE accepts it for.
var nodeNetworkVariantFields = []nodeNetworkVariantField{
	{name: "bridge_ports", types: []string{nodeNetworkTypeBridge}, configured: func(m nodeNetworkModel) bool { return isConfiguredValue(m.BridgePorts) }},
	{name: "bridge_vlan_aware", types: []string{nodeNetworkTypeBridge}, configured: func(m nodeNetworkModel) bool { return isConfiguredValue(m.BridgeVLANAware) }},
	{name: "bridge_vids", types: []string{nodeNetworkTypeBridge}, configured: func(m nodeNetworkModel) bool { return isConfiguredValue(m.BridgeVIDs) }},
	{name: "slaves", types: []string{nodeNetworkTypeBond}, configured: func(m nodeNetworkModel) bool { return isConfiguredValue(m.Slaves) }},
	{name: "bond_mode", types: []string{nodeNetworkTypeBond, nodeNetworkTypeOVSBond}, configured: func(m nodeNetworkModel) bool { return isConfiguredValue(m.BondMode) }},
	{name: "bond_xmit_hash_policy", types: []string{nodeNetworkTypeBond}, configured: func(m nodeNetworkModel) bool { return isConfiguredValue(m.BondXmitHashPolicy) }},
	{name: "bond_primary", types: []string{nodeNetworkTypeBond}, configured: func(m nodeNetworkModel) bool { return isConfiguredValue(m.BondPrimary) }},
	{name: "vlan_id", types: []string{nodeNetworkTypeVLAN}, configured: func(m nodeNetworkModel) bool { return isConfiguredValue(m.VLANID) }},
	{name: "vlan_raw_device", types: []string{nodeNetworkTypeVLAN}, configured: func(m nodeNetworkModel) bool { return isConfiguredValue(m.VLANRawDevice) }},
	{name: "autostart", types: []string{nodeNetworkTypeBridge, nodeNetworkTypeBond, nodeNetworkTypeVLAN, nodeNetworkTypeEth, nodeNetworkTypeOVSBridge}, configured: func(m nodeNetworkModel) bool { return isConfiguredValue(m.Autostart) }},
	{name: "ovs_tag", types: nodeNetworkOVSPortTypes, configured: func(m nodeNetworkModel) bool { return isConfiguredValue(m.OVSTag) }},
	{name: "ovs_options", types: nodeNetworkOVSPortTypes, configured: func(m nodeNetworkModel) bool { return isConfiguredValue(m.OVSOptions) }},
	{name: "ovs_bridge", types: nodeNetworkOVSPortTypes, configured: func(m nodeNetworkModel) bool { return isConfiguredValue(m.OVSBridge) }},
	{name: "ovs_bonds", types: []string{nodeNetworkTypeOVSBond}, configured: func(m nodeNetworkModel) bool { return isConfiguredValue(m.OVSBonds) }},
}

func nodeNetworkPortValues(value types.Set) []string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	var ports []string
	for _, element := range value.Elements() {
		if element.IsNull() || element.IsUnknown() {
			continue
		}
		stringValue, ok := element.(types.String)
		if !ok {
			continue
		}
		ports = append(ports, stringValue.ValueString())
	}
	return ports
}

func nodeNetworkRequestFromModels(ctx context.Context, config, plan nodeNetworkModel) (NodeNetworkInterfaceRequest, diag.Diagnostics) {
	request := NodeNetworkInterfaceRequest{Type: plan.Type.ValueString()}
	var diags diag.Diagnostics
	if isConfiguredValue(config.BridgePorts) {
		ports, portDiags := expandStringSet(ctx, plan.BridgePorts)
		diags.Append(portDiags...)
		request.BridgePorts = append([]string{}, ports...)
	}
	if isConfiguredValue(config.Slaves) {
		slaves, slaveDiags := expandStringSet(ctx, plan.Slaves)
		diags.Append(slaveDiags...)
		request.Slaves = append([]string{}, slaves...)
	}
	setString := func(configured types.String, planned **string, value types.String) {
		if isConfiguredValue(configured) {
			*planned = stringPointer(value)
		}
	}
	setBool := func(configured types.Bool, planned **bool, value types.Bool) {
		if isConfiguredValue(configured) {
			*planned = boolPointerValue(value)
		}
	}
	setInt64 := func(configured types.Int64, planned **int64, value types.Int64) {
		if isConfiguredValue(configured) {
			*planned = int64PointerValue(value)
		}
	}
	setBool(config.Autostart, &request.Autostart, plan.Autostart)
	setInt64(config.MTU, &request.MTU, plan.MTU)
	setString(config.Comments, &request.Comments, plan.Comments)
	setString(config.Comments6, &request.Comments6, plan.Comments6)
	setString(config.Address, &request.Address, plan.Address)
	setString(config.Netmask, &request.Netmask, plan.Netmask)
	setString(config.Gateway, &request.Gateway, plan.Gateway)
	setString(config.Address6, &request.Address6, plan.Address6)
	setInt64(config.Netmask6, &request.Netmask6, plan.Netmask6)
	setString(config.Gateway6, &request.Gateway6, plan.Gateway6)
	setBool(config.BridgeVLANAware, &request.BridgeVLANAware, plan.BridgeVLANAware)
	setString(config.BridgeVIDs, &request.BridgeVIDs, plan.BridgeVIDs)
	setString(config.BondMode, &request.BondMode, plan.BondMode)
	setString(config.BondXmitHashPolicy, &request.BondXmitHashPolicy, plan.BondXmitHashPolicy)
	setString(config.BondPrimary, &request.BondPrimary, plan.BondPrimary)
	setInt64(config.VLANID, &request.VLANID, plan.VLANID)
	setString(config.VLANRawDevice, &request.VLANRawDevice, plan.VLANRawDevice)
	setString(config.OVSOptions, &request.OVSOptions, plan.OVSOptions)
	setString(config.OVSBridge, &request.OVSBridge, plan.OVSBridge)
	if isConfiguredValue(config.OVSTag) {
		request.OVSTag = int64PointerValue(plan.OVSTag)
	}
	if isConfiguredValue(config.OVSBonds) {
		bonds, bondDiags := expandStringSet(ctx, plan.OVSBonds)
		diags.Append(bondDiags...)
		request.OVSBonds = append([]string{}, bonds...)
	}
	return request, diags
}

func nodeNetworkManagedFields(config nodeNetworkModel) []string {
	var fields []string
	addString := func(key string, value types.String) {
		if isConfiguredValue(value) {
			fields = append(fields, key)
		}
	}
	addBool := func(key string, value types.Bool) {
		if isConfiguredValue(value) {
			fields = append(fields, key)
		}
	}
	addInt64 := func(key string, value types.Int64) {
		if isConfiguredValue(value) {
			fields = append(fields, key)
		}
	}
	addBool("autostart", config.Autostart)
	addInt64("mtu", config.MTU)
	addString("comments", config.Comments)
	addString("comments6", config.Comments6)
	addString("address", config.Address)
	addString("netmask", config.Netmask)
	addString("gateway", config.Gateway)
	addString("address6", config.Address6)
	addInt64("netmask6", config.Netmask6)
	addString("gateway6", config.Gateway6)
	if isConfiguredValue(config.BridgePorts) {
		fields = append(fields, "bridge_ports")
	}
	addBool("bridge_vlan_aware", config.BridgeVLANAware)
	addString("bridge_vids", config.BridgeVIDs)
	if isConfiguredValue(config.Slaves) {
		fields = append(fields, "slaves")
	}
	addString("bond_mode", config.BondMode)
	addString("bond_xmit_hash_policy", config.BondXmitHashPolicy)
	addString("bond-primary", config.BondPrimary)
	addInt64("vlan-id", config.VLANID)
	addString("vlan-raw-device", config.VLANRawDevice)
	if isConfiguredValue(config.OVSTag) {
		fields = append(fields, "ovs_tag")
	}
	addString("ovs_options", config.OVSOptions)
	addString("ovs_bridge", config.OVSBridge)
	if isConfiguredValue(config.OVSBonds) {
		fields = append(fields, "ovs_bonds")
	}
	slices.Sort(fields)
	return fields
}

func nodeNetworkDeleteKeys(config nodeNetworkModel, previouslyManaged []string) []string {
	current := nodeNetworkManagedFields(config)
	var deleted []string
	for _, key := range previouslyManaged {
		if !slices.Contains(current, key) {
			deleted = append(deleted, key)
		}
	}
	slices.Sort(deleted)
	return deleted
}

func (r *NodeNetworkInterfaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config nodeNetworkModel
	var plan nodeNetworkModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	createReq, diags := nodeNetworkRequestFromModels(ctx, config, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.CreateNodeNetworkInterface(ctx, plan.Node.ValueString(), plan.Iface.ValueString(), createReq); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Node Network Interface", err.Error())
		return
	}
	state, stateDiags := r.readState(ctx, plan.Node.ValueString(), plan.Iface.ValueString())
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *NodeNetworkInterfaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nodeNetworkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.Node.ValueString(), state.Iface.ValueString())
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

func (r *NodeNetworkInterfaceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config nodeNetworkModel
	var plan nodeNetworkModel
	var prior nodeNetworkModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManaged, diags := readNodeNetworkManagedFields(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Fail fast on a vanished interface: PVE's update does not report absence through a 404.
	if _, err := r.client.GetNodeNetworkInterface(ctx, prior.Node.ValueString(), prior.Iface.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Read Current Proxmox Node Network Interface", err.Error())
		return
	}
	updateReq, requestDiags := nodeNetworkRequestFromModels(ctx, config, plan)
	resp.Diagnostics.Append(requestDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateReq.Delete = nodeNetworkDeleteKeys(config, previouslyManaged)
	if err := r.client.UpdateNodeNetworkInterface(ctx, prior.Node.ValueString(), prior.Iface.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Node Network Interface", err.Error())
		return
	}
	state, stateDiags := r.readState(ctx, prior.Node.ValueString(), prior.Iface.ValueString())
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *NodeNetworkInterfaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nodeNetworkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteNodeNetworkInterface(ctx, state.Node.ValueString(), state.Iface.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Node Network Interface", err.Error())
	}
}

func (r *NodeNetworkInterfaceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, iface, ok := strings.Cut(req.ID, "/")
	if !ok || !proxmoxNodeNamePattern.MatchString(node) || !proxmoxIfacePattern.MatchString(iface) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected an import identifier in <node>/<iface> form, for example node1/vmbr1.")
		return
	}
	id := node + "/" + iface
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("iface"), iface)...)
}

func (r *NodeNetworkInterfaceResource) readState(ctx context.Context, node, iface string) (nodeNetworkModel, diag.Diagnostics) {
	entry, err := r.client.GetNodeNetworkInterface(ctx, node, iface)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nodeNetworkModel{ID: types.StringNull()}, nil
		}
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox Node Network Interface", fmt.Sprintf("Unable to read interface %q on node %q: %s", iface, node, err))
		return nodeNetworkModel{}, diags
	}
	return nodeNetworkStateFromAPI(ctx, node, entry)
}

func nodeNetworkStateFromAPI(ctx context.Context, node string, entry NodeNetworkInterface) (nodeNetworkModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	ovsPorts, ovsPortDiags := stringSetValue(ctx, strings.Fields(entry.OVSPorts))
	diags.Append(ovsPortDiags...)
	ovsBonds, ovsBondDiags := stringSetValue(ctx, strings.Fields(entry.OVSBonds))
	diags.Append(ovsBondDiags...)
	// Proxmox writes an empty port list as `none`, which is not an interface name.
	bridgePorts, bridgeDiags := stringSetValue(ctx, nodeNetworkPortFields(entry.BridgePorts))
	diags.Append(bridgeDiags...)
	slaves, slaveDiags := stringSetValue(ctx, nodeNetworkPortFields(entry.Slaves))
	diags.Append(slaveDiags...)
	return nodeNetworkModel{
		ID:                 types.StringValue(node + "/" + entry.Iface),
		Node:               types.StringValue(node),
		Iface:              types.StringValue(entry.Iface),
		Type:               types.StringValue(entry.Type),
		Autostart:          nodesBoolOrNull(entry.Autostart),
		MTU:                int64OrNull(entry.MTU.Ptr()),
		Comments:           stringOrNull(entry.Comments),
		Comments6:          stringOrNull(entry.Comments6),
		Address:            stringOrNull(entry.Address),
		Netmask:            stringOrNull(entry.Netmask),
		Gateway:            stringOrNull(entry.Gateway),
		Address6:           stringOrNull(entry.Address6),
		Netmask6:           int64OrNull(entry.Netmask6.Ptr()),
		Gateway6:           stringOrNull(entry.Gateway6),
		BridgePorts:        bridgePorts,
		BridgeVLANAware:    nodesBoolOrNull(entry.BridgeVLANAware),
		BridgeVIDs:         stringOrNull(entry.BridgeVIDs),
		Slaves:             slaves,
		BondMode:           stringOrNull(entry.BondMode),
		BondXmitHashPolicy: stringOrNull(entry.BondXmitHashPolicy),
		BondPrimary:        stringOrNull(entry.BondPrimary),
		VLANID:             int64OrNull(entry.VLANID.Ptr()),
		VLANRawDevice:      stringOrNull(entry.VLANRawDevice),
		OVSPorts:           ovsPorts,
		OVSTag:             int64OrNull(entry.OVSTag.Ptr()),
		OVSOptions:         stringOrNull(entry.OVSOptions),
		OVSBridge:          stringOrNull(entry.OVSBridge),
		OVSBonds:           ovsBonds,
	}, diags
}

func nodesBoolOrNull(value proxmoxOptionalBool) types.Bool {
	if value.Ptr() == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*value.Ptr())
}

func nodeNetworkPortFields(value string) []string {
	if strings.EqualFold(strings.TrimSpace(value), "none") {
		return nil
	}
	return strings.Fields(value)
}

func (r *NodeNetworkInterfaceResource) storeManagedFields(ctx context.Context, config nodeNetworkModel, private nodeNetworkPrivateData, diags *diag.Diagnostics) {
	managedFields, err := json.Marshal(nodeNetworkManagedFields(config))
	if err != nil {
		diags.AddError("Unable to Store Node Network Interface State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, nodeNetworkManagedFieldsKey, managedFields)...)
}

func readNodeNetworkManagedFields(ctx context.Context, private nodeNetworkPrivateData) ([]string, diag.Diagnostics) {
	value, diags := private.GetKey(ctx, nodeNetworkManagedFieldsKey)
	if diags.HasError() || len(value) == 0 {
		return nil, diags
	}
	var fields []string
	if err := json.Unmarshal(value, &fields); err != nil {
		diags.AddError("Unable to Read Node Network Interface State", fmt.Sprintf("unable to decode managed fields: %v", err))
	}
	return fields, diags
}

var (
	nodeNetworkOVSPortTypes   = []string{nodeNetworkTypeOVSPort, nodeNetworkTypeOVSIntPort, nodeNetworkTypeOVSBond}
	nodeNetworkLinuxBondModes = []string{
		"balance-rr", "active-backup", "balance-xor", "broadcast",
		"802.3ad", "balance-tlb", "balance-alb",
	}
	nodeNetworkOVSBondModes = []string{"active-backup", "balance-slb", "lacp-balance-slb", "lacp-balance-tcp"}
)
