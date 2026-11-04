// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"
	"path/filepath"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource = &StorageUploadResource{}
	_ resource.Resource = &StorageDownloadURLResource{}
)

type StorageUploadResource struct{ client *Client }
type StorageDownloadURLResource struct{ client *Client }

type storageUploadModel struct {
	ID                types.String `tfsdk:"id"`
	Node              types.String `tfsdk:"node"`
	Storage           types.String `tfsdk:"storage"`
	Source            types.String `tfsdk:"source"`
	Content           types.String `tfsdk:"content"`
	Filename          types.String `tfsdk:"filename"`
	Checksum          types.String `tfsdk:"checksum"`
	ChecksumAlgorithm types.String `tfsdk:"checksum_algorithm"`
	VolumeID          types.String `tfsdk:"volume_id"`
	UPID              types.String `tfsdk:"upid"`
	Triggers          types.Map    `tfsdk:"triggers"`
}
type storageDownloadURLModel struct {
	ID                 types.String `tfsdk:"id"`
	Node               types.String `tfsdk:"node"`
	Storage            types.String `tfsdk:"storage"`
	URL                types.String `tfsdk:"url"`
	Content            types.String `tfsdk:"content"`
	Filename           types.String `tfsdk:"filename"`
	Checksum           types.String `tfsdk:"checksum"`
	ChecksumAlgorithm  types.String `tfsdk:"checksum_algorithm"`
	Compression        types.String `tfsdk:"compression"`
	VerifyCertificates types.Bool   `tfsdk:"verify_certificates"`
	VolumeID           types.String `tfsdk:"volume_id"`
	UPID               types.String `tfsdk:"upid"`
	Triggers           types.Map    `tfsdk:"triggers"`
}

func NewStorageUploadResource() resource.Resource      { return &StorageUploadResource{} }
func NewStorageDownloadURLResource() resource.Resource { return &StorageDownloadURLResource{} }

func (r *StorageUploadResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage_upload"
}
func (r *StorageDownloadURLResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage_download_url"
}

func storageTransferBase() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id":        schema.StringAttribute{Computed: true},
		"node":      schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"storage":   schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"content":   schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"volume_id": schema.StringAttribute{Computed: true},
		"upid":      schema.StringAttribute{Computed: true},
		"triggers":  schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifierRequiresReplace()}},
	}
}

func (r *StorageUploadResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := storageTransferBase()
	attrs["source"] = schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attrs["filename"] = schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attrs["checksum"] = schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attrs["checksum_algorithm"] = schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	resp.Schema = schema.Schema{MarkdownDescription: "Uploads a local file to storage through `POST /nodes/{node}/storage/{storage}/upload`. The file is read at apply time; changing its content requires a `triggers` change. Destroy deletes the uploaded volume.", Attributes: attrs}
}
func (r *StorageDownloadURLResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := storageTransferBase()
	attrs["url"] = schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attrs["filename"] = schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attrs["checksum"] = schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attrs["checksum_algorithm"] = schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attrs["compression"] = schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attrs["verify_certificates"] = schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}}
	resp.Schema = schema.Schema{MarkdownDescription: "Downloads a URL directly into storage through `POST /nodes/{node}/storage/{storage}/download-url`. Destroy deletes the downloaded volume.", Attributes: attrs}
}

func (r *StorageUploadResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *StorageDownloadURLResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *StorageUploadResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan storageUploadModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	filename := stringValue(plan.Filename)
	if filename == "" {
		filename = filepath.Base(plan.Source.ValueString())
	}
	data, err := os.ReadFile(plan.Source.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Upload Source", err.Error())
		return
	}
	upid, _, err := r.client.UploadStorageFile(ctx, UploadStorageFileRequest{Node: plan.Node.ValueString(), Storage: plan.Storage.ValueString(), Content: plan.Content.ValueString(), Filename: filename, Data: data, Checksum: stringValue(plan.Checksum), ChecksumAlgorithm: stringValue(plan.ChecksumAlgorithm)})
	if err != nil {
		resp.Diagnostics.AddError("Unable to Upload Storage File", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	plan.ID = types.StringValue(storageVolumeID(plan.Storage.ValueString(), plan.Content.ValueString(), filename))
	plan.Filename = types.StringValue(filename)
	plan.VolumeID = types.StringValue(plan.ID.ValueString())
	plan.UPID = stringOrNull(upid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *StorageUploadResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *StorageUploadResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *StorageUploadResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state storageUploadModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.deleteVolume(ctx, state.Node.ValueString(), state.Storage.ValueString(), state.VolumeID.ValueString(), &resp.Diagnostics)
}

func (r *StorageDownloadURLResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan storageDownloadURLModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.DownloadStorageURL(ctx, plan.Node.ValueString(), plan.Storage.ValueString(), plan.URL.ValueString(), plan.Content.ValueString(), plan.Filename.ValueString(), stringValue(plan.Checksum), stringValue(plan.ChecksumAlgorithm), stringValue(plan.Compression), plan.VerifyCertificates.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Download Storage URL", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	plan.ID = types.StringValue(storageVolumeID(plan.Storage.ValueString(), plan.Content.ValueString(), plan.Filename.ValueString()))
	plan.VolumeID = types.StringValue(plan.ID.ValueString())
	plan.UPID = stringOrNull(upid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *StorageDownloadURLResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *StorageDownloadURLResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *StorageDownloadURLResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state storageDownloadURLModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.deleteVolume(ctx, state.Node.ValueString(), state.Storage.ValueString(), state.VolumeID.ValueString(), &resp.Diagnostics)
}

func (r *StorageUploadResource) deleteVolume(ctx context.Context, node, storage, volume string, diags *diag.Diagnostics) {
	if err := r.client.DeleteStorageVolume(ctx, node, storage, volume); err != nil && !isNotFoundError(err) {
		diags.AddError("Unable to Delete Uploaded Volume", err.Error())
	}
}
func (r *StorageDownloadURLResource) deleteVolume(ctx context.Context, node, storage, volume string, diags *diag.Diagnostics) {
	if err := r.client.DeleteStorageVolume(ctx, node, storage, volume); err != nil && !isNotFoundError(err) {
		diags.AddError("Unable to Delete Downloaded Volume", err.Error())
	}
}

func validStorageContent(content string) bool {
	return content == "iso" || content == "vztmpl" || content == "import"
}

func (r *StorageUploadResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config storageUploadModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.Content.IsUnknown() || config.Content.IsNull() {
		return
	}
	if !validStorageContent(config.Content.ValueString()) {
		resp.Diagnostics.AddError("Invalid Storage Content", "content must be iso, vztmpl, or import")
	}
}
func (r *StorageDownloadURLResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config storageDownloadURLModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.Content.IsUnknown() || config.Content.IsNull() {
		return
	}
	if !validStorageContent(config.Content.ValueString()) {
		resp.Diagnostics.AddError("Invalid Storage Content", "content must be iso, vztmpl, or import")
	}
}
