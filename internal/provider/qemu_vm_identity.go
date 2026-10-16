// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// Proxmox reuses a freed VMID for the next guest, so node/VMID alone cannot
// tell the tracked guest from an unrelated guest that took its VMID after
// the tracked one disappeared. The recorded name is the identity check: a
// guest at the tracked VMID under any other name means the tracked guest is
// gone, and it must be neither adopted by refresh nor stopped or deleted.

// qemuVMPendingNameKey holds a rename whose PUT was sent but not confirmed,
// so a lost reply cannot make the renamed guest look foreign.
const qemuVMPendingNameKey = "qemu-vm-pending-name"

// qemuVMOwnedNames returns the names under which the tracked guest may
// appear. Empty means no name was recorded (imported or unnamed guests), so
// identity is not checked.
func qemuVMOwnedNames(ctx context.Context, state qemuVMModel, private privateStateReader) ([]string, diag.Diagnostics) {
	if state.Name.IsNull() || state.Name.IsUnknown() || state.Name.ValueString() == "" {
		return nil, nil
	}
	names := []string{state.Name.ValueString()}
	data, diags := private.GetKey(ctx, qemuVMPendingNameKey)
	if diags.HasError() || len(data) == 0 {
		return names, diags
	}
	var pending string
	if err := json.Unmarshal(data, &pending); err != nil {
		diags.AddError("Unable to Read Proxmox QEMU VM State", fmt.Sprintf("unable to decode pending rename: %v", err))
		return nil, diags
	}
	return append(names, pending), diags
}

// writeQemuVMPendingName records an unconfirmed rename; "" clears it.
func writeQemuVMPendingName(ctx context.Context, private privateStateWriter, name string) diag.Diagnostics {
	if name == "" {
		return private.SetKey(ctx, qemuVMPendingNameKey, nil)
	}
	encoded, err := json.Marshal(name)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Unable to Store Proxmox QEMU VM State", fmt.Sprintf("unable to encode pending rename: %v", err))
		return diags
	}
	return private.SetKey(ctx, qemuVMPendingNameKey, encoded)
}

func qemuVMOwnsGuest(owned []string, liveName string) bool {
	return len(owned) == 0 || slices.Contains(owned, liveName)
}

func qemuVMForeignGuestWarning(node string, vmID int64, owned []string, liveName string) diag.Diagnostics {
	var diags diag.Diagnostics
	diags.AddWarning(
		"Proxmox QEMU VM Replaced by a Different Guest",
		fmt.Sprintf("VMID %d on node %q now holds a different guest named %q, not the tracked %q. The tracked guest no longer exists; the other guest is left untouched.", vmID, node, liveName, owned[0]),
	)
	return diags
}
