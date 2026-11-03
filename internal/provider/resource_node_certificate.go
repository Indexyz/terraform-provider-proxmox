// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &NodeCertificateResource{}
var _ resource.ResourceWithImportState = &NodeCertificateResource{}
var _ resource.ResourceWithValidateConfig = &NodeCertificateResource{}

type NodeCertificateResource struct{ client *Client }

type nodeCertificateModel struct {
	ID            types.String `tfsdk:"id"`
	Node          types.String `tfsdk:"node"`
	Certificates  types.String `tfsdk:"certificates"`
	Key           types.String `tfsdk:"key"`
	Force         types.Bool   `tfsdk:"force"`
	Restart       types.Bool   `tfsdk:"restart"`
	Fingerprint   types.String `tfsdk:"fingerprint"`
	Issuer        types.String `tfsdk:"issuer"`
	Subject       types.String `tfsdk:"subject"`
	SAN           types.List   `tfsdk:"san"`
	NotAfter      types.String `tfsdk:"not_after"`
	NotBefore     types.String `tfsdk:"not_before"`
	PublicKeyType types.String `tfsdk:"public_key_type"`
	PublicKeyBits types.Int64  `tfsdk:"public_key_bits"`
}

func NewNodeCertificateResource() resource.Resource { return &NodeCertificateResource{} }

func (r *NodeCertificateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_certificate"
}

func (r *NodeCertificateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Installs a custom TLS certificate chain and key for one node through `/nodes/{node}/certificates/custom`, replacing the self-signed certificate. Destroy removes the custom certificate, so the node falls back to its previous one. The private key and chain are sensitive but can be read back from the API.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Computed: true, MarkdownDescription: "Node name."},
			"node":            schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"certificates":    schema.StringAttribute{Required: true, Sensitive: true, MarkdownDescription: "PEM encoded certificate chain, leaf first.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"key":             schema.StringAttribute{Required: true, Sensitive: true, MarkdownDescription: "PEM encoded private key.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"force":           schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Replace even if the new certificate is invalid (for example self-signed without a matching SAN).", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"restart":         schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Restart pveproxy after installing.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"fingerprint":     schema.StringAttribute{Computed: true, MarkdownDescription: "SHA-256 fingerprint of the installed certificate."},
			"issuer":          schema.StringAttribute{Computed: true, MarkdownDescription: "Issuer distinguished name."},
			"subject":         schema.StringAttribute{Computed: true, MarkdownDescription: "Subject distinguished name."},
			"san":             schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Subject alternative names."},
			"not_after":       schema.StringAttribute{Computed: true, MarkdownDescription: "End of validity."},
			"not_before":      schema.StringAttribute{Computed: true, MarkdownDescription: "Start of validity."},
			"public_key_type": schema.StringAttribute{Computed: true, MarkdownDescription: "Public key algorithm."},
			"public_key_bits": schema.Int64Attribute{Computed: true, MarkdownDescription: "Public key size."},
		},
	}
}

func (r *NodeCertificateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *NodeCertificateResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeCertificateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if isConfiguredValue(config.Node) && !proxmoxNodeNamePattern.MatchString(config.Node.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("node"), "Invalid node name", "node must be an existing cluster node name.")
	}
	if isConfiguredValue(config.Certificates) && !strings.Contains(config.Certificates.ValueString(), "BEGIN CERTIFICATE") {
		resp.Diagnostics.AddAttributeError(path.Root("certificates"), "Invalid certificate chain", "certificates must contain PEM encoded certificates.")
	}
	if isConfiguredValue(config.Key) && !strings.Contains(config.Key.ValueString(), "PRIVATE KEY") {
		resp.Diagnostics.AddAttributeError(path.Root("key"), "Invalid private key", "key must be a PEM encoded private key.")
	}
}

func (r *NodeCertificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nodeCertificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := plan.Node.ValueString()
	if err := r.client.UploadNodeCertificate(ctx, node, plan.Certificates.ValueString(), plan.Key.ValueString(), plan.Force.ValueBool(), plan.Restart.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Unable to Upload Proxmox Node Certificate", err.Error())
		return
	}
	state, diags := r.readState(ctx, node, plan.Certificates, plan.Key, plan.Force, plan.Restart)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NodeCertificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nodeCertificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.Node.ValueString(), state.Certificates, state.Key, state.Force, state.Restart)
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

func (r *NodeCertificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All inputs replace, so an update only refreshes state.
	var plan nodeCertificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, plan.Node.ValueString(), plan.Certificates, plan.Key, plan.Force, plan.Restart)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *NodeCertificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nodeCertificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetNodeCertificate(ctx, state.Node.ValueString(), nodeCustomCertificateFile)
	if errors.Is(err, errNotFound) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Node Certificate", err.Error())
		return
	}
	if err := r.client.DeleteNodeCertificate(ctx, state.Node.ValueString(), state.Restart.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Node Certificate", err.Error())
		return
	}
	_ = current
}

func (r *NodeCertificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node := strings.TrimSpace(req.ID)
	if !proxmoxNodeNamePattern.MatchString(node) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected a node name.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
}

func (r *NodeCertificateResource) readState(ctx context.Context, node string, priorCertificates, priorKey types.String, priorForce, priorRestart types.Bool) (nodeCertificateModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	certificate, err := r.client.GetNodeCertificate(ctx, node, nodeCustomCertificateFile)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nodeCertificateModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Node Certificate", err.Error())
		return nodeCertificateModel{}, diags
	}
	san := types.ListNull(types.StringType)
	if len(certificate.SAN) > 0 {
		value, listDiags := types.ListValueFrom(ctx, types.StringType, certificate.SAN)
		diags.Append(listDiags...)
		san = value
	}
	return nodeCertificateModel{
		ID:            types.StringValue(node),
		Node:          types.StringValue(node),
		Certificates:  priorCertificates,
		Key:           priorKey,
		Force:         priorForce,
		Restart:       priorRestart,
		Fingerprint:   stringOrNull(certificate.Fingerprint),
		Issuer:        stringOrNull(certificate.Issuer),
		Subject:       stringOrNull(certificate.Subject),
		SAN:           san,
		NotAfter:      certificateTime(certificate.NotAfter),
		NotBefore:     certificateTime(certificate.NotBefore),
		PublicKeyType: stringOrNull(certificate.PublicKeyType),
		PublicKeyBits: int64OrNull(&certificate.PublicKeyBits),
	}, diags
}

func certificateTime(epoch certificateEpoch) types.String {
	if epoch <= 0 {
		return types.StringNull()
	}
	return types.StringValue(time.Unix(int64(epoch), 0).UTC().Format(time.RFC3339))
}
