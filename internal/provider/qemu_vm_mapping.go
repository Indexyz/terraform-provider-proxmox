// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var qemuVMNetworkModels = map[string]struct{}{
	"e1000": {}, "e1000-82540em": {}, "e1000-82544gc": {}, "e1000-82545em": {}, "e1000e": {},
	"i82551": {}, "i82557b": {}, "i82559er": {}, "ne2k_isa": {}, "ne2k_pci": {}, "pcnet": {},
	"rtl8139": {}, "virtio": {}, "vmxnet3": {},
}

func qemuVMStateFromAPI(ctx context.Context, node string, vmID int64, config QemuVMConfig, status QemuVMStatus, prior *qemuVMModel) (qemuVMModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	commonValue, commonDiags := qemuVMCommonStateValue(ctx, config, prior)
	diags.Append(commonDiags...)
	cloudInitValue, cloudInitDiags := qemuVMCloudInitStateValue(ctx, config, prior)
	diags.Append(cloudInitDiags...)
	networkValue, networkRaw, networkDiags := qemuVMNetworkStateValue(ctx, config.Network)
	diags.Append(networkDiags...)
	diskValue, diskRaw, diskDiags := qemuVMDiskStateValue(ctx, config.Disk)
	diags.Append(diskDiags...)
	tpmStateValue, extraConfigRaw, tpmStateDiags := qemuVMTPMStateValue(ctx, config.ExtraConfig)
	diags.Append(tpmStateDiags...)
	efiDiskValue, extraConfigRaw, efiDiskDiags := qemuVMEFIDiskStateValue(ctx, extraConfigRaw)
	diags.Append(efiDiskDiags...)
	vgaValue, extraConfigRaw, vgaDiags := qemuVMVGAStateValue(ctx, extraConfigRaw)
	diags.Append(vgaDiags...)
	rawValue, rawDiags := qemuVMRawStateValue(ctx, extraConfigRaw, networkRaw, diskRaw)
	diags.Append(rawDiags...)
	cloneValue := qemuVMCloneStateValue(prior)
	protection := false
	if value := config.Protection.Ptr(); value != nil {
		protection = *value
	}
	tablet := false
	if value := config.Tablet.Ptr(); value != nil {
		tablet = *value
	}

	return qemuVMModel{
		ID:          types.StringValue(qemuVMID(node, vmID)),
		Node:        types.StringValue(node),
		VMID:        types.Int64Value(vmID),
		Name:        stringOrNull(config.Name),
		Description: stringOrNull(config.Description),
		Tags:        stringOrNull(config.Tags),
		Template:    boolOrNull(config.Template.Ptr()),
		Pool:        stringOrNull(config.Pool),
		OnBoot:      boolOrNull(config.OnBoot.Ptr()),
		Protection:  types.BoolValue(protection),
		SCSIHW:      stringOrNull(config.SCSIHW),
		Tablet:      types.BoolValue(tablet),
		Startup:     stringOrNull(config.Startup),
		Bios:        stringOrNull(config.Bios),
		Machine:     stringOrNull(config.Machine),
		Agent:       stringOrNull(config.Agent),
		Cores:       int64OrNull(config.Cores.Ptr()),
		Sockets:     int64OrNull(config.Sockets.Ptr()),
		Memory:      int64OrNull(config.Memory.Ptr()),
		CPU:         stringOrNull(config.CPU),
		OSType:      stringOrNull(config.OSType),
		Boot:        stringOrNull(config.Boot),
		Common:      commonValue,
		CloudInit:   cloudInitValue,
		Network:     networkValue,
		Disk:        diskValue,
		EFIDisk:     efiDiskValue,
		TPMState:    tpmStateValue,
		VGA:         vgaValue,
		Raw:         rawValue,
		Clone:       cloneValue,
		Status:      stringOrNull(status.Status),
		Uptime:      int64OrNull(status.Uptime.Ptr()),
	}, diags
}

func qemuVMID(node string, vmID int64) string {
	return fmt.Sprintf("%s/%d", node, vmID)
}

func parseQemuVMImportID(id string) (string, int64, error) {
	parts := strings.Split(id, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", 0, fmt.Errorf("expected import identifier in node/vmid form")
	}

	vmID, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("invalid vmid %q: %w", strings.TrimSpace(parts[1]), err)
	}

	return strings.TrimSpace(parts[0]), vmID, nil
}

func qemuVMCreateRequestFromModel(ctx context.Context, model qemuVMModel) (CreateQemuVMRequest, diag.Diagnostics) {
	configReq, diags := qemuVMConfigRequestFromModel(ctx, model)
	if diags.HasError() {
		return CreateQemuVMRequest{}, diags
	}

	return CreateQemuVMRequest{
		VMID:                model.VMID.ValueInt64(),
		qemuVMConfigRequest: configReq,
	}, diags
}

func qemuVMUpdateRequestFromModel(ctx context.Context, model qemuVMModel) (UpdateQemuVMRequest, diag.Diagnostics) {
	configReq, diags := qemuVMConfigRequestFromModel(ctx, model)
	if diags.HasError() {
		return UpdateQemuVMRequest{}, diags
	}

	return UpdateQemuVMRequest{qemuVMConfigRequest: configReq}, diags
}

func qemuVMCloneRequestFromModel(ctx context.Context, model qemuVMModel) (CloneQemuVMRequest, diag.Diagnostics) {
	clone, diags := expandQemuVMCloneModel(ctx, model.Clone)
	if diags.HasError() {
		return CloneQemuVMRequest{}, diags
	}

	return CloneQemuVMRequest{
		SourceNode:   firstNonEmpty(stringValue(clone.SourceNode), stringValue(model.Node)),
		SourceVMID:   qemuInt64Value(clone.SourceVMID),
		TargetNode:   stringValue(model.Node),
		NewID:        qemuInt64Value(model.VMID),
		Name:         stringPointerValue(model.Name),
		Description:  stringPointerValue(model.Description),
		Pool:         stringPointerValue(model.Pool),
		Full:         boolPointerValue(clone.Full),
		SnapshotName: stringPointerValue(clone.SnapshotName),
		Storage:      stringPointerValue(clone.Storage),
		Format:       stringPointerValue(clone.Format),
		BWLimit:      int64PointerValue(clone.BWLimit),
	}, diags
}

