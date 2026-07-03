// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type storageModel struct {
	ID          types.String `tfsdk:"id"`
	Storage     types.String `tfsdk:"storage"`
	Type        types.String `tfsdk:"type"`
	Content     types.String `tfsdk:"content"`
	Nodes       types.String `tfsdk:"nodes"`
	Disable     types.Bool   `tfsdk:"disable"`
	Shared      types.Bool   `tfsdk:"shared"`
	Path        types.String `tfsdk:"path"`
	Pool        types.String `tfsdk:"pool"`
	VGName      types.String `tfsdk:"vg_name"`
	ThinPool    types.String `tfsdk:"thin_pool"`
	Server      types.String `tfsdk:"server"`
	Export      types.String `tfsdk:"export"`
	Share       types.String `tfsdk:"share"`
	Username    types.String `tfsdk:"username"`
	Password    types.String `tfsdk:"password"`
	Monhost     types.String `tfsdk:"monhost"`
	Datastore   types.String `tfsdk:"datastore"`
	Namespace   types.String `tfsdk:"namespace"`
	Fingerprint types.String `tfsdk:"fingerprint"`
	SMBVersion  types.String `tfsdk:"smb_version"`
	Options     types.String `tfsdk:"options"`
	Format      types.String `tfsdk:"format"`
	Mkdir       types.Bool   `tfsdk:"mkdir"`
	Sparse      types.Bool   `tfsdk:"sparse"`
	NoCOW       types.Bool   `tfsdk:"nocow"`
	KRBD        types.Bool   `tfsdk:"krbd"`
	Blocksize   types.String `tfsdk:"blocksize"`
	FSName      types.String `tfsdk:"fs_name"`
	Raw         types.Object `tfsdk:"raw"`
}

type storageRawModel struct {
	ExtraConfig types.Map `tfsdk:"extra_config"`
}

func storageRawAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"extra_config": types.MapType{ElemType: types.StringType},
	}
}

func storageStateFromAPI(ctx context.Context, config Storage, prior *storageModel) (storageModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	rawValue, rawDiags := storageRawStateValue(ctx, config.ExtraConfig)
	diags.Append(rawDiags...)

	password := stringOrNull(config.Password)
	if password.IsNull() && prior != nil && !prior.Password.IsNull() && !prior.Password.IsUnknown() {
		password = prior.Password
	}

	return storageModel{
		ID:          types.StringValue(config.Storage),
		Storage:     types.StringValue(config.Storage),
		Type:        types.StringValue(config.Type),
		Content:     stringOrNull(config.Content),
		Nodes:       stringOrNull(config.Nodes),
		Disable:     boolOrNull(config.Disable.Ptr()),
		Shared:      boolOrNull(config.Shared.Ptr()),
		Path:        stringOrNull(config.Path),
		Pool:        stringOrNull(config.Pool),
		VGName:      stringOrNull(config.VGName),
		ThinPool:    stringOrNull(config.ThinPool),
		Server:      stringOrNull(config.Server),
		Export:      stringOrNull(config.Export),
		Share:       stringOrNull(config.Share),
		Username:    stringOrNull(config.Username),
		Password:    password,
		Monhost:     stringOrNull(config.Monhost),
		Datastore:   stringOrNull(config.Datastore),
		Namespace:   stringOrNull(config.Namespace),
		Fingerprint: stringOrNull(config.Fingerprint),
		SMBVersion:  stringOrNull(config.SMBVersion),
		Options:     stringOrNull(config.Options),
		Format:      stringOrNull(config.Format),
		Mkdir:       boolOrNull(config.Mkdir.Ptr()),
		Sparse:      boolOrNull(config.Sparse.Ptr()),
		NoCOW:       boolOrNull(config.NoCOW.Ptr()),
		KRBD:        boolOrNull(config.KRBD.Ptr()),
		Blocksize:   stringOrNull(config.Blocksize),
		FSName:      stringOrNull(config.FSName),
		Raw:         rawValue,
	}, diags
}

func storageRawStateValue(ctx context.Context, source map[string]string) (types.Object, diag.Diagnostics) {
	if len(source) == 0 {
		return types.ObjectNull(storageRawAttrTypes()), nil
	}
	extraConfig, diags := types.MapValueFrom(ctx, types.StringType, source)
	if diags.HasError() {
		return types.ObjectNull(storageRawAttrTypes()), diags
	}
	result, objDiags := types.ObjectValueFrom(ctx, storageRawAttrTypes(), storageRawModel{ExtraConfig: extraConfig})
	diags.Append(objDiags...)
	return result, diags
}

