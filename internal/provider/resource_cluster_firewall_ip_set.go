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

var _ resource.Resource = &ClusterFirewallIPSetResource{}
var _ resource.ResourceWithImportState = &ClusterFirewallIPSetResource{}

const clusterFirewallIPSetManagedFieldsKey = "cluster-firewall-ip-set-managed-fields"

type ClusterFirewallIPSetResource struct {
	client *Client
}

type clusterFirewallIPSetModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Comment types.String `tfsdk:"comment"`
}

func NewClusterFirewallIPSetResource() resource.Resource {
	return &ClusterFirewallIPSetResource{}
}

func (r *ClusterFirewallIPSetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_firewall_ip_set"
}

func (r *ClusterFirewallIPSetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a named cluster firewall IP set through `/cluster/firewall/ipset`. Entries are managed with `proxmox_cluster_firewall_ip_set_entry`.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, MarkdownDescription: "IP set name."},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Stable IP set name. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"comment": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "IP set comment."},
		},
	}
}

func (r *ClusterFirewallIPSetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ClusterFirewallIPSetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterFirewallIPSetModel
	var config clusterFirewallIPSetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	set := ClusterFirewallIPSet{Name: plan.Name.ValueString()}
	if !plan.Comment.IsNull() && !plan.Comment.IsUnknown() {
		set.Comment = plan.Comment.ValueString()
	}
	if err := r.client.CreateClusterFirewallIPSet(ctx, set); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Cluster Firewall IP Set", err.Error())
		return
	}
	state, diags := r.readState(ctx, set.Name)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		fields := []string{}
		if !config.Comment.IsNull() && !config.Comment.IsUnknown() {
			fields = append(fields, "comment")
		}
		storeClusterFirewallManagedFields(ctx, resp.Private, clusterFirewallIPSetManagedFieldsKey, fields, &resp.Diagnostics)
	}
}

func (r *ClusterFirewallIPSetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterFirewallIPSetModel
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

func (r *ClusterFirewallIPSetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config clusterFirewallIPSetModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	managed, privateDiags := readClusterFirewallManagedFields(ctx, req.Private, clusterFirewallIPSetManagedFieldsKey)
	resp.Diagnostics.Append(privateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetClusterFirewallIPSet(ctx, config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Cluster Firewall IP Set", err.Error())
		return
	}
	current.Comment = clusterFirewallManagedString(current.Comment, config.Comment, clusterFirewallFieldManaged(managed, "comment"))
	if err := r.client.UpdateClusterFirewallIPSet(ctx, current); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Cluster Firewall IP Set", err.Error())
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
		storeClusterFirewallManagedFields(ctx, resp.Private, clusterFirewallIPSetManagedFieldsKey, fields, &resp.Diagnostics)
	}
}

func (r *ClusterFirewallIPSetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterFirewallIPSetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteClusterFirewallIPSet(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Cluster Firewall IP Set", err.Error())
	}
}

func (r *ClusterFirewallIPSetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if strings.TrimSpace(req.ID) == "" {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "expected a non-empty IP set name")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

func (r *ClusterFirewallIPSetResource) readState(ctx context.Context, name string) (clusterFirewallIPSetModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	set, err := r.client.GetClusterFirewallIPSet(ctx, name)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return clusterFirewallIPSetModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Cluster Firewall IP Set", err.Error())
		return clusterFirewallIPSetModel{}, diags
	}
	return clusterFirewallIPSetModel{
		ID:      types.StringValue(set.Name),
		Name:    types.StringValue(set.Name),
		Comment: stringOrNull(set.Comment),
	}, diags
}
