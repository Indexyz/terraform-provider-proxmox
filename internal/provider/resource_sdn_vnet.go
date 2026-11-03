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

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SDNVnetResource{}
var _ resource.ResourceWithImportState = &SDNVnetResource{}
var _ resource.ResourceWithValidateConfig = &SDNVnetResource{}

const sdnVnetManagedFieldsKey = "sdn-vnet-managed-fields"

var sdnVnetIDPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]*[a-zA-Z0-9]$`)

type SDNVnetResource struct {
	client *Client
}

type sdnVnetModel struct {
	ID           types.String `tfsdk:"id"`
	Vnet         types.String `tfsdk:"vnet"`
	Zone         types.String `tfsdk:"zone"`
	Alias        types.String `tfsdk:"alias"`
	Tag          types.Int64  `tfsdk:"tag"`
	VLANAware    types.Bool   `tfsdk:"vlanaware"`
	IsolatePorts types.Bool   `tfsdk:"isolate_ports"`
	State        types.String `tfsdk:"state"`
	Pending      types.Map    `tfsdk:"pending"`
}

func NewSDNVnetResource() resource.Resource {
	return &SDNVnetResource{}
}

func (r *SDNVnetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_vnet"
}

func (r *SDNVnetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox VE SDN VNet through `/cluster/sdn/vnets`. VNets stage like zones and only take effect when `proxmox_sdn_apply` commits. Proxmox refuses to delete a VNet that still has subnets and refuses `vlanaware = true` while subnets exist. Requires `SDN.Allocate` on the parent zone.",
		Attributes: map[string]schema.Attribute{
			"id":    schema.StringAttribute{Computed: true, MarkdownDescription: "VNet identifier (same as `vnet`).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"vnet":  schema.StringAttribute{Required: true, MarkdownDescription: "VNet identifier: starts with a letter, only letters and digits, 2 to 8 characters. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"zone":  schema.StringAttribute{Required: true, MarkdownDescription: "Zone the VNet belongs to. Moving a VNet between zones is allowed while no subnets exist."},
			"alias": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Free-form alias shown in the UI."},
			"tag":   schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "VLAN tag for VLAN/QinQ zones or VXLAN VNI for VXLAN/EVPN zones, between 1 and 16777215."},
			"vlanaware": schema.BoolAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Let VLANs pass through the VNet. Not compatible with subnets.",
			},
			"isolate_ports": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Set the isolated port property on every bridge interface of this VNet."},
			"state":         schema.StringAttribute{Computed: true, MarkdownDescription: "Proxmox staging state: `new` or `changed` while the VNet is not committed yet."},
			"pending":       schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Properties Proxmox currently reports as staged but not committed, with `deleted` for removals."},
		},
	}
}

func (r *SDNVnetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SDNVnetResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config sdnVnetModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateSDNVnetConfig(config)...)
}

func validateSDNVnetConfig(config sdnVnetModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.Vnet) {
		vnet := config.Vnet.ValueString()
		if len(vnet) < 2 || len(vnet) > 8 || !sdnVnetIDPattern.MatchString(vnet) {
			diags.AddAttributeError(path.Root("vnet"), "Invalid SDN VNet identifier", "vnet must start with a letter, contain only letters and digits, and be 2 to 8 characters long.")
		}
	}
	if isConfiguredValue(config.Zone) && !sdnVnetIDPattern.MatchString(config.Zone.ValueString()) {
		diags.AddAttributeError(path.Root("zone"), "Invalid SDN zone identifier", "zone must be an existing SDN zone id: a letter, letters and digits, 2 to 8 characters.")
	}
	if isConfiguredValue(config.Tag) && (config.Tag.ValueInt64() < 1 || config.Tag.ValueInt64() > 16777215) {
		diags.AddAttributeError(path.Root("tag"), "Invalid VNet tag", "tag must be between 1 and 16777215.")
	}
	if isConfiguredValue(config.Alias) && len(config.Alias.ValueString()) > 256 {
		diags.AddAttributeError(path.Root("alias"), "Invalid VNet alias", "alias must not exceed 256 characters.")
	}
	return diags
}

func sdnVnetRequestFromModels(config, plan sdnVnetModel) SDNVnetRequest {
	request := SDNVnetRequest{Zone: plan.Zone.ValueString()}
	if isConfiguredValue(config.Alias) {
		request.Alias = stringPointer(plan.Alias)
	}
	if isConfiguredValue(config.Tag) {
		request.Tag = int64PointerValue(plan.Tag)
	}
	if isConfiguredValue(config.VLANAware) {
		request.VLANAware = boolPointerValue(plan.VLANAware)
	}
	if isConfiguredValue(config.IsolatePorts) {
		request.IsolatePorts = boolPointerValue(plan.IsolatePorts)
	}
	return request
}

func sdnVnetManagedFields(config sdnVnetModel) []string {
	var fields []string
	if isConfiguredValue(config.Alias) {
		fields = append(fields, "alias")
	}
	if isConfiguredValue(config.Tag) {
		fields = append(fields, "tag")
	}
	if isConfiguredValue(config.VLANAware) {
		fields = append(fields, "vlanaware")
	}
	if isConfiguredValue(config.IsolatePorts) {
		fields = append(fields, "isolate-ports")
	}
	slices.Sort(fields)
	return fields
}

func sdnVnetDeleteKeys(config sdnVnetModel, previouslyManaged []string) []string {
	current := sdnVnetManagedFields(config)
	var deleted []string
	for _, key := range previouslyManaged {
		if !slices.Contains(current, key) {
			deleted = append(deleted, key)
		}
	}
	slices.Sort(deleted)
	return deleted
}

func (r *SDNVnetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config sdnVnetModel
	var plan sdnVnetModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	createReq := sdnVnetRequestFromModels(config, plan)
	vnet := plan.Vnet.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		createReq.LockToken = token
		return r.client.CreateSDNVnet(ctx, vnet, createReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, vnet)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNVnetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sdnVnetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.Vnet.ValueString())
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

func (r *SDNVnetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config sdnVnetModel
	var plan sdnVnetModel
	var prior sdnVnetModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManaged, diags := readSDNVnetManagedFields(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetSDNVnet(ctx, prior.Vnet.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN VNet", err.Error())
		return
	}
	updateReq := sdnVnetRequestFromModels(config, plan)
	updateReq.Delete = sdnVnetDeleteKeys(config, previouslyManaged)
	vnet := prior.Vnet.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		updateReq.LockToken = token
		if current.Digest != "" {
			updateReq.Digest = &current.Digest
		}
		return r.client.UpdateSDNVnet(ctx, vnet, updateReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, vnet)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNVnetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sdnVnetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.GetSDNVnet(ctx, state.Vnet.ValueString()); err != nil {
		if errors.Is(err, errNotFound) {
			return
		}
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN VNet", err.Error())
		return
	}
	vnet := state.Vnet.ValueString()
	withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		return r.client.DeleteSDNVnet(ctx, vnet, token)
	})
}

func (r *SDNVnetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if len(req.ID) < 2 || len(req.ID) > 8 || !sdnVnetIDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected an SDN VNet identifier: a letter, letters and digits only, 2 to 8 characters.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vnet"), req.ID)...)
}

func (r *SDNVnetResource) readState(ctx context.Context, vnet string) (sdnVnetModel, diag.Diagnostics) {
	entry, err := r.client.GetSDNVnet(ctx, vnet)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return sdnVnetModel{ID: types.StringNull()}, nil
		}
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox SDN VNet", fmt.Sprintf("Unable to read SDN VNet %q: %s", vnet, err))
		return sdnVnetModel{}, diags
	}
	if entry.State == "deleted" {
		return sdnVnetModel{ID: types.StringNull()}, nil
	}
	return sdnVnetStateFromAPI(ctx, entry)
}

func sdnVnetStateFromAPI(ctx context.Context, entry SDNVnet) (sdnVnetModel, diag.Diagnostics) {
	staged := sdnPendingMap(entry.Pending)
	state := sdnVnetModel{
		ID:           types.StringValue(entry.Vnet),
		Vnet:         types.StringValue(entry.Vnet),
		Zone:         sdnVnetStagedString(entry.Zone, staged["zone"]),
		Alias:        sdnVnetStagedString(entry.Alias, staged["alias"]),
		Tag:          sdnVnetStagedInt64(entry.Tag.Ptr(), staged["tag"]),
		VLANAware:    sdnVnetStagedBool(entry.VLANAware.Ptr(), staged["vlanaware"]),
		IsolatePorts: sdnVnetStagedBool(entry.IsolatePorts.Ptr(), staged["isolate-ports"]),
		State:        stringOrNull(entry.State),
		Pending:      types.MapNull(types.StringType),
	}
	var diags diag.Diagnostics
	if len(staged) > 0 {
		pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		diags.Append(pendingDiags...)
		state.Pending = pending
	}
	return state, diags
}

func sdnVnetStagedString(running, staged string) types.String {
	return sdnStagedString(running, staged)
}

func sdnVnetStagedInt64(running *int64, staged string) types.Int64 {
	return sdnStagedInt64(running, staged)
}

func sdnVnetStagedBool(running *bool, staged string) types.Bool {
	return sdnStagedBool(running, staged)
}

func (r *SDNVnetResource) storeManagedFields(ctx context.Context, config sdnVnetModel, private nodeNetworkPrivateData, diags *diag.Diagnostics) {
	managedFields, err := json.Marshal(sdnVnetManagedFields(config))
	if err != nil {
		diags.AddError("Unable to Store SDN VNet State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, sdnVnetManagedFieldsKey, managedFields)...)
}

func readSDNVnetManagedFields(ctx context.Context, private nodeNetworkPrivateData) ([]string, diag.Diagnostics) {
	value, diags := private.GetKey(ctx, sdnVnetManagedFieldsKey)
	if diags.HasError() || len(value) == 0 {
		return nil, diags
	}
	var fields []string
	if err := json.Unmarshal(value, &fields); err != nil {
		diags.AddError("Unable to Read SDN VNet State", fmt.Sprintf("unable to decode managed fields: %v", err))
	}
	return fields, diags
}