func validateQemuVMRawConflicts(ctx context.Context, model qemuVMModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rawModel, rawDiags := expandQemuVMRawModel(ctx, model.Raw)
	diags.Append(rawDiags...)
	if diags.HasError() || rawModel.ExtraConfig.IsNull() || rawModel.ExtraConfig.IsUnknown() {
		return diags
	}

	var rawKeys map[string]string
	diags.Append(rawModel.ExtraConfig.ElementsAs(ctx, &rawKeys, false)...)
	if diags.HasError() {
		return diags
	}

	for _, key := range qemuVMTypedConfigKeys(ctx, model, &diags) {
		if _, ok := rawKeys[key]; ok {
			diags.AddAttributeError(
				path.Root("raw").AtName("extra_config").AtMapKey(key),
				"Conflicting raw.extra_config entry",
				fmt.Sprintf("The raw.extra_config entry %q overlaps with a typed QEMU configuration field. Remove one source of truth for that Proxmox config key.", key),
			)
		}
	}

	return diags
}

func qemuObjectAsOptions() basetypes.ObjectAsOptions {
	return basetypes.ObjectAsOptions{}
}

func qemuVMConfigRequestFromModel(ctx context.Context, model qemuVMModel) (qemuVMConfigRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	common, commonDiags := expandQemuVMCommonModel(ctx, model.Common)
	diags.Append(commonDiags...)
	cloudInit, cloudInitDiags := expandQemuVMCloudInitModel(ctx, model.CloudInit)
	diags.Append(cloudInitDiags...)
	network, networkDiags := expandQemuVMNetworkModelMap(ctx, model.Network)
	diags.Append(networkDiags...)
	disk, diskDiags := expandQemuVMDiskModelMap(ctx, model.Disk)
	diags.Append(diskDiags...)
	efiDisk, efiDiskDiags := expandQemuVMEFIDiskModel(ctx, model.EFIDisk)
	diags.Append(efiDiskDiags...)
	vga, vgaDiags := expandQemuVMVGAModel(ctx, model.VGA)
	diags.Append(vgaDiags...)
	tpmState, tpmStateDiags := expandQemuVMTPMStateModel(ctx, model.TPMState)
	diags.Append(tpmStateDiags...)
	raw, rawDiags := expandQemuVMRawModel(ctx, model.Raw)
	diags.Append(rawDiags...)
	if diags.HasError() {
		return qemuVMConfigRequest{}, diags
	}

	ipConfigValues, ipConfigDiags := encodeQemuVMIPConfigMap(ctx, cloudInit.IPConfig)
	diags.Append(ipConfigDiags...)
	networkValues := encodeQemuVMNetworkMap(network)
	diskValues := encodeQemuVMDiskMap(disk)
	if diags.HasError() {
		return qemuVMConfigRequest{}, diags
	}

	var extraConfig map[string]string
	if !raw.ExtraConfig.IsNull() && !raw.ExtraConfig.IsUnknown() {
		diags.Append(raw.ExtraConfig.ElementsAs(ctx, &extraConfig, false)...)
	}
	if !qemuVMTPMStateModelIsEmpty(tpmState) {
		if extraConfig == nil {
			extraConfig = map[string]string{}
		}
		extraConfig["tpmstate0"] = encodeQemuVMTPMState(tpmState)
	}
	if !qemuVMEFIDiskModelIsEmpty(efiDisk) {
		if extraConfig == nil {
			extraConfig = map[string]string{}
		}
		extraConfig["efidisk0"] = encodeQemuVMEFIDisk(efiDisk)
	}
	if encoded := encodeQemuVMVGA(vga); encoded != "" {
		if extraConfig == nil {
			extraConfig = map[string]string{}
		}
		extraConfig["vga"] = encoded
	}
	if diags.HasError() {
		return qemuVMConfigRequest{}, diags
	}

	return qemuVMConfigRequest{
		Name:        stringPointerValue(model.Name),
		Description: stringPointerValue(model.Description),
		Tags:        stringPointerValue(model.Tags),
		Pool:        stringPointerValue(model.Pool),
		OnBoot:      boolPointerValue(model.OnBoot),
		Protection:  boolPointerValue(model.Protection),
		SCSIHW:      stringPointerValue(model.SCSIHW),
		Tablet:      boolPointerValue(model.Tablet),
		Startup:     stringPointerValue(model.Startup),
		Bios:        stringPointerValue(model.Bios),
		Machine:     stringPointerValue(model.Machine),
		Agent:       stringPointerValue(model.Agent),
		Cores:       int64PointerValue(model.Cores),
		Sockets:     int64PointerValue(model.Sockets),
		Memory:      int64PointerValue(model.Memory),
		CPU:         stringPointerValue(model.CPU),
		OSType:      stringPointerValue(model.OSType),
		Boot:        stringPointerValue(model.Boot),
		Hotplug:     stringPointerValue(common.Hotplug),
		CICustom:    stringPointerValue(cloudInit.CICustom),
		CIPassword:  stringPointerValue(cloudInit.CIPassword),
		CIType:      stringPointerValue(cloudInit.CIType),
		CIUpgrade:   boolPointerValue(cloudInit.CIUpgrade),
		CIUser:      stringPointerValue(cloudInit.CIUser),
		SSHKeys:     stringPointerValue(cloudInit.SSHKeys),
		IPConfig:    ipConfigValues,
		Network:     networkValues,
		Disk:        diskValues,
		ExtraConfig: extraConfig,
	}, diags
}

