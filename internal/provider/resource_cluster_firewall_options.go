// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &ClusterFirewallOptionsResource{}
var _ resource.ResourceWithImportState = &ClusterFirewallOptionsResource{}

type ClusterFirewallOptionsResource struct {
	client *Client
}

type clusterFirewallOptionsModel struct {
	ID            types.String `tfsdk:"id"`
	Enable        types.Bool   `tfsdk:"enable"`
	Ebtables      types.Bool   `tfsdk:"ebtables"`
	LogRateLimit  types.String `tfsdk:"log_ratelimit"`
	PolicyForward types.String `tfsdk:"policy_forward"`
	PolicyIn      types.String `tfsdk:"policy_in"`
	PolicyOut     types.String `tfsdk:"policy_out"`
}

func NewClusterFirewallOptionsResource() resource.Resource {
	return &ClusterFirewallOptionsResource{}
}

func (r *ClusterFirewallOptionsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_firewall_options"
}

func (r *ClusterFirewallOptionsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Proxmox VE cluster-wide firewall options through `/cluster/firewall/options`.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "The fixed identifier `cluster`."},
			"enable":         schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable the firewall cluster-wide."},
			"ebtables":       schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable ebtables rules cluster-wide."},
			"log_ratelimit":  schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Proxmox log rate-limit property string, for example `enable=1,burst=5,rate=1/second`."},
			"policy_forward": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Default forwarded traffic policy: `ACCEPT` or `DROP`."},
			"policy_in":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Default incoming traffic policy: `ACCEPT`, `REJECT`, or `DROP`."},
			"policy_out":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Default outgoing traffic policy: `ACCEPT`, `REJECT`, or `DROP`."},
		},
	}
}

func (r *ClusterFirewallOptionsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func clusterFirewallOptionsRequestFromModel(model clusterFirewallOptionsModel) ClusterFirewallOptionsRequest {
	return ClusterFirewallOptionsRequest{
		Enable:        boolPointerValue(model.Enable),
		Ebtables:      boolPointerValue(model.Ebtables),
		LogRateLimit:  stringPointer(model.LogRateLimit),
		PolicyForward: stringPointer(model.PolicyForward),
		PolicyIn:      stringPointer(model.PolicyIn),
		PolicyOut:     stringPointer(model.PolicyOut),
	}
}

func clusterFirewallOptionsDeleteKeys(plan, prior clusterFirewallOptionsModel) []string {
	var keys []string
	keys = appendDeletedBool(keys, "enable", plan.Enable, prior.Enable)
	keys = appendDeletedBool(keys, "ebtables", plan.Ebtables, prior.Ebtables)
	keys = appendDeletedString(keys, "log_ratelimit", plan.LogRateLimit, prior.LogRateLimit)
	keys = appendDeletedString(keys, "policy_forward", plan.PolicyForward, prior.PolicyForward)
	keys = appendDeletedString(keys, "policy_in", plan.PolicyIn, prior.PolicyIn)
	keys = appendDeletedString(keys, "policy_out", plan.PolicyOut, prior.PolicyOut)
	return keys
}

func (r *ClusterFirewallOptionsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterFirewallOptionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateClusterFirewallOptions(ctx, clusterFirewallOptionsRequestFromModel(plan)); err != nil {
		resp.Diagnostics.AddError("Unable to Set Proxmox Cluster Firewall Options", err.Error())
		return
	}
	state, diags := r.readState(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ClusterFirewallOptionsResource) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	state, diags := r.readState(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.ID.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ClusterFirewallOptionsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config clusterFirewallOptionsModel
	var prior clusterFirewallOptionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateReq := clusterFirewallOptionsRequestFromModel(config)
	updateReq.Delete = clusterFirewallOptionsDeleteKeys(config, prior)
	if err := r.client.UpdateClusterFirewallOptions(ctx, updateReq); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Cluster Firewall Options", err.Error())
		return
	}
	state, diags := r.readState(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ClusterFirewallOptionsResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.ResetClusterFirewallOptions(ctx); err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Unable to Reset Proxmox Cluster Firewall Options", err.Error())
	}
}

func (r *ClusterFirewallOptionsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID != "cluster" {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "expected identifier cluster")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), "cluster")...)
}

func (r *ClusterFirewallOptionsResource) readState(ctx context.Context) (clusterFirewallOptionsModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	options, err := r.client.GetClusterFirewallOptions(ctx)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return clusterFirewallOptionsModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Cluster Firewall Options", err.Error())
		return clusterFirewallOptionsModel{}, diags
	}
	return clusterFirewallOptionsModel{
		ID:            types.StringValue("cluster"),
		Enable:        boolOrNull(options.Enable.Ptr()),
		Ebtables:      boolOrNull(options.Ebtables.Ptr()),
		LogRateLimit:  stringOrNull(options.LogRateLimit),
		PolicyForward: stringOrNull(options.PolicyForward),
		PolicyIn:      stringOrNull(options.PolicyIn),
		PolicyOut:     stringOrNull(options.PolicyOut),
	}, diags
}
