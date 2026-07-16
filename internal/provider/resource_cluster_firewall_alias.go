// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &ClusterFirewallAliasResource{}
var _ resource.ResourceWithImportState = &ClusterFirewallAliasResource{}

const clusterFirewallAliasManagedFieldsKey = "cluster-firewall-alias-managed-fields"

type ClusterFirewallAliasResource struct {
	client *Client
}

type clusterFirewallAliasModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	CIDR    types.String `tfsdk:"cidr"`
	Comment types.String `tfsdk:"comment"`
}

func NewClusterFirewallAliasResource() resource.Resource {
	return &ClusterFirewallAliasResource{}
}

func (r *ClusterFirewallAliasResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_firewall_alias"
}

func (r *ClusterFirewallAliasResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a cluster firewall IP or network alias through `/cluster/firewall/aliases`.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, MarkdownDescription: "Alias name."},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Stable alias name. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"cidr":    schema.StringAttribute{Required: true, MarkdownDescription: "IPv4/IPv6 address or CIDR network."},
			"comment": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Alias comment."},
		},
	}
}

func (r *ClusterFirewallAliasResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ClusterFirewallAliasResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterFirewallAliasModel
	var config clusterFirewallAliasModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	alias := ClusterFirewallAlias{Name: plan.Name.ValueString(), CIDR: plan.CIDR.ValueString()}
	if !plan.Comment.IsNull() && !plan.Comment.IsUnknown() {
		alias.Comment = plan.Comment.ValueString()
	}
	if err := r.client.CreateClusterFirewallAlias(ctx, alias); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Cluster Firewall Alias", err.Error())
		return
	}
	state, diags := r.readState(ctx, alias.Name)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		fields := []string{}
		if !config.Comment.IsNull() && !config.Comment.IsUnknown() {
			fields = append(fields, "comment")
		}
		storeClusterFirewallManagedFields(ctx, resp.Private, clusterFirewallAliasManagedFieldsKey, fields, &resp.Diagnostics)
	}
}

func (r *ClusterFirewallAliasResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterFirewallAliasModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.Name.ValueString())
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

func (r *ClusterFirewallAliasResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config clusterFirewallAliasModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	managed, privateDiags := readClusterFirewallManagedFields(ctx, req.Private, clusterFirewallAliasManagedFieldsKey)
	resp.Diagnostics.Append(privateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetClusterFirewallAlias(ctx, config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Cluster Firewall Alias", err.Error())
		return
	}
	current.CIDR = config.CIDR.ValueString()
	current.Comment = clusterFirewallManagedString(current.Comment, config.Comment, clusterFirewallFieldManaged(managed, "comment"))
	if err := r.client.UpdateClusterFirewallAlias(ctx, current); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Cluster Firewall Alias", err.Error())
		return
	}
	state, diags := r.readState(ctx, current.Name)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		fields := []string{}
		if !config.Comment.IsNull() && !config.Comment.IsUnknown() {
			fields = append(fields, "comment")
		}
		storeClusterFirewallManagedFields(ctx, resp.Private, clusterFirewallAliasManagedFieldsKey, fields, &resp.Diagnostics)
	}
}

func (r *ClusterFirewallAliasResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterFirewallAliasModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetClusterFirewallAlias(ctx, state.Name.ValueString())
	if errors.Is(err, errNotFound) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Cluster Firewall Alias", err.Error())
		return
	}
	if err := r.client.DeleteClusterFirewallAlias(ctx, current.Name, current.Digest); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Cluster Firewall Alias", err.Error())
	}
}

func (r *ClusterFirewallAliasResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if strings.TrimSpace(req.ID) == "" {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "expected a non-empty alias name")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

func (r *ClusterFirewallAliasResource) readState(ctx context.Context, name string) (clusterFirewallAliasModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	alias, err := r.client.GetClusterFirewallAlias(ctx, name)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return clusterFirewallAliasModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Cluster Firewall Alias", err.Error())
		return clusterFirewallAliasModel{}, diags
	}
	return clusterFirewallAliasModel{
		ID:      types.StringValue(alias.Name),
		Name:    types.StringValue(alias.Name),
		CIDR:    types.StringValue(alias.CIDR),
		Comment: stringOrNull(alias.Comment),
	}, diags
}
