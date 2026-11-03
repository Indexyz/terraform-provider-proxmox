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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &StorageVolumeResource{}
var _ resource.ResourceWithImportState = &StorageVolumeResource{}
var _ resource.Resource = &StorageVolumeTransferResource{}

type StorageVolumeResource struct{ client *Client }
type StorageVolumeTransferResource struct{ client *Client }

type storageVolumeModel struct {
	ID        types.String `tfsdk:"id"`
	Node      types.String `tfsdk:"node"`
	Storage   types.String `tfsdk:"storage"`
	Volume    types.String `tfsdk:"volume"`
	Notes     types.String `tfsdk:"notes"`
	Protected types.Bool   `tfsdk:"protected"`
	Format    types.String `tfsdk:"format"`
	Size      types.Int64  `tfsdk:"size"`
	Used      types.Int64  `tfsdk:"used"`
}

type storageVolumeTransferModel struct {
	ID         types.String `tfsdk:"id"`
	Node       types.String `tfsdk:"node"`
	Storage    types.String `tfsdk:"storage"`
	Volume     types.String `tfsdk:"volume"`
	Target     types.String `tfsdk:"target"`
	TargetNode types.String `tfsdk:"target_node"`
	Triggers   types.Map    `tfsdk:"triggers"`
	UPID       types.String `tfsdk:"upid"`
}

func NewStorageVolumeResource() resource.Resource {
	return &StorageVolumeResource{}
}

func NewStorageVolumeTransferResource() resource.Resource {
	return &StorageVolumeTransferResource{}
}

func (r *StorageVolumeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage_volume"
}

func (r *StorageVolumeTransferResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage_volume_transfer"
}

func (r *StorageVolumeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages notes and the protected flag of an existing storage volume through `PUT /nodes/{node}/storage/{storage}/content/{volume}`. Create expects the volume to exist, destroy deletes it. Import as `<node>/<storage>/<volume>`.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<node>/<storage>/<volume>` form."},
			"node":      schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"storage":   schema.StringAttribute{Required: true, MarkdownDescription: "Storage.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"volume":    schema.StringAttribute{Required: true, MarkdownDescription: "Volume identifier such as `local:iso/debian.iso`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"notes":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Volume notes."},
			"protected": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Protect the volume from removal.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"format":    schema.StringAttribute{Computed: true, MarkdownDescription: "Volume format."},
			"size":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Volume size in bytes."},
			"used":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Used bytes."},
		},
	}
}

func (r *StorageVolumeTransferResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Copies or moves a volume through `POST /nodes/{node}/storage/{storage}/content/{volume}` and waits for the worker. Every change replaces the instance, destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<storage>/<volume>` form."},
			"node":        schema.StringAttribute{Required: true, MarkdownDescription: "Source node.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"storage":     schema.StringAttribute{Required: true, MarkdownDescription: "Source storage.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"volume":      schema.StringAttribute{Required: true, MarkdownDescription: "Source volume.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"target":      schema.StringAttribute{Required: true, MarkdownDescription: "Target volume identifier.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"target_node": schema.StringAttribute{Optional: true, MarkdownDescription: "Target node when moving across nodes.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"triggers":    schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to transfer again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"upid":        schema.StringAttribute{Computed: true, MarkdownDescription: "Worker task UPID."},
		},
	}
}

func (r *StorageVolumeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *StorageVolumeTransferResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *StorageVolumeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *StorageVolumeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *StorageVolumeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state storageVolumeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.refresh(ctx, state.Node.ValueString(), state.Storage.ValueString(), state.Volume.ValueString(), &resp.State, &resp.Diagnostics)
}

func (r *StorageVolumeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state storageVolumeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteStorageVolume(ctx, state.Node.ValueString(), state.Storage.ValueString(), state.Volume.ValueString())
	if err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Storage Volume", err.Error())
	}
}

func (r *StorageVolumeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 3)
	if len(parts) != 3 {
		resp.Diagnostics.AddAttributeError(path.Root("id"), "Invalid import id", "use <node>/<storage>/<volume>.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("storage"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("volume"), parts[2])...)
}

func (r *StorageVolumeResource) apply(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan storageVolumeModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	node, storage, volume := plan.Node.ValueString(), plan.Storage.ValueString(), plan.Volume.ValueString()
	if err := r.client.SetStorageVolumeMetadata(ctx, node, storage, volume, stringValue(plan.Notes), plan.Protected.ValueBool()); err != nil {
		diags.AddError("Unable to Set Proxmox Storage Volume Metadata", err.Error())
		return
	}
	r.refresh(ctx, node, storage, volume, state, diags)
}

func (r *StorageVolumeResource) refresh(ctx context.Context, node, storage, volume string, state *tfsdk.State, diags *diag.Diagnostics) {
	metadata, err := r.client.StorageVolume(ctx, node, storage, volume)
	if err != nil {
		if errors.Is(err, errNotFound) {
			state.RemoveResource(ctx)
			return
		}
		diags.AddError("Unable to Read Proxmox Storage Volume", err.Error())
		return
	}
	next := storageVolumeModel{
		ID: types.StringValue(fmt.Sprintf("%s/%s/%s", node, storage, volume)), Node: types.StringValue(node),
		Storage: types.StringValue(storage), Volume: types.StringValue(volume), Notes: stringOrNull(metadata.Notes),
		Protected: nodesBoolOrNull(metadata.Protected), Format: stringOrNull(metadata.Format),
		Size: types.Int64Value(metadata.Size), Used: types.Int64Value(metadata.Used),
	}
	diags.Append(state.Set(ctx, &next)...)
}

func (r *StorageVolumeTransferResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *StorageVolumeTransferResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *StorageVolumeTransferResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *StorageVolumeTransferResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *StorageVolumeTransferResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan storageVolumeTransferModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	upid, err := r.client.TransferStorageVolume(ctx, plan.Node.ValueString(), plan.Storage.ValueString(), plan.Volume.ValueString(), plan.Target.ValueString(), stringValue(plan.TargetNode))
	if err != nil {
		diags.AddError("Unable to Transfer Proxmox Storage Volume", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, diags) {
		return
	}
	next := plan
	next.ID = types.StringValue(plan.Storage.ValueString() + "/" + plan.Volume.ValueString())
	next.UPID = types.StringValue(upid)
	diags.Append(state.Set(ctx, &next)...)
}
