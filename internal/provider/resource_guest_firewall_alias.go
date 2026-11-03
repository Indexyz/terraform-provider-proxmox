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

var _ resource.Resource = &GuestFirewallAliasResource{}
var _ resource.ResourceWithImportState = &GuestFirewallAliasResource{}
var _ resource.ResourceWithValidateConfig = &GuestFirewallAliasResource{}

const guestFirewallAliasManagedFieldsKey = "guest-firewall-alias-managed-fields"

type GuestFirewallAliasResource struct {
	client *Client
}

type guestFirewallAliasModel struct {
	ID        types.String `tfsdk:"id"`
	Node      types.String `tfsdk:"node"`
	GuestType types.String `tfsdk:"guest_type"`
	VMID      types.Int64  `tfsdk:"vmid"`
	Name      types.String `tfsdk:"name"`
	CIDR      types.String `tfsdk:"cidr"`
	Comment   types.String `tfsdk:"comment"`
}

func NewGuestFirewallAliasResource() resource.Resource {
	return &GuestFirewallAliasResource{}
}

func (r *GuestFirewallAliasResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_firewall_alias"
}

func (r *GuestFirewallAliasResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a QEMU/LXC firewall alias through `/nodes/{node}/{guest}/{vmid}/firewall/aliases`. Guests must exist; PVE does not validate alias references when deleting.",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<node>/<guest_type>/<vmid>/<name>` form."},
			"node":       schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node that runs the guest. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"guest_type": schema.StringAttribute{Required: true, MarkdownDescription: "`qemu` or `lxc`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"vmid":       schema.Int64Attribute{Required: true, MarkdownDescription: "Guest VMID. Changes require replacement.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"name":       schema.StringAttribute{Required: true, MarkdownDescription: "Stable alias name. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"cidr":       schema.StringAttribute{Required: true, MarkdownDescription: "IPv4/IPv6 address or CIDR network."},
			"comment":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Alias comment."},
		},
	}
}

func (r *GuestFirewallAliasResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GuestFirewallAliasResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config guestFirewallAliasModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateGuestFirewallScope(config.Node, config.GuestType, config.VMID)...)
}

func (r *GuestFirewallAliasResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan guestFirewallAliasModel
	var config guestFirewallAliasModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	alias := GuestFirewallAlias{Name: plan.Name.ValueString(), CIDR: plan.CIDR.ValueString()}
	if isConfiguredValue(plan.Comment) {
		alias.Comment = plan.Comment.ValueString()
	}
	scope := guestFirewallScopeFromModel(plan.Node, plan.GuestType, plan.VMID)
	if err := r.client.CreateGuestFirewallAlias(ctx, scope.Node, scope.GuestType, scope.VMID, alias); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Guest Firewall Alias", err.Error())
		return
	}
	state, diags := r.readState(ctx, scope, alias.Name)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		fields := []string{}
		if isConfiguredValue(config.Comment) {
			fields = append(fields, "comment")
		}
		storeClusterFirewallManagedFields(ctx, resp.Private, guestFirewallAliasManagedFieldsKey, fields, &resp.Diagnostics)
	}
}

func (r *GuestFirewallAliasResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state guestFirewallAliasModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	scope := guestFirewallScopeFromModel(state.Node, state.GuestType, state.VMID)
	refreshed, diags := r.readState(ctx, scope, state.Name.ValueString())
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

func (r *GuestFirewallAliasResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config guestFirewallAliasModel
	var state guestFirewallAliasModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	managed, privateDiags := readClusterFirewallManagedFields(ctx, req.Private, guestFirewallAliasManagedFieldsKey)
	resp.Diagnostics.Append(privateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	scope := guestFirewallScopeFromModel(config.Node, config.GuestType, config.VMID)
	name := config.Name.ValueString()
	current, err := r.client.GetGuestFirewallAlias(ctx, scope.Node, scope.GuestType, scope.VMID, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Guest Firewall Alias", err.Error())
		return
	}
	current.CIDR = config.CIDR.ValueString()
	current.Comment = clusterFirewallManagedString(current.Comment, config.Comment, clusterFirewallFieldManaged(managed, "comment"))
	if err := r.client.UpdateGuestFirewallAlias(ctx, scope.Node, scope.GuestType, scope.VMID, current); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Guest Firewall Alias", err.Error())
		return
	}
	refreshed, diags := r.readState(ctx, scope, current.Name)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
		fields := []string{}
		if isConfiguredValue(config.Comment) {
			fields = append(fields, "comment")
		}
		storeClusterFirewallManagedFields(ctx, resp.Private, guestFirewallAliasManagedFieldsKey, fields, &resp.Diagnostics)
	}
}

func (r *GuestFirewallAliasResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state guestFirewallAliasModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	scope := guestFirewallScopeFromModel(state.Node, state.GuestType, state.VMID)
	current, err := r.client.GetGuestFirewallAlias(ctx, scope.Node, scope.GuestType, scope.VMID, state.Name.ValueString())
	if errors.Is(err, errNotFound) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Guest Firewall Alias", err.Error())
		return
	}
	if err := r.client.DeleteGuestFirewallAlias(ctx, scope.Node, scope.GuestType, scope.VMID, current.Name, current.Digest); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Guest Firewall Alias", err.Error())
	}
}

func (r *GuestFirewallAliasResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	scope, name, diags := parseGuestFirewallImportID(req.ID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), scope.Node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("guest_type"), scope.GuestType)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vmid"), scope.VMID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}

func (r *GuestFirewallAliasResource) readState(ctx context.Context, scope guestFirewallScope, name string) (guestFirewallAliasModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	alias, err := r.client.GetGuestFirewallAlias(ctx, scope.Node, scope.GuestType, scope.VMID, name)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return guestFirewallAliasModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Guest Firewall Alias", err.Error())
		return guestFirewallAliasModel{}, diags
	}
	return guestFirewallAliasModel{
		ID:        types.StringValue(guestFirewallID(scope, alias.Name)),
		Node:      types.StringValue(scope.Node),
		GuestType: types.StringValue(scope.GuestType),
		VMID:      types.Int64Value(scope.VMID),
		Name:      types.StringValue(alias.Name),
		CIDR:      types.StringValue(alias.CIDR),
		Comment:   stringOrNull(alias.Comment),
	}, diags
}

func guestFirewallID(scope guestFirewallScope, name string) string {
	return fmt.Sprintf("%s/%s/%d/%s", scope.Node, scope.GuestType, scope.VMID, name)
}

func stringsTrimIsEmpty(value string) bool { return strings.TrimSpace(value) == "" }
