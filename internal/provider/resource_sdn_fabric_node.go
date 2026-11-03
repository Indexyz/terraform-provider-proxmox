// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
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

var _ resource.Resource = &SDNFabricNodeResource{}
var _ resource.ResourceWithImportState = &SDNFabricNodeResource{}
var _ resource.ResourceWithValidateConfig = &SDNFabricNodeResource{}

const sdnFabricNodeManagedFieldsKey = "sdn-fabric-node-managed-fields"

var sdnFabricNodeRoles = []string{"internal", "external"}

type SDNFabricNodeResource struct {
	client *Client
}

type sdnFabricNodeModel struct {
	ID         types.String `tfsdk:"id"`
	FabricID   types.String `tfsdk:"fabric_id"`
	NodeID     types.String `tfsdk:"node_id"`
	Protocol   types.String `tfsdk:"protocol"`
	IP         types.String `tfsdk:"ip"`
	IP6        types.String `tfsdk:"ip6"`
	Interfaces types.List   `tfsdk:"interfaces"`
	PublicKey  types.String `tfsdk:"public_key"`
	Role       types.String `tfsdk:"role"`
	Endpoint   types.String `tfsdk:"endpoint"`
	AllowedIPs types.List   `tfsdk:"allowed_ips"`
	Peers      types.List   `tfsdk:"peers"`
	State      types.String `tfsdk:"state"`
	Pending    types.Map    `tfsdk:"pending"`
}

func NewSDNFabricNodeResource() resource.Resource {
	return &SDNFabricNodeResource{}
}

func (r *SDNFabricNodeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_fabric_node"
}

func (r *SDNFabricNodeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one node of a Proxmox VE SDN fabric through `/cluster/sdn/fabrics/node/{fabric_id}`. Interface, peer and allowed-IP entries stay Proxmox property strings. Fabric nodes stage like the other SDN objects and only take effect when `proxmox_sdn_apply` commits.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `fabric_id/node_id` form.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"fabric_id": schema.StringAttribute{Required: true, MarkdownDescription: "Fabric the node belongs to. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"node_id":   schema.StringAttribute{Required: true, MarkdownDescription: "Node name inside the fabric. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"protocol":  schema.StringAttribute{Required: true, MarkdownDescription: "Fabric protocol: `openfabric`, `ospf`, `wireguard`, or `bgp`. Must match the fabric. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"ip":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "IPv4 address of this node."},
			"ip6":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "IPv6 address of this node."},
			"interfaces": schema.ListAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				MarkdownDescription: "Interface entries as Proxmox property strings, for example `name=eth1` or `name=eth1,hello_multiplier=3`.",
			},
			"public_key": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "WireGuard public key of the external node."},
			"role":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "WireGuard role: `internal` or `external`."},
			"endpoint":   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "WireGuard endpoint of the node."},
			"allowed_ips": schema.ListAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				MarkdownDescription: "CIDRs routable through this WireGuard node.",
			},
			"peers": schema.ListAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				MarkdownDescription: "WireGuard peer entries as Proxmox property strings.",
			},
			"state":   schema.StringAttribute{Computed: true, MarkdownDescription: "Proxmox staging state: `new` or `changed` while the node is not committed yet."},
			"pending": schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Properties Proxmox currently reports as staged but not committed, with `deleted` for removals."},
		},
	}
}

func (r *SDNFabricNodeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SDNFabricNodeResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config sdnFabricNodeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateSDNFabricNodeConfig(config)...)
}

