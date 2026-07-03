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

func lxcContainerStateFromAPI(ctx context.Context, node string, vmID int64, config LXCContainerConfig, status LXCContainerStatus, prior *lxcContainerModel) (lxcContainerModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	networkValue, networkDiags := stringMapStateValue(ctx, config.Network)
	diags.Append(networkDiags...)
	mountPointValue, mountPointDiags := stringMapStateValue(ctx, config.MountPoint)
	diags.Append(mountPointDiags...)
	rawValue, rawDiags := lxcContainerRawStateValue(ctx, config.ExtraConfig)
	diags.Append(rawDiags...)

	ostemplate := types.StringNull()
	rootfs := stringOrNull(config.RootFS)
	if prior != nil {
		if !prior.OSTemplate.IsNull() && !prior.OSTemplate.IsUnknown() {
			ostemplate = prior.OSTemplate
		}
		if !prior.RootFS.IsNull() && !prior.RootFS.IsUnknown() {
			rootfs = prior.RootFS
		}
	}

	onboot := false
	if value := config.OnBoot.Ptr(); value != nil {
		onboot = *value
	}
	protection := false
	if value := config.Protection.Ptr(); value != nil {
		protection = *value
	}
	unprivileged := false
	if value := config.Unprivileged.Ptr(); value != nil {
		unprivileged = *value
	}

	return lxcContainerModel{
		ID:           types.StringValue(lxcContainerID(node, vmID)),
		Node:         types.StringValue(node),
		VMID:         types.Int64Value(vmID),
		OSTemplate:   ostemplate,
		Hostname:     stringOrNull(config.Hostname),
		Description:  stringOrNull(config.Description),
		Tags:         stringOrNull(config.Tags),
		Arch:         stringOrNull(config.Arch),
		Cores:        int64OrNull(config.Cores.Ptr()),
		CPULimit:     float64OrNull(config.CPULimit.Ptr()),
		CPUUnits:     int64OrNull(config.CPUUnits.Ptr()),
		Memory:       int64OrNull(config.Memory.Ptr()),
		Swap:         int64OrNull(config.Swap.Ptr()),
		OnBoot:       types.BoolValue(onboot),
		Protection:   types.BoolValue(protection),
		Startup:      stringOrNull(config.Startup),
		Unprivileged: types.BoolValue(unprivileged),
		Features:     stringOrNull(config.Features),
		Console:      boolOrNull(config.Console.Ptr()),
		TTY:          int64OrNull(config.TTY.Ptr()),
		CMode:        stringOrNull(config.CMode),
		Hookscript:   stringOrNull(config.Hookscript),
		OSType:       stringOrNull(config.OSType),
		RootFS:       rootfs,
		Nameserver:   stringOrNull(config.Nameserver),
		Searchdomain: stringOrNull(config.Searchdomain),
		Timezone:     stringOrNull(config.Timezone),
		Network:      networkValue,
		MountPoint:   mountPointValue,
		Raw:          rawValue,
		Clone:        lxcContainerCloneStateValue(prior),
		Status:       stringOrNull(status.Status),
		Uptime:       int64OrNull(status.Uptime.Ptr()),
	}, diags
}

func lxcContainerID(node string, vmID int64) string {
	return fmt.Sprintf("%s/%d", node, vmID)
}

func parseLXCContainerImportID(id string) (string, int64, error) {
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

func lxcContainerCreateRequestFromModel(ctx context.Context, model lxcContainerModel) (CreateLXCContainerRequest, diag.Diagnostics) {
	configReq, diags := lxcContainerConfigRequestFromModel(ctx, model)
	if diags.HasError() {
		return CreateLXCContainerRequest{}, diags
	}

	return CreateLXCContainerRequest{
		VMID:                      model.VMID.ValueInt64(),
		OSTemplate:                stringPointerValue(model.OSTemplate),
		lxcContainerConfigRequest: configReq,
	}, diags
}

func lxcContainerCloneRequestFromModel(ctx context.Context, model lxcContainerModel) (CloneLXCContainerRequest, diag.Diagnostics) {
	clone, diags := expandLXCContainerCloneModel(ctx, model.Clone)
	if diags.HasError() {
		return CloneLXCContainerRequest{}, diags
	}

	return CloneLXCContainerRequest{
		SourceNode:   firstNonEmpty(stringValue(clone.SourceNode), stringValue(model.Node)),
		SourceVMID:   clone.SourceVMID.ValueInt64(),
		TargetNode:   stringValue(model.Node),
		NewID:        model.VMID.ValueInt64(),
		Hostname:     stringPointerValue(model.Hostname),
		Description:  stringPointerValue(model.Description),
		Pool:         nil,
		Full:         boolPointerValue(clone.Full),
		SnapshotName: stringPointerValue(clone.SnapshotName),
		Storage:      stringPointerValue(clone.Storage),
		BWLimit:      int64PointerValue(clone.BWLimit),
	}, diags
}

func lxcContainerCloneStateValue(prior *lxcContainerModel) types.Object {
	if prior == nil || prior.Clone.IsNull() || prior.Clone.IsUnknown() {
		return types.ObjectNull(lxcContainerCloneAttrTypes())
	}
	return prior.Clone
}

func expandLXCContainerCloneModel(ctx context.Context, value types.Object) (lxcContainerCloneModel, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return lxcContainerCloneModel{}, nil
	}
	var result lxcContainerCloneModel
	return result, value.As(ctx, &result, basetypes.ObjectAsOptions{})
}

