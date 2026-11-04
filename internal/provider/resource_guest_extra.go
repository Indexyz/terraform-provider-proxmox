// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource = &GuestStatusActionResource{}
	_ resource.Resource = &GuestTemplateResource{}
	_ resource.Resource = &GuestCloudinitResource{}
	_ resource.Resource = &GuestDBusVMStateResource{}
	_ resource.Resource = &GuestUnlinkResource{}
	_ resource.Resource = &LXCResizeResource{}
)

type GuestStatusActionResource struct{ client *Client }
type GuestTemplateResource struct{ client *Client }
type GuestCloudinitResource struct{ client *Client }
type GuestDBusVMStateResource struct{ client *Client }
type GuestUnlinkResource struct{ client *Client }
type LXCResizeResource struct{ client *Client }

type guestStatusActionModel struct {
	ID           types.String `tfsdk:"id"`
	Type         types.String `tfsdk:"type"`
	Node         types.String `tfsdk:"node"`
	VMID         types.Int64  `tfsdk:"vmid"`
	Action       types.String `tfsdk:"action"`
	Timeout      types.Int64  `tfsdk:"timeout"`
	ForceStop    types.Int64  `tfsdk:"force_stop"`
	ToDisk       types.Bool   `tfsdk:"to_disk"`
	StateStorage types.String `tfsdk:"state_storage"`
	UPID         types.String `tfsdk:"upid"`
	Triggers     types.Map    `tfsdk:"triggers"`
}
type guestTemplateModel struct {
	ID       types.String `tfsdk:"id"`
	Type     types.String `tfsdk:"type"`
	Node     types.String `tfsdk:"node"`
	VMID     types.Int64  `tfsdk:"vmid"`
	Disks    types.List   `tfsdk:"disks"`
	Triggers types.Map    `tfsdk:"triggers"`
}
type guestCloudinitModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	VMID     types.Int64  `tfsdk:"vmid"`
	Triggers types.Map    `tfsdk:"triggers"`
}
type guestDBusModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	VMID     types.Int64  `tfsdk:"vmid"`
	Action   types.String `tfsdk:"action"`
	Triggers types.Map    `tfsdk:"triggers"`
}
type guestUnlinkModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	VMID     types.Int64  `tfsdk:"vmid"`
	IDList   types.List   `tfsdk:"idlist"`
	Force    types.Bool   `tfsdk:"force"`
	Triggers types.Map    `tfsdk:"triggers"`
}
type lxcResizeModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	VMID     types.Int64  `tfsdk:"vmid"`
	Disk     types.String `tfsdk:"disk"`
	Size     types.String `tfsdk:"size"`
	Digest   types.String `tfsdk:"digest"`
	Triggers types.Map    `tfsdk:"triggers"`
}

func NewGuestStatusActionResource() resource.Resource { return &GuestStatusActionResource{} }
func NewGuestTemplateResource() resource.Resource     { return &GuestTemplateResource{} }
func NewGuestCloudinitResource() resource.Resource    { return &GuestCloudinitResource{} }
func NewGuestDBusVMStateResource() resource.Resource  { return &GuestDBusVMStateResource{} }
func NewGuestUnlinkResource() resource.Resource       { return &GuestUnlinkResource{} }
func NewLXCResizeResource() resource.Resource         { return &LXCResizeResource{} }

func (r *GuestStatusActionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_status_action"
}
func (r *GuestTemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_template"
}
func (r *GuestCloudinitResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_cloudinit"
}
func (r *GuestDBusVMStateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_dbus_vmstate"
}
func (r *GuestUnlinkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_unlink"
}
func (r *LXCResizeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lxc_resize"
}

func guestTypeAttribute() schema.StringAttribute {
	return schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
}

