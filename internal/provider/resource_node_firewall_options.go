// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &NodeFirewallOptionsResource{}
var _ resource.ResourceWithImportState = &NodeFirewallOptionsResource{}

type NodeFirewallOptionsResource struct {
	client *Client
}

type nodeFirewallOptionsModel struct {
	ID                               types.String `tfsdk:"id"`
	Node                             types.String `tfsdk:"node"`
	Enable                           types.Bool   `tfsdk:"enable"`
	LogLevelIn                       types.String `tfsdk:"log_level_in"`
	LogLevelOut                      types.String `tfsdk:"log_level_out"`
	LogLevelForward                  types.String `tfsdk:"log_level_forward"`
	LogNFConntrack                   types.Bool   `tfsdk:"log_nf_conntrack"`
	NFConntrackAllowInvalid          types.Bool   `tfsdk:"nf_conntrack_allow_invalid"`
	NFConntrackMax                   types.Int64  `tfsdk:"nf_conntrack_max"`
	NFConntrackTCPTimeoutEstablished types.Int64  `tfsdk:"nf_conntrack_tcp_timeout_established"`
	NFConntrackTCPSynRecvTimeout     types.Int64  `tfsdk:"nf_conntrack_tcp_timeout_syn_recv"`
	NFConntrackHelpers               types.String `tfsdk:"nf_conntrack_helpers"`
	Ndp                              types.Bool   `tfsdk:"ndp"`
	Nosmurfs                         types.Bool   `tfsdk:"nosmurfs"`
	ProtectionSynflood               types.Bool   `tfsdk:"protection_synflood"`
	ProtectionSynfloodBurst          types.Int64  `tfsdk:"protection_synflood_burst"`
	ProtectionSynfloodRate           types.Int64  `tfsdk:"protection_synflood_rate"`
	SmurfLogLevel                    types.String `tfsdk:"smurf_log_level"`
	TCPFlagsLogLevel                 types.String `tfsdk:"tcp_flags_log_level"`
	TCPFlags                         types.Bool   `tfsdk:"tcp_flags"`
	Nftables                         types.Bool   `tfsdk:"nftables"`
}

func NewNodeFirewallOptionsResource() resource.Resource {
	return &NodeFirewallOptionsResource{}
}

func (r *NodeFirewallOptionsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_firewall_options"
}