func lxcContainerUpdateRequestFromModel(ctx context.Context, plan lxcContainerModel, prior lxcContainerModel) (UpdateLXCContainerRequest, diag.Diagnostics) {
	configReq, diags := lxcContainerConfigRequestFromModel(ctx, plan)
	if diags.HasError() {
		return UpdateLXCContainerRequest{}, diags
	}

	configReq.Arch = nil
	configReq.Unprivileged = nil
	configReq.RootFS = nil
	configReq.Delete = lxcContainerDeleteKeys(ctx, plan, prior, &diags)
	if diags.HasError() {
		return UpdateLXCContainerRequest{}, diags
	}

	return UpdateLXCContainerRequest{lxcContainerConfigRequest: configReq}, diags
}

func validateLXCContainerRawConflicts(ctx context.Context, model lxcContainerModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rawModel, rawDiags := expandLXCContainerRawModel(ctx, model.Raw)
	diags.Append(rawDiags...)
	if diags.HasError() || rawModel.ExtraConfig.IsNull() || rawModel.ExtraConfig.IsUnknown() {
		return diags
	}

	var rawKeys map[string]string
	diags.Append(rawModel.ExtraConfig.ElementsAs(ctx, &rawKeys, false)...)
	if diags.HasError() {
		return diags
	}

	for key := range rawKeys {
		if isReservedLXCContainerRawKey(key) {
			diags.AddAttributeError(
				path.Root("raw").AtName("extra_config").AtMapKey(key),
				"Conflicting raw.extra_config entry",
				fmt.Sprintf("The raw.extra_config entry %q overlaps with a typed LXC configuration field. Remove one source of truth for that Proxmox config key.", key),
			)
		}
	}

	return diags
}

func lxcContainerConfigRequestFromModel(ctx context.Context, model lxcContainerModel) (lxcContainerConfigRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	network, networkDiags := expandStringMap(ctx, model.Network)
	diags.Append(networkDiags...)
	mountPoint, mountPointDiags := expandStringMap(ctx, model.MountPoint)
	diags.Append(mountPointDiags...)
	diags.Append(validateLXCContainerMapKeys(ctx, model)...)
	raw, rawDiags := expandLXCContainerRawModel(ctx, model.Raw)
	diags.Append(rawDiags...)
	if diags.HasError() {
		return lxcContainerConfigRequest{}, diags
	}

	var extraConfig map[string]string
	if !raw.ExtraConfig.IsNull() && !raw.ExtraConfig.IsUnknown() {
		diags.Append(raw.ExtraConfig.ElementsAs(ctx, &extraConfig, false)...)
	}
	if diags.HasError() {
		return lxcContainerConfigRequest{}, diags
	}

	return lxcContainerConfigRequest{
		Hostname:     stringPointerValue(model.Hostname),
		Description:  stringPointerValue(model.Description),
		Tags:         stringPointerValue(model.Tags),
		Arch:         stringPointerValue(model.Arch),
		Startup:      stringPointerValue(model.Startup),
		Features:     stringPointerValue(model.Features),
		Console:      boolPointerValue(model.Console),
		TTY:          int64PointerValue(model.TTY),
		CMode:        stringPointerValue(model.CMode),
		Hookscript:   stringPointerValue(model.Hookscript),
		OSType:       stringPointerValue(model.OSType),
		RootFS:       stringPointerValue(model.RootFS),
		Nameserver:   stringPointerValue(model.Nameserver),
		Searchdomain: stringPointerValue(model.Searchdomain),
		Timezone:     stringPointerValue(model.Timezone),
		OnBoot:       boolPointerValue(model.OnBoot),
		Protection:   boolPointerValue(model.Protection),
		Unprivileged: boolPointerValue(model.Unprivileged),
		Cores:        int64PointerValue(model.Cores),
		CPULimit:     float64PointerValue(model.CPULimit),
		CPUUnits:     int64PointerValue(model.CPUUnits),
		Memory:       int64PointerValue(model.Memory),
		Swap:         int64PointerValue(model.Swap),
		Network:      network,
		MountPoint:   mountPoint,
		ExtraConfig:  extraConfig,
	}, diags
}

