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

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SDNControllerResource{}
var _ resource.ResourceWithImportState = &SDNControllerResource{}
var _ resource.ResourceWithValidateConfig = &SDNControllerResource{}

const sdnControllerManagedFieldsKey = "sdn-controller-managed-fields"

const (
	sdnControllerTypeBGP    = "bgp"
	sdnControllerTypeEVPN   = "evpn"
	sdnControllerTypeFaucet = "faucet"
	sdnControllerTypeISIS   = "isis"
)

var (
	sdnISISDomainPattern  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	sdnISISNetPattern     = regexp.MustCompile(`^[a-fA-F0-9]{2}(\.[a-fA-F0-9]{4}){3,9}\.[a-fA-F0-9]{2}$`)
	sdnControllerTypes    = []string{sdnControllerTypeBGP, sdnControllerTypeEVPN, sdnControllerTypeFaucet, sdnControllerTypeISIS}
	sdnControllerBGPModes = []string{"auto", "external", "internal"}
)

// sdnControllerTypeFields lists the fields each controller type accepts.
var sdnControllerTypeFields = map[string][]string{
	sdnControllerTypeBGP: {
		"node", "asn", "peers", "bgp_multipath_as_path_relax", "ebgp", "ebgp_multihop", "loopback", "route_map_in", "route_map_out",
	},
	sdnControllerTypeEVPN: {
		"asn", "peers", "fabric", "route_map_in", "route_map_out", "nodes", "peer_group_name", "ebgp_multihop", "bgp_mode",
	},
	sdnControllerTypeFaucet: {"route_map_in", "route_map_out", "ebgp_multihop"},
	sdnControllerTypeISIS:   {"isis_domain", "isis_net", "isis_ifaces", "node", "loopback"},
}

var sdnControllerRequiredFields = map[string][]string{
	sdnControllerTypeBGP:  {"node", "asn", "peers"},
	sdnControllerTypeEVPN: {"asn"},
	sdnControllerTypeISIS: {"isis_domain", "isis_net", "isis_ifaces", "node"},
}

type SDNControllerResource struct {
	client *Client
}

type sdnControllerModel struct {
	ID             types.String `tfsdk:"id"`
	Controller     types.String `tfsdk:"controller"`
	Type           types.String `tfsdk:"type"`
	ASN            types.Int64  `tfsdk:"asn"`
	Node           types.String `tfsdk:"node"`
	Peers          types.Set    `tfsdk:"peers"`
	Nodes          types.Set    `tfsdk:"nodes"`
	Fabric         types.String `tfsdk:"fabric"`
	PeerGroupName  types.String `tfsdk:"peer_group_name"`
	BGPMode        types.String `tfsdk:"bgp_mode"`
	EBGP           types.Bool   `tfsdk:"ebgp"`
	EBGPMultihop   types.Int64  `tfsdk:"ebgp_multihop"`
	MultipathRelax types.Bool   `tfsdk:"bgp_multipath_as_path_relax"`
	Loopback       types.String `tfsdk:"loopback"`
	ISISDomain     types.String `tfsdk:"isis_domain"`
	ISISIfaces     types.Set    `tfsdk:"isis_ifaces"`
	ISISNet        types.String `tfsdk:"isis_net"`
	RouteMapIn     types.String `tfsdk:"route_map_in"`
	RouteMapOut    types.String `tfsdk:"route_map_out"`
	State          types.String `tfsdk:"state"`
	Pending        types.Map    `tfsdk:"pending"`
}

func NewSDNControllerResource() resource.Resource {
	return &SDNControllerResource{}
}

func (r *SDNControllerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_controller"
}

