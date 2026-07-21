// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &PoolResource{}
var _ resource.ResourceWithImportState = &PoolResource{}

type PoolResource struct {
	client *Client
}

type PoolResourceModel struct {
	ID         types.String `tfsdk:"id"`
	PoolID     types.String `tfsdk:"pool_id"`
	Comment    types.String `tfsdk:"comment"`
	AllowMove  types.Bool   `tfsdk:"allow_move"`
	VMIDs      types.Set    `tfsdk:"vm_ids"`
	StorageIDs types.Set    `tfsdk:"storage_ids"`
	Members    types.List   `tfsdk:"members"`
}

type PoolResourceMemberItem struct {
	ID        types.String `tfsdk:"id"`
	Node      types.String `tfsdk:"node"`
	StorageID types.String `tfsdk:"storage_id"`
	Type      types.String `tfsdk:"type"`
	VMID      types.Int64  `tfsdk:"vm_id"`
}

func poolResourceMemberAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"id":         types.StringType,
		"node":       types.StringType,
		"storage_id": types.StringType,
		"type":       types.StringType,
		"vm_id":      types.Int64Type,
	}
}

func NewPoolResource() resource.Resource {
	return &PoolResource{}
}

func (r *PoolResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pool"
}

func (r *PoolResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Proxmox VE pools through `/pools`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Pool identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"pool_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Pool identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional pool comment.",
			},
			"allow_move": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Allow adding a guest that already belongs to another pool.",
			},
			"vm_ids": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.Int64Type,
				MarkdownDescription: "Guest VMIDs managed as members of this pool.",
			},
			"storage_ids": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Storage IDs managed as members of this pool.",
			},
			"members": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved pool members returned by Proxmox.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":         schema.StringAttribute{Computed: true},
						"node":       schema.StringAttribute{Computed: true},
						"storage_id": schema.StringAttribute{Computed: true},
						"type":       schema.StringAttribute{Computed: true},
						"vm_id":      schema.Int64Attribute{Computed: true},
					},
				},
			},
		},
	}
}

func (r *PoolResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PoolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PoolResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	comment := stringPointer(plan.Comment)
	if err := r.client.CreatePool(ctx, plan.PoolID.ValueString(), comment); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Pool", err.Error())
		return
	}

	if err := r.reconcilePoolMembership(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Unable to Configure Proxmox Pool Membership", err.Error())
		return
	}

	state, diags := r.readPoolState(ctx, plan.PoolID.ValueString(), plan.AllowMove)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *PoolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PoolResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	refreshed, diags := r.readPoolState(ctx, state.PoolID.ValueString(), state.AllowMove)
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

func (r *PoolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PoolResourceModel
	var state PoolResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	currentPool, err := r.client.GetPool(ctx, state.PoolID.ValueString())
	if err != nil {
		if errors.Is(err, errNotFound) {
			resp.Diagnostics.AddError(
				"Proxmox Pool No Longer Exists",
				fmt.Sprintf("Pool %q was not found in Proxmox VE.", state.PoolID.ValueString()),
			)
			return
		}

		resp.Diagnostics.AddError("Unable to Read Current Proxmox Pool", err.Error())
		return
	}

	if commentChanged(plan.Comment, currentPool.Comment) {
		if err := r.client.UpdatePool(ctx, UpdatePoolRequest{
			PoolID:  plan.PoolID.ValueString(),
			Comment: stringPointer(plan.Comment),
		}); err != nil {
			resp.Diagnostics.AddError("Unable to Update Proxmox Pool Comment", err.Error())
			return
		}
	}

	desiredVMIDs, diags := expandInt64Set(ctx, plan.VMIDs)
	resp.Diagnostics.Append(diags...)
	desiredStorageIDs, diags := expandStringSet(ctx, plan.StorageIDs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	currentVMIDs, currentStorageIDs, _ := flattenPoolMembers(currentPool)
	addVMIDs, removeVMIDs := diffInt64s(currentVMIDs, desiredVMIDs)
	addStorageIDs, removeStorageIDs := diffStrings(currentStorageIDs, desiredStorageIDs)

	if len(removeVMIDs) > 0 || len(removeStorageIDs) > 0 {
		if err := r.client.UpdatePool(ctx, UpdatePoolRequest{
			PoolID:     plan.PoolID.ValueString(),
			Delete:     true,
			StorageIDs: removeStorageIDs,
			VMIDs:      removeVMIDs,
		}); err != nil {
			resp.Diagnostics.AddError("Unable to Remove Proxmox Pool Members", err.Error())
			return
		}
	}

	if len(addVMIDs) > 0 || len(addStorageIDs) > 0 {
		if err := r.client.UpdatePool(ctx, UpdatePoolRequest{
			PoolID:     plan.PoolID.ValueString(),
			AllowMove:  boolValueFromType(plan.AllowMove),
			StorageIDs: addStorageIDs,
			VMIDs:      addVMIDs,
		}); err != nil {
			resp.Diagnostics.AddError("Unable to Add Proxmox Pool Members", err.Error())
			return
		}
	}

	refreshed, diags := r.readPoolState(ctx, plan.PoolID.ValueString(), plan.AllowMove)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *PoolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PoolResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	currentPool, err := r.client.GetPool(ctx, state.PoolID.ValueString())
	if err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Unable to Read Proxmox Pool Before Delete", err.Error())
		return
	}

	if err == nil {
		currentVMIDs, currentStorageIDs, _ := flattenPoolMembers(currentPool)
		if len(currentVMIDs) > 0 || len(currentStorageIDs) > 0 {
			if err := r.client.UpdatePool(ctx, UpdatePoolRequest{
				PoolID:     state.PoolID.ValueString(),
				Delete:     true,
				StorageIDs: currentStorageIDs,
				VMIDs:      currentVMIDs,
			}); err != nil {
				resp.Diagnostics.AddError("Unable to Empty Proxmox Pool", err.Error())
				return
			}
		}
	}

	if err := r.client.DeletePool(ctx, state.PoolID.ValueString()); err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Pool", err.Error())
	}
}