func qemuVMCommonStateValue(ctx context.Context, config QemuVMConfig, _ *qemuVMModel) (types.Object, diag.Diagnostics) {
	if config.Hotplug == "" {
		return types.ObjectNull(qemuVMCommonAttrTypes()), nil
	}
	return types.ObjectValueFrom(ctx, qemuVMCommonAttrTypes(), qemuVMCommonModel{Hotplug: types.StringValue(config.Hotplug)})
}

func qemuVMCloudInitStateValue(ctx context.Context, config QemuVMConfig, prior *qemuVMModel) (types.Object, diag.Diagnostics) {
	cloudInit := qemuVMCloudInitModel{
		CICustom:   stringOrNull(config.CICustom),
		CIPassword: stringOrNull(config.CIPassword),
		CIType:     stringOrNull(config.CIType),
		CIUpgrade:  boolOrNull(config.CIUpgrade.Ptr()),
		CIUser:     stringOrNull(config.CIUser),
		SSHKeys:    stringOrNull(config.SSHKeys),
	}

	if cloudInit.CIPassword.IsNull() && prior != nil && !prior.CloudInit.IsNull() && !prior.CloudInit.IsUnknown() {
		previous, diags := expandQemuVMCloudInitModel(ctx, prior.CloudInit)
		if diags.HasError() {
			return types.Object{}, diags
		}
		cloudInit.CIPassword = previous.CIPassword
	}

	ipConfigValue, diags := qemuVMIPConfigStateValue(ctx, config.IPConfig)
	if diags.HasError() {
		return types.Object{}, diags
	}
	cloudInit.IPConfig = ipConfigValue

	if qemuVMCloudInitModelIsEmpty(cloudInit) {
		return types.ObjectNull(qemuVMCloudInitAttrTypes()), nil
	}
	return types.ObjectValueFrom(ctx, qemuVMCloudInitAttrTypes(), cloudInit)
}

func qemuVMIPConfigStateValue(ctx context.Context, source map[string]string) (types.Map, diag.Diagnostics) {
	if len(source) == 0 {
		return types.MapNull(types.ObjectType{AttrTypes: qemuVMIPConfigAttrTypes()}), nil
	}

	items := make(map[string]qemuVMIPConfigModel)
	for key, raw := range source {
		parsed, ok := parseQemuVMIPConfig(raw)
		if ok {
			items[key] = parsed
		}
	}
	if len(items) == 0 {
		return types.MapNull(types.ObjectType{AttrTypes: qemuVMIPConfigAttrTypes()}), nil
	}
	return types.MapValueFrom(ctx, types.ObjectType{AttrTypes: qemuVMIPConfigAttrTypes()}, items)
}

func qemuVMNetworkStateValue(ctx context.Context, source map[string]string) (types.Map, map[string]string, diag.Diagnostics) {
	unsupported := map[string]string{}
	if len(source) == 0 {
		return types.MapNull(types.ObjectType{AttrTypes: qemuVMNetworkAttrTypes()}), nil, nil
	}

	items := make(map[string]qemuVMNetworkModel)
	for key, raw := range source {
		parsed, ok := parseQemuVMNetwork(raw)
		if ok {
			items[key] = parsed
		} else {
			unsupported[key] = raw
		}
	}
	if len(items) == 0 {
		return types.MapNull(types.ObjectType{AttrTypes: qemuVMNetworkAttrTypes()}), unsupported, nil
	}
	value, diags := types.MapValueFrom(ctx, types.ObjectType{AttrTypes: qemuVMNetworkAttrTypes()}, items)
	return value, unsupported, diags
}

func qemuVMDiskStateValue(ctx context.Context, source map[string]string) (types.Map, map[string]string, diag.Diagnostics) {
	unsupported := map[string]string{}
	if len(source) == 0 {
		return types.MapNull(types.ObjectType{AttrTypes: qemuVMDiskAttrTypes()}), nil, nil
	}

	items := make(map[string]qemuVMDiskModel)
	for key, raw := range source {
		parsed, ok := parseQemuVMDisk(raw)
		if ok {
			items[key] = parsed
		} else {
			unsupported[key] = raw
		}
	}
	if len(items) == 0 {
		return types.MapNull(types.ObjectType{AttrTypes: qemuVMDiskAttrTypes()}), unsupported, nil
	}
	value, diags := types.MapValueFrom(ctx, types.ObjectType{AttrTypes: qemuVMDiskAttrTypes()}, items)
	return value, unsupported, diags
}

func qemuVMRawStateValue(ctx context.Context, base map[string]string, networkRaw map[string]string, diskRaw map[string]string) (types.Object, diag.Diagnostics) {
	extra := map[string]string{}
	for key, value := range base {
		extra[key] = value
	}
	for key, value := range networkRaw {
		extra[key] = value
	}
	for key, value := range diskRaw {
		extra[key] = value
	}
	if len(extra) == 0 {
		return types.ObjectNull(qemuVMRawAttrTypes()), nil
	}
	mapValue, diags := types.MapValueFrom(ctx, types.StringType, extra)
	if diags.HasError() {
		return types.Object{}, diags
	}
	return types.ObjectValueFrom(ctx, qemuVMRawAttrTypes(), qemuVMRawModel{ExtraConfig: mapValue})
}

func qemuVMCloneStateValue(prior *qemuVMModel) types.Object {
	if prior == nil || prior.Clone.IsNull() || prior.Clone.IsUnknown() {
		return types.ObjectNull(qemuVMCloneAttrTypes())
	}
	return prior.Clone
}

func qemuVMEFIDiskStateValue(ctx context.Context, base map[string]string) (types.Object, map[string]string, diag.Diagnostics) {
	if len(base) == 0 {
		return types.ObjectNull(qemuVMEFIDiskAttrTypes()), nil, nil
	}

	extra := make(map[string]string, len(base))
	for key, value := range base {
		if key == "efidisk0" {
			continue
		}
		extra[key] = value
	}

	raw, ok := base["efidisk0"]
	if !ok {
		if len(extra) == 0 {
			extra = nil
		}
		return types.ObjectNull(qemuVMEFIDiskAttrTypes()), extra, nil
	}

	parsed, ok := parseQemuVMEFIDisk(raw)
	if !ok {
		extra["efidisk0"] = raw
		return types.ObjectNull(qemuVMEFIDiskAttrTypes()), extra, nil
	}

	value, diags := types.ObjectValueFrom(ctx, qemuVMEFIDiskAttrTypes(), parsed)
	if diags.HasError() {
		return types.Object{}, nil, diags
	}
	if len(extra) == 0 {
		extra = nil
	}
	return value, extra, diags
}

