// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &NodeACMECertificateResource{}
var _ resource.ResourceWithImportState = &NodeACMECertificateResource{}
var _ resource.ResourceWithValidateConfig = &NodeACMECertificateResource{}

type NodeACMECertificateResource struct{ client *Client }

type nodeACMECertificateModel struct {
	ID            types.String `tfsdk:"id"`
	Node          types.String `tfsdk:"node"`
	Account       types.String `tfsdk:"account"`
	Domains       types.Set    `tfsdk:"domains"`
	DomainPlugins types.Map    `tfsdk:"domain_plugins"`
	Force         types.Bool   `tfsdk:"force"`
	Triggers      types.Map    `tfsdk:"triggers"`
	UPID          types.String `tfsdk:"upid"`
}

func NewNodeACMECertificateResource() resource.Resource { return &NodeACMECertificateResource{} }

func (r *NodeACMECertificateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_acme_certificate"
}

func (r *NodeACMECertificateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Orders an ACME certificate for one node through `/nodes/{node}/certificates/acme/certificate` and waits for the order task. The order needs an ACME account, an ACME plugin for the challenge and a node domain that resolves publicly. Every attribute change replaces the instance and orders anew; destroy revokes the certificate. Changing the certificate does not restart pveproxy itself.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Certificate identifier (the node name).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"node":           schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node to order a certificate for. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"account":        schema.StringAttribute{Required: true, MarkdownDescription: "ACME account name used for the order. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"domains":        schema.SetAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Domains to put into the certificate. Must resolve publicly to this node. Changes require replacement."},
			"domain_plugins": schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Optional map of domain to ACME plugin id for the challenge; unmapped domains fall back to the wildcard plugin."},
			"force":          schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Overwrite an existing certificate.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"triggers":       schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when a new order is due; changing any of them orders again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"upid":           schema.StringAttribute{Computed: true, MarkdownDescription: "Worker task UPID of the last order."},
		},
	}
}

func (r *NodeACMECertificateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *NodeACMECertificateResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeACMECertificateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if isConfiguredValue(config.Node) && !proxmoxNodeNamePattern.MatchString(config.Node.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("node"), "Invalid node name", "node must be an existing cluster node name.")
	}
	if isConfiguredValue(config.Account) && !notificationNamePattern.MatchString(config.Account.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("account"), "Invalid ACME account name", "account must be a valid configuration id.")
	}
	if isConfiguredValue(config.Domains) && len(config.Domains.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(path.Root("domains"), "Missing ACME domains", "domains needs at least one domain.")
	}
	if isConfiguredValue(config.Triggers) && len(config.Triggers.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(path.Root("triggers"), "Missing ACME order triggers", "triggers must contain at least one value so a re-order can be triggered.")
	}
}

func (r *NodeACMECertificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nodeACMECertificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.order(ctx, &plan, &resp.Diagnostics, &resp.State)
}

func (r *NodeACMECertificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// The order is a command; keep the recorded state without querying the CA.
}

func (r *NodeACMECertificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan nodeACMECertificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.order(ctx, &plan, &resp.Diagnostics, &resp.State)
}

func (r *NodeACMECertificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nodeACMECertificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := state.Node.ValueString()
	upid, err := r.client.RevokeNodeACMECertificate(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Revoke Proxmox ACME Certificate", err.Error())
		return
	}
	if err := r.client.waitForNodeTask(ctx, node, upid); err != nil {
		resp.Diagnostics.AddError("Unable to Revoke Proxmox ACME Certificate", fmt.Sprintf("revoke task %q did not finish: %s", upid, err))
	}
}

func (r *NodeACMECertificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node := strings.TrimSpace(req.ID)
	if !proxmoxNodeNamePattern.MatchString(node) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected a node name.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
}

func (r *NodeACMECertificateResource) order(ctx context.Context, plan *nodeACMECertificateModel, diags *diag.Diagnostics, state *tfsdk.State) {
	node := plan.Node.ValueString()
	plugins, pluginDiags := expandNodeACMEDomainPlugins(ctx, plan.DomainPlugins)
	diags.Append(pluginDiags...)
	if diags.HasError() {
		return
	}
	acme := "account=" + plan.Account.ValueString()
	// Per-domain entries carry the domains when plugins are configured; adding
	// them to the account string as well trips Proxmox duplicate detection.
	if len(plugins) == 0 {
		if domains := stringSetValues(plan.Domains); len(domains) > 0 {
			acme += ",domains=" + strings.Join(domains, ";")
		}
	}
	settings, err := r.client.NodeACMESettings(ctx, node)
	if err != nil {
		diags.AddError("Unable to Read Proxmox Node Config", err.Error())
		return
	}
	if err := r.client.ConfigureNodeACME(ctx, node, acme, plugins, settings.Digest); err != nil {
		diags.AddError("Unable to Configure Proxmox Node ACME Settings", err.Error())
		return
	}
	upid, err := r.client.OrderNodeACMECertificate(ctx, node, plan.Force.ValueBool())
	if err != nil {
		diags.AddError("Unable to Order Proxmox ACME Certificate", err.Error())
		return
	}
	if err := r.client.waitForNodeTask(ctx, node, upid); err != nil {
		diags.AddError("Proxmox ACME Certificate Order Failed", fmt.Sprintf("order task %q did not finish: %s", upid, err))
		return
	}
	next := *plan
	next.ID = types.StringValue(node)
	next.UPID = types.StringValue(upid)
	diags.Append(state.Set(ctx, &next)...)
}

var _ = tfsdk.State{}

func expandNodeACMEDomainPlugins(ctx context.Context, value types.Map) (map[string]string, diag.Diagnostics) {
	plugins := map[string]string{}
	if !isConfiguredValue(value) {
		return plugins, nil
	}
	return plugins, value.ElementsAs(ctx, &plugins, false)
}