func storageRequestFromModel(ctx context.Context, plan storageModel, prior storageModel) (StorageRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	raw, rawDiags := expandStorageRawModel(ctx, plan.Raw)
	diags.Append(rawDiags...)
	if diags.HasError() {
		return StorageRequest{}, diags
	}
	var extraConfig map[string]string
	if !raw.ExtraConfig.IsNull() && !raw.ExtraConfig.IsUnknown() {
		diags.Append(raw.ExtraConfig.ElementsAs(ctx, &extraConfig, false)...)
	}
	return StorageRequest{
		Storage:     plan.Storage.ValueString(),
		Type:        plan.Type.ValueString(),
		Content:     stringPointerValue(plan.Content),
		Nodes:       stringPointerValue(plan.Nodes),
		Disable:     boolPointerValue(plan.Disable),
		Shared:      boolPointerValue(plan.Shared),
		Path:        stringPointerValue(plan.Path),
		Pool:        stringPointerValue(plan.Pool),
		VGName:      stringPointerValue(plan.VGName),
		ThinPool:    stringPointerValue(plan.ThinPool),
		Server:      stringPointerValue(plan.Server),
		Export:      stringPointerValue(plan.Export),
		Share:       stringPointerValue(plan.Share),
		Username:    stringPointerValue(plan.Username),
		Password:    stringPointerValue(plan.Password),
		Monhost:     stringPointerValue(plan.Monhost),
		Datastore:   stringPointerValue(plan.Datastore),
		Namespace:   stringPointerValue(plan.Namespace),
		Fingerprint: stringPointerValue(plan.Fingerprint),
		SMBVersion:  stringPointerValue(plan.SMBVersion),
		Options:     stringPointerValue(plan.Options),
		Format:      stringPointerValue(plan.Format),
		Mkdir:       boolPointerValue(plan.Mkdir),
		Sparse:      boolPointerValue(plan.Sparse),
		NoCOW:       boolPointerValue(plan.NoCOW),
		KRBD:        boolPointerValue(plan.KRBD),
		Blocksize:   stringPointerValue(plan.Blocksize),
		FSName:      stringPointerValue(plan.FSName),
		Delete:      storageDeleteKeys(plan, prior),
		ExtraConfig: extraConfig,
	}, diags
}

func storageDeleteKeys(plan storageModel, prior storageModel) []string {
	var keys []string
	keys = appendDeletedString(keys, "content", plan.Content, prior.Content)
	keys = appendDeletedString(keys, "nodes", plan.Nodes, prior.Nodes)
	keys = appendDeletedString(keys, "path", plan.Path, prior.Path)
	keys = appendDeletedString(keys, "pool", plan.Pool, prior.Pool)
	keys = appendDeletedString(keys, "vgname", plan.VGName, prior.VGName)
	keys = appendDeletedString(keys, "thinpool", plan.ThinPool, prior.ThinPool)
	keys = appendDeletedString(keys, "server", plan.Server, prior.Server)
	keys = appendDeletedString(keys, "export", plan.Export, prior.Export)
	keys = appendDeletedString(keys, "share", plan.Share, prior.Share)
	keys = appendDeletedString(keys, "username", plan.Username, prior.Username)
	keys = appendDeletedString(keys, "monhost", plan.Monhost, prior.Monhost)
	keys = appendDeletedString(keys, "datastore", plan.Datastore, prior.Datastore)
	keys = appendDeletedString(keys, "namespace", plan.Namespace, prior.Namespace)
	keys = appendDeletedString(keys, "fingerprint", plan.Fingerprint, prior.Fingerprint)
	keys = appendDeletedString(keys, "smbversion", plan.SMBVersion, prior.SMBVersion)
	keys = appendDeletedString(keys, "options", plan.Options, prior.Options)
	keys = appendDeletedString(keys, "format", plan.Format, prior.Format)
	keys = appendDeletedString(keys, "blocksize", plan.Blocksize, prior.Blocksize)
	keys = appendDeletedString(keys, "fs-name", plan.FSName, prior.FSName)
	keys = appendDeletedBool(keys, "disable", plan.Disable, prior.Disable)
	keys = appendDeletedBool(keys, "shared", plan.Shared, prior.Shared)
	keys = appendDeletedBool(keys, "mkdir", plan.Mkdir, prior.Mkdir)
	keys = appendDeletedBool(keys, "sparse", plan.Sparse, prior.Sparse)
	keys = appendDeletedBool(keys, "nocow", plan.NoCOW, prior.NoCOW)
	keys = appendDeletedBool(keys, "krbd", plan.KRBD, prior.KRBD)
	return keys
}

func expandStorageRawModel(ctx context.Context, value types.Object) (storageRawModel, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return storageRawModel{ExtraConfig: types.MapNull(types.StringType)}, nil
	}
	var result storageRawModel
	diags := value.As(ctx, &result, qemuObjectAsOptions())
	return result, diags
}
