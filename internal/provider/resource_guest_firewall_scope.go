// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type guestFirewallScope struct {
	Node      string
	GuestType string
	VMID      int64
}

func guestFirewallScopeFromModel(node, guestType types.String, vmid types.Int64) guestFirewallScope {
	return guestFirewallScope{Node: node.ValueString(), GuestType: guestType.ValueString(), VMID: vmid.ValueInt64()}
}

func validateGuestFirewallScope(node, guestType types.String, vmid types.Int64) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(node) && !proxmoxNodeNamePattern.MatchString(node.ValueString()) {
		diags.AddAttributeError(path.Root("node"), "Invalid node name", "node must be an existing cluster node name.")
	}
	if isConfiguredValue(guestType) && guestType.ValueString() != "qemu" && guestType.ValueString() != "lxc" {
		diags.AddAttributeError(path.Root("guest_type"), "Invalid guest type", "guest_type must be qemu or lxc.")
	}
	if isConfiguredValue(vmid) && vmid.ValueInt64() < 100 {
		diags.AddAttributeError(path.Root("vmid"), "Invalid guest VMID", "vmid must be at least 100.")
	}
	return diags
}

// parseGuestFirewallImportID splits `<node>/<guest_type>/<vmid>/<name>`.
func parseGuestFirewallImportID(id string) (guestFirewallScope, string, diag.Diagnostics) {
	var diags diag.Diagnostics
	parts := strings.Split(id, "/")
	if len(parts) != 4 {
		diags.AddError("Unexpected Import Identifier", "Expected `<node>/<guest_type>/<vmid>/<name>`.")
		return guestFirewallScope{}, "", diags
	}
	vmid, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		diags.AddError("Unexpected Import Identifier", fmt.Sprintf("vmid %q is not a number.", parts[2]))
		return guestFirewallScope{}, "", diags
	}
	scope := guestFirewallScope{Node: parts[0], GuestType: parts[1], VMID: vmid}
	diags.Append(validateGuestFirewallScope(types.StringValue(parts[0]), types.StringValue(parts[1]), types.Int64Value(vmid))...)
	if stringsTrimIsEmpty(parts[3]) {
		diags.AddError("Unexpected Import Identifier", "The object name must not be empty.")
	}
	return scope, parts[3], diags
}
