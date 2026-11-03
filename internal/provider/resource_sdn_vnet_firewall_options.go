// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SDNVnetFirewallOptionsResource{}
var _ resource.ResourceWithImportState = &SDNVnetFirewallOptionsResource{}
var _ resource.ResourceWithValidateConfig = &SDNVnetFirewallOptionsResource{}

const sdnVnetFirewallOptionsManagedKey = "sdn-vnet-firewall-options-managed-fields"

// sdnVnetFirewallOptionKeys are all options the resource owns and resets.
var sdnVnetFirewallOptionKeys = []string{"enable", "log_level_forward", "policy_forward"}

var sdnFirewallLogLevels = []string{"emerg", "alert", "crit", "err", "warning", "notice", "info", "debug", "nolog"}

type SDNVnetFirewallOptionsResource struct {
	client *Client
}

type sdnVnetFirewallOptionsModel struct {
	ID              types.String `tfsdk:"id"`
	Vnet            types.String `tfsdk:"vnet"`
	Enable          types.Bool   `tfsdk:"enable"`
	PolicyForward   types.String `tfsdk:"policy_forward"`
	LogLevelForward types.String `tfsdk:"log_level_forward"`
}

func NewSDNVnetFirewallOptionsResource() resource.Resource {
	return &SDNVnetFirewallOptionsResource{}
}

func (r *SDNVnetFirewallOptionsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_vnet_firewall_options"
}

func (r *SDNVnetFirewallOptionsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the firewall options of one Proxmox VE SDN VNet through `/cluster/sdn/vnets/{vnet}/firewall/options`. Firewall options are not staged: they apply immediately and are independent of `proxmox_sdn_apply`. Requires `SDN.Allocate` on the VNet.",
		Attributes: map[string]schema.Attribute{
			"id":     schema.StringAttribute{Computed: true, MarkdownDescription: "VNet identifier (same as `vnet`).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"vnet":   schema.StringAttribute{Required: true, MarkdownDescription: "VNet the options belong to. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"enable": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable the firewall for this VNet."},
			"policy_forward": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Forward policy: `ACCEPT` or `DROP`.",
			},
			"log_level_forward": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Log level for forwarded traffic, for example `info` or `nolog`.",
			},
		},
	}
}

func (r *SDNVnetFirewallOptionsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SDNVnetFirewallOptionsResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config sdnVnetFirewallOptionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateSDNVnetFirewallOptions(config)...)
}

func validateSDNVnetFirewallOptions(config sdnVnetFirewallOptionsModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.Vnet) && !sdnVnetIDPattern.MatchString(config.Vnet.ValueString()) {
		diags.AddAttributeError(path.Root("vnet"), "Invalid SDN VNet identifier", "vnet must be an existing SDN VNet id.")
	}
	if isConfiguredValue(config.PolicyForward) && !slices.Contains([]string{"ACCEPT", "DROP"}, config.PolicyForward.ValueString()) {
		diags.AddAttributeError(path.Root("policy_forward"), "Invalid forward policy", "policy_forward must be ACCEPT or DROP.")
	}
	if isConfiguredValue(config.LogLevelForward) && !slices.Contains(sdnFirewallLogLevels, config.LogLevelForward.ValueString()) {
		diags.AddAttributeError(path.Root("log_level_forward"), "Invalid log level", "log_level_forward must be a Proxmox firewall log level.")
	}
	return diags
}

func sdnVnetFirewallOptionsManagedFields(config sdnVnetFirewallOptionsModel) []string {
	var fields []string
	if isConfiguredValue(config.Enable) {
		fields = append(fields, "enable")
	}
	if isConfiguredValue(config.PolicyForward) {
		fields = append(fields, "policy_forward")
	}
	if isConfiguredValue(config.LogLevelForward) {
		fields = append(fields, "log_level_forward")
	}
	slices.Sort(fields)
	return fields
}

func (r *SDNVnetFirewallOptionsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config sdnVnetFirewallOptionsModel
	var plan sdnVnetFirewallOptionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.applyOptions(ctx, config, plan); err != nil {
		resp.Diagnostics.AddError("Unable to Set Proxmox VNet Firewall Options", err.Error())
		return
	}
	state, diags := r.readState(ctx, plan.Vnet.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNVnetFirewallOptionsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sdnVnetFirewallOptionsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.Vnet.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *SDNVnetFirewallOptionsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config sdnVnetFirewallOptionsModel
	var plan sdnVnetFirewallOptionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManaged, diags := readSDNVnetFirewallOptionsManagedFields(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.applyOptionsWithDeletion(ctx, config, plan, previouslyManaged); err != nil {
		resp.Diagnostics.AddError("Unable to Set Proxmox VNet Firewall Options", err.Error())
		return
	}
	state, stateDiags := r.readState(ctx, plan.Vnet.ValueString())
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNVnetFirewallOptionsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sdnVnetFirewallOptionsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetSDNVnetFirewallOptions(ctx, state.Vnet.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox VNet Firewall Options", err.Error())
		return
	}
	request := SDNVnetFirewallOptionsRequest{Delete: sdnVnetFirewallOptionKeys}
	if current.Digest != "" {
		request.Digest = &current.Digest
	}
	if err := r.client.SetSDNVnetFirewallOptions(ctx, state.Vnet.ValueString(), request); err != nil {
		resp.Diagnostics.AddError("Unable to Reset Proxmox VNet Firewall Options", err.Error())
	}
}

func (r *SDNVnetFirewallOptionsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !sdnVnetIDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected an SDN VNet identifier.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vnet"), req.ID)...)
}

func (r *SDNVnetFirewallOptionsResource) applyOptions(ctx context.Context, config, plan sdnVnetFirewallOptionsModel) error {
	return r.applyOptionsWithDeletion(ctx, config, plan, nil)
}

func (r *SDNVnetFirewallOptionsResource) applyOptionsWithDeletion(ctx context.Context, config, plan sdnVnetFirewallOptionsModel, previouslyManaged []string) error {
	vnet := plan.Vnet.ValueString()
	current, err := r.client.GetSDNVnetFirewallOptions(ctx, vnet)
	if err != nil {
		return err
	}
	request := SDNVnetFirewallOptionsRequest{}
	if isConfiguredValue(config.Enable) {
		request.Enable = boolPointerValue(plan.Enable)
	}
	if isConfiguredValue(config.PolicyForward) {
		request.PolicyForward = stringPointer(plan.PolicyForward)
	}
	if isConfiguredValue(config.LogLevelForward) {
		request.LogLevelForward = stringPointer(plan.LogLevelForward)
	}
	if len(previouslyManaged) > 0 {
		request.Delete = sdnDeleteKeys(sdnVnetFirewallOptionsManagedFields(config), previouslyManaged)
	}
	if current.Digest != "" {
		request.Digest = &current.Digest
	}
	return r.client.SetSDNVnetFirewallOptions(ctx, vnet, request)
}

func (r *SDNVnetFirewallOptionsResource) readState(ctx context.Context, vnet string) (sdnVnetFirewallOptionsModel, diag.Diagnostics) {
	options, err := r.client.GetSDNVnetFirewallOptions(ctx, vnet)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox VNet Firewall Options", fmt.Sprintf("Unable to read VNet firewall options of %q: %s", vnet, err))
		return sdnVnetFirewallOptionsModel{}, diags
	}
	return sdnVnetFirewallOptionsModel{
		ID:              types.StringValue(vnet),
		Vnet:            types.StringValue(vnet),
		Enable:          boolOrNull(options.Enable.Ptr()),
		PolicyForward:   stringOrNull(options.PolicyForward),
		LogLevelForward: stringOrNull(options.LogLevelForward),
	}, nil
}

func (r *SDNVnetFirewallOptionsResource) storeManagedFields(ctx context.Context, config sdnVnetFirewallOptionsModel, private nodeNetworkPrivateData, diags *diag.Diagnostics) {
	managedFields, err := json.Marshal(sdnVnetFirewallOptionsManagedFields(config))
	if err != nil {
		diags.AddError("Unable to Store VNet Firewall Options State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, sdnVnetFirewallOptionsManagedKey, managedFields)...)
}

func readSDNVnetFirewallOptionsManagedFields(ctx context.Context, private nodeNetworkPrivateData) ([]string, diag.Diagnostics) {
	value, diags := private.GetKey(ctx, sdnVnetFirewallOptionsManagedKey)
	if diags.HasError() || len(value) == 0 {
		return nil, diags
	}
	var fields []string
	if err := json.Unmarshal(value, &fields); err != nil {
		diags.AddError("Unable to Read VNet Firewall Options State", fmt.Sprintf("unable to decode managed fields: %v", err))
	}
	return fields, diags
}
