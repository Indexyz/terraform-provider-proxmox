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
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &QemuVMResource{}
var _ resource.ResourceWithImportState = &QemuVMResource{}

type QemuVMResource struct {
	client *Client
}

func NewQemuVMResource() resource.Resource {
	return &QemuVMResource{}
}

func (r *QemuVMResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_vm"
}

func (r *QemuVMResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceschema.Schema{
		MarkdownDescription: "Manages a minimal Proxmox VE QEMU virtual machine through `/nodes/{node}/qemu` and `/config`.",
		Attributes:          qemuVMResourceAttributes(),
	}
}

func (r *QemuVMResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *QemuVMResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan qemuVMModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.CreateQemuVM(ctx, plan.Node.ValueString(), qemuVMCreateRequestFromModel(plan)); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox QEMU VM", err.Error())
		return
	}

	state, diags := r.readQemuVMState(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *QemuVMResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state qemuVMModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	refreshed, diags := r.readQemuVMState(ctx, state.Node.ValueString(), state.VMID.ValueInt64())
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

func (r *QemuVMResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan qemuVMModel
	var state qemuVMModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.GetQemuVMConfig(ctx, state.Node.ValueString(), state.VMID.ValueInt64())
	if err != nil {
		if errors.Is(err, errNotFound) {
			resp.Diagnostics.AddError(
				"Proxmox QEMU VM No Longer Exists",
				fmt.Sprintf("QEMU virtual machine %q no longer exists on node %q.", qemuVMID(state.Node.ValueString(), state.VMID.ValueInt64()), state.Node.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError("Unable to Read Current Proxmox QEMU VM", err.Error())
		return
	}

	if err := r.client.UpdateQemuVM(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), qemuVMUpdateRequestFromModel(plan)); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox QEMU VM", err.Error())
		return
	}

	refreshed, diags := r.readQemuVMState(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *QemuVMResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state qemuVMModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteQemuVM(ctx, state.Node.ValueString(), state.VMID.ValueInt64()); err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Unable to Delete Proxmox QEMU VM", err.Error())
	}
}

func (r *QemuVMResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, vmID, err := parseQemuVMImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Import Identifier", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), qemuVMID(node, vmID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vm_id"), vmID)...)
}

func (r *QemuVMResource) readQemuVMState(ctx context.Context, node string, vmID int64) (qemuVMModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	config, err := r.client.GetQemuVMConfig(ctx, node, vmID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return qemuVMModel{ID: types.StringNull()}, diags
		}

		diags.AddError("Unable to Read Proxmox QEMU VM Config", err.Error())
		return qemuVMModel{}, diags
	}

	status, err := r.client.GetQemuVMStatus(ctx, node, vmID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return qemuVMModel{ID: types.StringNull()}, diags
		}

		diags.AddError("Unable to Read Proxmox QEMU VM Status", err.Error())
		return qemuVMModel{}, diags
	}

	return qemuVMStateFromAPI(node, vmID, config, status), diags
}