func (r *SDNControllerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox VE SDN controller through `/cluster/sdn/controllers`. Supported types are `bgp`, `evpn`, `faucet` and `isis`. Controllers stage like the other SDN objects and only take effect when `proxmox_sdn_apply` commits.",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{Computed: true, MarkdownDescription: "Controller identifier (same as `controller`).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"controller": schema.StringAttribute{Required: true, MarkdownDescription: "Controller identifier: starts with a letter, letters/digits/`-`/`_`, 2 to 64 characters. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"type":       schema.StringAttribute{Required: true, MarkdownDescription: "Controller type: `bgp`, `evpn`, `faucet`, or `isis`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"asn":        schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Local autonomous system number. Required for `bgp` and `evpn`."},
			"node":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Node the controller runs on. Required for `bgp` and `isis`."},
			"peers":      schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Peer IP addresses. Required for `bgp`."},
			"nodes":      schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Nodes the `evpn` controller is active on."},
			"fabric":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "SDN fabric used as the underlay of an `evpn` controller."},
			"peer_group_name": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "BGP peer group name of an `evpn` controller; Proxmox defaults to `VTEP`.",
			},
			"bgp_mode":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "`evpn` BGP mode: `auto` (Proxmox default), `external`, or `internal`."},
			"ebgp":          schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable eBGP (`bgp` only)."},
			"ebgp_multihop": schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Maximum eBGP hop count, 1 to 255."},
			"bgp_multipath_as_path_relax": schema.BoolAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Consider equal-length AS paths for multipath computation (`bgp` only).",
			},
			"loopback":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Loopback interface providing the router IP."},
			"isis_domain":   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "IS-IS domain name. Required for `isis`."},
			"isis_ifaces":   schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Interfaces where IS-IS is active. Required for `isis`."},
			"isis_net":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "IS-IS network entity title. Required for `isis`."},
			"route_map_in":  schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Route map applied to incoming routes."},
			"route_map_out": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Route map applied to outgoing routes."},
			"state":         schema.StringAttribute{Computed: true, MarkdownDescription: "Proxmox staging state: `new` or `changed` while the controller is not committed yet."},
			"pending":       schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Properties Proxmox currently reports as staged but not committed, with `deleted` for removals."},
		},
	}
}

func (r *SDNControllerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SDNControllerResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config sdnControllerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateSDNControllerConfig(config)...)
}

