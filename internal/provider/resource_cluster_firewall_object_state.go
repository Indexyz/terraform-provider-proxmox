// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type privateStateReader interface {
	GetKey(context.Context, string) ([]byte, diag.Diagnostics)
}

type privateStateWriter interface {
	SetKey(context.Context, string, []byte) diag.Diagnostics
}

func readClusterFirewallManagedFields(ctx context.Context, private privateStateReader, key string) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	data, privateDiags := private.GetKey(ctx, key)
	diags.Append(privateDiags...)
	if diags.HasError() || len(data) == 0 {
		return nil, diags
	}
	var fields []string
	if err := json.Unmarshal(data, &fields); err != nil {
		diags.AddError("Unable to Read Firewall Object State", fmt.Sprintf("unable to decode managed fields: %v", err))
	}
	return fields, diags
}

func clusterFirewallFieldManaged(fields []string, field string) bool {
	return slices.Contains(fields, field)
}

func clusterFirewallManagedString(current string, config types.String, managed bool) string {
	if !config.IsNull() && !config.IsUnknown() {
		return config.ValueString()
	}
	if managed {
		return ""
	}
	return current
}

func clusterFirewallManagedBool(current proxmoxOptionalBool, config types.Bool, managed bool) proxmoxOptionalBool {
	if !config.IsNull() && !config.IsUnknown() {
		return proxmoxOptionalBool{value: boolPointerValue(config)}
	}
	if managed {
		return proxmoxOptionalBool{value: boolPointerValue(types.BoolValue(false))}
	}
	return current
}

func storeClusterFirewallManagedFields(ctx context.Context, private privateStateWriter, key string, fields []string, diags *diag.Diagnostics) {
	slices.Sort(fields)
	data, err := json.Marshal(fields)
	if err != nil {
		diags.AddError("Unable to Store Firewall Object State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, key, data)...)
}