func validateLXCContainerMapKeys(ctx context.Context, model lxcContainerModel) diag.Diagnostics {
	var diags diag.Diagnostics
	validateLXCContainerMapKeySet(ctx, &diags, model.Network, path.Root("network"), isLXCContainerNetworkKey, "Invalid LXC network key", "LXC network map keys must use Proxmox network slots such as net0 or net1.")
	validateLXCContainerMapKeySet(ctx, &diags, model.MountPoint, path.Root("mount_point"), isLXCContainerMountPointKey, "Invalid LXC mount_point key", "LXC mount_point map keys must use Proxmox mount-point slots such as mp0 or mp1.")
	return diags
}

func validateLXCContainerMapKeySet(ctx context.Context, diags *diag.Diagnostics, value types.Map, base path.Path, valid func(string) bool, summary string, detail string) {
	items, mapDiags := expandStringMap(ctx, value)
	diags.Append(mapDiags...)
	if mapDiags.HasError() {
		return
	}
	for key := range items {
		if !valid(key) {
			diags.AddAttributeError(base.AtMapKey(key), summary, detail)
		}
	}
}

func lxcContainerRawStateValue(ctx context.Context, extraConfig map[string]string) (types.Object, diag.Diagnostics) {
	if len(extraConfig) == 0 {
		return types.ObjectNull(lxcContainerRawAttrTypes()), nil
	}

	mapValue, diags := types.MapValueFrom(ctx, types.StringType, extraConfig)
	if diags.HasError() {
		return types.Object{}, diags
	}
	return types.ObjectValueFrom(ctx, lxcContainerRawAttrTypes(), lxcContainerRawModel{ExtraConfig: mapValue})
}

func stringMapStateValue(ctx context.Context, source map[string]string) (types.Map, diag.Diagnostics) {
	if len(source) == 0 {
		return types.MapNull(types.StringType), nil
	}
	return types.MapValueFrom(ctx, types.StringType, source)
}

func expandLXCContainerRawModel(ctx context.Context, value types.Object) (lxcContainerRawModel, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return lxcContainerRawModel{ExtraConfig: types.MapNull(types.StringType)}, nil
	}
	var result lxcContainerRawModel
	diags := value.As(ctx, &result, qemuObjectAsOptions())
	return result, diags
}

func expandStringMap(ctx context.Context, value types.Map) (map[string]string, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}
	result := map[string]string{}
	diags := value.ElementsAs(ctx, &result, false)
	return result, diags
}

func lxcContainerDeleteKeys(ctx context.Context, plan lxcContainerModel, prior lxcContainerModel, diags *diag.Diagnostics) []string {
	deleteKeys := []string{}

	deleteKeys = appendDeletedString(deleteKeys, "hostname", plan.Hostname, prior.Hostname)
	deleteKeys = appendDeletedString(deleteKeys, "description", plan.Description, prior.Description)
	deleteKeys = appendDeletedString(deleteKeys, "tags", plan.Tags, prior.Tags)
	deleteKeys = appendDeletedInt64(deleteKeys, "cores", plan.Cores, prior.Cores)
	deleteKeys = appendDeletedInt64(deleteKeys, "memory", plan.Memory, prior.Memory)
	deleteKeys = appendDeletedInt64(deleteKeys, "swap", plan.Swap, prior.Swap)
	deleteKeys = appendDeletedBool(deleteKeys, "onboot", plan.OnBoot, prior.OnBoot)
	deleteKeys = appendDeletedBool(deleteKeys, "protection", plan.Protection, prior.Protection)
	deleteKeys = appendDeletedString(deleteKeys, "startup", plan.Startup, prior.Startup)
	deleteKeys = appendDeletedString(deleteKeys, "features", plan.Features, prior.Features)
	deleteKeys = appendDeletedString(deleteKeys, "ostype", plan.OSType, prior.OSType)
	deleteKeys = appendDeletedString(deleteKeys, "nameserver", plan.Nameserver, prior.Nameserver)
	deleteKeys = appendDeletedString(deleteKeys, "searchdomain", plan.Searchdomain, prior.Searchdomain)
	deleteKeys = appendDeletedString(deleteKeys, "timezone", plan.Timezone, prior.Timezone)

	deleteKeys = appendDeletedMapKeys(ctx, deleteKeys, plan.Network, prior.Network, diags)
	deleteKeys = appendDeletedMapKeys(ctx, deleteKeys, plan.MountPoint, prior.MountPoint, diags)
	deleteKeys = appendDeletedRawKeys(ctx, deleteKeys, plan.Raw, prior.Raw, diags)

	sort.Strings(deleteKeys)
	return deleteKeys
}