func validateSDNControllerConfig(config sdnControllerModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.Controller) {
		controller := config.Controller.ValueString()
		if len(controller) < 2 || len(controller) > 64 || !sdnControllerIDPattern.MatchString(controller) {
			diags.AddAttributeError(path.Root("controller"), "Invalid SDN controller identifier", "controller must start with a letter, contain only letters, digits, `-`, and `_`, and be 2 to 64 characters long.")
		}
	}
	if isConfiguredValue(config.ASN) && (config.ASN.ValueInt64() < 0 || config.ASN.ValueInt64() > 4294967295) {
		diags.AddAttributeError(path.Root("asn"), "Invalid ASN", "asn must be between 0 and 4294967295.")
	}
	if isConfiguredValue(config.EBGPMultihop) && (config.EBGPMultihop.ValueInt64() < 1 || config.EBGPMultihop.ValueInt64() > 255) {
		diags.AddAttributeError(path.Root("ebgp_multihop"), "Invalid eBGP hop count", "ebgp_multihop must be between 1 and 255.")
	}
	if isConfiguredValue(config.BGPMode) && !slices.Contains(sdnControllerBGPModes, config.BGPMode.ValueString()) {
		diags.AddAttributeError(path.Root("bgp_mode"), "Invalid BGP mode", "bgp_mode must be auto, external, or internal.")
	}
	if isConfiguredValue(config.ISISDomain) && !sdnISISDomainPattern.MatchString(config.ISISDomain.ValueString()) {
		diags.AddAttributeError(path.Root("isis_domain"), "Invalid IS-IS domain", "isis_domain must start with a letter or digit and contain only letters, digits, `.`, `-`, and `_`.")
	}
	if isConfiguredValue(config.ISISNet) && !sdnISISNetPattern.MatchString(config.ISISNet.ValueString()) {
		diags.AddAttributeError(path.Root("isis_net"), "Invalid IS-IS NET", "isis_net must be a network entity title such as `49.0001.1111.1111.1111.00`.")
	}
	if isConfiguredValue(config.Loopback) && !proxmoxIfacePattern.MatchString(config.Loopback.ValueString()) {
		diags.AddAttributeError(path.Root("loopback"), "Invalid loopback interface", "loopback must be an interface name.")
	}
	if isConfiguredValue(config.Node) && !proxmoxNodeNamePattern.MatchString(config.Node.ValueString()) {
		diags.AddAttributeError(path.Root("node"), "Invalid node name", "node must be a cluster node name.")
	}
	for _, node := range sdnZoneSetValues(config.Nodes) {
		if !proxmoxNodeNamePattern.MatchString(node) {
			diags.AddAttributeError(path.Root("nodes"), "Invalid node name", fmt.Sprintf("%q is not a valid cluster node name.", node))
		}
	}
	for _, peer := range sdnZoneSetValues(config.Peers) {
		if net.ParseIP(peer) == nil {
			diags.AddAttributeError(path.Root("peers"), "Invalid peer address", fmt.Sprintf("%q is not an IP address.", peer))
		}
	}
	for _, iface := range sdnZoneSetValues(config.ISISIfaces) {
		if !proxmoxIfacePattern.MatchString(iface) {
			diags.AddAttributeError(path.Root("isis_ifaces"), "Invalid IS-IS interface", fmt.Sprintf("%q is not an interface name.", iface))
		}
	}

	if !isConfiguredValue(config.Type) {
		return diags
	}
	controllerType := config.Type.ValueString()
	if !slices.Contains(sdnControllerTypes, controllerType) {
		diags.AddAttributeError(path.Root("type"), "Invalid SDN controller type", "type must be bgp, evpn, faucet, or isis.")
		return diags
	}
	present := map[string]bool{
		"asn":                         isConfiguredValue(config.ASN),
		"node":                        isConfiguredValue(config.Node),
		"peers":                       isConfiguredValue(config.Peers),
		"nodes":                       isConfiguredValue(config.Nodes),
		"fabric":                      isConfiguredValue(config.Fabric),
		"peer_group_name":             isConfiguredValue(config.PeerGroupName),
		"bgp_mode":                    isConfiguredValue(config.BGPMode),
		"ebgp":                        isConfiguredValue(config.EBGP),
		"ebgp_multihop":               isConfiguredValue(config.EBGPMultihop),
		"bgp_multipath_as_path_relax": isConfiguredValue(config.MultipathRelax),
		"loopback":                    isConfiguredValue(config.Loopback),
		"isis_domain":                 isConfiguredValue(config.ISISDomain),
		"isis_ifaces":                 isConfiguredValue(config.ISISIfaces),
		"isis_net":                    isConfiguredValue(config.ISISNet),
		"route_map_in":                isConfiguredValue(config.RouteMapIn),
		"route_map_out":               isConfiguredValue(config.RouteMapOut),
	}
	allowed := sdnControllerTypeFields[controllerType]
	for _, name := range sdnSortedKeys(present) {
		if present[name] && !slices.Contains(allowed, name) {
			diags.AddAttributeError(path.Root(name), "Unsupported field for controller type", fmt.Sprintf("%s is not valid for %s controllers.", name, controllerType))
		}
	}
	for _, name := range sdnControllerRequiredFields[controllerType] {
		if !present[name] {
			diags.AddAttributeError(path.Root(name), "Missing controller field", fmt.Sprintf("%s is required for %s controllers.", name, controllerType))
		}
	}
	return diags
}

func sdnSortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func sdnControllerRequestFromModels(ctx context.Context, config, plan sdnControllerModel) (SDNControllerRequest, diag.Diagnostics) {
	request := SDNControllerRequest{Type: plan.Type.ValueString()}
	var diags diag.Diagnostics
	setString := func(configured types.String, planned **string, value types.String) {
		if isConfiguredValue(configured) {
			*planned = stringPointer(value)
		}
	}
	setString(config.Node, &request.Node, plan.Node)
	setString(config.Fabric, &request.Fabric, plan.Fabric)
	setString(config.PeerGroupName, &request.PeerGroupName, plan.PeerGroupName)
	setString(config.BGPMode, &request.BGPMode, plan.BGPMode)
	setString(config.Loopback, &request.Loopback, plan.Loopback)
	setString(config.ISISDomain, &request.ISISDomain, plan.ISISDomain)
	setString(config.ISISNet, &request.ISISNet, plan.ISISNet)
	setString(config.RouteMapIn, &request.RouteMapIn, plan.RouteMapIn)
	setString(config.RouteMapOut, &request.RouteMapOut, plan.RouteMapOut)
	if isConfiguredValue(config.ASN) {
		request.ASN = int64PointerValue(plan.ASN)
	}
	if isConfiguredValue(config.EBGPMultihop) {
		request.EBGPMultihop = int64PointerValue(plan.EBGPMultihop)
	}
	if isConfiguredValue(config.EBGP) {
		request.EBGP = boolPointerValue(plan.EBGP)
	}
	if isConfiguredValue(config.MultipathRelax) {
		request.MultipathASRelax = boolPointerValue(plan.MultipathRelax)
	}
	if isConfiguredValue(config.Peers) {
		peers, peerDiags := expandStringSet(ctx, plan.Peers)
		diags.Append(peerDiags...)
		request.Peers = append([]string{}, peers...)
	}
	if isConfiguredValue(config.Nodes) {
		nodes, nodeDiags := expandStringSet(ctx, plan.Nodes)
		diags.Append(nodeDiags...)
		request.Nodes = append([]string{}, nodes...)
	}
	if isConfiguredValue(config.ISISIfaces) {
		ifaces, ifaceDiags := expandStringSet(ctx, plan.ISISIfaces)
		diags.Append(ifaceDiags...)
		request.ISISIfaces = append([]string{}, ifaces...)
	}
	return request, diags
}

func sdnControllerManagedFields(config sdnControllerModel) []string {
	var fields []string
	for _, field := range []struct {
		name       string
		configured bool
	}{
		{"asn", isConfiguredValue(config.ASN)},
		{"node", isConfiguredValue(config.Node)},
		{"peers", isConfiguredValue(config.Peers)},
		{"nodes", isConfiguredValue(config.Nodes)},
		{"fabric", isConfiguredValue(config.Fabric)},
		{"peer-group-name", isConfiguredValue(config.PeerGroupName)},
		{"bgp-mode", isConfiguredValue(config.BGPMode)},
		{"ebgp", isConfiguredValue(config.EBGP)},
		{"ebgp-multihop", isConfiguredValue(config.EBGPMultihop)},
		{"bgp-multipath-as-path-relax", isConfiguredValue(config.MultipathRelax)},
		{"loopback", isConfiguredValue(config.Loopback)},
		{"isis-domain", isConfiguredValue(config.ISISDomain)},
		{"isis-ifaces", isConfiguredValue(config.ISISIfaces)},
		{"isis-net", isConfiguredValue(config.ISISNet)},
		{"route-map-in", isConfiguredValue(config.RouteMapIn)},
		{"route-map-out", isConfiguredValue(config.RouteMapOut)},
	} {
		if field.configured {
			fields = append(fields, field.name)
		}
	}
	slices.Sort(fields)
	return fields
}

func (r *SDNControllerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config sdnControllerModel
	var plan sdnControllerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	createReq, diags := sdnControllerRequestFromModels(ctx, config, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	controller := plan.Controller.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		createReq.LockToken = token
		return r.client.CreateSDNController(ctx, controller, createReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, controller)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNControllerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sdnControllerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.Controller.ValueString())
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

func (r *SDNControllerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config sdnControllerModel
	var plan sdnControllerModel
	var prior sdnControllerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManaged, diags := readSDNControllerManagedFields(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetSDNController(ctx, prior.Controller.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN Controller", err.Error())
		return
	}
	updateReq, requestDiags := sdnControllerRequestFromModels(ctx, config, plan)
	resp.Diagnostics.Append(requestDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateReq.Delete = sdnDeleteKeys(sdnControllerManagedFields(config), previouslyManaged)
	controller := prior.Controller.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		updateReq.LockToken = token
		if current.Digest != "" {
			updateReq.Digest = &current.Digest
		}
		return r.client.UpdateSDNController(ctx, controller, updateReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, controller)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNControllerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sdnControllerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.GetSDNController(ctx, state.Controller.ValueString()); err != nil {
		if errors.Is(err, errNotFound) {
			return
		}
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN Controller", err.Error())
		return
	}
	controller := state.Controller.ValueString()
	withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		return r.client.DeleteSDNController(ctx, controller, token)
	})
}

func (r *SDNControllerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if len(req.ID) < 2 || len(req.ID) > 64 || !sdnControllerIDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected an SDN controller identifier: a letter, letters/digits/`-`/`_`, 2 to 64 characters.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("controller"), req.ID)...)
}

