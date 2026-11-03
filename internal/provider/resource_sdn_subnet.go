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
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SDNSubnetResource{}
var _ resource.ResourceWithImportState = &SDNSubnetResource{}
var _ resource.ResourceWithValidateConfig = &SDNSubnetResource{}

const sdnSubnetManagedFieldsKey = "sdn-subnet-managed-fields"

type SDNSubnetResource struct {
	client *Client
}

type sdnSubnetModel struct {
	ID            types.String `tfsdk:"id"`
	Vnet          types.String `tfsdk:"vnet"`
	CIDR          types.String `tfsdk:"cidr"`
	Zone          types.String `tfsdk:"zone"`
	Gateway       types.String `tfsdk:"gateway"`
	SNAT          types.Bool   `tfsdk:"snat"`
	DNSZonePrefix types.String `tfsdk:"dnszoneprefix"`
	DHCPRanges    types.List   `tfsdk:"dhcp_range"`
	DHCPDNSServer types.String `tfsdk:"dhcp_dns_server"`
	State         types.String `tfsdk:"state"`
	Pending       types.Map    `tfsdk:"pending"`
}

type sdnDHCPRangeModel struct {
	StartAddress types.String `tfsdk:"start_address"`
	EndAddress   types.String `tfsdk:"end_address"`
}

func sdnDHCPRangeAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"start_address": types.StringType,
		"end_address":   types.StringType,
	}
}

var sdnDNSNamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`)

func NewSDNSubnetResource() resource.Resource {
	return &SDNSubnetResource{}
}

func (r *SDNSubnetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_subnet"
}

func (r *SDNSubnetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox VE SDN subnet through `/cluster/sdn/vnets/{vnet}/subnets`. Subnets stage like zones and VNets and only take effect when `proxmox_sdn_apply` commits. The parent VNet must be `vlanaware = false`. Requires `SDN.Allocate` on the VNet.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true, MarkdownDescription: "Internal Proxmox subnet id (`<zone>-<network>-<mask>`).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"vnet": schema.StringAttribute{Required: true, MarkdownDescription: "Parent VNet. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"cidr": schema.StringAttribute{Required: true, MarkdownDescription: "Subnet in CIDR form, for example `10.1.0.0/24`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"zone": schema.StringAttribute{Computed: true, MarkdownDescription: "Zone of the parent VNet, as reported by Proxmox."},
			"gateway": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Gateway address inside the subnet. Proxmox assigns it to the VNet for layer-3 zones.",
			},
			"snat":            schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Masquerade traffic from this subnet when the Proxmox firewall is active."},
			"dnszoneprefix":   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "DNS zone prefix, for example `adm` yields `<host>.adm.<dnszone>`."},
			"dhcp_dns_server": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "DNS server announced to DHCP clients."},
			"dhcp_range": schema.ListNestedAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "DHCP address ranges inside the subnet.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"start_address": schema.StringAttribute{Required: true, MarkdownDescription: "First address of the range."},
						"end_address":   schema.StringAttribute{Required: true, MarkdownDescription: "Last address of the range."},
					},
				},
			},
			"state":   schema.StringAttribute{Computed: true, MarkdownDescription: "Proxmox staging state: `new` or `changed` while the subnet is not committed yet."},
			"pending": schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Properties Proxmox currently reports as staged but not committed, with `deleted` for removals."},
		},
	}
}

func (r *SDNSubnetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SDNSubnetResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config sdnSubnetModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateSDNSubnetConfig(config)...)
}

func validateSDNSubnetConfig(config sdnSubnetModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.Vnet) && !sdnVnetIDPattern.MatchString(config.Vnet.ValueString()) {
		diags.AddAttributeError(path.Root("vnet"), "Invalid SDN VNet identifier", "vnet must be an existing SDN VNet id: a letter, letters and digits, 2 to 8 characters.")
	}
	if isConfiguredValue(config.CIDR) {
		if _, _, err := net.ParseCIDR(config.CIDR.ValueString()); err != nil {
			diags.AddAttributeError(path.Root("cidr"), "Invalid subnet CIDR", fmt.Sprintf("%q is not a CIDR network: %s", config.CIDR.ValueString(), err))
		}
	}
	if isConfiguredValue(config.Gateway) && net.ParseIP(config.Gateway.ValueString()) == nil {
		diags.AddAttributeError(path.Root("gateway"), "Invalid gateway address", "gateway must be an IP address.")
	}
	if isConfiguredValue(config.DHCPDNSServer) && net.ParseIP(config.DHCPDNSServer.ValueString()) == nil {
		diags.AddAttributeError(path.Root("dhcp_dns_server"), "Invalid DNS server address", "dhcp_dns_server must be an IP address.")
	}
	if isConfiguredValue(config.DNSZonePrefix) && !sdnDNSNamePattern.MatchString(config.DNSZonePrefix.ValueString()) {
		diags.AddAttributeError(path.Root("dnszoneprefix"), "Invalid DNS zone prefix", "dnszoneprefix must be a single DNS label.")
	}
	for index, dhcpRange := range sdnDHCPRangeValues(config.DHCPRanges) {
		if net.ParseIP(dhcpRange.StartAddress) == nil {
			diags.AddAttributeError(path.Root("dhcp_range").AtListIndex(index).AtName("start_address"), "Invalid range start", "start_address must be an IP address.")
		}
		if net.ParseIP(dhcpRange.EndAddress) == nil {
			diags.AddAttributeError(path.Root("dhcp_range").AtListIndex(index).AtName("end_address"), "Invalid range end", "end_address must be an IP address.")
		}
	}
	return diags
}

func sdnDHCPRangeValues(value types.List) []SDNDHCPRange {
	if !isConfiguredValue(value) {
		return nil
	}
	var ranges []SDNDHCPRange
	for _, element := range value.Elements() {
		object, ok := element.(types.Object)
		if !ok || object.IsNull() || object.IsUnknown() {
			continue
		}
		attrs := object.Attributes()
		start, _ := attrs["start_address"].(types.String)
		end, _ := attrs["end_address"].(types.String)
		ranges = append(ranges, SDNDHCPRange{StartAddress: start.ValueString(), EndAddress: end.ValueString()})
	}
	return ranges
}

func sdnSubnetRequestFromModels(config, plan sdnSubnetModel) SDNSubnetRequest {
	request := SDNSubnetRequest{Vnet: plan.Vnet.ValueString()}
	if isConfiguredValue(config.Gateway) {
		request.Gateway = stringPointer(plan.Gateway)
	}
	if isConfiguredValue(config.SNAT) {
		request.SNAT = boolPointerValue(plan.SNAT)
	}
	if isConfiguredValue(config.DNSZonePrefix) {
		request.DNSZonePrefix = stringPointer(plan.DNSZonePrefix)
	}
	if isConfiguredValue(config.DHCPDNSServer) {
		request.DHCPDNSServer = stringPointer(plan.DHCPDNSServer)
	}
	if isConfiguredValue(config.DHCPRanges) {
		ranges := sdnDHCPRangeValues(plan.DHCPRanges)
		if ranges == nil {
			ranges = []SDNDHCPRange{}
		}
		request.DHCPRanges = ranges
	}
	return request
}

func sdnSubnetManagedFields(config sdnSubnetModel) []string {
	var fields []string
	addString := func(key string, value types.String) {
		if isConfiguredValue(value) {
			fields = append(fields, key)
		}
	}
	addString("gateway", config.Gateway)
	addString("dnszoneprefix", config.DNSZonePrefix)
	addString("dhcp-dns-server", config.DHCPDNSServer)
	if isConfiguredValue(config.SNAT) {
		fields = append(fields, "snat")
	}
	if isConfiguredValue(config.DHCPRanges) {
		fields = append(fields, "dhcp-range")
	}
	slices.Sort(fields)
	return fields
}

func sdnSubnetDeleteKeys(config sdnSubnetModel, previouslyManaged []string) []string {
	current := sdnSubnetManagedFields(config)
	var deleted []string
	for _, key := range previouslyManaged {
		if !slices.Contains(current, key) {
			deleted = append(deleted, key)
		}
	}
	slices.Sort(deleted)
	return deleted
}

func (r *SDNSubnetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config sdnSubnetModel
	var plan sdnSubnetModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	createReq := sdnSubnetRequestFromModels(config, plan)
	vnet := plan.Vnet.ValueString()
	cidr := plan.CIDR.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		createReq.LockToken = token
		return r.client.CreateSDNSubnet(ctx, vnet, cidr, createReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, vnet, cidr)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNSubnetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sdnSubnetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.Vnet.ValueString(), state.CIDR.ValueString())
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

func (r *SDNSubnetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config sdnSubnetModel
	var plan sdnSubnetModel
	var prior sdnSubnetModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManaged, diags := readSDNSubnetManagedFields(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetSDNSubnet(ctx, prior.Vnet.ValueString(), prior.CIDR.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN Subnet", err.Error())
		return
	}
	updateReq := sdnSubnetRequestFromModels(config, plan)
	updateReq.Delete = sdnSubnetDeleteKeys(config, previouslyManaged)
	vnet := prior.Vnet.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		updateReq.LockToken = token
		if current.Digest != "" {
			updateReq.Digest = &current.Digest
		}
		return r.client.UpdateSDNSubnet(ctx, vnet, current.Subnet, updateReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, vnet, prior.CIDR.ValueString())
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNSubnetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sdnSubnetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetSDNSubnet(ctx, state.Vnet.ValueString(), state.CIDR.ValueString())
	if err != nil {
		if errors.Is(err, errNotFound) {
			return
		}
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN Subnet", err.Error())
		return
	}
	vnet := state.Vnet.ValueString()
	withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		return r.client.DeleteSDNSubnet(ctx, vnet, current.Subnet, token)
	})
}

func (r *SDNSubnetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	vnet, cidr, ok := strings.Cut(req.ID, "/")
	if !ok || !sdnVnetIDPattern.MatchString(vnet) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected an import identifier in <vnet>/<cidr> form, for example vn1/10.1.0.0/24.")
		return
	}
	if _, _, err := net.ParseCIDR(cidr); err != nil {
		resp.Diagnostics.AddError("Unexpected Import Identifier", fmt.Sprintf("%q is not a CIDR network: %s", cidr, err))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vnet"), vnet)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cidr"), cidr)...)
}

func (r *SDNSubnetResource) readState(ctx context.Context, vnet, cidr string) (sdnSubnetModel, diag.Diagnostics) {
	entry, err := r.client.GetSDNSubnet(ctx, vnet, cidr)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return sdnSubnetModel{ID: types.StringNull()}, nil
		}
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox SDN Subnet", fmt.Sprintf("Unable to read SDN subnet %q on VNet %q: %s", cidr, vnet, err))
		return sdnSubnetModel{}, diags
	}
	if entry.State == "deleted" {
		return sdnSubnetModel{ID: types.StringNull()}, nil
	}
	return sdnSubnetStateFromAPI(ctx, entry)
}

func sdnSubnetStateFromAPI(ctx context.Context, entry SDNSubnet) (sdnSubnetModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	staged := sdnPendingMap(entry.Pending)
	ranges, rangeDiags := sdnSubnetStagedRanges(ctx, entry, staged["dhcp-range"])
	diags.Append(rangeDiags...)
	state := sdnSubnetModel{
		ID:            types.StringValue(entry.Subnet),
		Vnet:          types.StringValue(entry.Vnet),
		CIDR:          types.StringValue(entry.CIDR),
		Zone:          stringOrNull(entry.Zone),
		Gateway:       sdnStagedString(entry.Gateway, staged["gateway"]),
		SNAT:          sdnStagedBool(entry.SNAT.Ptr(), staged["snat"]),
		DNSZonePrefix: sdnStagedString(entry.DNSZonePrefix, staged["dnszoneprefix"]),
		DHCPRanges:    ranges,
		DHCPDNSServer: sdnStagedString(entry.DHCPDNSServer, staged["dhcp-dns-server"]),
		State:         stringOrNull(entry.State),
		Pending:       types.MapNull(types.StringType),
	}
	if len(staged) > 0 {
		pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		diags.Append(pendingDiags...)
		state.Pending = pending
	}
	return state, diags
}

// sdnSubnetStagedRanges decodes DHCP ranges from the staged JSON array, which
// arrives as objects instead of the property strings used on the wire.
func sdnSubnetStagedRanges(ctx context.Context, entry SDNSubnet, staged string) (types.List, diag.Diagnostics) {
	running := entry.DHCPRanges
	switch staged {
	case "":
	case "deleted":
		return types.ListNull(types.ObjectType{AttrTypes: sdnDHCPRangeAttrTypes()}), nil
	default:
		var decoded []SDNDHCPRange
		if err := json.Unmarshal([]byte(staged), &decoded); err != nil {
			return types.ListNull(types.ObjectType{AttrTypes: sdnDHCPRangeAttrTypes()}), diag.Diagnostics{diag.NewErrorDiagnostic(
				"Unable to Decode Proxmox SDN Subnet DHCP Ranges",
				fmt.Sprintf("unexpected staged dhcp-range value %q: %s", staged, err),
			)}
		}
		running = decoded
	}
	if running == nil {
		return types.ListNull(types.ObjectType{AttrTypes: sdnDHCPRangeAttrTypes()}), nil
	}
	objects := make([]sdnDHCPRangeModel, 0, len(running))
	for _, dhcpRange := range running {
		objects = append(objects, sdnDHCPRangeModel{
			StartAddress: types.StringValue(dhcpRange.StartAddress),
			EndAddress:   types.StringValue(dhcpRange.EndAddress),
		})
	}
	return types.ListValueFrom(ctx, types.ObjectType{AttrTypes: sdnDHCPRangeAttrTypes()}, objects)
}

func (r *SDNSubnetResource) storeManagedFields(ctx context.Context, config sdnSubnetModel, private nodeNetworkPrivateData, diags *diag.Diagnostics) {
	managedFields, err := json.Marshal(sdnSubnetManagedFields(config))
	if err != nil {
		diags.AddError("Unable to Store SDN Subnet State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, sdnSubnetManagedFieldsKey, managedFields)...)
}

func readSDNSubnetManagedFields(ctx context.Context, private nodeNetworkPrivateData) ([]string, diag.Diagnostics) {
	value, diags := private.GetKey(ctx, sdnSubnetManagedFieldsKey)
	if diags.HasError() || len(value) == 0 {
		return nil, diags
	}
	var fields []string
	if err := json.Unmarshal(value, &fields); err != nil {
		diags.AddError("Unable to Read SDN Subnet State", fmt.Sprintf("unable to decode managed fields: %v", err))
	}
	return fields, diags
}
