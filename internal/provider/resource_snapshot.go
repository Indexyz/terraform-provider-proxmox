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
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// snapshotResource implements both guest snapshot resources. The Proxmox
// snapshot API is identical for QEMU and LXC apart from the guest-kind URL
// segment, so one implementation serves `proxmox_qemu_snapshot` and
// `proxmox_lxc_snapshot`, parameterized by snapshotKind.
type snapshotResource struct {
	client *Client
	kind   snapshotKind
	guest  string // "VM" or "container"; used in schema descriptions
}

func NewQemuSnapshotResource() resource.Resource {
	return &snapshotResource{kind: snapshotKindQEMU, guest: "VM"}
}

func NewLXCSnapshotResource() resource.Resource {
	return &snapshotResource{kind: snapshotKindLXC, guest: "container"}
}

func (r *snapshotResource) label() string {
	return strings.ToUpper(string(r.kind))
}

func (r *snapshotResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + string(r.kind) + "_snapshot"
}

type snapshotModel struct {
	ID          types.String `tfsdk:"id"`
	Node        types.String `tfsdk:"node"`
	VMID        types.Int64  `tfsdk:"vm_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Parent      types.String `tfsdk:"parent"`
	Snaptime    types.Int64  `tfsdk:"snaptime"`
}

func (r *snapshotResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceschema.Schema{
		MarkdownDescription: fmt.Sprintf("Manages a Proxmox VE %s %s snapshot through `/nodes/{node}/%s/{vmid}/snapshot`.", r.label(), r.guest, r.kind),
		Attributes: map[string]resourceschema.Attribute{
			"id": resourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Terraform identifier in `node/vm_id/name` form.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"node": resourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: fmt.Sprintf("Proxmox node that owns the %s.", r.guest),
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"vm_id": resourceschema.Int64Attribute{
				Required:            true,
				MarkdownDescription: fmt.Sprintf("Numeric VMID of the %s.", r.guest),
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

func (r *snapshotResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *snapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan snapshotModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if strings.TrimSpace(plan.Name.ValueString()) == "" {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Missing snapshot name", "The name attribute must be a non-empty value.")
		return
	}

	if err := r.client.createSnapshot(ctx, r.kind, plan.Node.ValueString(), plan.VMID.ValueInt64(), plan.Name.ValueString(), stringPointerValue(plan.Description)); err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to Create Proxmox %s Snapshot", r.label()), err.Error())
		return
	}

	state, diags := r.readSnapshotState(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), plan.Name.ValueString(), &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *snapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state snapshotModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readSnapshotState(ctx, state.Node.ValueString(), state.VMID.ValueInt64(), state.Name.ValueString(), &state)
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

func (r *snapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan snapshotModel
	var state snapshotModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if plan.Description.ValueString() != state.Description.ValueString() {
		if err := r.client.updateSnapshot(ctx, r.kind, plan.Node.ValueString(), plan.VMID.ValueInt64(), plan.Name.ValueString(), plan.Description.ValueString()); err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Unable to Update Proxmox %s Snapshot", r.label()), err.Error())
			return
		}
	}
	refreshed, diags := r.readSnapshotState(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), plan.Name.ValueString(), &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *snapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state snapshotModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.deleteSnapshot(ctx, r.kind, state.Node.ValueString(), state.VMID.ValueInt64(), state.Name.ValueString()); err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to Delete Proxmox %s Snapshot", r.label()), err.Error())
	}
}

func (r *snapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, vmID, name, err := parseSnapshotImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Import Identifier", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), snapshotID(node, vmID, name))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vm_id"), vmID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}

func (r *snapshotResource) readSnapshotState(ctx context.Context, node string, vmID int64, name string, prior *snapshotModel) (snapshotModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	snap, err := r.client.getSnapshot(ctx, r.kind, node, vmID, name)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return snapshotModel{ID: types.StringNull()}, diags
		}
		diags.AddError(fmt.Sprintf("Unable to Read Proxmox %s Snapshot", r.label()), err.Error())
		return snapshotModel{}, diags
	}
	description := stringOrNull(snap.Description)
	if prior != nil && description.IsNull() && !prior.Description.IsNull() && !prior.Description.IsUnknown() {
		description = prior.Description
	}
	return snapshotModel{
		ID:          types.StringValue(snapshotID(node, vmID, name)),
		Node:        types.StringValue(node),
		VMID:        types.Int64Value(vmID),
		Name:        types.StringValue(name),
		Description: description,
		Parent:      stringOrNull(snap.Parent),
		Snaptime:    int64OrNull(snap.Snaptime.Ptr()),
	}, diags
}
