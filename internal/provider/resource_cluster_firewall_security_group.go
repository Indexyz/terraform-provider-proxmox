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

var _ resource.Resource = &ClusterFirewallSecurityGroupResource{}
var _ resource.ResourceWithImportState = &ClusterFirewallSecurityGroupResource{}

const clusterFirewallSecurityGroupManagedFieldsKey = "cluster-firewall-security-group-managed-fields"

type ClusterFirewallSecurityGroupResource struct {
	client *Client
}

type clusterFirewallSecurityGroupModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Comment types.String `tfsdk:"comment"`
}

func NewClusterFirewallSecurityGroupResource() resource.Resource {
	return &ClusterFirewallSecurityGroupResource{}
}

func (r *ClusterFirewallSecurityGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_firewall_security_group"
}

func (r *ClusterFirewallSecurityGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a named cluster firewall security group. Add group rules with `proxmox_firewall_rule` using `scope = \"security_group\"`.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, MarkdownDescription: "Security group name."},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Stable security group name. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"comment": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Security group comment."},
		},
	}
}

func (r *ClusterFirewallSecurityGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ClusterFirewallSecurityGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterFirewallSecurityGroupModel
	var config clusterFirewallSecurityGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	group := ClusterFirewallSecurityGroup{Name: plan.Name.ValueString()}
	if !plan.Comment.IsNull() && !plan.Comment.IsUnknown() {
		group.Comment = plan.Comment.ValueString()
	}
	if err := r.client.CreateClusterFirewallSecurityGroup(ctx, group); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Cluster Firewall Security Group", err.Error())
		return
	}
	state, diags := r.readState(ctx, group.Name)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		fields := []string{}
		if !config.Comment.IsNull() && !config.Comment.IsUnknown() {
			fields = append(fields, "comment")
		}
		storeClusterFirewallManagedFields(ctx, resp.Private, clusterFirewallSecurityGroupManagedFieldsKey, fields, &resp.Diagnostics)
	}
}

func (r *ClusterFirewallSecurityGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterFirewallSecurityGroupModel
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

func (r *ClusterFirewallSecurityGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config clusterFirewallSecurityGroupModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	managed, privateDiags := readClusterFirewallManagedFields(ctx, req.Private, clusterFirewallSecurityGroupManagedFieldsKey)
	resp.Diagnostics.Append(privateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetClusterFirewallSecurityGroup(ctx, config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Cluster Firewall Security Group", err.Error())
		return
	}
	current.Comment = clusterFirewallManagedString(current.Comment, config.Comment, clusterFirewallFieldManaged(managed, "comment"))
	if err := r.client.UpdateClusterFirewallSecurityGroup(ctx, current); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Cluster Firewall Security Group", err.Error())
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
		storeClusterFirewallManagedFields(ctx, resp.Private, clusterFirewallSecurityGroupManagedFieldsKey, fields, &resp.Diagnostics)
	}
}

func (r *ClusterFirewallSecurityGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterFirewallSecurityGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteClusterFirewallSecurityGroup(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Cluster Firewall Security Group", err.Error())
	}
}

func (r *ClusterFirewallSecurityGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if strings.TrimSpace(req.ID) == "" {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "expected a non-empty security group name")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

func (r *ClusterFirewallSecurityGroupResource) readState(ctx context.Context, name string) (clusterFirewallSecurityGroupModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	group, err := r.client.GetClusterFirewallSecurityGroup(ctx, name)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return clusterFirewallSecurityGroupModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Cluster Firewall Security Group", err.Error())
		return clusterFirewallSecurityGroupModel{}, diags
	}
	return clusterFirewallSecurityGroupModel{
		ID:      types.StringValue(group.Name),
		Name:    types.StringValue(group.Name),
		Comment: stringOrNull(group.Comment),
	}, diags
}
