// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func qemuVMStateFromAPI(node string, vmID int64, config QemuVMConfig, status QemuVMStatus) qemuVMModel {
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
		Status:      stringOrNull(status.Status),
		Uptime:      int64OrNull(status.Uptime.Ptr()),
	}
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

func qemuVMCreateRequestFromModel(model qemuVMModel) CreateQemuVMRequest {
	return CreateQemuVMRequest{
		VMID:        model.VMID.ValueInt64(),
		Name:        stringPointerValue(model.Name),
		Description: stringPointerValue(model.Description),
		Tags:        stringPointerValue(model.Tags),
		Pool:        stringPointerValue(model.Pool),
		OnBoot:      boolPointerValue(model.OnBoot),
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
	}
}

func qemuVMUpdateRequestFromModel(model qemuVMModel) UpdateQemuVMRequest {
	return UpdateQemuVMRequest{
		Name:        stringPointerValue(model.Name),
		Description: stringPointerValue(model.Description),
		Tags:        stringPointerValue(model.Tags),
		Pool:        stringPointerValue(model.Pool),
		OnBoot:      boolPointerValue(model.OnBoot),
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
	}
}
