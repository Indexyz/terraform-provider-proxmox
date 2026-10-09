// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var qemuResizeSlot = regexp.MustCompile(`^(ide[0-3]|sata[0-5]|scsi([0-9]|[12][0-9]|30)|virtio([0-9]|1[0-5]))$`)
var qemuDiskSize = regexp.MustCompile(`^(\d+(?:\.\d+)?)([KMGT]?)$`)

func validateQemuVMDiskResize(ctx context.Context, model qemuVMModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if model.DiskResize.IsNull() || model.DiskResize.IsUnknown() {
		return diags
	}
	disks, diskDiags := expandQemuVMDiskModelMap(ctx, model.Disk)
	diags.Append(diskDiags...)
	raw, rawDiags := expandQemuVMRawModel(ctx, model.Raw)
	diags.Append(rawDiags...)
	for slot, value := range model.DiskResize.Elements() {
		p := path.Root("disk_resize").AtMapKey(slot)
		if !qemuResizeSlot.MatchString(slot) {
			diags.AddAttributeError(p, "Invalid resize slot", "Use an existing ide, sata, scsi or virtio hard disk slot.")
		}
		n := value.(types.Int64)
		if !n.IsUnknown() && (n.IsNull() || n.ValueInt64() < 1 || n.ValueInt64() > 1048576) {
			diags.AddAttributeError(p, "Invalid resize size", "Absolute size must be an integer from 1 to 1048576 GiB.")
		}
		if _, ok := disks[slot]; ok {
			diags.AddAttributeError(p, "Conflicting disk configuration", "Do not configure the same slot in disk and disk_resize; resize must preserve its existing volume.")
		}
		if _, ok := raw.ExtraConfig.Elements()[slot]; ok {
			diags.AddAttributeError(p, "Conflicting raw disk", "Do not configure a resized slot in raw.extra_config.")
		}
	}
	return diags
}

func qemuDiskSizeBytes(size string) (float64, error) {
	parts := qemuDiskSize.FindStringSubmatch(size)
	if parts == nil {
		return 0, fmt.Errorf("invalid observed disk size %q", size)
	}
	n, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, fmt.Errorf("parse disk size %q: %w", size, err)
	}
	for i := 0; i < strings.Index(" KMGT", parts[2]); i++ {
		n *= 1024
	}
	if n <= 0 || math.IsInf(n, 0) {
		return 0, fmt.Errorf("invalid observed disk size %q", size)
	}
	return n, nil
}

// ResizeQemuDisk uses absolute sizes so retries cannot grow a disk twice.
func (c *Client) ResizeQemuDisk(ctx context.Context, node string, vmID int64, slot string, sizeGiB int64, digest string) error {
	form := url.Values{"disk": {slot}, "size": {fmt.Sprintf("%dG", sizeGiB)}}
	if digest != "" {
		form.Set("digest", digest)
	}
	var upid string
	if err := c.do(ctx, http.MethodPut, fmt.Sprintf("/nodes/%s/qemu/%d/resize", url.PathEscape(node), vmID), nil, form, &upid); err != nil {
		return err
	}
	if err := validateQemuTaskAck(upid, fmt.Sprintf("resize task for VM %d disk %q", vmID, slot)); err != nil {
		return err
	}
	return c.waitForNodeTask(ctx, node, upid)
}

func (r *QemuVMResource) resizeDisks(ctx context.Context, model qemuVMModel) error {
	if model.DiskResize.IsNull() || len(model.DiskResize.Elements()) == 0 {
		return nil
	}
	var targets map[string]int64
	if diags := model.DiskResize.ElementsAs(ctx, &targets, false); diags.HasError() {
		return fmt.Errorf("read resize targets: %v", diags)
	}
	slots := make([]string, 0, len(targets))
	for slot := range targets {
		slots = append(slots, slot)
	}
	slices.Sort(slots)
	for _, slot := range slots {
		// Fresh config and digest per disk: a preceding resize changes digest.
		config, err := r.client.GetQemuVMConfig(ctx, model.Node.ValueString(), model.VMID.ValueInt64())
		if err != nil {
			return fmt.Errorf("read disk %s before resize: %w", slot, err)
		}
		wire, exists := config.Disk[slot]
		if !exists {
			return fmt.Errorf("disk %s does not exist", slot)
		}
		// Parse only resize-relevant wire keys; unrelated drive options must
		// not prevent safely resizing an inherited disk.
		var size, volume, media string
		for i, part := range splitQemuConfigSegments(wire) {
			key, value, ok := splitQemuConfigKeyValue(part)
			if i == 0 && !ok {
				volume = part
			}
			switch key {
			case "file", "volume":
				volume = value
			case "size":
				size = value
			case "media":
				media = value
			}
		}
		if media == "cdrom" || !strings.Contains(volume, ":") || strings.Contains(volume, "cloudinit") || strings.HasSuffix(volume, ".iso") {
			return fmt.Errorf("disk %s is not a resizable hard disk", slot)
		}
		current, err := qemuDiskSizeBytes(size)
		if err != nil {
			return fmt.Errorf("disk %s: %w", slot, err)
		}
		target := float64(targets[slot]) * (1 << 30)
		if target < current {
			return fmt.Errorf("disk %s cannot shrink from %s to %dG", slot, size, targets[slot])
		}
		if target == current {
			continue
		}
		digest := config.ExtraConfig["digest"]
		if digest == "" {
			return fmt.Errorf("disk %s: missing configuration digest before resize", slot)
		}
		if err := r.client.ResizeQemuDisk(ctx, model.Node.ValueString(), model.VMID.ValueInt64(), slot, targets[slot], digest); err != nil {
			return fmt.Errorf("resize disk %s: %w", slot, err)
		}
		observed, err := r.client.GetQemuVMConfig(ctx, model.Node.ValueString(), model.VMID.ValueInt64())
		if err != nil {
			return fmt.Errorf("verify resized disk %s: %w", slot, err)
		}
		var actualSize string
		for _, part := range splitQemuConfigSegments(observed.Disk[slot]) {
			if key, value, ok := splitQemuConfigKeyValue(part); ok && key == "size" {
				actualSize = value
			}
		}
		actual, err := qemuDiskSizeBytes(actualSize)
		if err != nil {
			return fmt.Errorf("verify disk %s: %w", slot, err)
		}
		if actual != target {
			return fmt.Errorf("disk %s resize did not converge: got %s, wanted %dG", slot, actualSize, targets[slot])
		}
	}
	return nil
}
