// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource = &QemuSendKeyResource{}
	_ resource.Resource = &QemuMonitorResource{}
	_ resource.Resource = &QemuCloudInitPendingResource{}
	_ resource.Resource = &QemuAgentFileResource{}
	_ resource.Resource = &QemuAgentFSFreezeResource{}
	_ resource.Resource = &QemuAgentFstrimResource{}
	_ resource.Resource = &GuestMoveDiskResource{}
	_ resource.Resource = &GuestMoveVolumeResource{}
	_ resource.Resource = &GuestRemoteMigrateResource{}
)

type QemuSendKeyResource struct{ client *Client }
type QemuMonitorResource struct{ client *Client }
type QemuCloudInitPendingResource struct{ client *Client }
type QemuAgentFileResource struct{ client *Client }
type QemuAgentFSFreezeResource struct{ client *Client }
type QemuAgentFstrimResource struct{ client *Client }
type GuestMoveDiskResource struct{ client *Client }
type GuestMoveVolumeResource struct{ client *Client }
type GuestRemoteMigrateResource struct{ client *Client }

type qemuSendKeyModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	VMID     types.Int64  `tfsdk:"vmid"`
	Key      types.String `tfsdk:"key"`
	Triggers types.Map    `tfsdk:"triggers"`
}
type qemuMonitorModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	VMID     types.Int64  `tfsdk:"vmid"`
	Command  types.String `tfsdk:"command"`
	Triggers types.Map    `tfsdk:"triggers"`
	Result   types.String `tfsdk:"result"`
}
type qemuCloudInitPendingModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	VMID     types.Int64  `tfsdk:"vmid"`
	Values   types.Map    `tfsdk:"values"`
	Triggers types.Map    `tfsdk:"triggers"`
}
type qemuAgentFileModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	VMID     types.Int64  `tfsdk:"vmid"`
	File     types.String `tfsdk:"file"`
	Content  types.String `tfsdk:"content"`
	Encode   types.Bool   `tfsdk:"encode"`
	Triggers types.Map    `tfsdk:"triggers"`
}
type qemuAgentFSFreezeModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	VMID     types.Int64  `tfsdk:"vmid"`
	Triggers types.Map    `tfsdk:"triggers"`
}
type qemuAgentFstrimModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	VMID     types.Int64  `tfsdk:"vmid"`
	Triggers types.Map    `tfsdk:"triggers"`
}
type guestMoveDiskModel struct {
	ID           types.String `tfsdk:"id"`
	Node         types.String `tfsdk:"node"`
	VMID         types.Int64  `tfsdk:"vmid"`
	Disk         types.String `tfsdk:"disk"`
	Storage      types.String `tfsdk:"storage"`
	TargetDisk   types.String `tfsdk:"target_disk"`
	TargetVMID   types.Int64  `tfsdk:"target_vmid"`
	Delete       types.Bool   `tfsdk:"delete"`
	Format       types.String `tfsdk:"format"`
	BWLimit      types.Int64  `tfsdk:"bwlimit"`
	Digest       types.String `tfsdk:"digest"`
	TargetDigest types.String `tfsdk:"target_digest"`
	Triggers     types.Map    `tfsdk:"triggers"`
	UPID         types.String `tfsdk:"upid"`
}
type guestMoveVolumeModel struct {
	ID           types.String `tfsdk:"id"`
	Type         types.String `tfsdk:"type"`
	Node         types.String `tfsdk:"node"`
	VMID         types.Int64  `tfsdk:"vmid"`
	Volume       types.String `tfsdk:"volume"`
	Storage      types.String `tfsdk:"storage"`
	TargetVolume types.String `tfsdk:"target_volume"`
	TargetVMID   types.Int64  `tfsdk:"target_vmid"`
	Delete       types.Bool   `tfsdk:"delete"`
	BWLimit      types.Int64  `tfsdk:"bwlimit"`
	Digest       types.String `tfsdk:"digest"`
	TargetDigest types.String `tfsdk:"target_digest"`
	Triggers     types.Map    `tfsdk:"triggers"`
	UPID         types.String `tfsdk:"upid"`
}
type guestRemoteMigrateModel struct {
	ID             types.String `tfsdk:"id"`
	Type           types.String `tfsdk:"type"`
	Node           types.String `tfsdk:"node"`
	VMID           types.Int64  `tfsdk:"vmid"`
	TargetEndpoint types.String `tfsdk:"target_endpoint"`
	TargetStorage  types.String `tfsdk:"target_storage"`
	TargetBridge   types.String `tfsdk:"target_bridge"`
	TargetVMID     types.Int64  `tfsdk:"target_vmid"`
	Online         types.Bool   `tfsdk:"online"`
	Restart        types.Bool   `tfsdk:"restart"`
	Delete         types.Bool   `tfsdk:"delete"`
	BWLimit        types.Int64  `tfsdk:"bwlimit"`
	Timeout        types.Int64  `tfsdk:"timeout"`
	Triggers       types.Map    `tfsdk:"triggers"`
	UPID           types.String `tfsdk:"upid"`
}