func (r *PoolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("pool_id"), req, resp)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *PoolResource) reconcilePoolMembership(ctx context.Context, plan PoolResourceModel) error {
	vmIDs, diags := expandInt64Set(ctx, plan.VMIDs)
	if diags.HasError() {
		return fmt.Errorf("unable to decode `vm_ids`: %v", diags)
	}

	storageIDs, diags := expandStringSet(ctx, plan.StorageIDs)
	if diags.HasError() {
		return fmt.Errorf("unable to decode `storage_ids`: %v", diags)
	}

	if len(vmIDs) == 0 && len(storageIDs) == 0 {
		return nil
	}

	return r.client.UpdatePool(ctx, UpdatePoolRequest{
		PoolID:     plan.PoolID.ValueString(),
		AllowMove:  boolValueFromType(plan.AllowMove),
		StorageIDs: storageIDs,
		VMIDs:      vmIDs,
	})
}

func (r *PoolResource) readPoolState(ctx context.Context, poolID string, allowMove types.Bool) (PoolResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	pool, err := r.client.GetPool(ctx, poolID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return PoolResourceModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Pool", err.Error())
		return PoolResourceModel{}, diags
	}

	vmIDs, storageIDs, members := flattenPoolMembers(pool)
	vmIDSet, vmDiags := int64SetValue(ctx, vmIDs)
	storageIDSet, storageDiags := stringSetValue(ctx, storageIDs)
	memberList, memberDiags := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: poolResourceMemberAttrTypes()}, members)
	diags.Append(vmDiags...)
	diags.Append(storageDiags...)
	diags.Append(memberDiags...)
	if diags.HasError() {
		return PoolResourceModel{}, diags
	}

	return PoolResourceModel{
		ID:         types.StringValue(pool.PoolID),
		PoolID:     types.StringValue(pool.PoolID),
		Comment:    stringOrNull(pool.Comment),
		AllowMove:  allowMove,
		VMIDs:      vmIDSet,
		StorageIDs: storageIDSet,
		Members:    memberList,
	}, diags
}

func flattenPoolMembers(pool Pool) ([]int64, []string, []PoolResourceMemberItem) {
	vmIDs := make([]int64, 0)
	storageIDs := make([]string, 0)
	members := make([]PoolResourceMemberItem, 0, len(pool.Members))

	for _, member := range pool.Members {
		item := PoolResourceMemberItem{
			ID:        stringOrNull(member.ID),
			Node:      stringOrNull(member.Node),
			StorageID: stringOrNull(member.Storage),
			Type:      stringOrNull(member.Type),
			VMID:      int64OrNull(member.VMID),
		}
		members = append(members, item)

		switch member.Type {
		case "qemu", "lxc", "openvz":
			if member.VMID != nil {
				vmIDs = append(vmIDs, *member.VMID)
			}
		case "storage":
			if member.Storage != "" {
				storageIDs = append(storageIDs, member.Storage)
			}
		}
	}

	return sortedInt64s(vmIDs), sortedStrings(storageIDs), members
}

func stringPointer(value types.String) *string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	stringValue := value.ValueString()
	return &stringValue
}

func boolValueFromType(value types.Bool) bool {
	if value.IsNull() || value.IsUnknown() {
		return false
	}
	return value.ValueBool()
}

func commentChanged(desired types.String, current string) bool {
	if desired.IsNull() || desired.IsUnknown() {
		return current != ""
	}
	return desired.ValueString() != current
}
