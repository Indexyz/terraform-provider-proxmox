// Copyright IBM Corp. 2021, 2026
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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &StorageResource{}
var _ resource.ResourceWithImportState = &StorageResource{}

type StorageResource struct {
	client *Client
}

func NewStorageResource() resource.Resource {
	return &StorageResource{}
}

func (r *StorageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage"
}

func (r *StorageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceschema.Schema{
		MarkdownDescription: "Manages a Proxmox VE storage pool through `/storage`.",
		Attributes: map[string]resourceschema.Attribute{
			"id": resourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Storage identifier (same as `storage`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"storage": resourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The storage identifier. Changes require replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"type": resourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Storage type (e.g. `dir`, `lvm`, `lvmthin`, `nfs`, `zfs`, `rbd`, `cephfs`, `cifs`, `iscsi`, `pbs`, `btrfs`, `zfspool`, `esxi`, `iscsidirect`). Changes require replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"content":     resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Allowed content types (comma-separated, e.g. `images,rootdir,vztmpl,iso,backup,snippets`)."},
			"nodes":       resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "List of cluster nodes this storage applies to."},
			"disable":     resourceschema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Disable the storage."},
			"shared":      resourceschema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Indicate this is a single storage shared across nodes."},
			"path":        resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "File system path (for `dir` type)."},
			"pool":        resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Pool name (for `rbd`/`zfs`)."},
			"vg_name":     resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Volume group name (for `lvm`/`lvmthin`)."},
			"thin_pool":   resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "LVM thin pool LV name (for `lvmthin`)."},
			"server":      resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Server IP or DNS name (for `nfs`/`cifs`/`iscsi`/`pbs`/`rbd`)."},
			"export":      resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "NFS export path (for `nfs`)."},
			"share":       resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "CIFS share (for `cifs`)."},
			"username":    resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Username for accessing the share/datastore."},
			"password":    resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Password for accessing the share/datastore.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"monhost":     resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Monitor host IP addresses (for external Ceph)."},
			"datastore":   resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "PBS datastore name (for `pbs`)."},
			"namespace":   resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Namespace (for `pbs`/`rbd`)."},
			"fingerprint": resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Certificate SHA256 fingerprint (for `pbs`)."},
			"smb_version": resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "SMB protocol version (for `cifs`)."},
			"options":     resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "NFS/CIFS mount options."},
			"format":      resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Default image format."},
			"mkdir":       resourceschema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Create the directory if it doesn't exist."},
			"sparse":      resourceschema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Use sparse volumes."},
			"nocow":       resourceschema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Set the NOCOW flag (for `btrfs`/`zfspool`)."},
			"krbd":        resourceschema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Always access rbd through krbd kernel module."},
			"blocksize":   resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Block size."},
			"fs_name":     resourceschema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Ceph filesystem name (for `cephfs`)."},
			"raw": resourceschema.SingleNestedAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Escape hatch for `/storage` keys that this provider version does not type yet.",
				Attributes: map[string]resourceschema.Attribute{
					"extra_config": resourceschema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Raw Proxmox storage config entries keyed by their exact config key."},
				},
			},
		},
	}
}

func (r *StorageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *StorageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan storageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if plan.Storage.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(path.Root("storage"), "Missing storage identifier", "The storage attribute must be a non-empty value.")
		return
	}
	createReq, diags := storageRequestFromModel(ctx, plan, storageModel{})
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	createReq.Delete = nil
	if err := r.client.CreateStorage(ctx, createReq); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Storage", err.Error())
		return
	}
	state, diags := r.readStorageState(ctx, plan.Storage.ValueString(), &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *StorageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state storageModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readStorageState(ctx, state.Storage.ValueString(), &state)
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

func (r *StorageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan storageModel
	var state storageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateReq, diags := storageRequestFromModel(ctx, plan, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !updateReq.IsEmpty() {
		if err := r.client.UpdateStorage(ctx, plan.Storage.ValueString(), updateReq); err != nil {
			resp.Diagnostics.AddError("Unable to Update Proxmox Storage", err.Error())
			return
		}
	}
	refreshed, diags := r.readStorageState(ctx, plan.Storage.ValueString(), &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *StorageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state storageModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteStorage(ctx, state.Storage.ValueString()); err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Storage", err.Error())
	}
}

func (r *StorageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("storage"), req.ID)...)
}

func (r *StorageResource) readStorageState(ctx context.Context, id string, prior *storageModel) (storageModel, diag.Diagnostics) {
	config, err := r.client.GetStorage(ctx, id)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return storageModel{ID: types.StringNull()}, nil
		}
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox Storage", fmt.Sprintf("Unable to read storage %q: %s", id, err))
		return storageModel{}, diags
	}
	return storageStateFromAPI(ctx, config, prior)
}
