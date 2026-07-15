// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &StorageFileDownloadResource{}
var _ resource.ResourceWithValidateConfig = &StorageFileDownloadResource{}

var storageFileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type StorageFileDownloadResource struct {
	client *Client
}

type storageFileDownloadModel struct {
	ID                 types.String `tfsdk:"id"`
	Node               types.String `tfsdk:"node"`
	Storage            types.String `tfsdk:"storage"`
	VolumeID           types.String `tfsdk:"volume_id"`
	Content            types.String `tfsdk:"content"`
	Filename           types.String `tfsdk:"filename"`
	URL                types.String `tfsdk:"url"`
	Checksum           types.String `tfsdk:"checksum"`
	ChecksumAlgorithm  types.String `tfsdk:"checksum_algorithm"`
	Compression        types.String `tfsdk:"compression"`
	VerifyCertificates types.Bool   `tfsdk:"verify_certificates"`
	Format             types.String `tfsdk:"format"`
	Path               types.String `tfsdk:"path"`
	Size               types.Int64  `tfsdk:"size"`
	Used               types.Int64  `tfsdk:"used"`
	Notes              types.String `tfsdk:"notes"`
	Protected          types.Bool   `tfsdk:"protected"`
}

func NewStorageFileDownloadResource() resource.Resource {
	return &StorageFileDownloadResource{}
}

func (r *StorageFileDownloadResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage_file_download"
}

func (r *StorageFileDownloadResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replaceString := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Downloads an ISO, LXC template, or import image into Proxmox storage through `/nodes/{node}/storage/{storage}/download-url`, waits for the task, and manages the resulting storage content item.",
		Attributes: map[string]schema.Attribute{
			"id":                  schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `node/storage/volume_id` form."},
			"node":                schema.StringAttribute{Required: true, MarkdownDescription: "Proxmox node that performs the download. Changes require replacement.", PlanModifiers: replaceString},
			"storage":             schema.StringAttribute{Required: true, MarkdownDescription: "Destination storage identifier. Changes require replacement.", PlanModifiers: replaceString},
			"volume_id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Proxmox volume identifier of the downloaded file."},
			"content":             schema.StringAttribute{Required: true, MarkdownDescription: "Storage content type: `iso`, `vztmpl`, or `import`. Changes require replacement.", PlanModifiers: replaceString},
			"filename":            schema.StringAttribute{Required: true, MarkdownDescription: "Destination filename. Use only letters, numbers, dots, underscores, and hyphens. Changes require replacement.", PlanModifiers: replaceString},
			"url":                 schema.StringAttribute{Required: true, Sensitive: true, MarkdownDescription: "HTTP or HTTPS URL to download. Stored in Terraform state. Changes require replacement.", PlanModifiers: replaceString},
			"checksum":            schema.StringAttribute{Optional: true, MarkdownDescription: "Expected file checksum. Must be set together with `checksum_algorithm`. Changes require replacement.", PlanModifiers: replaceString},
			"checksum_algorithm":  schema.StringAttribute{Optional: true, MarkdownDescription: "Checksum algorithm: `md5`, `sha1`, `sha224`, `sha256`, `sha384`, or `sha512`. Changes require replacement.", PlanModifiers: replaceString},
			"compression":         schema.StringAttribute{Optional: true, MarkdownDescription: "Compression algorithm used to decompress the download. Changes require replacement.", PlanModifiers: replaceString},
			"verify_certificates": schema.BoolAttribute{Optional: true, MarkdownDescription: "Verify TLS certificates. Proxmox defaults to true. Changes require replacement.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"format":              schema.StringAttribute{Computed: true, MarkdownDescription: "Downloaded storage content format."},
			"path":                schema.StringAttribute{Computed: true, MarkdownDescription: "Server-side path of the downloaded file."},
			"size":                schema.Int64Attribute{Computed: true, MarkdownDescription: "File size in bytes."},
			"used":                schema.Int64Attribute{Computed: true, MarkdownDescription: "Reported used bytes."},
			"notes":               schema.StringAttribute{Computed: true, MarkdownDescription: "Storage content notes."},
			"protected":           schema.BoolAttribute{Computed: true, MarkdownDescription: "Storage content protection status when supported."},
		},
	}
}

func (r *StorageFileDownloadResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *StorageFileDownloadResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config storageFileDownloadModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateStorageFileDownloadConfig(config)...)
}