func (r *NodeFirewallOptionsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Proxmox VE per-node firewall options through `/nodes/{node}/firewall/options`.",
		Attributes: map[string]schema.Attribute{
			"id":                                   schema.StringAttribute{Computed: true, MarkdownDescription: "The node name.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"node":                                 schema.StringAttribute{Required: true, MarkdownDescription: "Proxmox node name. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"enable":                               schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable the node firewall.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"log_level_in":                         schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Log level for incoming traffic."},
			"log_level_out":                        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Log level for outgoing traffic."},
			"log_level_forward":                    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Log level for forwarded traffic."},
			"log_nf_conntrack":                     schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable logging of conntrack information."},
			"nf_conntrack_allow_invalid":           schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Allow invalid packets in conntrack."},
			"nf_conntrack_max":                     schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Maximum number of conntrack entries."},
			"nf_conntrack_tcp_timeout_established": schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Conntrack established timeout in seconds."},
			"nf_conntrack_tcp_timeout_syn_recv":    schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Conntrack SYN-RECV timeout in seconds."},
			"nf_conntrack_helpers":                 schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Enabled conntrack helpers."},
			"ndp":                                  schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable NDP (Neighbor Discovery Protocol)."},
			"nosmurfs":                             schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable smurf logging."},
			"protection_synflood":                  schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable SYN flood protection."},
			"protection_synflood_burst":            schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "SYN flood burst limit."},
			"protection_synflood_rate":             schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "SYN flood rate limit."},
			"smurf_log_level":                      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Log level for smurf packets."},
			"tcp_flags_log_level":                  schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Log level for illegal TCP flags."},
			"tcp_flags":                            schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable TCP flags filtering."},
			"nftables":                             schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable nftables.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *NodeFirewallOptionsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func firewallOptionsRequestFromModel(plan nodeFirewallOptionsModel, prior nodeFirewallOptionsModel) NodeFirewallOptionsRequest {
	req := NodeFirewallOptionsRequest{
		Enable:                           boolPointerValue(plan.Enable),
		LogLevelIn:                       stringPointer(plan.LogLevelIn),
		LogLevelOut:                      stringPointer(plan.LogLevelOut),
		LogLevelForward:                  stringPointer(plan.LogLevelForward),
		LogNFConntrack:                   boolPointerValue(plan.LogNFConntrack),
		NFConntrackAllowInvalid:          boolPointerValue(plan.NFConntrackAllowInvalid),
		NFConntrackMax:                   int64PointerValue(plan.NFConntrackMax),
		NFConntrackTCPTimeoutEstablished: int64PointerValue(plan.NFConntrackTCPTimeoutEstablished),
		NFConntrackTCPSynRecvTimeout:     int64PointerValue(plan.NFConntrackTCPSynRecvTimeout),
		NFConntrackHelpers:               stringPointer(plan.NFConntrackHelpers),
		Ndp:                              boolPointerValue(plan.Ndp),
		Nosmurfs:                         boolPointerValue(plan.Nosmurfs),
		ProtectionSynflood:               boolPointerValue(plan.ProtectionSynflood),
		ProtectionSynfloodBurst:          int64PointerValue(plan.ProtectionSynfloodBurst),
		ProtectionSynfloodRate:           int64PointerValue(plan.ProtectionSynfloodRate),
		SmurfLogLevel:                    stringPointer(plan.SmurfLogLevel),
		TCPFlagsLogLevel:                 stringPointer(plan.TCPFlagsLogLevel),
		TCPFlags:                         boolPointerValue(plan.TCPFlags),
		Nftables:                         boolPointerValue(plan.Nftables),
	}
	return req
}

func firewallOptionsDeleteKeys(plan nodeFirewallOptionsModel, prior nodeFirewallOptionsModel) []string {
	var keys []string
	keys = appendDeletedString(keys, "log_level_in", plan.LogLevelIn, prior.LogLevelIn)
	keys = appendDeletedString(keys, "log_level_out", plan.LogLevelOut, prior.LogLevelOut)
	keys = appendDeletedString(keys, "log_level_forward", plan.LogLevelForward, prior.LogLevelForward)
	keys = appendDeletedString(keys, "nf_conntrack_helpers", plan.NFConntrackHelpers, prior.NFConntrackHelpers)
	keys = appendDeletedString(keys, "smurf_log_level", plan.SmurfLogLevel, prior.SmurfLogLevel)
	keys = appendDeletedString(keys, "tcp_flags_log_level", plan.TCPFlagsLogLevel, prior.TCPFlagsLogLevel)
	return keys
}

func (r *NodeFirewallOptionsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nodeFirewallOptionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateReq := firewallOptionsRequestFromModel(plan, nodeFirewallOptionsModel{})
	if err := r.client.UpdateNodeFirewallOptions(ctx, plan.Node.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError("Unable to Set Proxmox Node Firewall Options", err.Error())
		return
	}
	state, diags := r.readState(ctx, plan.Node.ValueString(), &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NodeFirewallOptionsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nodeFirewallOptionsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.Node.ValueString(), &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *NodeFirewallOptionsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan nodeFirewallOptionsModel
	var state nodeFirewallOptionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateReq := firewallOptionsRequestFromModel(plan, state)
	updateReq.Delete = firewallOptionsDeleteKeys(plan, state)
	if err := r.client.UpdateNodeFirewallOptions(ctx, plan.Node.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Node Firewall Options", err.Error())
		return
	}
	refreshed, diags := r.readState(ctx, plan.Node.ValueString(), &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *NodeFirewallOptionsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nodeFirewallOptionsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteNodeFirewallOptions(ctx, state.Node.ValueString()); err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Unable to Reset Proxmox Node Firewall Options", err.Error())
	}
}

func (r *NodeFirewallOptionsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *NodeFirewallOptionsResource) readState(ctx context.Context, node string, prior *nodeFirewallOptionsModel) (nodeFirewallOptionsModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	opts, err := r.client.GetNodeFirewallOptions(ctx, node)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nodeFirewallOptionsModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Node Firewall Options", fmt.Sprintf("Unable to read options for node %q: %s", node, err))
		return nodeFirewallOptionsModel{}, diags
	}
	return nodeFirewallOptionsModel{
		ID:                               types.StringValue(node),
		Node:                             types.StringValue(node),
		Enable:                           boolOrNull(opts.Enable.Ptr()),
		LogLevelIn:                       stringOrNull(opts.LogLevelIn),
		LogLevelOut:                      stringOrNull(opts.LogLevelOut),
		LogLevelForward:                  stringOrNull(opts.LogLevelForward),
		LogNFConntrack:                   boolOrNull(opts.LogNFConntrack.Ptr()),
		NFConntrackAllowInvalid:          boolOrNull(opts.NFConntrackAllowInvalid.Ptr()),
		NFConntrackMax:                   int64OrNull(opts.NFConntrackMax.Ptr()),
		NFConntrackTCPTimeoutEstablished: int64OrNull(opts.NFConntrackTCPTimeoutEstablished.Ptr()),
		NFConntrackTCPSynRecvTimeout:     int64OrNull(opts.NFConntrackTCPSynRecvTimeout.Ptr()),
		NFConntrackHelpers:               stringOrNull(opts.NFConntrackHelpers),
		Ndp:                              boolOrNull(opts.Ndp.Ptr()),
		Nosmurfs:                         boolOrNull(opts.Nosmurfs.Ptr()),
		ProtectionSynflood:               boolOrNull(opts.ProtectionSynflood.Ptr()),
		ProtectionSynfloodBurst:          int64OrNull(opts.ProtectionSynfloodBurst.Ptr()),
		ProtectionSynfloodRate:           int64OrNull(opts.ProtectionSynfloodRate.Ptr()),
		SmurfLogLevel:                    stringOrNull(opts.SmurfLogLevel),
		TCPFlagsLogLevel:                 stringOrNull(opts.TCPFlagsLogLevel),
		TCPFlags:                         boolOrNull(opts.TCPFlags.Ptr()),
		Nftables:                         boolOrNull(opts.Nftables.Ptr()),
	}, diags
}