func appendDeletedString(keys []string, key string, plan types.String, prior types.String) []string {
	if !prior.IsNull() && !prior.IsUnknown() && (plan.IsNull() || plan.IsUnknown()) {
		return append(keys, key)
	}
	return keys
}

func appendDeletedInt64(keys []string, key string, plan types.Int64, prior types.Int64) []string {
	if !prior.IsNull() && !prior.IsUnknown() && (plan.IsNull() || plan.IsUnknown()) {
		return append(keys, key)
	}
	return keys
}

func appendDeletedBool(keys []string, key string, plan types.Bool, prior types.Bool) []string {
	if !prior.IsNull() && !prior.IsUnknown() && (plan.IsNull() || plan.IsUnknown()) {
		return append(keys, key)
	}
	return keys
}

func appendDeletedMapKeys(ctx context.Context, keys []string, plan types.Map, prior types.Map, diags *diag.Diagnostics) []string {
	priorMap, priorDiags := expandStringMap(ctx, prior)
	diags.Append(priorDiags...)
	if len(priorMap) == 0 {
		return keys
	}
	planMap, planDiags := expandStringMap(ctx, plan)
	diags.Append(planDiags...)
	if diags.HasError() {
		return keys
	}

	for key := range priorMap {
		if _, ok := planMap[key]; !ok {
			keys = append(keys, key)
		}
	}
	return keys
}

func appendDeletedRawKeys(ctx context.Context, keys []string, plan types.Object, prior types.Object, diags *diag.Diagnostics) []string {
	priorRaw, priorDiags := expandLXCContainerRawModel(ctx, prior)
	diags.Append(priorDiags...)
	if priorRaw.ExtraConfig.IsNull() || priorRaw.ExtraConfig.IsUnknown() {
		return keys
	}

	var priorMap map[string]string
	diags.Append(priorRaw.ExtraConfig.ElementsAs(ctx, &priorMap, false)...)
	if len(priorMap) == 0 {
		return keys
	}
	planRaw, planDiags := expandLXCContainerRawModel(ctx, plan)
	diags.Append(planDiags...)
	if diags.HasError() {
		return keys
	}

	planMap := map[string]string{}
	if !planRaw.ExtraConfig.IsNull() && !planRaw.ExtraConfig.IsUnknown() {
		diags.Append(planRaw.ExtraConfig.ElementsAs(ctx, &planMap, false)...)
	}
	if diags.HasError() {
		return keys
	}

	for key := range priorMap {
		if _, ok := planMap[key]; !ok {
			keys = append(keys, key)
		}
	}
	return keys
}

func isReservedLXCContainerRawKey(key string) bool {
	reserved := map[string]struct{}{
		"hostname": {}, "description": {}, "tags": {}, "cores": {}, "cpulimit": {}, "cpuunits": {}, "memory": {}, "swap": {},
		"onboot": {}, "protection": {}, "startup": {}, "features": {}, "console": {}, "tty": {}, "cmode": {}, "hookscript": {}, "ostype": {},
		"nameserver": {}, "searchdomain": {}, "timezone": {}, "arch": {}, "unprivileged": {},
		"ostemplate": {}, "rootfs": {},
	}
	if _, ok := reserved[key]; ok {
		return true
	}
	return isLXCContainerNetworkKey(key) || isLXCContainerMountPointKey(key)
}