func validateStorageFileDownloadConfig(config storageFileDownloadModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if !config.Content.IsNull() && !config.Content.IsUnknown() && !slices.Contains([]string{"iso", "vztmpl", "import"}, config.Content.ValueString()) {
		diags.AddAttributeError(path.Root("content"), "Invalid storage content type", "content must be iso, vztmpl, or import")
	}
	if !config.Filename.IsNull() && !config.Filename.IsUnknown() && !storageFileNamePattern.MatchString(config.Filename.ValueString()) {
		diags.AddAttributeError(path.Root("filename"), "Invalid storage filename", "filename must start with a letter or number and contain only letters, numbers, dots, underscores, and hyphens")
	}
	if !config.URL.IsNull() && !config.URL.IsUnknown() && !strings.HasPrefix(config.URL.ValueString(), "http://") && !strings.HasPrefix(config.URL.ValueString(), "https://") {
		diags.AddAttributeError(path.Root("url"), "Invalid download URL", "url must use http or https")
	}
	checksumSet := !config.Checksum.IsNull() && !config.Checksum.IsUnknown() && config.Checksum.ValueString() != ""
	algorithmSet := !config.ChecksumAlgorithm.IsNull() && !config.ChecksumAlgorithm.IsUnknown() && config.ChecksumAlgorithm.ValueString() != ""
	if checksumSet != algorithmSet {
		diags.AddError("Invalid checksum configuration", "checksum and checksum_algorithm must be set together")
	}
	if algorithmSet && !slices.Contains([]string{"md5", "sha1", "sha224", "sha256", "sha384", "sha512"}, config.ChecksumAlgorithm.ValueString()) {
		diags.AddAttributeError(path.Root("checksum_algorithm"), "Invalid checksum algorithm", "checksum_algorithm must be md5, sha1, sha224, sha256, sha384, or sha512")
	}
	return diags
}

func (r *StorageFileDownloadResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan storageFileDownloadModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	volumeID, err := r.client.DownloadStorageFile(ctx, DownloadStorageFileRequest{
		Node:               plan.Node.ValueString(),
		Storage:            plan.Storage.ValueString(),
		Content:            plan.Content.ValueString(),
		Filename:           plan.Filename.ValueString(),
		URL:                plan.URL.ValueString(),
		Checksum:           stringPointer(plan.Checksum),
		ChecksumAlgorithm:  stringPointer(plan.ChecksumAlgorithm),
		Compression:        stringPointer(plan.Compression),
		VerifyCertificates: boolPointerValue(plan.VerifyCertificates),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to Download Proxmox Storage File", err.Error())
		return
	}
	plan.VolumeID = types.StringValue(volumeID)
	plan.ID = types.StringValue(storageFileDownloadID(plan.Node.ValueString(), plan.Storage.ValueString(), volumeID))
	state, diags := r.readState(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *StorageFileDownloadResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state storageFileDownloadModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state)
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

func (r *StorageFileDownloadResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state storageFileDownloadModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *StorageFileDownloadResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state storageFileDownloadModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteStorageFile(ctx, state.Node.ValueString(), state.Storage.ValueString(), state.VolumeID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Storage File", err.Error())
	}
}

func (r *StorageFileDownloadResource) readState(ctx context.Context, state storageFileDownloadModel) (storageFileDownloadModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	file, err := r.client.GetStorageFile(ctx, state.Node.ValueString(), state.Storage.ValueString(), state.VolumeID.ValueString())
	if err != nil {
		if errors.Is(err, errNotFound) {
			return storageFileDownloadModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Storage File", err.Error())
		return storageFileDownloadModel{}, diags
	}
	state.ID = types.StringValue(storageFileDownloadID(state.Node.ValueString(), state.Storage.ValueString(), state.VolumeID.ValueString()))
	state.Format = stringOrNull(file.Format)
	state.Path = stringOrNull(file.Path)
	state.Size = int64OrNull(file.Size.Ptr())
	state.Used = int64OrNull(file.Used.Ptr())
	state.Notes = stringOrNull(file.Notes)
	state.Protected = boolOrNull(file.Protected.Ptr())
	return state, diags
}

func storageFileDownloadID(node, storage, volume string) string {
	return strings.Join([]string{node, storage, volume}, "/")
}