func qemuVMTPMStateValue(ctx context.Context, base map[string]string) (types.Object, map[string]string, diag.Diagnostics) {
	if len(base) == 0 {
		return types.ObjectNull(qemuVMTPMStateAttrTypes()), nil, nil
	}

	extra := make(map[string]string, len(base))
	for key, value := range base {
		if key == "tpmstate0" {
			continue
		}
		extra[key] = value
	}

	raw, ok := base["tpmstate0"]
	if !ok {
		if len(extra) == 0 {
			extra = nil
		}
		return types.ObjectNull(qemuVMTPMStateAttrTypes()), extra, nil
	}

	parsed, ok := parseQemuVMTPMState(raw)
	if !ok {
		extra["tpmstate0"] = raw
		return types.ObjectNull(qemuVMTPMStateAttrTypes()), extra, nil
	}

	value, diags := types.ObjectValueFrom(ctx, qemuVMTPMStateAttrTypes(), parsed)
	if diags.HasError() {
		return types.Object{}, nil, diags
	}
	if len(extra) == 0 {
		extra = nil
	}
	return value, extra, diags
}

func qemuVMVGAStateValue(ctx context.Context, base map[string]string) (types.Object, map[string]string, diag.Diagnostics) {
	if len(base) == 0 {
		return types.ObjectNull(qemuVMVGAAttrTypes()), nil, nil
	}

	extra := make(map[string]string, len(base))
	for key, value := range base {
		if key == "vga" {
			continue
		}
		extra[key] = value
	}

	raw, ok := base["vga"]
	if !ok {
		if len(extra) == 0 {
			extra = nil
		}
		return types.ObjectNull(qemuVMVGAAttrTypes()), extra, nil
	}

	parsed, ok := parseQemuVMVGA(raw)
	if !ok {
		extra["vga"] = raw
		return types.ObjectNull(qemuVMVGAAttrTypes()), extra, nil
	}

	value, diags := types.ObjectValueFrom(ctx, qemuVMVGAAttrTypes(), parsed)
	if diags.HasError() {
		return types.Object{}, nil, diags
	}
	if len(extra) == 0 {
		extra = nil
	}
	return value, extra, diags
}

func qemuVMTypedConfigKeys(ctx context.Context, model qemuVMModel, diags *diag.Diagnostics) []string {
	keys := []string{"protection", "scsihw", "tablet"}

	common, commonDiags := expandQemuVMCommonModel(ctx, model.Common)
	diags.Append(commonDiags...)
	if !common.Hotplug.IsNull() && !common.Hotplug.IsUnknown() {
		keys = append(keys, "hotplug")
	}

	cloudInit, cloudInitDiags := expandQemuVMCloudInitModel(ctx, model.CloudInit)
	diags.Append(cloudInitDiags...)
	for _, pair := range []struct {
		key   string
		value types.String
	}{
		{"cicustom", cloudInit.CICustom},
		{"cipassword", cloudInit.CIPassword},
		{"citype", cloudInit.CIType},
		{"ciuser", cloudInit.CIUser},
		{"sshkeys", cloudInit.SSHKeys},
	} {
		if !pair.value.IsNull() && !pair.value.IsUnknown() {
			keys = append(keys, pair.key)
		}
	}
	if !cloudInit.CIUpgrade.IsNull() && !cloudInit.CIUpgrade.IsUnknown() {
		keys = append(keys, "ciupgrade")
	}
	if !cloudInit.IPConfig.IsNull() && !cloudInit.IPConfig.IsUnknown() {
		var ipConfig map[string]qemuVMIPConfigModel
		diags.Append(cloudInit.IPConfig.ElementsAs(ctx, &ipConfig, false)...)
		for key := range ipConfig {
			keys = append(keys, key)
		}
	}

	if !model.Network.IsNull() && !model.Network.IsUnknown() {
		var network map[string]qemuVMNetworkModel
		diags.Append(model.Network.ElementsAs(ctx, &network, false)...)
		for key := range network {
			keys = append(keys, key)
		}
	}

	if !model.Disk.IsNull() && !model.Disk.IsUnknown() {
		var disk map[string]qemuVMDiskModel
		diags.Append(model.Disk.ElementsAs(ctx, &disk, false)...)
		for key := range disk {
			keys = append(keys, key)
		}
	}

	efiDisk, efiDiskDiags := expandQemuVMEFIDiskModel(ctx, model.EFIDisk)
	diags.Append(efiDiskDiags...)
	if !qemuVMEFIDiskModelIsEmpty(efiDisk) {
		keys = append(keys, "efidisk0")
	}

	tpmState, tpmStateDiags := expandQemuVMTPMStateModel(ctx, model.TPMState)
	diags.Append(tpmStateDiags...)
	if !qemuVMTPMStateModelIsEmpty(tpmState) {
		keys = append(keys, "tpmstate0")
	}

	vga, vgaDiags := expandQemuVMVGAModel(ctx, model.VGA)
	diags.Append(vgaDiags...)
	if !qemuVMVGAModelIsEmpty(vga) {
		keys = append(keys, "vga")
	}

	sort.Strings(keys)
	return keys
}

func expandQemuVMCommonModel(ctx context.Context, value types.Object) (qemuVMCommonModel, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return qemuVMCommonModel{}, nil
	}
	var result qemuVMCommonModel
	return result, value.As(ctx, &result, qemuObjectAsOptions())
}