func (r *GuestStatusActionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Runs one guest power action (`start`, `stop`, `shutdown`, `suspend`, `resume`, `reboot`, or QEMU-only `reset`) and waits for the node task. Unlike the declarative `power` attribute this resource re-runs whenever `triggers` changes.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true}, "type": guestTypeAttribute(),
		"node":          schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"vmid":          schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"action":        schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"timeout":       schema.Int64Attribute{Optional: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"force_stop":    schema.Int64Attribute{Optional: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"to_disk":       schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"state_storage": schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"upid":          schema.StringAttribute{Computed: true},
		"triggers":      schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifierRequiresReplace()}},
	}}
}
func (r *GuestTemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Converts a guest into a template through `POST /nodes/{node}/{type}/{vmid}/template`. Destroy does not revert the template flag.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true}, "type": guestTypeAttribute(),
		"node":     schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"vmid":     schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"disks":    schema.ListAttribute{Optional: true, ElementType: types.StringType, PlanModifiers: []planmodifier.List{listPlanModifierRequiresReplace()}},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifierRequiresReplace()}},
	}}
}
func (r *GuestCloudinitResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Regenerates the QEMU cloud-init drive through `PUT /nodes/{node}/qemu/{vmid}/cloudinit`. The guest must be stopped.", Attributes: map[string]schema.Attribute{
		"id":       schema.StringAttribute{Computed: true},
		"node":     schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"vmid":     schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifierRequiresReplace()}},
	}}
}
func (r *GuestDBusVMStateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Starts or stops the QEMU DBus VMState helper through `POST /nodes/{node}/qemu/{vmid}/dbus-vmstate`. The API requires the VM PID to be visible on the addressed node.", Attributes: map[string]schema.Attribute{
		"id":       schema.StringAttribute{Computed: true},
		"node":     schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"vmid":     schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"action":   schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifierRequiresReplace()}},
	}}
}
func (r *GuestUnlinkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Unlinks disks from a QEMU guest config through `PUT /nodes/{node}/qemu/{vmid}/unlink` without deleting the underlying volumes.", Attributes: map[string]schema.Attribute{
		"id":       schema.StringAttribute{Computed: true},
		"node":     schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"vmid":     schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"idlist":   schema.ListAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.List{listPlanModifierRequiresReplace()}},
		"force":    schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifierRequiresReplace()}},
	}}
}
func (r *LXCResizeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Resizes an LXC mount point through `PUT /nodes/{node}/lxc/{vmid}/resize`.", Attributes: map[string]schema.Attribute{
		"id":       schema.StringAttribute{Computed: true},
		"node":     schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"vmid":     schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"disk":     schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"size":     schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"digest":   schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifierRequiresReplace()}},
	}}
}

