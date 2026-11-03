// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &GuestMigrationResource{}
var _ resource.ResourceWithValidateConfig = &GuestMigrationResource{}

type GuestMigrationResource struct{ client *Client }

type guestMigrationModel struct {
	ID             types.String `tfsdk:"id"`
	Type           types.String `tfsdk:"type"`
	Node           types.String `tfsdk:"node"`
	VMID           types.Int64  `tfsdk:"vmid"`
	Target         types.String `tfsdk:"target"`
	Online         types.Bool   `tfsdk:"online"`
	WithLocalDisks types.Bool   `tfsdk:"with_local_disks"`
	Restart        types.Bool   `tfsdk:"restart"`
	Timeout        types.Int64  `tfsdk:"timeout"`
	TargetStorage  types.String `tfsdk:"target_storage"`
	Triggers       types.Map    `tfsdk:"triggers"`
	UPID           types.String `tfsdk:"upid"`
}

func NewGuestMigrationResource() resource.Resource { return &GuestMigrationResource{} }

func (r *GuestMigrationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_migration"
}

func (r *GuestMigrationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Migrates a guest to another cluster node through `POST /nodes/{node}/{type}/{vmid}/migrate` and waits for the worker. Every change replaces the instance, destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<type>/<vmid>` form."},
			"type":             schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("qemu"), MarkdownDescription: "Guest type: `qemu` or `lxc`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"node":             schema.StringAttribute{Required: true, MarkdownDescription: "Source node.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"vmid":             schema.Int64Attribute{Required: true, MarkdownDescription: "Guest VMID.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"target":           schema.StringAttribute{Required: true, MarkdownDescription: "Target node.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"online":           schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Live migration for VMs, restart migration for containers.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"with_local_disks": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "QEMU only: migrate local disks too.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"restart":          schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "LXC only: restart the container on the target while migrating.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"timeout":          schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(180), MarkdownDescription: "LXC only: shutdown timeout in seconds.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"target_storage":   schema.StringAttribute{Optional: true, MarkdownDescription: "Target storage for local disks.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"triggers":         schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to migrate again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"upid":             schema.StringAttribute{Computed: true, MarkdownDescription: "Worker task UPID."},
		},
	}
}

func (r *GuestMigrationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *GuestMigrationResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config guestMigrationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateNodeNameResource(config.Node, resp)
	if isConfiguredValue(config.Type) && !slicesContains([]string{"qemu", "lxc"}, config.Type.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("type"), "Invalid guest type", "type must be qemu or lxc.")
	}
	if isConfiguredValue(config.Target) && isConfiguredValue(config.Node) && config.Target.ValueString() == config.Node.ValueString() {
		resp.Diagnostics.AddAttributeError(path.Root("target"), "Same source and target", "target must differ from node.")
	}
	if isConfiguredValue(config.VMID) && config.VMID.ValueInt64() < 100 {
		resp.Diagnostics.AddAttributeError(path.Root("vmid"), "Invalid VMID", "vmid must be at least 100.")
	}
}

func (r *GuestMigrationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *GuestMigrationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *GuestMigrationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *GuestMigrationResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *GuestMigrationResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan guestMigrationModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	guestType := plan.Type.ValueString()
	upid, err := r.client.MigrateGuest(ctx, guestType, plan.Node.ValueString(), plan.VMID.ValueInt64(), plan.Target.ValueString(),
		plan.Online.ValueBool(), plan.WithLocalDisks.ValueBool(), plan.Restart.ValueBool(), plan.Timeout.ValueInt64(), stringValue(plan.TargetStorage))
	if err != nil {
		diags.AddError("Unable to Migrate Proxmox Guest", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, diags) {
		return
	}
	next := plan
	next.ID = types.StringValue(fmt.Sprintf("%s/%d", guestType, plan.VMID.ValueInt64()))
	next.UPID = types.StringValue(upid)
	diags.Append(state.Set(ctx, &next)...)
}
