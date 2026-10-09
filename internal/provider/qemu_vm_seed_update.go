// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

const qemuSeedAttachmentKey = "qemu-nocloud-attachment"

type qemuSeedAttachment struct {
	Node    string `json:"node"`
	VMID    int64  `json:"vmid"`
	Slot    string `json:"slot"`
	Volume  string `json:"volume"`
	Pending string `json:"pending,omitempty"`
}

func readQemuSeedAttachment(ctx context.Context, private privateStateReader) (qemuSeedAttachment, diag.Diagnostics) {
	var attachment qemuSeedAttachment
	data, diags := private.GetKey(ctx, qemuSeedAttachmentKey)
	if diags.HasError() || len(data) == 0 {
		return attachment, diags
	}
	if err := json.Unmarshal(data, &attachment); err != nil {
		diags.AddError("Unable to Read NoCloud Attachment History", err.Error())
	}
	return attachment, diags
}

func writeQemuSeedAttachment(ctx context.Context, private privateStateWriter, attachment qemuSeedAttachment) diag.Diagnostics {
	data, err := json.Marshal(attachment)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Unable to Store NoCloud Attachment History", err.Error())
		return diags
	}
	return private.SetKey(ctx, qemuSeedAttachmentKey, data)
}

func qemuSeedFromModel(ctx context.Context, model qemuVMModel) (qemuSeedAttachment, diag.Diagnostics) {
	slot := qemuVMNoCloudSlotValue(model)
	if slot == "" {
		return qemuSeedAttachment{}, nil
	}
	disks, diags := expandQemuVMDiskModelMap(ctx, model.Disk)
	volume := ""
	if stringValue(disks[slot].Media) == "cdrom" {
		volume = stringValue(disks[slot].Volume)
	}
	return qemuSeedAttachment{Node: model.Node.ValueString(), VMID: model.VMID.ValueInt64(), Slot: slot, Volume: volume}, diags
}

// Authorize only the marked slot; all other inherited media retain the
// existing ownership and double-seed checks. Refresh state is never evidence.
func authorizeQemuSeedUpdate(model qemuVMModel, planned map[string]string, current QemuVMConfig, history qemuSeedAttachment) (map[string]string, bool, error) {
	slot := qemuVMNoCloudSlotValue(model)
	target := planned[slot]
	wire := current.Disk[slot]
	volume := qemuVMWireDiskVolume(wire)
	if history.Pending != "" && (history.Node != model.Node.ValueString() || history.VMID != model.VMID.ValueInt64() || history.Slot != slot || history.Pending != target) {
		return nil, false, fmt.Errorf("finish the pending NoCloud attachment to %q before changing its target or VM identity", history.Pending)
	}
	if target == volume {
		return current.Disk, false, nil
	}
	if !qemuVMWireDiskHasMediaCDROM(wire) || !strings.HasSuffix(strings.ToLower(volume), ".iso") {
		return current.Disk, true, nil // ordinary empty/PVE drive checks must authorize this mutation
	}
	owned := history.Node == model.Node.ValueString() && history.VMID == model.VMID.ValueInt64() && history.Slot == slot && (volume == history.Volume || volume == history.Pending)
	explicit := model.NoCloudUpdateFrom.ValueString() != "" && model.NoCloudUpdateFrom.ValueString() == volume
	if !owned && !explicit {
		return nil, false, fmt.Errorf("NoCloud slot %q contains unproven ISO %q; refresh/import does not grant ownership. To migrate an existing workspace, explicitly authorize its verified old seed with nocloud_seed_update_from", slot, volume)
	}
	inherited := maps.Clone(current.Disk)
	inherited[slot] = target + ",media=cdrom"
	return inherited, true, nil
}