func expandQemuVMCloudInitModel(ctx context.Context, value types.Object) (qemuVMCloudInitModel, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return qemuVMCloudInitModel{}, nil
	}
	var result qemuVMCloudInitModel
	return result, value.As(ctx, &result, qemuObjectAsOptions())
}

func expandQemuVMEFIDiskModel(ctx context.Context, value types.Object) (qemuVMEFIDiskModel, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return qemuVMEFIDiskModel{}, nil
	}
	var result qemuVMEFIDiskModel
	return result, value.As(ctx, &result, qemuObjectAsOptions())
}

func expandQemuVMTPMStateModel(ctx context.Context, value types.Object) (qemuVMTPMStateModel, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return qemuVMTPMStateModel{}, nil
	}
	var result qemuVMTPMStateModel
	return result, value.As(ctx, &result, qemuObjectAsOptions())
}

func expandQemuVMRawModel(ctx context.Context, value types.Object) (qemuVMRawModel, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return qemuVMRawModel{}, nil
	}
	var result qemuVMRawModel
	return result, value.As(ctx, &result, qemuObjectAsOptions())
}

func expandQemuVMCloneModel(ctx context.Context, value types.Object) (qemuVMCloneModel, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return qemuVMCloneModel{}, nil
	}
	var result qemuVMCloneModel
	return result, value.As(ctx, &result, qemuObjectAsOptions())
}

func expandQemuVMNetworkModelMap(ctx context.Context, value types.Map) (map[string]qemuVMNetworkModel, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}
	var result map[string]qemuVMNetworkModel
	return result, value.ElementsAs(ctx, &result, false)
}

func expandQemuVMDiskModelMap(ctx context.Context, value types.Map) (map[string]qemuVMDiskModel, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}
	var result map[string]qemuVMDiskModel
	return result, value.ElementsAs(ctx, &result, false)
}

func encodeQemuVMIPConfigMap(ctx context.Context, value types.Map) (map[string]string, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}
	var items map[string]qemuVMIPConfigModel
	if diags := value.ElementsAs(ctx, &items, false); diags.HasError() {
		return nil, diags
	}
	result := make(map[string]string, len(items))
	for key, item := range items {
		result[key] = encodeQemuVMIPConfig(item)
	}
	return result, nil
}

func encodeQemuVMNetworkMap(items map[string]qemuVMNetworkModel) map[string]string {
	if len(items) == 0 {
		return nil
	}
	result := make(map[string]string, len(items))
	for key, item := range items {
		result[key] = encodeQemuVMNetwork(item)
	}
	return result
}

func encodeQemuVMDiskMap(items map[string]qemuVMDiskModel) map[string]string {
	if len(items) == 0 {
		return nil
	}
	result := make(map[string]string, len(items))
	for key, item := range items {
		result[key] = encodeQemuVMDisk(item)
	}
	return result
}

func parseQemuVMEFIDisk(raw string) (qemuVMEFIDiskModel, bool) {
	if strings.TrimSpace(raw) == "" {
		return qemuVMEFIDiskModel{}, false
	}
	parts := splitQemuConfigSegments(raw)
	item := qemuVMEFIDiskModel{}
	for index, segment := range parts {
		key, value, ok := splitQemuConfigKeyValue(segment)
		if !ok {
			if index != 0 {
				return qemuVMEFIDiskModel{}, false
			}
			item.Volume = types.StringValue(strings.TrimSpace(segment))
			if storage := qemuVMDiskStorageFromVolume(strings.TrimSpace(segment)); storage != "" {
				item.Storage = types.StringValue(storage)
			}
			continue
		}
		switch key {
		case "file", "volume":
			item.Volume = types.StringValue(value)
			if storage := qemuVMDiskStorageFromVolume(value); storage != "" {
				item.Storage = types.StringValue(storage)
			}
		case "size":
			item.Size = types.StringValue(value)
		case "efitype":
			item.EFIType = types.StringValue(value)
		case "format":
			item.Format = types.StringValue(value)
		case "ms-cert":
			item.MSCert = types.StringValue(value)
		case "pre-enrolled-keys":
			parsed, ok := parseQemuVMConfigBool(value)
			if !ok {
				return qemuVMEFIDiskModel{}, false
			}
			item.PreEnrolledKeys = types.BoolValue(parsed)
		default:
			return qemuVMEFIDiskModel{}, false
		}
	}
	return item, !item.Volume.IsNull() && !item.Volume.IsUnknown()
}

func parseQemuVMTPMState(raw string) (qemuVMTPMStateModel, bool) {
	if strings.TrimSpace(raw) == "" {
		return qemuVMTPMStateModel{}, false
	}
	parts := splitQemuConfigSegments(raw)
	item := qemuVMTPMStateModel{}
	for index, segment := range parts {
		key, value, ok := splitQemuConfigKeyValue(segment)
		if !ok {
			if index != 0 {
				return qemuVMTPMStateModel{}, false
			}
			trimmed := strings.TrimSpace(segment)
			if trimmed == "" {
				return qemuVMTPMStateModel{}, false
			}
			item.Volume = types.StringValue(trimmed)
			if storage := qemuVMDiskStorageFromVolume(trimmed); storage != "" {
				item.Storage = types.StringValue(storage)
			}
			continue
		}
		if strings.TrimSpace(value) == "" {
			return qemuVMTPMStateModel{}, false
		}
		switch key {
		case "file", "volume":
			item.Volume = types.StringValue(value)
			if storage := qemuVMDiskStorageFromVolume(value); storage != "" {
				item.Storage = types.StringValue(storage)
			}
		case "size":
			item.Size = types.StringValue(value)
		case "format":
			item.Format = types.StringValue(value)
		case "version":
			item.Version = types.StringValue(value)
		default:
			return qemuVMTPMStateModel{}, false
		}
	}
	return item, !item.Volume.IsNull() && !item.Volume.IsUnknown()
}

