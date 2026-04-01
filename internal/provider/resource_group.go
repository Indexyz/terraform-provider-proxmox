// Copyright (c) HashiCorp, Inc.
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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &GroupResource{}
var _ resource.ResourceWithImportState = &GroupResource{}

type GroupResource struct {
	client *Client
}

type GroupResourceModel struct {
	ID      types.String `tfsdk:"id"`
	GroupID types.String `tfsdk:"group_id"`
	Comment types.String `tfsdk:"comment"`
	Members types.List   `tfsdk:"members"`
}

func NewGroupResource() resource.Resource {
	return &GroupResource{}
}

func (r *GroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (r *GroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Proxmox VE access groups through `/access/groups`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Group identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Proxmox group identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional group comment.",
			},
			"members": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Users currently assigned to the group.",
			},
		},
	}
}

func (r *GroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan GroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.CreateGroup(ctx, plan.GroupID.ValueString(), stringPointer(plan.Comment)); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Group", err.Error())
		return
	}

	state, diags := r.readGroupState(ctx, plan.GroupID.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *GroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state GroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	refreshed, diags := r.readGroupState(ctx, state.GroupID.ValueString())
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

func (r *GroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan GroupResourceModel
	var state GroupResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	current, err := r.client.GetGroup(ctx, state.GroupID.ValueString())
	if err != nil {
		if errors.Is(err, errNotFound) {
			resp.Diagnostics.AddError(
				"Proxmox Group No Longer Exists",
				fmt.Sprintf("Group %q was not found in Proxmox VE.", state.GroupID.ValueString()),
			)
			return
		}

		resp.Diagnostics.AddError("Unable to Read Current Proxmox Group", err.Error())
		return
	}

	if commentChanged(plan.Comment, current.Comment) {
		if err := r.client.UpdateGroup(ctx, plan.GroupID.ValueString(), stringPointer(plan.Comment)); err != nil {
			resp.Diagnostics.AddError("Unable to Update Proxmox Group", err.Error())
			return
		}
	}

	refreshed, diags := r.readGroupState(ctx, plan.GroupID.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *GroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state GroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteGroup(ctx, state.GroupID.ValueString()); err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Group", err.Error())
	}
}

func (r *GroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("group_id"), req, resp)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *GroupResource) readGroupState(ctx context.Context, groupID string) (GroupResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	group, err := r.client.GetGroup(ctx, groupID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return GroupResourceModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Group", err.Error())
		return GroupResourceModel{}, diags
	}

	members, memberDiags := stringListValue(ctx, group.Members)
	diags.Append(memberDiags...)
	if diags.HasError() {
		return GroupResourceModel{}, diags
	}

	return GroupResourceModel{
		ID:      types.StringValue(group.GroupID),
		GroupID: types.StringValue(group.GroupID),
		Comment: stringOrNull(group.Comment),
		Members: members,
	}, diags
}
