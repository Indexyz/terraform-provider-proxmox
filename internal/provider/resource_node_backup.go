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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &NodeVzdumpResource{}
var _ resource.ResourceWithValidateConfig = &NodeVzdumpResource{}
var _ resource.Resource = &NodeBackupPruneResource{}
var _ resource.ResourceWithValidateConfig = &NodeBackupPruneResource{}

type NodeVzdumpResource struct{ client *Client }
type NodeBackupPruneResource struct{ client *Client }

type nodeVzdumpModel struct {
	ID            types.String `tfsdk:"id"`
	Node          types.String `tfsdk:"node"`
	VMID          types.Int64  `tfsdk:"vmid"`
	Storage       types.String `tfsdk:"storage"`
	Mode          types.String `tfsdk:"mode"`
	Compress      types.String `tfsdk:"compress"`
	NotesTemplate types.String `tfsdk:"notes_template"`
	Protected     types.Bool   `tfsdk:"protected"`
	Quiet         types.Bool   `tfsdk:"quiet"`
	Triggers      types.Map    `tfsdk:"triggers"`
	UPID          types.String `tfsdk:"upid"`
}

type nodeBackupPruneModel struct {
	ID           types.String `tfsdk:"id"`
	Node         types.String `tfsdk:"node"`
	Storage      types.String `tfsdk:"storage"`
	VMID         types.Int64  `tfsdk:"vmid"`
	Type         types.String `tfsdk:"type"`
	PruneBackups types.String `tfsdk:"prune_backups"`
	Triggers     types.Map    `tfsdk:"triggers"`
	UPID         types.String `tfsdk:"upid"`
}

func NewNodeVzdumpResource() resource.Resource      { return &NodeVzdumpResource{} }
func NewNodeBackupPruneResource() resource.Resource { return &NodeBackupPruneResource{} }

func (r *NodeVzdumpResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_vzdump"
}

func (r *NodeBackupPruneResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_backup_prune"
}

func (r *NodeVzdumpResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Starts a `vzdump` backup of one guest through `POST /nodes/{node}/vzdump` and waits for the worker. The action is imperative: every change replaces the instance, destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<node>/<vmid>` form."},
			"node":           schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"vmid":           schema.Int64Attribute{Required: true, MarkdownDescription: "Guest VMID to back up. Changes require replacement.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"storage":        schema.StringAttribute{Required: true, MarkdownDescription: "Backup storage. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"mode":           schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("snapshot"), MarkdownDescription: "Backup mode: `snapshot`, `suspend` or `stop`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"compress":       schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("zstd"), MarkdownDescription: "Compression: `0`, `1`, `gzip`, `lzo`, `zstd`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"notes_template": schema.StringAttribute{Optional: true, MarkdownDescription: "Template for the backup notes.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"protected":      schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Mark the backup as protected.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"quiet":          schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Suppress progress output.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"triggers":       schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to back up again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"upid":           schema.StringAttribute{Computed: true, MarkdownDescription: "Worker task UPID."},
		},
	}
}

func (r *NodeBackupPruneResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Prunes backups of one guest on a storage through `DELETE /nodes/{node}/storage/{storage}/prunebackups` and waits for the worker. Every change replaces the instance, destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id":            schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<node>/<storage>/<vmid>` form."},
			"node":          schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"storage":       schema.StringAttribute{Required: true, MarkdownDescription: "Backup storage. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"vmid":          schema.Int64Attribute{Required: true, MarkdownDescription: "Guest VMID whose backups are pruned. Changes require replacement.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"type":          schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("qemu"), MarkdownDescription: "Guest type: `qemu` or `lxc`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"prune_backups": schema.StringAttribute{Required: true, MarkdownDescription: "Prune policy such as `keep-daily=7,keep-last=3`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"triggers":      schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to prune again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"upid":          schema.StringAttribute{Computed: true, MarkdownDescription: "Worker task UPID."},
		},
	}
}

func (r *NodeVzdumpResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *NodeBackupPruneResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *NodeVzdumpResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeVzdumpModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateNodeNameResource(config.Node, resp)
	if isConfiguredValue(config.Mode) && !slicesContains([]string{"snapshot", "suspend", "stop"}, config.Mode.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("mode"), "Invalid backup mode", "mode must be snapshot, suspend or stop.")
	}
	if isConfiguredValue(config.VMID) && config.VMID.ValueInt64() < 100 {
		resp.Diagnostics.AddAttributeError(path.Root("vmid"), "Invalid guest VMID", "vmid must be at least 100.")
	}
}

func (r *NodeBackupPruneResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeBackupPruneModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateNodeNameResource(config.Node, resp)
	if isConfiguredValue(config.Type) && !slicesContains([]string{"qemu", "lxc"}, config.Type.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("type"), "Invalid guest type", "type must be qemu or lxc.")
	}
	if isConfiguredValue(config.VMID) && config.VMID.ValueInt64() < 100 {
		resp.Diagnostics.AddAttributeError(path.Root("vmid"), "Invalid guest VMID", "vmid must be at least 100.")
	}
}

func (r *NodeVzdumpResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeVzdumpResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeVzdumpResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *NodeVzdumpResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan nodeVzdumpModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	node := plan.Node.ValueString()
	upid, err := r.client.RunVzdump(ctx, node, plan.VMID.ValueInt64(), plan.Storage.ValueString(), plan.Mode.ValueString(), plan.Compress.ValueString(), stringValue(plan.NotesTemplate), plan.Protected.ValueBool(), plan.Quiet.ValueBool())
	if err != nil {
		diags.AddError("Unable to Start Proxmox Backup", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, diags) {
		return
	}
	next := plan
	next.ID = types.StringValue(fmt.Sprintf("%s/%d", node, plan.VMID.ValueInt64()))
	next.UPID = types.StringValue(upid)
	diags.Append(state.Set(ctx, &next)...)
}

func (r *NodeBackupPruneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeBackupPruneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeBackupPruneResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *NodeBackupPruneResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan nodeBackupPruneModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	node, storage := plan.Node.ValueString(), plan.Storage.ValueString()
	upid, err := r.client.PruneBackups(ctx, node, storage, plan.PruneBackups.ValueString(), plan.Type.ValueString(), plan.VMID.ValueInt64())
	if err != nil {
		diags.AddError("Unable to Prune Proxmox Backups", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, diags) {
		return
	}
	next := plan
	next.ID = types.StringValue(fmt.Sprintf("%s/%s/%d", node, storage, plan.VMID.ValueInt64()))
	next.UPID = types.StringValue(upid)
	diags.Append(state.Set(ctx, &next)...)
}

func (r *NodeVzdumpResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
	// Imperative action; state is kept.
}

func (r *NodeBackupPruneResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
	// Imperative action; state is kept.
}
