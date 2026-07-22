// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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

var _ resource.Resource = &GuestFirewallOptionsResource{}
var _ resource.ResourceWithImportState = &GuestFirewallOptionsResource{}

type GuestFirewallOptionsResource struct {
	client *Client
}

type guestFirewallOptionsModel struct {
	ID          types.String `tfsdk:"id"`
	Node        types.String `tfsdk:"node"`
	VMID        types.Int64  `tfsdk:"vm_id"`
	GuestType   types.String `tfsdk:"guest_type"`
	Enable      types.Bool   `tfsdk:"enable"`
	DHCP        types.Bool   `tfsdk:"dhcp"`
	IPFilter    types.Bool   `tfsdk:"ipfilter"`
	MACFilter   types.Bool   `tfsdk:"macfilter"`
	LogLevelIn  types.String `tfsdk:"log_level_in"`
	LogLevelOut types.String `tfsdk:"log_level_out"`
	PolicyIn    types.String `tfsdk:"policy_in"`
	PolicyOut   types.String `tfsdk:"policy_out"`
	NDP         types.Bool   `tfsdk:"ndp"`
	RADV        types.Bool   `tfsdk:"radv"`
}

func NewGuestFirewallOptionsResource() resource.Resource {
	return &GuestFirewallOptionsResource{}
}

func (r *GuestFirewallOptionsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_firewall_options"
}

func (r *GuestFirewallOptionsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Proxmox VE per-guest firewall options through `/nodes/{node}/{qemu|lxc}/{vmid}/firewall/options`. Applies to both QEMU VMs and LXC containers.",
		Attributes: map[string]schema.Attribute{
			"id":            schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `node/vm_id/guest_type` form."},
			"node":          schema.StringAttribute{Required: true, MarkdownDescription: "Proxmox node name. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"vm_id":         schema.Int64Attribute{Required: true, MarkdownDescription: "Numeric VMID of the guest. Changes require replacement.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"guest_type":    schema.StringAttribute{Required: true, MarkdownDescription: "Guest type: `qemu` or `lxc`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"enable":        schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable/disable firewall rules."},
			"dhcp":          schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable DHCP."},
			"ipfilter":      schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable default IP filters."},
			"macfilter":     schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable/disable MAC address filter (default: true)."},
			"log_level_in":  schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Log level for incoming traffic."},
			"log_level_out": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Log level for outgoing traffic."},
			"policy_in":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Input policy (DROP/ACCEPT/REJECT)."},
			"policy_out":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Output policy (DROP/ACCEPT/REJECT)."},
			"ndp":           schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable NDP (default: true)."},
			"radv":          schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Allow sending Router Advertisement."},
		},
	}
}

func (r *GuestFirewallOptionsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func guestFWRequestFromModel(model guestFirewallOptionsModel) GuestFirewallOptionsRequest {
	return GuestFirewallOptionsRequest{
		Enable:      boolPointerValue(model.Enable),
		DHCP:        boolPointerValue(model.DHCP),
		IPFilter:    boolPointerValue(model.IPFilter),
		MACFilter:   boolPointerValue(model.MACFilter),
		LogLevelIn:  stringPointer(model.LogLevelIn),
		LogLevelOut: stringPointer(model.LogLevelOut),
		PolicyIn:    stringPointer(model.PolicyIn),
		PolicyOut:   stringPointer(model.PolicyOut),
		NDP:         boolPointerValue(model.NDP),
		RADV:        boolPointerValue(model.RADV),
	}
}

func (r *GuestFirewallOptionsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan guestFirewallOptionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	kind, err := validateGuestType(plan.GuestType.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid guest_type", err.Error())
		return
	}
	if err := r.client.UpdateGuestFirewallOptions(ctx, kind, plan.Node.ValueString(), plan.VMID.ValueInt64(), guestFWRequestFromModel(plan)); err != nil {
		resp.Diagnostics.AddError("Unable to Set Proxmox Guest Firewall Options", err.Error())
		return
	}
	state, diags := r.readState(ctx, kind, plan.Node.ValueString(), plan.VMID.ValueInt64())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *GuestFirewallOptionsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state guestFirewallOptionsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	kind := state.GuestType.ValueString()
	refreshed, diags := r.readState(ctx, kind, state.Node.ValueString(), state.VMID.ValueInt64())
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

func (r *GuestFirewallOptionsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan guestFirewallOptionsModel
	var state guestFirewallOptionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	kind := plan.GuestType.ValueString()
	if err := r.client.UpdateGuestFirewallOptions(ctx, kind, plan.Node.ValueString(), plan.VMID.ValueInt64(), guestFWRequestFromModel(plan)); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Guest Firewall Options", err.Error())
		return
	}
	refreshed, diags := r.readState(ctx, kind, plan.Node.ValueString(), plan.VMID.ValueInt64())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *GuestFirewallOptionsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state guestFirewallOptionsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	kind := state.GuestType.ValueString()
	if err := r.client.ResetGuestFirewallOptions(ctx, kind, state.Node.ValueString(), state.VMID.ValueInt64()); err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Unable to Reset Proxmox Guest Firewall Options", err.Error())
	}
}

func (r *GuestFirewallOptionsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "expected identifier in node/vm_id/guest_type form")
		return
	}
	vmID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Import Identifier", fmt.Sprintf("expected vm_id in node/vm_id/guest_type to be an integer: %v", err))
		return
	}
	guestType, err := validateGuestType(parts[2])
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Import Identifier", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vm_id"), vmID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("guest_type"), guestType)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func validateGuestType(t string) (string, error) {
	t = strings.ToLower(strings.TrimSpace(t))
	if t != "qemu" && t != "lxc" {
		return "", fmt.Errorf("guest_type must be 'qemu' or 'lxc', got %q", t)
	}
	return t, nil
}

func (r *GuestFirewallOptionsResource) readState(ctx context.Context, kind, node string, vmID int64) (guestFirewallOptionsModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	opts, err := r.client.GetGuestFirewallOptions(ctx, kind, node, vmID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return guestFirewallOptionsModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Guest Firewall Options", err.Error())
		return guestFirewallOptionsModel{}, diags
	}
	return guestFirewallOptionsModel{
		ID:          types.StringValue(fmt.Sprintf("%s/%d/%s", node, vmID, kind)),
		Node:        types.StringValue(node),
		VMID:        types.Int64Value(vmID),
		GuestType:   types.StringValue(kind),
		Enable:      boolOrNull(opts.Enable.Ptr()),
		DHCP:        boolOrNull(opts.DHCP.Ptr()),
		IPFilter:    boolOrNull(opts.IPFilter.Ptr()),
		MACFilter:   boolOrNull(opts.MACFilter.Ptr()),
		LogLevelIn:  stringOrNull(opts.LogLevelIn),
		LogLevelOut: stringOrNull(opts.LogLevelOut),
		PolicyIn:    stringOrNull(opts.PolicyIn),
		PolicyOut:   stringOrNull(opts.PolicyOut),
		NDP:         boolOrNull(opts.NDP.Ptr()),
		RADV:        boolOrNull(opts.RADV.Ptr()),
	}, diags
}
