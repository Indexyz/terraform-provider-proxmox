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
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &LXCContainerResource{}
var _ resource.ResourceWithImportState = &LXCContainerResource{}
var _ resource.ResourceWithValidateConfig = &LXCContainerResource{}

type LXCContainerResource struct {
	client *Client
}

func NewLXCContainerResource() resource.Resource {
	return &LXCContainerResource{}
}

func (r *LXCContainerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lxc_container"
}

func (r *LXCContainerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceschema.Schema{
		MarkdownDescription: "Manages a Proxmox VE LXC container through `/nodes/{node}/lxc`, `/config`, and `/status/current` endpoints.",
		Attributes:          lxcContainerResourceAttributes(),
	}
}

func (r *LXCContainerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *LXCContainerResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config lxcContainerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(validateLXCContainerRawConflicts(ctx, config)...)
	resp.Diagnostics.Append(validateLXCContainerMapKeys(ctx, config)...)
}

func (r *LXCContainerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan lxcContainerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Clone.IsNull() && !plan.Clone.IsUnknown() {
		cloneReq, diags := lxcContainerCloneRequestFromModel(ctx, plan)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := r.client.CloneLXCContainer(ctx, cloneReq); err != nil {
			resp.Diagnostics.AddError("Unable to Clone Proxmox LXC Container", err.Error())
			return
		}

		updateReq, diags := lxcContainerUpdateRequestFromModel(ctx, plan, lxcContainerModel{})
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if !updateReq.IsEmpty() {
			if err := r.client.UpdateLXCContainer(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), updateReq); err != nil {
				resp.Diagnostics.AddError("Unable to Update Cloned Proxmox LXC Container", err.Error())
				return
			}
		}
	} else {
		validateRequiredLXCContainerCreateAttribute(&resp.Diagnostics, path.Root("ostemplate"), plan.OSTemplate, "ostemplate")
		validateRequiredLXCContainerCreateAttribute(&resp.Diagnostics, path.Root("rootfs"), plan.RootFS, "rootfs")
		if resp.Diagnostics.HasError() {
			return
		}

		createReq, diags := lxcContainerCreateRequestFromModel(ctx, plan)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		if err := r.client.CreateLXCContainer(ctx, plan.Node.ValueString(), createReq); err != nil {
			resp.Diagnostics.AddError("Unable to Create Proxmox LXC Container", err.Error())
			return
		}
	}

	state, diags := r.readLXCContainerState(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *LXCContainerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state lxcContainerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	refreshed, diags := r.readLXCContainerState(ctx, state.Node.ValueString(), state.VMID.ValueInt64(), &state)
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

func (r *LXCContainerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan lxcContainerModel
	var state lxcContainerModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.GetLXCContainerConfig(ctx, state.Node.ValueString(), state.VMID.ValueInt64())
	if err != nil {
		if errors.Is(err, errNotFound) {
			resp.Diagnostics.AddError(
				"Proxmox LXC Container No Longer Exists",
				fmt.Sprintf("LXC container %q no longer exists on node %q.", lxcContainerID(state.Node.ValueString(), state.VMID.ValueInt64()), state.Node.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError("Unable to Read Current Proxmox LXC Container", err.Error())
		return
	}

	updateReq, diags := lxcContainerUpdateRequestFromModel(ctx, plan, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !updateReq.IsEmpty() {
		if err := r.client.UpdateLXCContainer(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), updateReq); err != nil {
			resp.Diagnostics.AddError("Unable to Update Proxmox LXC Container", err.Error())
			return
		}
	}

	refreshed, diags := r.readLXCContainerState(ctx, plan.Node.ValueString(), plan.VMID.ValueInt64(), &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *LXCContainerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state lxcContainerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteLXCContainer(ctx, state.Node.ValueString(), state.VMID.ValueInt64()); err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Unable to Delete Proxmox LXC Container", err.Error())
	}
}

func (r *LXCContainerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, vmID, err := parseLXCContainerImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Import Identifier", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), lxcContainerID(node, vmID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vm_id"), vmID)...)
}

func (r *LXCContainerResource) readLXCContainerState(ctx context.Context, node string, vmID int64, prior *lxcContainerModel) (lxcContainerModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	config, err := r.client.GetLXCContainerConfig(ctx, node, vmID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return lxcContainerModel{ID: types.StringNull()}, diags
		}

		diags.AddError("Unable to Read Proxmox LXC Container Config", err.Error())
		return lxcContainerModel{}, diags
	}

	status, err := r.client.GetLXCContainerStatus(ctx, node, vmID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return lxcContainerModel{ID: types.StringNull()}, diags
		}

		diags.AddError("Unable to Read Proxmox LXC Container Status", err.Error())
		return lxcContainerModel{}, diags
	}

	state, stateDiags := lxcContainerStateFromAPI(ctx, node, vmID, config, status, prior)
	diags.Append(stateDiags...)
	return state, diags
}

func validateRequiredLXCContainerCreateAttribute(diags *diag.Diagnostics, attrPath path.Path, value types.String, name string) {
	if value.IsNull() || value.IsUnknown() || strings.TrimSpace(value.ValueString()) == "" {
		diags.AddAttributeError(
			attrPath,
			"Missing Required LXC Container Create Attribute",
			fmt.Sprintf("The %s attribute must be set to a non-empty value when creating an LXC container.", name),
		)
	}
}

func (r UpdateLXCContainerRequest) IsEmpty() bool {
	return r.Hostname == nil &&
		r.Description == nil &&
		r.Tags == nil &&
		r.Startup == nil &&
		r.Features == nil &&
		r.Console == nil &&
		r.TTY == nil &&
		r.CMode == nil &&
		r.Hookscript == nil &&
		r.OSType == nil &&
		r.Nameserver == nil &&
		r.Searchdomain == nil &&
		r.Timezone == nil &&
		r.OnBoot == nil &&
		r.Protection == nil &&
		r.Cores == nil &&
		r.CPULimit == nil &&
		r.CPUUnits == nil &&
		r.Memory == nil &&
		r.Swap == nil &&
		len(r.Network) == 0 &&
		len(r.MountPoint) == 0 &&
		len(r.ExtraConfig) == 0 &&
		len(r.Delete) == 0
}