func parseQemuVMIPConfig(raw string) (qemuVMIPConfigModel, bool) {
	if strings.TrimSpace(raw) == "" {
		return qemuVMIPConfigModel{}, false
	}
	parts := splitQemuConfigSegments(raw)
	item := qemuVMIPConfigModel{}
	for _, segment := range parts {
		key, value, ok := splitQemuConfigKeyValue(segment)
		if !ok {
			return qemuVMIPConfigModel{}, false
		}
		switch key {
		case "ip":
			item.IPv4 = types.StringValue(value)
		case "gw":
			item.Gateway = types.StringValue(value)
		case "ip6":
			item.IPv6 = types.StringValue(value)
		case "gw6":
			item.Gateway6 = types.StringValue(value)
		default:
			return qemuVMIPConfigModel{}, false
		}
	}
	return item, true
}

func parseQemuVMNetwork(raw string) (qemuVMNetworkModel, bool) {
	if strings.TrimSpace(raw) == "" {
		return qemuVMNetworkModel{}, false
	}
	parts := splitQemuConfigSegments(raw)
	item := qemuVMNetworkModel{}
	for _, segment := range parts {
		key, value, ok := splitQemuConfigKeyValue(segment)
		if !ok {
			key = strings.TrimSpace(segment)
			value = ""
		}
		if _, isModel := qemuVMNetworkModels[key]; isModel {
			item.Model = types.StringValue(key)
			if value != "" {
				item.MACAddr = types.StringValue(value)
			}
			continue
		}
		switch key {
		case "model":
			item.Model = types.StringValue(value)
		case "bridge":
			item.Bridge = types.StringValue(value)
		case "macaddr":
			item.MACAddr = types.StringValue(value)
		case "tag":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return qemuVMNetworkModel{}, false
			}
			item.Tag = types.Int64Value(parsed)
		case "trunks":
			item.Trunks = types.StringValue(value)
		case "firewall":
			parsed, ok := parseQemuVMConfigBool(value)
			if !ok {
				return qemuVMNetworkModel{}, false
			}
			item.Firewall = types.BoolValue(parsed)
		case "link_down":
			parsed, ok := parseQemuVMConfigBool(value)
			if !ok {
				return qemuVMNetworkModel{}, false
			}
			item.LinkDown = types.BoolValue(parsed)
		case "mtu":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return qemuVMNetworkModel{}, false
			}
			item.MTU = types.Int64Value(parsed)
		case "queues":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return qemuVMNetworkModel{}, false
			}
			item.Queues = types.Int64Value(parsed)
		case "rate":
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return qemuVMNetworkModel{}, false
			}
			item.Rate = types.Float64Value(parsed)
		default:
			return qemuVMNetworkModel{}, false
		}
	}
	return item, !item.Model.IsNull() && !item.Model.IsUnknown()
}

func parseQemuVMDisk(raw string) (qemuVMDiskModel, bool) {
	if strings.TrimSpace(raw) == "" {
		return qemuVMDiskModel{}, false
	}
	parts := splitQemuConfigSegments(raw)
	item := qemuVMDiskModel{}
	for index, segment := range parts {
		key, value, ok := splitQemuConfigKeyValue(segment)
		if !ok {
			if index != 0 {
				return qemuVMDiskModel{}, false
			}
			item.Volume = types.StringValue(strings.TrimSpace(segment))
			if storage := qemuVMDiskStorageFromVolume(strings.TrimSpace(segment)); storage != "" {
				item.Storage = types.StringValue(storage)
			}
			continue
		}
		switch key {
		case "file", "volume":
			item.Volume = types.StringValue(value)
			if storage := qemuVMDiskStorageFromVolume(value); storage != "" {
				item.Storage = types.StringValue(storage)
			}
		case "size":
			item.Size = types.StringValue(value)
		case "media":
			item.Media = types.StringValue(value)
		case "cache":
			item.Cache = types.StringValue(value)
		case "discard":
			item.Discard = types.StringValue(value)
		case "iothread":
			parsed, ok := parseQemuVMConfigBool(value)
			if !ok {
				return qemuVMDiskModel{}, false
			}
			item.Iothread = types.BoolValue(parsed)
		case "replicate":
			parsed, ok := parseQemuVMConfigBool(value)
			if !ok {
				return qemuVMDiskModel{}, false
			}
			item.Replicate = types.BoolValue(parsed)
		case "backup":
			parsed, ok := parseQemuVMConfigBool(value)
			if !ok {
				return qemuVMDiskModel{}, false
			}
			item.Backup = types.BoolValue(parsed)
		case "shared":
			parsed, ok := parseQemuVMConfigBool(value)
			if !ok {
				return qemuVMDiskModel{}, false
			}
			item.Shared = types.BoolValue(parsed)
		case "snapshot":
			parsed, ok := parseQemuVMConfigBool(value)
			if !ok {
				return qemuVMDiskModel{}, false
			}
			item.Snapshot = types.BoolValue(parsed)
		case "serial":
			item.Serial = types.StringValue(value)
		case "iops":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return qemuVMDiskModel{}, false
			}
			item.IOPS = types.Int64Value(parsed)
		case "iops_max":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return qemuVMDiskModel{}, false
			}
			item.IOPSMax = types.Int64Value(parsed)
		case "iops_rd":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return qemuVMDiskModel{}, false
			}
			item.IOPSRd = types.Int64Value(parsed)
		case "iops_rd_max":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return qemuVMDiskModel{}, false
			}
			item.IOPSRdMax = types.Int64Value(parsed)
		case "iops_wr":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return qemuVMDiskModel{}, false
			}
			item.IOPSWr = types.Int64Value(parsed)
		case "iops_wr_max":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return qemuVMDiskModel{}, false
			}
			item.IOPSWrMax = types.Int64Value(parsed)
		case "mbps":
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return qemuVMDiskModel{}, false
			}
			item.MBPS = types.Float64Value(parsed)
		case "mbps_max":
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return qemuVMDiskModel{}, false
			}
			item.MBPSMax = types.Float64Value(parsed)
		case "mbps_rd":
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return qemuVMDiskModel{}, false
			}
			item.MBPSRd = types.Float64Value(parsed)
		case "mbps_rd_max":
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return qemuVMDiskModel{}, false
			}
			item.MBPSRdMax = types.Float64Value(parsed)
		case "mbps_wr":
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return qemuVMDiskModel{}, false
			}
			item.MBPSWr = types.Float64Value(parsed)
		case "mbps_wr_max":
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return qemuVMDiskModel{}, false
			}
			item.MBPSWrMax = types.Float64Value(parsed)
		case "ssd":
			parsed, ok := parseQemuVMConfigBool(value)
			if !ok {
				return qemuVMDiskModel{}, false
			}
			item.SSD = types.BoolValue(parsed)
		default:
			return qemuVMDiskModel{}, false
		}
	}
	return item, !item.Volume.IsNull() && !item.Volume.IsUnknown()
}