func (r *SDNControllerResource) readState(ctx context.Context, controller string) (sdnControllerModel, diag.Diagnostics) {
	entry, err := r.client.GetSDNController(ctx, controller)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return sdnControllerModel{ID: types.StringNull()}, nil
		}
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox SDN Controller", fmt.Sprintf("Unable to read SDN controller %q: %s", controller, err))
		return sdnControllerModel{}, diags
	}
	if entry.State == "deleted" {
		return sdnControllerModel{ID: types.StringNull()}, nil
	}
	return sdnControllerStateFromAPI(ctx, entry)
}

func sdnControllerStateFromAPI(ctx context.Context, entry SDNController) (sdnControllerModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	staged := sdnPendingMap(entry.Pending)
	peers, peerDiags := stringSetValue(ctx, sdnStagedList(entry.Peers, staged["peers"]))
	diags.Append(peerDiags...)
	nodes, nodeDiags := stringSetValue(ctx, sdnStagedList(entry.Nodes, staged["nodes"]))
	diags.Append(nodeDiags...)
	ifaces, ifaceDiags := stringSetValue(ctx, sdnStagedList(entry.ISISIfaces, staged["isis-ifaces"]))
	diags.Append(ifaceDiags...)
	state := sdnControllerModel{
		ID:             types.StringValue(entry.Controller),
		Controller:     types.StringValue(entry.Controller),
		Type:           types.StringValue(entry.Type),
		ASN:            sdnStagedInt64(entry.ASN.Ptr(), staged["asn"]),
		Node:           sdnStagedString(entry.Node, staged["node"]),
		Peers:          peers,
		Nodes:          nodes,
		Fabric:         sdnStagedString(entry.Fabric, staged["fabric"]),
		PeerGroupName:  sdnStagedString(entry.PeerGroupName, staged["peer-group-name"]),
		BGPMode:        sdnStagedString(entry.BGPMode, staged["bgp-mode"]),
		EBGP:           sdnStagedBool(entry.EBGP.Ptr(), staged["ebgp"]),
		EBGPMultihop:   sdnStagedInt64(entry.EBGPMultihop.Ptr(), staged["ebgp-multihop"]),
		MultipathRelax: sdnStagedBool(entry.MultipathASRelax.Ptr(), staged["bgp-multipath-as-path-relax"]),
		Loopback:       sdnStagedString(entry.Loopback, staged["loopback"]),
		ISISDomain:     sdnStagedString(entry.ISISDomain, staged["isis-domain"]),
		ISISIfaces:     ifaces,
		ISISNet:        sdnStagedString(entry.ISISNet, staged["isis-net"]),
		RouteMapIn:     sdnStagedString(entry.RouteMapIn, staged["route-map-in"]),
		RouteMapOut:    sdnStagedString(entry.RouteMapOut, staged["route-map-out"]),
		State:          stringOrNull(entry.State),
		Pending:        types.MapNull(types.StringType),
	}
	if len(staged) > 0 {
		pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		diags.Append(pendingDiags...)
		state.Pending = pending
	}
	return state, diags
}

func (r *SDNControllerResource) storeManagedFields(ctx context.Context, config sdnControllerModel, private nodeNetworkPrivateData, diags *diag.Diagnostics) {
	managedFields, err := json.Marshal(sdnControllerManagedFields(config))
	if err != nil {
		diags.AddError("Unable to Store SDN Controller State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, sdnControllerManagedFieldsKey, managedFields)...)
}

func readSDNControllerManagedFields(ctx context.Context, private nodeNetworkPrivateData) ([]string, diag.Diagnostics) {
	value, diags := private.GetKey(ctx, sdnControllerManagedFieldsKey)
	if diags.HasError() || len(value) == 0 {
		return nil, diags
	}
	var fields []string
	if err := json.Unmarshal(value, &fields); err != nil {
		diags.AddError("Unable to Read SDN Controller State", fmt.Sprintf("unable to decode managed fields: %v", err))
	}
	return fields, diags
}
