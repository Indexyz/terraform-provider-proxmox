// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
)

var (
	sdnZoneIDPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]*[a-zA-Z0-9]$`)
	// sdnZoneCommonManagedFields are the optional keys this resource owns and
	// may delete again.
)

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
		MarkdownDescription: "Manages a Proxmox VE SDN zone through `/cluster/sdn/zones`. Only the `simple` and `vlan` zone types are supported. Writes stay staged until `proxmox_sdn_apply` commits the whole SDN configuration; Proxmox reloads every node's network on commit. Destroy only removes the staged zone, and Proxmox refuses to delete a zone that a VNet still uses.",
		Attributes: map[string]schema.Attribute{
			"id":    schema.StringAttribute{Computed: true, MarkdownDescription: "Zone identifier (same as `zone`).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"zone":  schema.StringAttribute{Required: true, MarkdownDescription: "SDN zone identifier: starts with a letter, only letters and digits, 2 to 8 characters. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"type":  schema.StringAttribute{Required: true, MarkdownDescription: "Zone type: `simple` or `vlan`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"nodes": schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Cluster nodes the zone is created on. Proxmox stores them comma separated in sorted order."},
			"mtu":   schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "MTU for the bridges created for this zone's VNets."},
			"dns":   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "ID of the DNS server used by this zone."},
			"reversedns": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "ID of the reverse DNS server used by this zone.",
			},
			"dnszone": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Domain name of the zone. Requires `dns`."},
			"ipam":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "ID of the IPAM used by this zone. Proxmox refuses to change it while the zone already has subnets."},
			"dhcp":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "DHCP backend for this zone; Proxmox currently only knows `dnsmasq`."},
			"bridge":  schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Bridge whose VLANs the zone manages. Required for `vlan`, not allowed for `simple`."},
			"bridge_disable_mac_learning": schema.BoolAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Disable MAC learning on the `vlan` zone bridge.",
			},
			"state":   schema.StringAttribute{Computed: true, MarkdownDescription: "Proxmox staging state: `new` or `changed` while the zone is not committed yet."},
			"pending": schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Properties Proxmox currently reports as staged but not committed, with `deleted` for removals."},
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
	for _, node := range sdnZoneNodes(config.Nodes) {
		if !proxmoxNodeNamePattern.MatchString(node) {
			diags.AddAttributeError(path.Root("nodes"), "Invalid node name", fmt.Sprintf("%q is not a valid cluster node name.", node))
		}
	}

	if !isConfiguredValue(config.Type) {
		return diags
	}
	switch config.Type.ValueString() {
	case sdnZoneTypeSimple:
		if isConfiguredValue(config.Bridge) {
			diags.AddAttributeError(path.Root("bridge"), "Unexpected zone bridge", "bridge is only valid for a vlan zone.")
		}
		if isConfiguredValue(config.BridgeDisableMACLearning) {
			diags.AddAttributeError(path.Root("bridge_disable_mac_learning"), "Unexpected MAC learning flag", "bridge_disable_mac_learning is only valid for a vlan zone.")
		}
	case sdnZoneTypeVLAN:
		if !isConfiguredValue(config.Bridge) {
			diags.AddAttributeError(path.Root("bridge"), "Missing zone bridge", "bridge is required for a vlan zone.")
		}
	default:
		diags.AddAttributeError(path.Root("type"), "Invalid SDN zone type", "type must be simple or vlan.")
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