func NewQemuSendKeyResource() resource.Resource          { return &QemuSendKeyResource{} }
func NewQemuMonitorResource() resource.Resource          { return &QemuMonitorResource{} }
func NewQemuCloudInitPendingResource() resource.Resource { return &QemuCloudInitPendingResource{} }
func NewQemuAgentFileResource() resource.Resource        { return &QemuAgentFileResource{} }
func NewQemuAgentFSFreezeResource() resource.Resource    { return &QemuAgentFSFreezeResource{} }
func NewQemuAgentFstrimResource() resource.Resource      { return &QemuAgentFstrimResource{} }
func NewGuestMoveDiskResource() resource.Resource        { return &GuestMoveDiskResource{} }
func NewGuestMoveVolumeResource() resource.Resource      { return &GuestMoveVolumeResource{} }
func NewGuestRemoteMigrateResource() resource.Resource   { return &GuestRemoteMigrateResource{} }

func (r *QemuSendKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_sendkey"
}
func (r *QemuMonitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_monitor"
}
func (r *QemuCloudInitPendingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_cloudinit_pending"
}
func (r *QemuAgentFileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_agent_file_write"
}
func (r *QemuAgentFSFreezeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_agent_fsfreeze"
}
func (r *QemuAgentFstrimResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_agent_fstrim"
}
func (r *GuestMoveDiskResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_move_disk"
}
func (r *GuestMoveVolumeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_move_volume"
}
func (r *GuestRemoteMigrateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_remote_migrate"
}

func guestBaseAttrs(typeAttr bool) map[string]schema.Attribute {
	attrs := map[string]schema.Attribute{
		"id":   schema.StringAttribute{Computed: true},
		"node": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"vmid": schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
	}
	if typeAttr {
		attrs["type"] = schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("qemu"), PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	}
	return attrs
}

