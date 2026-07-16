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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &ClusterFirewallIPSetEntryResource{}
var _ resource.ResourceWithImportState = &ClusterFirewallIPSetEntryResource{}

const clusterFirewallIPSetEntryManagedFieldsKey = "cluster-firewall-ip-set-entry-managed-fields"

type ClusterFirewallIPSetEntryResource struct {
	client *Client
}

type clusterFirewallIPSetEntryModel struct {
	ID      types.String `tfsdk:"id"`
	IPSet   types.String `tfsdk:"ip_set"`
	CIDR    types.String `tfsdk:"cidr"`
	Comment types.String `tfsdk:"comment"`
	NoMatch types.Bool   `tfsdk:"nomatch"`
}

func NewClusterFirewallIPSetEntryResource() resource.Resource {
	return &ClusterFirewallIPSetEntryResource{}
}

func (r *ClusterFirewallIPSetEntryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_firewall_ip_set_entry"
}

func (r *ClusterFirewallIPSetEntryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an address, network, or alias entry in a cluster firewall IP set.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, MarkdownDescription: "Composite identifier in `ip_set/cidr` form."},
			"ip_set":  schema.StringAttribute{Required: true, MarkdownDescription: "Parent cluster firewall IP set name. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"cidr":    schema.StringAttribute{Required: true, MarkdownDescription: "IPv4/IPv6 address, CIDR network, or alias. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"comment": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Entry comment."},
			"nomatch": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Exclude this entry from the IP set match."},
		},
	}
}

func (r *ClusterFirewallIPSetEntryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ClusterFirewallIPSetEntryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterFirewallIPSetEntryModel
	var config clusterFirewallIPSetEntryModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entry := ClusterFirewallIPSetEntry{CIDR: plan.CIDR.ValueString()}
	if !plan.Comment.IsNull() && !plan.Comment.IsUnknown() {
		entry.Comment = plan.Comment.ValueString()
	}
	if !plan.NoMatch.IsNull() && !plan.NoMatch.IsUnknown() {
		entry.NoMatch = proxmoxOptionalBool{value: boolPointerValue(plan.NoMatch)}
	}
	if err := r.client.CreateClusterFirewallIPSetEntry(ctx, plan.IPSet.ValueString(), entry); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Cluster Firewall IP Set Entry", err.Error())
		return
	}
	state, diags := r.readState(ctx, plan.IPSet.ValueString(), entry.CIDR)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		fields := []string{}
		if !config.Comment.IsNull() && !config.Comment.IsUnknown() {
			fields = append(fields, "comment")
		}
		if !config.NoMatch.IsNull() && !config.NoMatch.IsUnknown() {
			fields = append(fields, "nomatch")
		}
		storeClusterFirewallManagedFields(ctx, resp.Private, clusterFirewallIPSetEntryManagedFieldsKey, fields, &resp.Diagnostics)
	}
}

func (r *ClusterFirewallIPSetEntryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterFirewallIPSetEntryModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.IPSet.ValueString(), state.CIDR.ValueString())
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

func (r *ClusterFirewallIPSetEntryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config clusterFirewallIPSetEntryModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	managed, privateDiags := readClusterFirewallManagedFields(ctx, req.Private, clusterFirewallIPSetEntryManagedFieldsKey)
	resp.Diagnostics.Append(privateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetClusterFirewallIPSetEntry(ctx, config.IPSet.ValueString(), config.CIDR.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Cluster Firewall IP Set Entry", err.Error())
		return
	}
	current.Comment = clusterFirewallManagedString(current.Comment, config.Comment, clusterFirewallFieldManaged(managed, "comment"))
	current.NoMatch = clusterFirewallManagedBool(current.NoMatch, config.NoMatch, clusterFirewallFieldManaged(managed, "nomatch"))
	if err := r.client.UpdateClusterFirewallIPSetEntry(ctx, config.IPSet.ValueString(), current); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Cluster Firewall IP Set Entry", err.Error())
		return
	}
	state, diags := r.readState(ctx, config.IPSet.ValueString(), current.CIDR)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		fields := []string{}
		if !config.Comment.IsNull() && !config.Comment.IsUnknown() {
			fields = append(fields, "comment")
		}
		if !config.NoMatch.IsNull() && !config.NoMatch.IsUnknown() {
			fields = append(fields, "nomatch")
		}
		storeClusterFirewallManagedFields(ctx, resp.Private, clusterFirewallIPSetEntryManagedFieldsKey, fields, &resp.Diagnostics)
	}
}

func (r *ClusterFirewallIPSetEntryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterFirewallIPSetEntryModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entry, err := r.client.GetClusterFirewallIPSetEntry(ctx, state.IPSet.ValueString(), state.CIDR.ValueString())
	if errors.Is(err, errNotFound) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Cluster Firewall IP Set Entry", err.Error())
		return
	}
	if err := r.client.DeleteClusterFirewallIPSetEntry(ctx, state.IPSet.ValueString(), state.CIDR.ValueString(), entry.Digest); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Cluster Firewall IP Set Entry", err.Error())
	}
}

func (r *ClusterFirewallIPSetEntryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	name, cidr, ok := strings.Cut(req.ID, "/")
	if !ok || strings.TrimSpace(name) == "" || strings.TrimSpace(cidr) == "" {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "expected identifier in ip_set/cidr form")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip_set"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cidr"), cidr)...)
}

func (r *ClusterFirewallIPSetEntryResource) readState(ctx context.Context, name, cidr string) (clusterFirewallIPSetEntryModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	entry, err := r.client.GetClusterFirewallIPSetEntry(ctx, name, cidr)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return clusterFirewallIPSetEntryModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Cluster Firewall IP Set Entry", err.Error())
		return clusterFirewallIPSetEntryModel{}, diags
	}
	nomatch := false
	if entry.NoMatch.Ptr() != nil {
		nomatch = *entry.NoMatch.Ptr()
	}
	return clusterFirewallIPSetEntryModel{
		ID:      types.StringValue(fmt.Sprintf("%s/%s", name, entry.CIDR)),
		IPSet:   types.StringValue(name),
		CIDR:    types.StringValue(entry.CIDR),
		Comment: stringOrNull(entry.Comment),
		NoMatch: types.BoolValue(nomatch),
	}, diags
}