func (r *GuestStatusActionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *GuestTemplateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *GuestCloudinitResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *GuestDBusVMStateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *GuestUnlinkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *LXCResizeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *GuestStatusActionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan guestStatusActionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	kind := plan.Type.ValueString()
	action := plan.Action.ValueString()
	if kind != "qemu" && kind != "lxc" {
		resp.Diagnostics.AddError("Invalid Guest Type", "type must be qemu or lxc")
		return
	}
	valid := map[string][]string{"qemu": {"start", "stop", "shutdown", "suspend", "resume", "reboot", "reset"}, "lxc": {"start", "stop", "shutdown", "suspend", "resume", "reboot"}}
	found := false
	for _, candidate := range valid[kind] {
		if candidate == action {
			found = true
			break
		}
	}
	if !found {
		resp.Diagnostics.AddError("Invalid Guest Action", action+" is not supported for "+kind)
		return
	}
	vmid := formatInt64(plan.VMID.ValueInt64())
	upid, err := r.client.GuestStatusAction(ctx, plan.Node.ValueString(), kind, vmid, action, plan.Timeout.ValueInt64(), plan.ForceStop.ValueInt64(), boolInt64(plan.ToDisk.ValueBool()), stringValue(plan.StateStorage))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Run Guest Action", err.Error())
		return
	}
	if upid != "" && !waitTask(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	plan.ID = types.StringValue(kind + "/" + vmid + "/" + action)
	plan.UPID = stringOrNull(upid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *GuestStatusActionResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *GuestStatusActionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan guestStatusActionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vmid := formatInt64(plan.VMID.ValueInt64())
	upid, err := r.client.GuestStatusAction(ctx, plan.Node.ValueString(), plan.Type.ValueString(), vmid, plan.Action.ValueString(), plan.Timeout.ValueInt64(), plan.ForceStop.ValueInt64(), boolInt64(plan.ToDisk.ValueBool()), stringValue(plan.StateStorage))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Run Guest Action", err.Error())
		return
	}
	if upid != "" && !waitTask(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	plan.ID = types.StringValue(plan.Type.ValueString() + "/" + vmid + "/" + plan.Action.ValueString())
	plan.UPID = stringOrNull(upid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *GuestStatusActionResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *GuestTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan guestTemplateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	disks := listStrings(plan.Disks)
	if err := r.client.GuestTemplate(ctx, plan.Node.ValueString(), plan.Type.ValueString(), formatInt64(plan.VMID.ValueInt64()), disks); err != nil {
		resp.Diagnostics.AddError("Unable to Create Guest Template", err.Error())
		return
	}
	plan.ID = types.StringValue(plan.Type.ValueString() + "/" + formatInt64(plan.VMID.ValueInt64()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *GuestTemplateResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *GuestTemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan guestTemplateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.GuestTemplate(ctx, plan.Node.ValueString(), plan.Type.ValueString(), formatInt64(plan.VMID.ValueInt64()), listStrings(plan.Disks)); err != nil {
		resp.Diagnostics.AddError("Unable to Create Guest Template", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *GuestTemplateResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *GuestCloudinitResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan guestCloudinitModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.GuestCloudinitRegenerate(ctx, plan.Node.ValueString(), formatInt64(plan.VMID.ValueInt64())); err != nil {
		resp.Diagnostics.AddError("Unable to Regenerate Cloud-Init", err.Error())
		return
	}
	plan.ID = types.StringValue("qemu/" + formatInt64(plan.VMID.ValueInt64()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *GuestCloudinitResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *GuestCloudinitResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan guestCloudinitModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.GuestCloudinitRegenerate(ctx, plan.Node.ValueString(), formatInt64(plan.VMID.ValueInt64())); err != nil {
		resp.Diagnostics.AddError("Unable to Regenerate Cloud-Init", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *GuestCloudinitResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *GuestDBusVMStateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan guestDBusModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if plan.Action.ValueString() != "start" && plan.Action.ValueString() != "stop" {
		resp.Diagnostics.AddError("Invalid DBus Action", "action must be start or stop")
		return
	}
	if err := r.client.GuestDBusVMState(ctx, plan.Node.ValueString(), formatInt64(plan.VMID.ValueInt64()), plan.Action.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Run DBus VMState Action", err.Error())
		return
	}
	plan.ID = types.StringValue("qemu/" + formatInt64(plan.VMID.ValueInt64()) + "/" + plan.Action.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *GuestDBusVMStateResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *GuestDBusVMStateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan guestDBusModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.GuestDBusVMState(ctx, plan.Node.ValueString(), formatInt64(plan.VMID.ValueInt64()), plan.Action.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Run DBus VMState Action", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *GuestDBusVMStateResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *GuestUnlinkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan guestUnlinkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.GuestUnlink(ctx, plan.Node.ValueString(), formatInt64(plan.VMID.ValueInt64()), listStrings(plan.IDList), plan.Force.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Unable to Unlink Guest Disks", err.Error())
		return
	}
	plan.ID = types.StringValue("qemu/" + formatInt64(plan.VMID.ValueInt64()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *GuestUnlinkResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *GuestUnlinkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan guestUnlinkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.GuestUnlink(ctx, plan.Node.ValueString(), formatInt64(plan.VMID.ValueInt64()), listStrings(plan.IDList), plan.Force.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Unable to Unlink Guest Disks", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *GuestUnlinkResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *LXCResizeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan lxcResizeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.LXCResize(ctx, plan.Node.ValueString(), formatInt64(plan.VMID.ValueInt64()), plan.Disk.ValueString(), plan.Size.ValueString(), stringValue(plan.Digest)); err != nil {
		resp.Diagnostics.AddError("Unable to Resize LXC Mount Point", err.Error())
		return
	}
	plan.ID = types.StringValue(formatInt64(plan.VMID.ValueInt64()) + "/" + plan.Disk.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *LXCResizeResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *LXCResizeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan lxcResizeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.LXCResize(ctx, plan.Node.ValueString(), formatInt64(plan.VMID.ValueInt64()), plan.Disk.ValueString(), plan.Size.ValueString(), stringValue(plan.Digest)); err != nil {
		resp.Diagnostics.AddError("Unable to Resize LXC Mount Point", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *LXCResizeResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func listPlanModifierRequiresReplace() planmodifier.List {
	return listRequiresReplaceModifier{}
}

type listRequiresReplaceModifier struct{}

func (m listRequiresReplaceModifier) Description(context.Context) string {
	return "Changes force replacement."
}
func (m listRequiresReplaceModifier) MarkdownDescription(context.Context) string {
	return "Changes force replacement."
}
func (m listRequiresReplaceModifier) PlanModifyList(_ context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if req.State.Raw.IsNull() || req.PlanValue.Equal(req.StateValue) {
		return
	}
	resp.RequiresReplace = true
}

func formatInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}

func (r *GuestStatusActionResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config guestStatusActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	kind := config.Type
	action := config.Action
	if kind.IsUnknown() || kind.IsNull() || action.IsUnknown() || action.IsNull() {
		return
	}
	valid := map[string][]string{"qemu": {"start", "stop", "shutdown", "suspend", "resume", "reboot", "reset"}, "lxc": {"start", "stop", "shutdown", "suspend", "resume", "reboot"}}
	if kind.ValueString() != "qemu" && kind.ValueString() != "lxc" {
		resp.Diagnostics.AddError("Invalid Guest Type", "type must be qemu or lxc")
		return
	}
	for _, candidate := range valid[kind.ValueString()] {
		if candidate == action.ValueString() {
			return
		}
	}
	resp.Diagnostics.AddError("Invalid Guest Action", action.ValueString()+" is not supported for "+kind.ValueString())
}

func (r *GuestDBusVMStateResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config guestDBusModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.Action.IsUnknown() || config.Action.IsNull() {
		return
	}
	if config.Action.ValueString() != "start" && config.Action.ValueString() != "stop" {
		resp.Diagnostics.AddError("Invalid DBus Action", "action must be start or stop")
	}
}