func (r *QemuSendKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Sends a key through `PUT /nodes/{node}/qemu/{vmid}/sendkey`.", Attributes: map[string]schema.Attribute{
		"id":       schema.StringAttribute{Computed: true},
		"node":     schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"vmid":     schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"key":      schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
	}}
}
func (r *QemuMonitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Runs a QEMU monitor command through `POST /nodes/{node}/qemu/{vmid}/monitor`.", Attributes: map[string]schema.Attribute{
		"id":       schema.StringAttribute{Computed: true},
		"node":     schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"vmid":     schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"command":  schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
		"result":   schema.StringAttribute{Computed: true},
	}}
}
func (r *QemuCloudInitPendingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Writes pending cloud-init values through `PUT /nodes/{node}/qemu/{vmid}/cloudinit/pending`. The endpoint has no read counterpart, so refresh keeps the last applied values.", Attributes: map[string]schema.Attribute{
		"id":       schema.StringAttribute{Computed: true},
		"node":     schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"vmid":     schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"values":   schema.MapAttribute{Required: true, ElementType: types.StringType},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
	}}
}
func (r *QemuAgentFileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Writes a file through the guest agent at `POST /nodes/{node}/qemu/{vmid}/agent/file-write`.", Attributes: map[string]schema.Attribute{
		"id":       schema.StringAttribute{Computed: true},
		"node":     schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"vmid":     schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"file":     schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"content":  schema.StringAttribute{Required: true, Sensitive: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"encode":   schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
	}}
}
func (r *QemuAgentFSFreezeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Freezes guest filesystems on create and thaws them on destroy.", Attributes: map[string]schema.Attribute{
		"id":       schema.StringAttribute{Computed: true},
		"node":     schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"vmid":     schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
	}}
}
func (r *QemuAgentFstrimResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Runs guest `fstrim` through `POST /nodes/{node}/qemu/{vmid}/agent/fstrim`.", Attributes: map[string]schema.Attribute{
		"id":       schema.StringAttribute{Computed: true},
		"node":     schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"vmid":     schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
	}}
}
func (r *GuestMoveDiskResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := guestBaseAttrs(false)
	for name, attr := range map[string]schema.Attribute{
		"disk":          schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"storage":       schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"target_disk":   schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"target_vmid":   schema.Int64Attribute{Optional: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"delete":        schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"format":        schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"bwlimit":       schema.Int64Attribute{Optional: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"digest":        schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"target_digest": schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"triggers":      schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
		"upid":          schema.StringAttribute{Computed: true},
	} {
		attrs[name] = attr
	}
	resp.Schema = schema.Schema{MarkdownDescription: "Moves a QEMU disk through `POST /nodes/{node}/qemu/{vmid}/move_disk` and waits for the worker.", Attributes: attrs}
}
func (r *GuestMoveVolumeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := guestBaseAttrs(true)
	for name, attr := range map[string]schema.Attribute{
		"volume":        schema.StringAttribute{Optional: true, MarkdownDescription: "Mount or disk key (`rootfs`, `mp0`, `scsi0`, ...).", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"storage":       schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"target_volume": schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"target_vmid":   schema.Int64Attribute{Optional: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"delete":        schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"bwlimit":       schema.Int64Attribute{Optional: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"digest":        schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"target_digest": schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"triggers":      schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
		"upid":          schema.StringAttribute{Computed: true},
	} {
		attrs[name] = attr
	}
	resp.Schema = schema.Schema{MarkdownDescription: "Moves a QEMU or LXC volume through `/nodes/{node}/{type}/{vmid}/move_volume`.", Attributes: attrs}
}
func (r *GuestRemoteMigrateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := guestBaseAttrs(true)
	for name, attr := range map[string]schema.Attribute{
		"target_endpoint": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"target_storage":  schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"target_bridge":   schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"target_vmid":     schema.Int64Attribute{Optional: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"online":          schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"restart":         schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"delete":          schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"bwlimit":         schema.Int64Attribute{Optional: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"timeout":         schema.Int64Attribute{Optional: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"triggers":        schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
		"upid":            schema.StringAttribute{Computed: true},
	} {
		attrs[name] = attr
	}
	resp.Schema = schema.Schema{MarkdownDescription: "Migrates a guest to another cluster endpoint through `/nodes/{node}/{type}/{vmid}/remote_migrate`.", Attributes: attrs}
}

func (r *QemuSendKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *QemuMonitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *QemuCloudInitPendingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *QemuAgentFileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *QemuAgentFSFreezeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *QemuAgentFstrimResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *GuestMoveDiskResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *GuestMoveVolumeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *GuestRemoteMigrateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *QemuSendKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan qemuSendKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := waitQemuRunning(ctx, r.client, plan.Node.ValueString(), plan.VMID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Guest Is Not Running", err.Error())
		return
	}
	if err := r.client.SendKey(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), plan.Key.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Send Key", err.Error())
		return
	}
	plan.ID = types.StringValue(fmt.Sprintf("%s/%d", plan.Node.ValueString(), plan.VMID.ValueInt64()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *QemuSendKeyResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *QemuSendKeyResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *QemuSendKeyResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *QemuMonitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan qemuMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := waitQemuRunning(ctx, r.client, plan.Node.ValueString(), plan.VMID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Guest Is Not Running", err.Error())
		return
	}
	result, err := r.client.QemuMonitor(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), plan.Command.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Run Monitor Command", err.Error())
		return
	}
	plan.ID = types.StringValue(fmt.Sprintf("%s/%d", plan.Node.ValueString(), plan.VMID.ValueInt64()))
	plan.Result = types.StringValue(result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *QemuMonitorResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *QemuMonitorResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *QemuMonitorResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *QemuCloudInitPendingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *QemuCloudInitPendingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *QemuCloudInitPendingResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *QemuCloudInitPendingResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
func (r *QemuCloudInitPendingResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan qemuCloudInitPendingModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	values := make(map[string]string, len(plan.Values.Elements()))
	for key, value := range plan.Values.Elements() {
		text, ok := value.(types.String)
		if !ok {
			diags.AddAttributeError(path.Root("values"), "Invalid value", "values must contain only strings.")
			return
		}
		values[key] = text.ValueString()
	}
	node := plan.Node.ValueString()
	if err := r.client.UpdateCloudInitPending(ctx, node, plan.VMID.ValueInt64(), values, nil); err != nil {
		diags.AddError("Unable to Update Pending Cloud-Init", err.Error())
		return
	}
	plan.ID = types.StringValue(fmt.Sprintf("%s/%d", node, plan.VMID.ValueInt64()))
	diags.Append(state.Set(ctx, &plan)...)
}

func (r *QemuAgentFileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan qemuAgentFileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.AgentFileWrite(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), plan.File.ValueString(), plan.Content.ValueString(), plan.Encode.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Unable to Write Guest File", err.Error())
		return
	}
	plan.ID = types.StringValue(fmt.Sprintf("%s/%d", plan.Node.ValueString(), plan.VMID.ValueInt64()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *QemuAgentFileResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *QemuAgentFileResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *QemuAgentFileResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *QemuAgentFSFreezeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan qemuAgentFSFreezeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.AgentFSFreeze(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), "freeze"); err != nil {
		resp.Diagnostics.AddError("Unable to Freeze Guest Filesystems", err.Error())
		return
	}
	plan.ID = types.StringValue(fmt.Sprintf("%s/%d", plan.Node.ValueString(), plan.VMID.ValueInt64()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *QemuAgentFSFreezeResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *QemuAgentFSFreezeResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *QemuAgentFSFreezeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state qemuAgentFSFreezeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.AgentFSFreeze(ctx, state.Node.ValueString(), state.VMID.ValueInt64(), "thaw"); err != nil {
		resp.Diagnostics.AddError("Unable to Thaw Guest Filesystems", err.Error())
	}
}

func (r *QemuAgentFstrimResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan qemuAgentFstrimModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.AgentFstrim(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Unable to Trim Guest Filesystems", err.Error())
		return
	}
	plan.ID = types.StringValue(fmt.Sprintf("%s/%d", plan.Node.ValueString(), plan.VMID.ValueInt64()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *QemuAgentFstrimResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *QemuAgentFstrimResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *QemuAgentFstrimResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *GuestMoveDiskResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *GuestMoveDiskResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *GuestMoveDiskResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *GuestMoveDiskResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
func (r *GuestMoveDiskResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan guestMoveDiskModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	upid, err := r.client.MoveQemuDisk(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), QemuMoveDisk{
		Disk: plan.Disk.ValueString(), Storage: plan.Storage.ValueString(), TargetDisk: stringValue(plan.TargetDisk), TargetVMID: plan.TargetVMID.ValueInt64(),
		Delete: plan.Delete.ValueBool(), Format: stringValue(plan.Format), BWLimit: plan.BWLimit.ValueInt64(), Digest: stringValue(plan.Digest), TargetDigest: stringValue(plan.TargetDigest),
	})
	if err != nil {
		diags.AddError("Unable to Move Disk", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, diags) {
		return
	}
	plan.ID = types.StringValue(fmt.Sprintf("%s/%d/%s", plan.Node.ValueString(), plan.VMID.ValueInt64(), plan.Disk.ValueString()))
	plan.UPID = types.StringValue(upid)
	diags.Append(state.Set(ctx, &plan)...)
}

func (r *GuestMoveVolumeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *GuestMoveVolumeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *GuestMoveVolumeResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *GuestMoveVolumeResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
func (r *GuestMoveVolumeResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan guestMoveVolumeModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	upid, err := r.client.MoveGuestVolume(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), GuestVolumeMove{
		GuestType: plan.Type.ValueString(), Volume: stringValue(plan.Volume), Storage: plan.Storage.ValueString(), TargetVol: stringValue(plan.TargetVolume),
		TargetVMID: plan.TargetVMID.ValueInt64(), Delete: plan.Delete.ValueBool(), BWLimit: plan.BWLimit.ValueInt64(), Digest: stringValue(plan.Digest), TargetDigest: stringValue(plan.TargetDigest),
	})
	if err != nil {
		diags.AddError("Unable to Move Volume", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, diags) {
		return
	}
	plan.ID = types.StringValue(fmt.Sprintf("%s/%d", plan.Node.ValueString(), plan.VMID.ValueInt64()))
	plan.UPID = types.StringValue(upid)
	diags.Append(state.Set(ctx, &plan)...)
}

func (r *GuestRemoteMigrateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *GuestRemoteMigrateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *GuestRemoteMigrateResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *GuestRemoteMigrateResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
func (r *GuestRemoteMigrateResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan guestRemoteMigrateModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	upid, err := r.client.RemoteMigrateGuest(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), GuestRemoteMigrate{
		GuestType: plan.Type.ValueString(), TargetEndpoint: plan.TargetEndpoint.ValueString(), TargetStorage: stringValue(plan.TargetStorage),
		TargetBridge: stringValue(plan.TargetBridge), TargetVMID: plan.TargetVMID.ValueInt64(), Online: plan.Online.ValueBool(),
		Restart: plan.Restart.ValueBool(), Delete: plan.Delete.ValueBool(), BWLimit: plan.BWLimit.ValueInt64(), Timeout: plan.Timeout.ValueInt64(),
	})
	if err != nil {
		diags.AddError("Unable to Migrate Guest Remotely", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, diags) {
		return
	}
	plan.ID = types.StringValue(fmt.Sprintf("%s/%d", plan.Node.ValueString(), plan.VMID.ValueInt64()))
	plan.UPID = types.StringValue(upid)
	diags.Append(state.Set(ctx, &plan)...)
}

func waitQemuRunning(ctx context.Context, client *Client, node string, vmid int64) error {
	deadline := time.Now().Add(15 * time.Second)
	for {
		status, err := client.GetQemuVMStatus(ctx, node, vmid)
		if err != nil {
			return err
		}
		if status.Status == "running" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("VM %d is not running", vmid)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