func validateSDNFabricNodeConfig(config sdnFabricNodeModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.FabricID) && !sdnFabricIDPattern.MatchString(config.FabricID.ValueString()) {
		diags.AddAttributeError(path.Root("fabric_id"), "Invalid fabric identifier", "fabric_id may contain letters, digits, and inner hyphens and must be at most 8 characters long.")
	}
	if isConfiguredValue(config.NodeID) && !proxmoxNodeNamePattern.MatchString(config.NodeID.ValueString()) {
		diags.AddAttributeError(path.Root("node_id"), "Invalid node name", "node_id must be a node name.")
	}
	if isConfiguredValue(config.IP) && net.ParseIP(config.IP.ValueString()) == nil {
		diags.AddAttributeError(path.Root("ip"), "Invalid node address", "ip must be an IP address.")
	}
	if isConfiguredValue(config.IP6) && net.ParseIP(config.IP6.ValueString()) == nil {
		diags.AddAttributeError(path.Root("ip6"), "Invalid node address", "ip6 must be an IP address.")
	}
	for _, allowed := range sdnListValues(config.AllowedIPs) {
		if _, _, err := net.ParseCIDR(allowed); err != nil {
			diags.AddAttributeError(path.Root("allowed_ips"), "Invalid allowed IP", fmt.Sprintf("%q is not a CIDR network.", allowed))
		}
	}
	if !isConfiguredValue(config.Protocol) {
		return diags
	}
	protocol := config.Protocol.ValueString()
	if !slices.Contains(sdnFabricProtocols, protocol) {
		diags.AddAttributeError(path.Root("protocol"), "Invalid fabric protocol", "protocol must be openfabric, ospf, wireguard, or bgp.")
		return diags
	}
	if protocol != "wireguard" {
		for _, field := range []struct {
			name       string
			configured bool
		}{
			{"public_key", isConfiguredValue(config.PublicKey)},
			{"role", isConfiguredValue(config.Role)},
			{"endpoint", isConfiguredValue(config.Endpoint)},
			{"allowed_ips", isConfiguredValue(config.AllowedIPs)},
			{"peers", isConfiguredValue(config.Peers)},
		} {
			if field.configured {
				diags.AddAttributeError(path.Root(field.name), "Unsupported field for fabric protocol", fmt.Sprintf("%s is only valid for wireguard fabric nodes.", field.name))
			}
		}
	}
	if isConfiguredValue(config.Role) && !slices.Contains(sdnFabricNodeRoles, config.Role.ValueString()) {
		diags.AddAttributeError(path.Root("role"), "Invalid WireGuard role", "role must be internal or external.")
	}
	return diags
}

func sdnFabricNodeRequestFromModels(config, plan sdnFabricNodeModel) SDNFabricNodeRequest {
	request := SDNFabricNodeRequest{Protocol: plan.Protocol.ValueString()}
	if isConfiguredValue(config.IP) {
		request.IP = stringPointer(plan.IP)
	}
	if isConfiguredValue(config.IP6) {
		request.IP6 = stringPointer(plan.IP6)
	}
	if isConfiguredValue(config.PublicKey) {
		request.PublicKey = stringPointer(plan.PublicKey)
	}
	if isConfiguredValue(config.Role) {
		request.Role = stringPointer(plan.Role)
	}
	if isConfiguredValue(config.Endpoint) {
		request.Endpoint = stringPointer(plan.Endpoint)
	}
	if isConfiguredValue(config.Interfaces) {
		request.Interfaces = sdnListValues(plan.Interfaces)
	}
	if isConfiguredValue(config.AllowedIPs) {
		request.AllowedIPs = sdnListValues(plan.AllowedIPs)
	}
	if isConfiguredValue(config.Peers) {
		request.Peers = sdnListValues(plan.Peers)
	}
	return request
}

func sdnFabricNodeManagedFields(config sdnFabricNodeModel) []string {
	var fields []string
	for _, field := range []struct {
		name       string
		configured bool
	}{
		{"ip", isConfiguredValue(config.IP)},
		{"ip6", isConfiguredValue(config.IP6)},
		{"interfaces", isConfiguredValue(config.Interfaces)},
		{"public_key", isConfiguredValue(config.PublicKey)},
		{"role", isConfiguredValue(config.Role)},
		{"endpoint", isConfiguredValue(config.Endpoint)},
		{"allowed_ips", isConfiguredValue(config.AllowedIPs)},
		{"peers", isConfiguredValue(config.Peers)},
	} {
		if field.configured {
			fields = append(fields, field.name)
		}
	}
	slices.Sort(fields)
	return fields
}

func (r *SDNFabricNodeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config sdnFabricNodeModel
	var plan sdnFabricNodeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	createReq := sdnFabricNodeRequestFromModels(config, plan)
	fabricID := plan.FabricID.ValueString()
	nodeID := plan.NodeID.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		createReq.LockToken = token
		return r.client.CreateSDNFabricNode(ctx, fabricID, nodeID, createReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, fabricID, nodeID)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNFabricNodeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sdnFabricNodeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.FabricID.ValueString(), state.NodeID.ValueString())
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