func encodeQemuVMIPConfig(item qemuVMIPConfigModel) string {
	segments := make([]string, 0, 4)
	appendStringConfig(&segments, "ip", item.IPv4)
	appendStringConfig(&segments, "gw", item.Gateway)
	appendStringConfig(&segments, "ip6", item.IPv6)
	appendStringConfig(&segments, "gw6", item.Gateway6)
	return strings.Join(segments, ",")
}

func encodeQemuVMNetwork(item qemuVMNetworkModel) string {
	segments := make([]string, 0, 9)
	model := stringValue(item.Model)
	mac := stringValue(item.MACAddr)
	if model != "" {
		if mac != "" {
			segments = append(segments, fmt.Sprintf("%s=%s", model, mac))
		} else {
			segments = append(segments, model)
		}
	}
	appendStringConfig(&segments, "bridge", item.Bridge)
	appendInt64Config(&segments, "tag", item.Tag)
	appendStringConfig(&segments, "trunks", item.Trunks)
	appendBoolConfig(&segments, "firewall", item.Firewall)
	appendBoolConfig(&segments, "link_down", item.LinkDown)
	appendInt64Config(&segments, "mtu", item.MTU)
	appendInt64Config(&segments, "queues", item.Queues)
	appendFloat64Config(&segments, "rate", item.Rate)
	return strings.Join(segments, ",")
}

func encodeQemuVMDisk(item qemuVMDiskModel) string {
	segments := make([]string, 0, 8)
	filePart := stringValue(item.Volume)
	if filePart == "" {
		storage := stringValue(item.Storage)
		size := stringValue(item.Size)
		switch {
		case storage != "" && size != "":
			filePart = fmt.Sprintf("%s:%s", storage, size)
		case storage != "":
			filePart = storage
		}
	}
	if filePart != "" {
		segments = append(segments, filePart)
	}
	appendStringConfig(&segments, "media", item.Media)
	appendStringConfig(&segments, "cache", item.Cache)
	appendStringConfig(&segments, "discard", item.Discard)
	appendBoolConfig(&segments, "iothread", item.Iothread)
	appendBoolConfig(&segments, "replicate", item.Replicate)
	appendBoolConfig(&segments, "ssd", item.SSD)
	appendBoolConfig(&segments, "backup", item.Backup)
	appendBoolConfig(&segments, "shared", item.Shared)
	appendBoolConfig(&segments, "snapshot", item.Snapshot)
	appendStringConfig(&segments, "serial", item.Serial)
	appendInt64Config(&segments, "iops", item.IOPS)
	appendInt64Config(&segments, "iops_max", item.IOPSMax)
	appendInt64Config(&segments, "iops_rd", item.IOPSRd)
	appendInt64Config(&segments, "iops_rd_max", item.IOPSRdMax)
	appendInt64Config(&segments, "iops_wr", item.IOPSWr)
	appendInt64Config(&segments, "iops_wr_max", item.IOPSWrMax)
	appendFloat64Config(&segments, "mbps", item.MBPS)
	appendFloat64Config(&segments, "mbps_max", item.MBPSMax)
	appendFloat64Config(&segments, "mbps_rd", item.MBPSRd)
	appendFloat64Config(&segments, "mbps_rd_max", item.MBPSRdMax)
	appendFloat64Config(&segments, "mbps_wr", item.MBPSWr)
	appendFloat64Config(&segments, "mbps_wr_max", item.MBPSWrMax)
	if stringValue(item.Volume) != "" {
		appendStringConfig(&segments, "size", item.Size)
	}
	return strings.Join(segments, ",")
}

func encodeQemuVMEFIDisk(item qemuVMEFIDiskModel) string {
	segments := make([]string, 0, 6)
	filePart := stringValue(item.Volume)
	if filePart == "" {
		storage := stringValue(item.Storage)
		size := stringValue(item.Size)
		switch {
		case storage != "" && size != "":
			filePart = fmt.Sprintf("%s:%s", storage, size)
		case storage != "":
			filePart = storage
		}
	}
	if filePart != "" {
		segments = append(segments, filePart)
	}
	appendStringConfig(&segments, "efitype", item.EFIType)
	appendStringConfig(&segments, "format", item.Format)
	appendStringConfig(&segments, "ms-cert", item.MSCert)
	appendBoolConfig(&segments, "pre-enrolled-keys", item.PreEnrolledKeys)
	if stringValue(item.Volume) != "" {
		appendStringConfig(&segments, "size", item.Size)
	}
	return strings.Join(segments, ",")
}

func encodeQemuVMTPMState(item qemuVMTPMStateModel) string {
	segments := make([]string, 0, 5)
	filePart := stringValue(item.Volume)
	if filePart == "" {
		storage := stringValue(item.Storage)
		size := stringValue(item.Size)
		switch {
		case storage != "" && size != "":
			filePart = fmt.Sprintf("%s:%s", storage, size)
		case storage != "":
			filePart = storage
		}
	}
	if filePart != "" {
		segments = append(segments, filePart)
	}
	appendStringConfig(&segments, "format", item.Format)
	if stringValue(item.Volume) != "" {
		appendStringConfig(&segments, "size", item.Size)
	}
	appendStringConfig(&segments, "version", item.Version)
	return strings.Join(segments, ",")
}

