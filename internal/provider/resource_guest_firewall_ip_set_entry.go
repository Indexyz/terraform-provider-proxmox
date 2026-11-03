// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &GuestFirewallIPSetEntryResource{}
var _ resource.ResourceWithImportState = &GuestFirewallIPSetEntryResource{}
var _ resource.ResourceWithValidateConfig = &GuestFirewallIPSetEntryResource{}

const guestFirewallIPSetEntryManagedFieldsKey = "guest-firewall-ipset-entry-managed-fields"

type GuestFirewallIPSetEntryResource struct {
	client *Client
}

type guestFirewallIPSetEntryModel struct {
	ID        types.String `tfsdk:"id"`
	Node      types.String `tfsdk:"node"`
	GuestType types.String `tfsdk:"guest_type"`
	VMID      types.Int64  `tfsdk:"vmid"`
	IPSet     types.String `tfsdk:"ip_set"`
	CIDR      types.String `tfsdk:"cidr"`
	Comment   types.String `tfsdk:"comment"`
	NoMatch   types.Bool   `tfsdk:"nomatch"`
}

func NewGuestFirewallIPSetEntryResource() resource.Resource {
	return &GuestFirewallIPSetEntryResource{}
}

func (r *GuestFirewallIPSetEntryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_firewall_ip_set_entry"
}

func (r *GuestFirewallIPSetEntryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an address, network, or alias entry in a QEMU/LXC firewall IP set. Guests must exist.",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<node>/<guest_type>/<vmid>/<ip_set>/<cidr>` form."},
			"node":       schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node that runs the guest. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"guest_type": schema.StringAttribute{Required: true, MarkdownDescription: "`qemu` or `lxc`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"vmid":       schema.Int64Attribute{Required: true, MarkdownDescription: "Guest VMID. Changes require replacement.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"ip_set":     schema.StringAttribute{Required: true, MarkdownDescription: "Parent firewall IP set name. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"cidr":       schema.StringAttribute{Required: true, MarkdownDescription: "IPv4/IPv6 address, CIDR network, or alias. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"comment":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Entry comment."},
			"nomatch":    schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Exclude this entry from the IP set match."},
		},
	}
}

func (r *GuestFirewallIPSetEntryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GuestFirewallIPSetEntryResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config guestFirewallIPSetEntryModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateGuestFirewallScope(config.Node, config.GuestType, config.VMID)...)
}

func (r *GuestFirewallIPSetEntryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan guestFirewallIPSetEntryModel
	var config guestFirewallIPSetEntryModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entry := GuestFirewallIPSetEntry{CIDR: plan.CIDR.ValueString()}
	if isConfiguredValue(plan.Comment) {
		entry.Comment = plan.Comment.ValueString()
	}
	if isConfiguredValue(plan.NoMatch) {
		entry.NoMatch = proxmoxOptionalBool{value: boolPointerValue(plan.NoMatch)}
	}
	scope := guestFirewallScopeFromModel(plan.Node, plan.GuestType, plan.VMID)
	name := plan.IPSet.ValueString()
	if err := r.client.CreateGuestFirewallIPSetEntry(ctx, scope.Node, scope.GuestType, scope.VMID, name, entry); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Guest Firewall IP Set Entry", err.Error())
		return
	}
	state, diags := r.readState(ctx, scope, name, entry.CIDR)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		fields := []string{}
		if isConfiguredValue(config.Comment) {
			fields = append(fields, "comment")
		}
		if isConfiguredValue(config.NoMatch) {
			fields = append(fields, "nomatch")
		}
		storeClusterFirewallManagedFields(ctx, resp.Private, guestFirewallIPSetEntryManagedFieldsKey, fields, &resp.Diagnostics)
	}
}

func (r *GuestFirewallIPSetEntryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state guestFirewallIPSetEntryModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	scope := guestFirewallScopeFromModel(state.Node, state.GuestType, state.VMID)
	refreshed, diags := r.readState(ctx, scope, state.IPSet.ValueString(), state.CIDR.ValueString())
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

func (r *GuestFirewallIPSetEntryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config guestFirewallIPSetEntryModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	managed, privateDiags := readClusterFirewallManagedFields(ctx, req.Private, guestFirewallIPSetEntryManagedFieldsKey)
	resp.Diagnostics.Append(privateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	scope := guestFirewallScopeFromModel(config.Node, config.GuestType, config.VMID)
	name := config.IPSet.ValueString()
	current, err := r.client.GetGuestFirewallIPSetEntry(ctx, scope.Node, scope.GuestType, scope.VMID, name, config.CIDR.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Guest Firewall IP Set Entry", err.Error())
		return
	}
	current.Comment = clusterFirewallManagedString(current.Comment, config.Comment, clusterFirewallFieldManaged(managed, "comment"))
	current.NoMatch = clusterFirewallManagedBool(current.NoMatch, config.NoMatch, clusterFirewallFieldManaged(managed, "nomatch"))
	if err := r.client.UpdateGuestFirewallIPSetEntry(ctx, scope.Node, scope.GuestType, scope.VMID, name, current); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Guest Firewall IP Set Entry", err.Error())
		return
	}
	state, diags := r.readState(ctx, scope, name, current.CIDR)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		fields := []string{}
		if isConfiguredValue(config.Comment) {
			fields = append(fields, "comment")
		}
		if isConfiguredValue(config.NoMatch) {
			fields = append(fields, "nomatch")
		}
		storeClusterFirewallManagedFields(ctx, resp.Private, guestFirewallIPSetEntryManagedFieldsKey, fields, &resp.Diagnostics)
	}
}

func (r *GuestFirewallIPSetEntryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state guestFirewallIPSetEntryModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	scope := guestFirewallScopeFromModel(state.Node, state.GuestType, state.VMID)
	entry, err := r.client.GetGuestFirewallIPSetEntry(ctx, scope.Node, scope.GuestType, scope.VMID, state.IPSet.ValueString(), state.CIDR.ValueString())
	if errors.Is(err, errNotFound) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Guest Firewall IP Set Entry", err.Error())
		return
	}
	if err := r.client.DeleteGuestFirewallIPSetEntry(ctx, scope.Node, scope.GuestType, scope.VMID, state.IPSet.ValueString(), state.CIDR.ValueString(), entry.Digest); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Guest Firewall IP Set Entry", err.Error())
	}
}

func (r *GuestFirewallIPSetEntryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 5 {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected `<node>/<guest_type>/<vmid>/<ip_set>/<cidr>`.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("guest_type"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vmid"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip_set"), parts[3])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cidr"), parts[4])...)
}

func (r *GuestFirewallIPSetEntryResource) readState(ctx context.Context, scope guestFirewallScope, name, cidr string) (guestFirewallIPSetEntryModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	entry, err := r.client.GetGuestFirewallIPSetEntry(ctx, scope.Node, scope.GuestType, scope.VMID, name, cidr)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return guestFirewallIPSetEntryModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Guest Firewall IP Set Entry", err.Error())
		return guestFirewallIPSetEntryModel{}, diags
	}
	nomatch := false
	if entry.NoMatch.Ptr() != nil {
		nomatch = *entry.NoMatch.Ptr()
	}
	return guestFirewallIPSetEntryModel{
		ID:        types.StringValue(fmt.Sprintf("%s/%s/%d/%s/%s", scope.Node, scope.GuestType, scope.VMID, name, entry.CIDR)),
		Node:      types.StringValue(scope.Node),
		GuestType: types.StringValue(scope.GuestType),
		VMID:      types.Int64Value(scope.VMID),
		IPSet:     types.StringValue(name),
		CIDR:      types.StringValue(entry.CIDR),
		Comment:   stringOrNull(entry.Comment),
		NoMatch:   types.BoolValue(nomatch),
	}, diags
}
