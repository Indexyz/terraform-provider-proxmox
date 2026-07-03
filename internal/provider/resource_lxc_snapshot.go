// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &LXCSnapshotResource{}
var _ resource.ResourceWithImportState = &LXCSnapshotResource{}

type LXCSnapshotResource struct {
	client *Client
}

func NewLXCSnapshotResource() resource.Resource {
	return &LXCSnapshotResource{}
}

func (r *LXCSnapshotResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lxc_snapshot"
}

type lxcSnapshotModel struct {
	ID          types.String `tfsdk:"id"`
	Node        types.String `tfsdk:"node"`
	VMID        types.Int64  `tfsdk:"vm_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Parent      types.String `tfsdk:"parent"`
	Snaptime    types.Int64  `tfsdk:"snaptime"`
}

func (r *LXCSnapshotResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceschema.Schema{
		MarkdownDescription: "Manages a Proxmox VE LXC container snapshot through `/nodes/{node}/lxc/{vmid}/snapshot`.",
		Attributes: map[string]resourceschema.Attribute{
			"id": resourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Terraform identifier in `node/vm_id/name` form.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"node": resourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Proxmox node that owns the container.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"vm_id": resourceschema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Numeric VMID of the container.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"name": resourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Snapshot name. Changes require replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Snapshot description managed through the snapshot config."},
			"parent":      resourceschema.StringAttribute{Computed: true, MarkdownDescription: "Parent snapshot name, if any."},
			"snaptime":    resourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Unix timestamp when the snapshot was created."},
		},
	}
}

func (r *LXCSnapshotResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *LXCSnapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan lxcSnapshotModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if strings.TrimSpace(plan.Name.ValueString()) == "" {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Missing snapshot name", "The name attribute must be a non-empty value.")
		return
	}

	createReq := CreateLXCSnapshotRequest{
		Node:        plan.Node.ValueString(),
		VMID:        plan.VMID.ValueInt64(),
		Name:        plan.Name.ValueString(),
		Description: stringPointerValue(plan.Description),
	}
	if err := r.client.CreateLXCSnapshot(ctx, createReq); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox LXC Snapshot", err.Error())
		return
	}

	state, diags := r.readLXCSnapshotState(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), plan.Name.ValueString(), &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *LXCSnapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state lxcSnapshotModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readLXCSnapshotState(ctx, state.Node.ValueString(), state.VMID.ValueInt64(), state.Name.ValueString(), &state)
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

func (r *LXCSnapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan lxcSnapshotModel
	var state lxcSnapshotModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if plan.Description.ValueString() != state.Description.ValueString() {
		if err := r.client.UpdateLXCSnapshot(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), plan.Name.ValueString(), plan.Description.ValueString()); err != nil {
			resp.Diagnostics.AddError("Unable to Update Proxmox LXC Snapshot", err.Error())
			return
		}
	}
	refreshed, diags := r.readLXCSnapshotState(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), plan.Name.ValueString(), &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *LXCSnapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state lxcSnapshotModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteLXCSnapshot(ctx, state.Node.ValueString(), state.VMID.ValueInt64(), state.Name.ValueString()); err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Unable to Delete Proxmox LXC Snapshot", err.Error())
	}
}

func (r *LXCSnapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, vmID, name, err := parseLXCSnapshotImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Import Identifier", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), lxcSnapshotID(node, vmID, name))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vm_id"), vmID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}

func (r *LXCSnapshotResource) readLXCSnapshotState(ctx context.Context, node string, vmID int64, name string, prior *lxcSnapshotModel) (lxcSnapshotModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	snap, err := r.client.GetLXCSnapshot(ctx, node, vmID, name)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return lxcSnapshotModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox LXC Snapshot", err.Error())
		return lxcSnapshotModel{}, diags
	}
	description := stringOrNull(snap.Description)
	if prior != nil && description.IsNull() && !prior.Description.IsNull() && !prior.Description.IsUnknown() {
		description = prior.Description
	}
	return lxcSnapshotModel{
		ID:          types.StringValue(lxcSnapshotID(node, vmID, name)),
		Node:        types.StringValue(node),
		VMID:        types.Int64Value(vmID),
		Name:        types.StringValue(name),
		Description: description,
		Parent:      stringOrNull(snap.Parent),
		Snaptime:    int64OrNull(snap.Snaptime.Ptr()),
	}, diags
}