func parseQemuVMVGA(raw string) (qemuVMVGAModel, bool) {
	if strings.TrimSpace(raw) == "" {
		return qemuVMVGAModel{}, false
	}
	parts := splitQemuConfigSegments(raw)
	item := qemuVMVGAModel{}
	for index, segment := range parts {
		key, value, ok := splitQemuConfigKeyValue(segment)
		if !ok {
			if index != 0 {
				return qemuVMVGAModel{}, false
			}
			trimmed := strings.TrimSpace(segment)
			if trimmed == "" {
				return qemuVMVGAModel{}, false
			}
			item.Type = types.StringValue(trimmed)
			continue
		}
		if strings.TrimSpace(value) == "" {
			return qemuVMVGAModel{}, false
		}
		switch key {
		case "memory":
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return qemuVMVGAModel{}, false
			}
			item.Memory = types.Int64Value(n)
		case "clipboard":
			item.Clipboard = types.StringValue(value)
		default:
			return qemuVMVGAModel{}, false
		}
	}
	return item, !item.Type.IsNull() && !item.Type.IsUnknown()
}

func encodeQemuVMVGA(item qemuVMVGAModel) string {
	if item.Type.IsNull() || item.Type.IsUnknown() {
		return ""
	}
	segments := []string{item.Type.ValueString()}
	appendInt64Config(&segments, "memory", item.Memory)
	appendStringConfig(&segments, "clipboard", item.Clipboard)
	return strings.Join(segments, ",")
}

func qemuVMVGAModelIsEmpty(model qemuVMVGAModel) bool {
	return (model.Type.IsNull() || model.Type.IsUnknown()) && model.Memory.IsNull() && model.Clipboard.IsNull()
}

func expandQemuVMVGAModel(ctx context.Context, value types.Object) (qemuVMVGAModel, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return qemuVMVGAModel{}, nil
	}
	var result qemuVMVGAModel
	diags := value.As(ctx, &result, qemuObjectAsOptions())
	return result, diags
}

func splitQemuConfigSegments(raw string) []string {
	segments := strings.Split(raw, ",")
	for i := range segments {
		segments[i] = strings.TrimSpace(segments[i])
	}
	return segments
}

func splitQemuConfigKeyValue(segment string) (string, string, bool) {
	parts := strings.SplitN(segment, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func parseQemuVMConfigBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "on", "yes", "true":
		return true, true
	case "0", "off", "no", "false":
		return false, true
	default:
		return false, false
	}
}

func qemuVMDiskStorageFromVolume(volume string) string {
	prefix, _, ok := strings.Cut(volume, ":")
	if !ok {
		return ""
	}
	return prefix
}

func appendStringConfig(segments *[]string, key string, value types.String) {
	if value.IsNull() || value.IsUnknown() {
		return
	}
	*segments = append(*segments, fmt.Sprintf("%s=%s", key, value.ValueString()))
}

func appendInt64Config(segments *[]string, key string, value types.Int64) {
	if value.IsNull() || value.IsUnknown() {
		return
	}
	*segments = append(*segments, fmt.Sprintf("%s=%d", key, value.ValueInt64()))
}

func appendBoolConfig(segments *[]string, key string, value types.Bool) {
	if value.IsNull() || value.IsUnknown() {
		return
	}
	if value.ValueBool() {
		*segments = append(*segments, fmt.Sprintf("%s=1", key))
	} else {
		*segments = append(*segments, fmt.Sprintf("%s=0", key))
	}
}

func appendFloat64Config(segments *[]string, key string, value types.Float64) {
	if value.IsNull() || value.IsUnknown() {
		return
	}
	*segments = append(*segments, fmt.Sprintf("%s=%s", key, strconv.FormatFloat(value.ValueFloat64(), 'f', -1, 64)))
}

func qemuVMCloudInitModelIsEmpty(model qemuVMCloudInitModel) bool {
	return model.CICustom.IsNull() && model.CIPassword.IsNull() && model.CIType.IsNull() && model.CIUpgrade.IsNull() && model.CIUser.IsNull() && (model.IPConfig.IsNull() || model.IPConfig.IsUnknown()) && model.SSHKeys.IsNull()
}

func qemuVMEFIDiskModelIsEmpty(model qemuVMEFIDiskModel) bool {
	return model.Storage.IsNull() && model.Volume.IsNull() && model.Size.IsNull() && model.EFIType.IsNull() && model.Format.IsNull() && model.MSCert.IsNull() && model.PreEnrolledKeys.IsNull()
}

func qemuVMTPMStateModelIsEmpty(model qemuVMTPMStateModel) bool {
	return model.Storage.IsNull() && model.Volume.IsNull() && model.Size.IsNull() && model.Format.IsNull() && model.Version.IsNull()
}

func isQemuVMIPConfigKey(key string) bool {
	return strings.HasPrefix(key, "ipconfig") && len(key) > len("ipconfig") && isDecimalString(key[len("ipconfig"):])
}

func isQemuVMNetworkKey(key string) bool {
	return strings.HasPrefix(key, "net") && len(key) > len("net") && isDecimalString(key[len("net"):])
}

func isQemuVMDiskKey(key string) bool {
	for _, prefix := range []string{"ide", "sata", "scsi", "virtio"} {
		if strings.HasPrefix(key, prefix) && len(key) > len(prefix) && isDecimalString(key[len(prefix):]) {
			return true
		}
	}
	return false
}

func isDecimalString(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func stringPointerValue(value types.String) *string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	result := value.ValueString()
	return &result
}

func boolPointerValue(value types.Bool) *bool {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	result := value.ValueBool()
	return &result
}

func int64PointerValue(value types.Int64) *int64 {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	result := value.ValueInt64()
	return &result
}

func qemuInt64Value(value types.Int64) int64 {
	if value.IsNull() || value.IsUnknown() {
		return 0
	}
	return value.ValueInt64()
}