func (r *SDNFabricNodeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config sdnFabricNodeModel
	var plan sdnFabricNodeModel
	var prior sdnFabricNodeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManaged, diags := readSDNFabricNodeManagedFields(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetSDNFabricNode(ctx, prior.FabricID.ValueString(), prior.NodeID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN Fabric Node", err.Error())
		return
	}
	updateReq := sdnFabricNodeRequestFromModels(config, plan)
	updateReq.Delete = sdnDeleteKeys(sdnFabricNodeManagedFields(config), previouslyManaged)
	fabricID := prior.FabricID.ValueString()
	nodeID := prior.NodeID.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		updateReq.LockToken = token
		if current.Digest != "" {
			updateReq.Digest = &current.Digest
		}
		return r.client.UpdateSDNFabricNode(ctx, fabricID, nodeID, updateReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, fabricID, nodeID)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNFabricNodeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sdnFabricNodeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.GetSDNFabricNode(ctx, state.FabricID.ValueString(), state.NodeID.ValueString()); err != nil {
		if errors.Is(err, errNotFound) {
			return
		}
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN Fabric Node", err.Error())
		return
	}
	fabricID := state.FabricID.ValueString()
	nodeID := state.NodeID.ValueString()
	withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		return r.client.DeleteSDNFabricNode(ctx, fabricID, nodeID, token)
	})
}

func (r *SDNFabricNodeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	fabricID, nodeID, ok := strings.Cut(req.ID, "/")
	if !ok || !sdnFabricIDPattern.MatchString(fabricID) || !proxmoxNodeNamePattern.MatchString(nodeID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected an import identifier in <fabric_id>/<node_id> form.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), fabricID+"/"+nodeID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("fabric_id"), fabricID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node_id"), nodeID)...)
}

func (r *SDNFabricNodeResource) readState(ctx context.Context, fabricID, nodeID string) (sdnFabricNodeModel, diag.Diagnostics) {
	entry, err := r.client.GetSDNFabricNode(ctx, fabricID, nodeID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return sdnFabricNodeModel{ID: types.StringNull()}, nil
		}
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox SDN Fabric Node", fmt.Sprintf("Unable to read SDN fabric node %q on fabric %q: %s", nodeID, fabricID, err))
		return sdnFabricNodeModel{}, diags
	}
	if entry.State == "deleted" {
		return sdnFabricNodeModel{ID: types.StringNull()}, nil
	}
	return sdnFabricNodeStateFromAPI(ctx, entry)
}

func sdnFabricNodeStateFromAPI(ctx context.Context, entry SDNFabricNode) (sdnFabricNodeModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	staged := sdnPendingMap(entry.Pending)
	interfaces, interfaceDiags := types.ListValueFrom(ctx, types.StringType, sdnStagedRawList(entry.Interfaces, staged["interfaces"]))
	diags.Append(interfaceDiags...)
	allowed, allowedDiags := types.ListValueFrom(ctx, types.StringType, sdnStagedRawList(entry.AllowedIPs, staged["allowed_ips"]))
	diags.Append(allowedDiags...)
	peers, peerDiags := types.ListValueFrom(ctx, types.StringType, sdnStagedRawList(entry.Peers, staged["peers"]))
	diags.Append(peerDiags...)
	state := sdnFabricNodeModel{
		ID:         types.StringValue(entry.FabricID + "/" + entry.NodeID),
		FabricID:   types.StringValue(entry.FabricID),
		NodeID:     types.StringValue(entry.NodeID),
		Protocol:   types.StringValue(entry.Protocol),
		IP:         sdnStagedString(entry.IP, staged["ip"]),
		IP6:        sdnStagedString(entry.IP6, staged["ip6"]),
		Interfaces: interfaces,
		PublicKey:  sdnStagedString(entry.PublicKey, staged["public_key"]),
		Role:       sdnStagedString(entry.Role, staged["role"]),
		Endpoint:   sdnStagedString(entry.Endpoint, staged["endpoint"]),
		AllowedIPs: allowed,
		Peers:      peers,
		State:      stringOrNull(entry.State),
		Pending:    types.MapNull(types.StringType),
	}
	if len(staged) > 0 {
		pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		diags.Append(pendingDiags...)
		state.Pending = pending
	}
	return state, diags
}

func (r *SDNFabricNodeResource) storeManagedFields(ctx context.Context, config sdnFabricNodeModel, private nodeNetworkPrivateData, diags *diag.Diagnostics) {
	managedFields, err := json.Marshal(sdnFabricNodeManagedFields(config))
	if err != nil {
		diags.AddError("Unable to Store SDN Fabric Node State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, sdnFabricNodeManagedFieldsKey, managedFields)...)
}

func readSDNFabricNodeManagedFields(ctx context.Context, private nodeNetworkPrivateData) ([]string, diag.Diagnostics) {
	value, diags := private.GetKey(ctx, sdnFabricNodeManagedFieldsKey)
	if diags.HasError() || len(value) == 0 {
		return nil, diags
	}
	var fields []string
	if err := json.Unmarshal(value, &fields); err != nil {
		diags.AddError("Unable to Read SDN Fabric Node State", fmt.Sprintf("unable to decode managed fields: %v", err))
	}
	return fields, diags
}
