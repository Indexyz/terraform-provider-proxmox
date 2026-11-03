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

var _ resource.Resource = &SDNFabricResource{}
var _ resource.ResourceWithImportState = &SDNFabricResource{}
var _ resource.ResourceWithValidateConfig = &SDNFabricResource{}

const sdnFabricManagedFieldsKey = "sdn-fabric-managed-fields"

var (
	sdnFabricIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,6}[a-zA-Z0-9]?$`)
	sdnFabricProtocols = []string{"openfabric", "ospf", "wireguard", "bgp"}
)

// sdnFabricTypeFields lists the optional fields each fabric protocol accepts.
var sdnFabricTypeFields = map[string][]string{
	"openfabric": {"ip_prefix", "ip6_prefix", "hello_interval", "csnp_interval", "route_filter"},
	"ospf":       {"ip_prefix", "ip6_prefix", "area", "route_filter", "redistribute"},
	"wireguard":  {"ip_prefix", "ip6_prefix", "persistent_keepalive"},
	"bgp":        {"ip_prefix", "ip6_prefix", "route_filter", "route_map_in", "route_map_out", "redistribute"},
}

type SDNFabricResource struct {
	client *Client
}

type sdnFabricModel struct {
	ID                  types.String  `tfsdk:"id"`
	FabricID            types.String  `tfsdk:"fabric_id"`
	Protocol            types.String  `tfsdk:"protocol"`
	IPPrefix            types.String  `tfsdk:"ip_prefix"`
	IP6Prefix           types.String  `tfsdk:"ip6_prefix"`
	HelloInterval       types.Float64 `tfsdk:"hello_interval"`
	CSNPInterval        types.Float64 `tfsdk:"csnp_interval"`
	Area                types.String  `tfsdk:"area"`
	RouteFilter         types.String  `tfsdk:"route_filter"`
	PersistentKeepalive types.Int64   `tfsdk:"persistent_keepalive"`
	RouteMapIn          types.String  `tfsdk:"route_map_in"`
	RouteMapOut         types.String  `tfsdk:"route_map_out"`
	Redistribute        types.List    `tfsdk:"redistribute"`
	State               types.String  `tfsdk:"state"`
	Pending             types.Map     `tfsdk:"pending"`
}

func NewSDNFabricResource() resource.Resource {
	return &SDNFabricResource{}
}

func (r *SDNFabricResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_fabric"
}

func (r *SDNFabricResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox VE SDN fabric through `/cluster/sdn/fabrics/fabric`. Fabrics are the routing underlay for `vxlan` and `evpn` objects and support the `openfabric`, `ospf`, `wireguard` and `bgp` protocols. Fabric lists such as `redistribute` stay Proxmox property strings. Fabrics stage like the other SDN objects and only take effect when `proxmox_sdn_apply` commits.",
		Attributes: map[string]schema.Attribute{
			"id":                   schema.StringAttribute{Computed: true, MarkdownDescription: "Fabric identifier (same as `fabric_id`).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"fabric_id":            schema.StringAttribute{Required: true, MarkdownDescription: "Fabric identifier: letters, digits, and inner hyphens, at most 8 characters. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"protocol":             schema.StringAttribute{Required: true, MarkdownDescription: "Fabric protocol: `openfabric`, `ospf`, `wireguard`, or `bgp`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"ip_prefix":            schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "IPv4 prefix handed out for node IPs."},
			"ip6_prefix":           schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "IPv6 prefix handed out for node IPs."},
			"hello_interval":       schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "OpenFabric hello interval in seconds, 1 to 600."},
			"csnp_interval":        schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "OpenFabric CSNP interval in seconds, 1 to 600."},
			"area":                 schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "OSPF area, either an IPv4 address or a 32-bit number."},
			"route_filter":         schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Prefix list id used to filter routes installed into the kernel table."},
			"persistent_keepalive": schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "WireGuard persistent keepalive interval in seconds, 0 to 65535."},
			"route_map_in":         schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Route map applied to routes imported from the fabric (`bgp`)."},
			"route_map_out":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Route map applied to routes exported into the fabric (`bgp`)."},
			"redistribute": schema.ListAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				MarkdownDescription: "Redistribution entries as Proxmox property strings, for example `source=connected` or `source=static,route-map=rm1`.",
			},
			"state":   schema.StringAttribute{Computed: true, MarkdownDescription: "Proxmox staging state: `new` or `changed` while the fabric is not committed yet."},
			"pending": schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Properties Proxmox currently reports as staged but not committed, with `deleted` for removals."},
		},
	}
}

func (r *SDNFabricResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SDNFabricResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config sdnFabricModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateSDNFabricConfig(config)...)
}

func validateSDNFabricConfig(config sdnFabricModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.FabricID) && !sdnFabricIDPattern.MatchString(config.FabricID.ValueString()) {
		diags.AddAttributeError(path.Root("fabric_id"), "Invalid fabric identifier", "fabric_id may contain letters, digits, and inner hyphens and must be at most 8 characters long.")
	}
	for _, prefix := range []struct {
		name  string
		value types.String
	}{
		{"ip_prefix", config.IPPrefix},
		{"ip6_prefix", config.IP6Prefix},
	} {
		if isConfiguredValue(prefix.value) {
			if _, _, err := net.ParseCIDR(prefix.value.ValueString()); err != nil {
				diags.AddAttributeError(path.Root(prefix.name), "Invalid prefix", fmt.Sprintf("%s must be a CIDR network.", prefix.name))
			}
		}
	}
	if isConfiguredValue(config.HelloInterval) && (config.HelloInterval.ValueFloat64() < 1 || config.HelloInterval.ValueFloat64() > 600) {
		diags.AddAttributeError(path.Root("hello_interval"), "Invalid hello interval", "hello_interval must be between 1 and 600.")
	}
	if isConfiguredValue(config.CSNPInterval) && (config.CSNPInterval.ValueFloat64() < 1 || config.CSNPInterval.ValueFloat64() > 600) {
		diags.AddAttributeError(path.Root("csnp_interval"), "Invalid CSNP interval", "csnp_interval must be between 1 and 600.")
	}
	if isConfiguredValue(config.PersistentKeepalive) && (config.PersistentKeepalive.ValueInt64() < 0 || config.PersistentKeepalive.ValueInt64() > 65535) {
		diags.AddAttributeError(path.Root("persistent_keepalive"), "Invalid keepalive", "persistent_keepalive must be between 0 and 65535.")
	}
	if !isConfiguredValue(config.Protocol) {
		return diags
	}
	protocol := config.Protocol.ValueString()
	if !slices.Contains(sdnFabricProtocols, protocol) {
		diags.AddAttributeError(path.Root("protocol"), "Invalid fabric protocol", "protocol must be openfabric, ospf, wireguard, or bgp.")
		return diags
	}
	allowed := sdnFabricTypeFields[protocol]
	for _, field := range []struct {
		name       string
		configured bool
	}{
		{"ip_prefix", isConfiguredValue(config.IPPrefix)},
		{"ip6_prefix", isConfiguredValue(config.IP6Prefix)},
		{"hello_interval", isConfiguredValue(config.HelloInterval)},
		{"csnp_interval", isConfiguredValue(config.CSNPInterval)},
		{"area", isConfiguredValue(config.Area)},
		{"route_filter", isConfiguredValue(config.RouteFilter)},
		{"persistent_keepalive", isConfiguredValue(config.PersistentKeepalive)},
		{"route_map_in", isConfiguredValue(config.RouteMapIn)},
		{"route_map_out", isConfiguredValue(config.RouteMapOut)},
		{"redistribute", isConfiguredValue(config.Redistribute)},
	} {
		if field.configured && !slices.Contains(allowed, field.name) {
			diags.AddAttributeError(path.Root(field.name), "Unsupported field for fabric protocol", fmt.Sprintf("%s is not valid for %s fabrics.", field.name, protocol))
		}
	}
	return diags
}

func sdnFabricRequestFromModels(config, plan sdnFabricModel) SDNFabricRequest {
	request := SDNFabricRequest{Protocol: plan.Protocol.ValueString()}
	setString := func(configured types.String, planned **string, value types.String) {
		if isConfiguredValue(configured) {
			*planned = stringPointer(value)
		}
	}
	setString(config.IPPrefix, &request.IPPrefix, plan.IPPrefix)
	setString(config.IP6Prefix, &request.IP6Prefix, plan.IP6Prefix)
	setString(config.Area, &request.Area, plan.Area)
	setString(config.RouteFilter, &request.RouteFilter, plan.RouteFilter)
	setString(config.RouteMapIn, &request.RouteMapIn, plan.RouteMapIn)
	setString(config.RouteMapOut, &request.RouteMapOut, plan.RouteMapOut)
	if isConfiguredValue(config.HelloInterval) {
		request.HelloInterval = float64PointerValue(plan.HelloInterval)
	}
	if isConfiguredValue(config.CSNPInterval) {
		request.CSNPInterval = float64PointerValue(plan.CSNPInterval)
	}
	if isConfiguredValue(config.PersistentKeepalive) {
		request.PersistentKeepalive = int64PointerValue(plan.PersistentKeepalive)
	}
	if isConfiguredValue(config.Redistribute) {
		request.Redistribute = sdnListValues(plan.Redistribute)
	}
	return request
}

func sdnListValues(value types.List) []string {
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

func sdnFabricManagedFields(config sdnFabricModel) []string {
	var fields []string
	for _, field := range []struct {
		name       string
		configured bool
	}{
		{"ip_prefix", isConfiguredValue(config.IPPrefix)},
		{"ip6_prefix", isConfiguredValue(config.IP6Prefix)},
		{"hello_interval", isConfiguredValue(config.HelloInterval)},
		{"csnp_interval", isConfiguredValue(config.CSNPInterval)},
		{"area", isConfiguredValue(config.Area)},
		{"route_filter", isConfiguredValue(config.RouteFilter)},
		{"persistent_keepalive", isConfiguredValue(config.PersistentKeepalive)},
		{"route_map_in", isConfiguredValue(config.RouteMapIn)},
		{"route_map_out", isConfiguredValue(config.RouteMapOut)},
		{"redistribute", isConfiguredValue(config.Redistribute)},
	} {
		if field.configured {
			fields = append(fields, field.name)
		}
	}
	slices.Sort(fields)
	return fields
}

func (r *SDNFabricResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config sdnFabricModel
	var plan sdnFabricModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	createReq := sdnFabricRequestFromModels(config, plan)
	id := plan.FabricID.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		createReq.LockToken = token
		return r.client.CreateSDNFabric(ctx, id, createReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, id)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNFabricResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sdnFabricModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.FabricID.ValueString())
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

func (r *SDNFabricResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config sdnFabricModel
	var plan sdnFabricModel
	var prior sdnFabricModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManaged, diags := readSDNFabricManagedFields(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetSDNFabric(ctx, prior.FabricID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN Fabric", err.Error())
		return
	}
	updateReq := sdnFabricRequestFromModels(config, plan)
	updateReq.Delete = sdnDeleteKeys(sdnFabricManagedFields(config), previouslyManaged)
	id := prior.FabricID.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		updateReq.LockToken = token
		if current.Digest != "" {
			updateReq.Digest = &current.Digest
		}
		return r.client.UpdateSDNFabric(ctx, id, updateReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, id)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNFabricResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sdnFabricModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.GetSDNFabric(ctx, state.FabricID.ValueString()); err != nil {
		if errors.Is(err, errNotFound) {
			return
		}
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN Fabric", err.Error())
		return
	}
	id := state.FabricID.ValueString()
	withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		return r.client.DeleteSDNFabric(ctx, id, token)
	})
}

func (r *SDNFabricResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !sdnFabricIDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected an SDN fabric identifier.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("fabric_id"), req.ID)...)
}

func (r *SDNFabricResource) readState(ctx context.Context, id string) (sdnFabricModel, diag.Diagnostics) {
	entry, err := r.client.GetSDNFabric(ctx, id)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return sdnFabricModel{ID: types.StringNull()}, nil
		}
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox SDN Fabric", fmt.Sprintf("Unable to read SDN fabric %q: %s", id, err))
		return sdnFabricModel{}, diags
	}
	if entry.State == "deleted" {
		return sdnFabricModel{ID: types.StringNull()}, nil
	}
	return sdnFabricStateFromAPI(ctx, entry)
}

func sdnFabricStateFromAPI(ctx context.Context, entry SDNFabric) (sdnFabricModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	staged := sdnPendingMap(entry.Pending)
	redistribute, listDiags := types.ListValueFrom(ctx, types.StringType, sdnStagedRawList(entry.Redistribute, staged["redistribute"]))
	diags.Append(listDiags...)
	state := sdnFabricModel{
		ID:                  types.StringValue(entry.ID),
		FabricID:            types.StringValue(entry.ID),
		Protocol:            types.StringValue(entry.Protocol),
		IPPrefix:            sdnStagedString(entry.IPPrefix, staged["ip_prefix"]),
		IP6Prefix:           sdnStagedString(entry.IP6Prefix, staged["ip6_prefix"]),
		HelloInterval:       sdnStagedFloat64(entry.HelloInterval, staged["hello_interval"]),
		CSNPInterval:        sdnStagedFloat64(entry.CSNPInterval, staged["csnp_interval"]),
		Area:                sdnStagedString(entry.Area, staged["area"]),
		RouteFilter:         sdnStagedString(entry.RouteFilter, staged["route_filter"]),
		PersistentKeepalive: sdnStagedInt64(entry.PersistentKeepalive.Ptr(), staged["persistent_keepalive"]),
		RouteMapIn:          sdnStagedString(entry.RouteMapIn, staged["route_map_in"]),
		RouteMapOut:         sdnStagedString(entry.RouteMapOut, staged["route_map_out"]),
		Redistribute:        redistribute,
		State:               stringOrNull(entry.State),
		Pending:             types.MapNull(types.StringType),
	}
	if len(staged) > 0 {
		pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		diags.Append(pendingDiags...)
		state.Pending = pending
	}
	return state, diags
}

func sdnStagedFloat64(running *float64, staged string) types.Float64 {
	if staged == "deleted" {
		return types.Float64Null()
	}
	if staged != "" {
		var parsed float64
		if _, err := fmt.Sscanf(staged, "%g", &parsed); err == nil {
			return types.Float64Value(parsed)
		}
	}
	return float64OrNull(running)
}

func (r *SDNFabricResource) storeManagedFields(ctx context.Context, config sdnFabricModel, private nodeNetworkPrivateData, diags *diag.Diagnostics) {
	managedFields, err := json.Marshal(sdnFabricManagedFields(config))
	if err != nil {
		diags.AddError("Unable to Store SDN Fabric State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, sdnFabricManagedFieldsKey, managedFields)...)
}

func readSDNFabricManagedFields(ctx context.Context, private nodeNetworkPrivateData) ([]string, diag.Diagnostics) {
	value, diags := private.GetKey(ctx, sdnFabricManagedFieldsKey)
	if diags.HasError() || len(value) == 0 {
		return nil, diags
	}
	var fields []string
	if err := json.Unmarshal(value, &fields); err != nil {
		diags.AddError("Unable to Read SDN Fabric State", fmt.Sprintf("unable to decode managed fields: %v", err))
	}
	return fields, diags
}
