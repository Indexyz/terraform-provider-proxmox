// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Typed compound devices: hostpci, usb, rng and virtiofs. Proxmox encodes each
// slot as a property string whose default key may be positional; whole slots the
// parsers do not understand stay in raw.extra_config.

var (
	qemuVMHostPciSlot    = regexp.MustCompile(`^hostpci(1[0-5]|[0-9])$`)
	qemuVMUSBSlot        = regexp.MustCompile(`^usb(1[0-3]|[0-9])$`)
	qemuVMRNGSlot        = regexp.MustCompile(`^rng0$`)
	qemuVMVirtiofsSlot   = regexp.MustCompile(`^virtiofs[0-9]$`)
	qemuVMHostPCIHost    = regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-9a-fA-F](;[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-9a-fA-F])*$`)
	qemuVMUSBHost        = regexp.MustCompile(`^(?:[0-9]+-[0-9]+(?:\.[0-9]+)*|[0-9a-fA-F]{4}:[0-9a-fA-F]{4}|[Ss][Pp][Ii][Cc][Ee])$`)
	qemuVMHostPCIMdev    = regexp.MustCompile(`^[^/.:]+$`)
	qemuVMHostPCIRomFile = regexp.MustCompile(`^[^,;]+$`)
	qemuVMHostPCIID      = regexp.MustCompile(`^0x[0-9a-fA-F]{4}$`)
	qemuVMVirtiofsDirID  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]+$`)
	qemuVMHostPCIDriver  = []string{"vfio", "keep"}
	qemuVMVirtiofsCaches = []string{"auto", "always", "metadata", "never"}
	qemuVMRNGSource      = []string{"/dev/urandom", "/dev/random", "/dev/hwrng"}
)

type qemuVMHostPciModel struct {
	Host        types.String `tfsdk:"host"`
	Mapping     types.String `tfsdk:"mapping"`
	Mdev        types.String `tfsdk:"mdev"`
	PCIE        types.Bool   `tfsdk:"pcie"`
	XVGA        types.Bool   `tfsdk:"x_vga"`
	ROMBar      types.Bool   `tfsdk:"rombar"`
	ROMFile     types.String `tfsdk:"rom_file"`
	LegacyIGD   types.Bool   `tfsdk:"legacy_igd"`
	VendorID    types.String `tfsdk:"vendor_id"`
	DeviceID    types.String `tfsdk:"device_id"`
	SubVendorID types.String `tfsdk:"sub_vendor_id"`
	SubDeviceID types.String `tfsdk:"sub_device_id"`
	Driver      types.String `tfsdk:"driver"`
}

type qemuVMUSBModel struct {
	Host    types.String `tfsdk:"host"`
	Mapping types.String `tfsdk:"mapping"`
	USB3    types.Bool   `tfsdk:"usb3"`
}

type qemuVMRNGModel struct {
	Source   types.String `tfsdk:"source"`
	MaxBytes types.Int64  `tfsdk:"max_bytes"`
	Period   types.Int64  `tfsdk:"period"`
}

type qemuVMVirtiofsModel struct {
	DirID       types.String `tfsdk:"dirid"`
	Cache       types.String `tfsdk:"cache"`
	DirectIO    types.Bool   `tfsdk:"direct_io"`
	ExposeXAttr types.Bool   `tfsdk:"expose_xattr"`
	ExposeACL   types.Bool   `tfsdk:"expose_acl"`
}

func qemuVMHostPciAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"host":          types.StringType,
		"mapping":       types.StringType,
		"mdev":          types.StringType,
		"pcie":          types.BoolType,
		"x_vga":         types.BoolType,
		"rombar":        types.BoolType,
		"rom_file":      types.StringType,
		"legacy_igd":    types.BoolType,
		"vendor_id":     types.StringType,
		"device_id":     types.StringType,
		"sub_vendor_id": types.StringType,
		"sub_device_id": types.StringType,
		"driver":        types.StringType,
	}
}

func qemuVMUSBAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"host":    types.StringType,
		"mapping": types.StringType,
		"usb3":    types.BoolType,
	}
}

func qemuVMRNGAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"source":    types.StringType,
		"max_bytes": types.Int64Type,
		"period":    types.Int64Type,
	}
}

func qemuVMVirtiofsAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"dirid":        types.StringType,
		"cache":        types.StringType,
		"direct_io":    types.BoolType,
		"expose_xattr": types.BoolType,
		"expose_acl":   types.BoolType,
	}
}

func isQemuVMHostPciKey(key string) bool { return qemuVMHostPciSlot.MatchString(key) }
func isQemuVMUSBKey(key string) bool     { return qemuVMUSBSlot.MatchString(key) }
func isQemuVMRNGKey(key string) bool     { return qemuVMRNGSlot.MatchString(key) }
func isQemuVMVirtiofsKey(key string) bool {
	return qemuVMVirtiofsSlot.MatchString(key)
}

func parseQemuVMHostPci(raw string) (qemuVMHostPciModel, bool) {
	if strings.TrimSpace(raw) == "" {
		return qemuVMHostPciModel{}, false
	}
	item := qemuVMHostPciModel{}
	for index, segment := range splitQemuConfigSegments(raw) {
		key, value, ok := splitQemuConfigKeyValue(segment)
		if !ok {
			if index != 0 {
				return qemuVMHostPciModel{}, false
			}
			item.Host = types.StringValue(strings.TrimSpace(segment))
			continue
		}
		switch key {
		case "host":
			item.Host = types.StringValue(value)
		case "mapping":
			item.Mapping = types.StringValue(value)
		case "mdev":
			item.Mdev = types.StringValue(value)
		case "pcie":
			item.PCIE = qemuVMOptionalBoolValue(value)
		case "x-vga":
			item.XVGA = qemuVMOptionalBoolValue(value)
		case "rombar":
			item.ROMBar = qemuVMOptionalBoolValue(value)
		case "romfile":
			item.ROMFile = types.StringValue(value)
		case "legacy-igd":
			item.LegacyIGD = qemuVMOptionalBoolValue(value)
		case "vendor-id":
			item.VendorID = types.StringValue(value)
		case "device-id":
			item.DeviceID = types.StringValue(value)
		case "sub-vendor-id":
			item.SubVendorID = types.StringValue(value)
		case "sub-device-id":
			item.SubDeviceID = types.StringValue(value)
		case "driver":
			item.Driver = types.StringValue(value)
		default:
			return qemuVMHostPciModel{}, false
		}
	}
	return item, true
}

func parseQemuVMUSB(raw string) (qemuVMUSBModel, bool) {
	if strings.TrimSpace(raw) == "" {
		return qemuVMUSBModel{}, false
	}
	item := qemuVMUSBModel{}
	for index, segment := range splitQemuConfigSegments(raw) {
		key, value, ok := splitQemuConfigKeyValue(segment)
		if !ok {
			if index != 0 {
				return qemuVMUSBModel{}, false
			}
			item.Host = types.StringValue(strings.TrimSpace(segment))
			continue
		}
		switch key {
		case "host":
			item.Host = types.StringValue(value)
		case "mapping":
			item.Mapping = types.StringValue(value)
		case "usb3":
			item.USB3 = qemuVMOptionalBoolValue(value)
		default:
			return qemuVMUSBModel{}, false
		}
	}
	return item, true
}

func parseQemuVMRNG(raw string) (qemuVMRNGModel, bool) {
	if strings.TrimSpace(raw) == "" {
		return qemuVMRNGModel{}, false
	}
	item := qemuVMRNGModel{}
	for index, segment := range splitQemuConfigSegments(raw) {
		key, value, ok := splitQemuConfigKeyValue(segment)
		if !ok {
			if index != 0 {
				return qemuVMRNGModel{}, false
			}
			item.Source = types.StringValue(strings.TrimSpace(segment))
			continue
		}
		switch key {
		case "source":
			item.Source = types.StringValue(value)
		case "max_bytes", "period":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return qemuVMRNGModel{}, false
			}
			if key == "max_bytes" {
				item.MaxBytes = types.Int64Value(parsed)
			} else {
				item.Period = types.Int64Value(parsed)
			}
		default:
			return qemuVMRNGModel{}, false
		}
	}
	return item, true
}

func parseQemuVMVirtiofs(raw string) (qemuVMVirtiofsModel, bool) {
	if strings.TrimSpace(raw) == "" {
		return qemuVMVirtiofsModel{}, false
	}
	item := qemuVMVirtiofsModel{}
	for index, segment := range splitQemuConfigSegments(raw) {
		key, value, ok := splitQemuConfigKeyValue(segment)
		if !ok {
			if index != 0 {
				return qemuVMVirtiofsModel{}, false
			}
			item.DirID = types.StringValue(strings.TrimSpace(segment))
			continue
		}
		switch key {
		case "dirid":
			item.DirID = types.StringValue(value)
		case "cache":
			item.Cache = types.StringValue(value)
		case "direct-io":
			item.DirectIO = qemuVMOptionalBoolValue(value)
		case "expose-xattr":
			item.ExposeXAttr = qemuVMOptionalBoolValue(value)
		case "expose-acl":
			item.ExposeACL = qemuVMOptionalBoolValue(value)
		default:
			return qemuVMVirtiofsModel{}, false
		}
	}
	return item, true
}

func qemuVMOptionalBoolValue(raw string) types.Bool {
	if raw == "1" {
		return types.BoolValue(true)
	}
	return types.BoolValue(false)
}

func encodeQemuVMHostPci(item qemuVMHostPciModel) string {
	segments := []string{}
	if host := stringValue(item.Host); host != "" {
		segments = append(segments, host)
	} else if mapping := stringValue(item.Mapping); mapping != "" {
		segments = append(segments, "mapping="+mapping)
	}
	appendStringConfig(&segments, "mdev", item.Mdev)
	appendBoolConfig(&segments, "pcie", item.PCIE)
	appendBoolConfig(&segments, "x-vga", item.XVGA)
	appendBoolConfig(&segments, "rombar", item.ROMBar)
	appendStringConfig(&segments, "romfile", item.ROMFile)
	appendBoolConfig(&segments, "legacy-igd", item.LegacyIGD)
	appendStringConfig(&segments, "vendor-id", item.VendorID)
	appendStringConfig(&segments, "device-id", item.DeviceID)
	appendStringConfig(&segments, "sub-vendor-id", item.SubVendorID)
	appendStringConfig(&segments, "sub-device-id", item.SubDeviceID)
	appendStringConfig(&segments, "driver", item.Driver)
	return strings.Join(segments, ",")
}

func encodeQemuVMUSB(item qemuVMUSBModel) string {
	segments := []string{}
	if host := stringValue(item.Host); host != "" {
		segments = append(segments, host)
	} else if mapping := stringValue(item.Mapping); mapping != "" {
		segments = append(segments, "mapping="+mapping)
	}
	appendBoolConfig(&segments, "usb3", item.USB3)
	return strings.Join(segments, ",")
}

func encodeQemuVMRNG(item qemuVMRNGModel) string {
	segments := []string{}
	if source := stringValue(item.Source); source != "" {
		segments = append(segments, source)
	}
	appendInt64Config(&segments, "max_bytes", item.MaxBytes)
	appendInt64Config(&segments, "period", item.Period)
	return strings.Join(segments, ",")
}

func encodeQemuVMVirtiofs(item qemuVMVirtiofsModel) string {
	segments := []string{}
	if dirID := stringValue(item.DirID); dirID != "" {
		segments = append(segments, dirID)
	}
	appendStringConfig(&segments, "cache", item.Cache)
	appendBoolConfig(&segments, "direct-io", item.DirectIO)
	appendBoolConfig(&segments, "expose-xattr", item.ExposeXAttr)
	appendBoolConfig(&segments, "expose-acl", item.ExposeACL)
	return strings.Join(segments, ",")
}

// validateQemuVMDeviceSlots validates slot names and the per-type default-key
// rules before anything is sent to Proxmox.
func validateQemuVMHostPciItem(slot string, item qemuVMHostPciModel) diag.Diagnostics {
	var diags diag.Diagnostics
	at := func(name string) path.Path { return qemuVMDevicePath(slot).AtName(name) }
	hasHost := isConfiguredValue(item.Host)
	hasMapping := isConfiguredValue(item.Mapping)
	switch {
	case !hasHost && !hasMapping:
		diags.AddAttributeError(qemuVMDevicePath(slot), "Missing host PCI source", "Either host or mapping must be configured for a hostpci slot.")
	case hasHost && hasMapping:
		diags.AddAttributeError(qemuVMDevicePath(slot), "Conflicting host PCI source", "Configure either host or mapping, not both.")
	case hasHost && !qemuVMHostPCIHost.MatchString(item.Host.ValueString()):
		diags.AddAttributeError(at("host"), "Invalid host PCI id", "host must use bus:dev.func hexadecimal form, optionally as a semicolon separated function list.")
	}
	if isConfiguredValue(item.Mdev) && !qemuVMHostPCIMdev.MatchString(item.Mdev.ValueString()) {
		diags.AddAttributeError(at("mdev"), "Invalid mediated device type", "mdev must not contain `/`, `.` or `:`.")
	}
	if isConfiguredValue(item.ROMFile) && !qemuVMHostPCIRomFile.MatchString(item.ROMFile.ValueString()) {
		diags.AddAttributeError(at("rom_file"), "Invalid PCI option ROM file", "rom_file must not contain `,` or `;`.")
	}
	for name, value := range map[string]types.String{"vendor_id": item.VendorID, "device_id": item.DeviceID, "sub_vendor_id": item.SubVendorID, "sub_device_id": item.SubDeviceID} {
		if isConfiguredValue(value) && !qemuVMHostPCIID.MatchString(value.ValueString()) {
			diags.AddAttributeError(at(name), "Invalid PCI id override", name+" must be a hexadecimal id such as 0x10de.")
		}
	}
	if isConfiguredValue(item.Driver) && !slices.Contains(qemuVMHostPCIDriver, item.Driver.ValueString()) {
		diags.AddAttributeError(at("driver"), "Invalid PCI driver", "driver must be vfio or keep.")
	}
	return diags
}

func validateQemuVMUSBItem(slot string, item qemuVMUSBModel) diag.Diagnostics {
	var diags diag.Diagnostics
	hasHost := isConfiguredValue(item.Host)
	hasMapping := isConfiguredValue(item.Mapping)
	switch {
	case !hasHost && !hasMapping:
		diags.AddAttributeError(qemuVMDevicePath(slot), "Missing USB source", "Either host or mapping must be configured for a usb slot.")
	case hasHost && hasMapping:
		diags.AddAttributeError(qemuVMDevicePath(slot), "Conflicting USB source", "Configure either host or mapping, not both.")
	case hasHost && !qemuVMUSBHost.MatchString(item.Host.ValueString()):
		diags.AddAttributeError(qemuVMDevicePath(slot).AtName("host"), "Invalid USB device", "host must be a bus-port path, vendor:product id, or spice.")
	}
	return diags
}

func validateQemuVMRNGItem(slot string, item qemuVMRNGModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if !isConfiguredValue(item.Source) {
		diags.AddAttributeError(qemuVMDevicePath(slot), "Missing entropy source", "source is required for an rng slot.")
	} else if !slices.Contains(qemuVMRNGSource, item.Source.ValueString()) {
		diags.AddAttributeError(qemuVMDevicePath(slot).AtName("source"), "Invalid entropy source", "source must be /dev/urandom, /dev/random, or /dev/hwrng.")
	}
	for name, value := range map[string]types.Int64{"max_bytes": item.MaxBytes, "period": item.Period} {
		if isConfiguredValue(value) && value.ValueInt64() < 0 {
			diags.AddAttributeError(qemuVMDevicePath(slot).AtName(name), "Invalid entropy limit", name+" must not be negative.")
		}
	}
	return diags
}

func validateQemuVMVirtiofsItem(slot string, item qemuVMVirtiofsModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if !isConfiguredValue(item.DirID) {
		diags.AddAttributeError(qemuVMDevicePath(slot), "Missing directory mapping", "dirid is required for a virtiofs slot.")
	} else if !qemuVMVirtiofsDirID.MatchString(item.DirID.ValueString()) {
		diags.AddAttributeError(qemuVMDevicePath(slot).AtName("dirid"), "Invalid directory mapping id", "dirid must be a valid configuration id.")
	}
	if isConfiguredValue(item.Cache) && !slices.Contains(qemuVMVirtiofsCaches, item.Cache.ValueString()) {
		diags.AddAttributeError(qemuVMDevicePath(slot).AtName("cache"), "Invalid virtiofs cache policy", "cache must be auto, always, metadata, or never.")
	}
	return diags
}

// expandQemuVMHostPciModelMap validates and expands the typed hostpci map.
func expandQemuVMHostPciModelMap(ctx context.Context, value types.Map) (map[string]string, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}
	var items map[string]qemuVMHostPciModel
	diags := value.ElementsAs(ctx, &items, false)
	if diags.HasError() {
		return nil, diags
	}
	result := make(map[string]string, len(items))
	for slot, item := range items {
		if !isQemuVMHostPciKey(slot) {
			diags.AddAttributeError(qemuVMDevicePath(slot), "Invalid hostpci slot", "hostpci slots use hostpci0 to hostpci15.")
			continue
		}
		diags.Append(validateQemuVMHostPciItem(slot, item)...)
		if encoded := encodeQemuVMHostPci(item); encoded != "" {
			result[slot] = encoded
		}
	}
	return result, diags
}

func expandQemuVMUSBModelMap(ctx context.Context, value types.Map) (map[string]string, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}
	var items map[string]qemuVMUSBModel
	diags := value.ElementsAs(ctx, &items, false)
	if diags.HasError() {
		return nil, diags
	}
	result := make(map[string]string, len(items))
	for slot, item := range items {
		if !isQemuVMUSBKey(slot) {
			diags.AddAttributeError(qemuVMDevicePath(slot), "Invalid usb slot", "usb slots use usb0 to usb13.")
			continue
		}
		diags.Append(validateQemuVMUSBItem(slot, item)...)
		if encoded := encodeQemuVMUSB(item); encoded != "" {
			result[slot] = encoded
		}
	}
	return result, diags
}

func expandQemuVMRNGModelMap(ctx context.Context, value types.Map) (map[string]string, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}
	var items map[string]qemuVMRNGModel
	diags := value.ElementsAs(ctx, &items, false)
	if diags.HasError() {
		return nil, diags
	}
	result := make(map[string]string, len(items))
	for slot, item := range items {
		if !isQemuVMRNGKey(slot) {
			diags.AddAttributeError(qemuVMDevicePath(slot), "Invalid rng slot", "the only rng slot is rng0.")
			continue
		}
		diags.Append(validateQemuVMRNGItem(slot, item)...)
		if encoded := encodeQemuVMRNG(item); encoded != "" {
			result[slot] = encoded
		}
	}
	return result, diags
}

func expandQemuVMVirtiofsModelMap(ctx context.Context, value types.Map) (map[string]string, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}
	var items map[string]qemuVMVirtiofsModel
	diags := value.ElementsAs(ctx, &items, false)
	if diags.HasError() {
		return nil, diags
	}
	result := make(map[string]string, len(items))
	for slot, item := range items {
		if !isQemuVMVirtiofsKey(slot) {
			diags.AddAttributeError(qemuVMDevicePath(slot), "Invalid virtiofs slot", "virtiofs slots use virtiofs0 to virtiofs9.")
			continue
		}
		diags.Append(validateQemuVMVirtiofsItem(slot, item)...)
		if encoded := encodeQemuVMVirtiofs(item); encoded != "" {
			result[slot] = encoded
		}
	}
	return result, diags
}

func qemuVMHostPciStateValue(ctx context.Context, source map[string]string) (types.Map, map[string]string, diag.Diagnostics) {
	return qemuVMDeviceStateValue(ctx, source, isQemuVMHostPciKey, parseQemuVMHostPci,
		types.ObjectType{AttrTypes: qemuVMHostPciAttrTypes()})
}

func qemuVMUSBStateValue(ctx context.Context, source map[string]string) (types.Map, map[string]string, diag.Diagnostics) {
	return qemuVMDeviceStateValue(ctx, source, isQemuVMUSBKey, parseQemuVMUSB,
		types.ObjectType{AttrTypes: qemuVMUSBAttrTypes()})
}

func qemuVMRNGStateValue(ctx context.Context, source map[string]string) (types.Map, map[string]string, diag.Diagnostics) {
	return qemuVMDeviceStateValue(ctx, source, isQemuVMRNGKey, parseQemuVMRNG,
		types.ObjectType{AttrTypes: qemuVMRNGAttrTypes()})
}

func qemuVMVirtiofsStateValue(ctx context.Context, source map[string]string) (types.Map, map[string]string, diag.Diagnostics) {
	return qemuVMDeviceStateValue(ctx, source, isQemuVMVirtiofsKey, parseQemuVMVirtiofs,
		types.ObjectType{AttrTypes: qemuVMVirtiofsAttrTypes()})
}

// qemuVMDeviceStateValue maps one typed slot family and returns the slots its
// parser could not understand so they stay observable through raw.
func qemuVMDeviceStateValue[T any](
	ctx context.Context,
	source map[string]string,
	isKey func(string) bool,
	parse func(string) (T, bool),
	objectType types.ObjectType,
) (types.Map, map[string]string, diag.Diagnostics) {
	unsupported := map[string]string{}
	if len(source) == 0 {
		return types.MapNull(objectType), nil, nil
	}
	items := make(map[string]T)
	for key, raw := range source {
		if !isKey(key) {
			continue
		}
		parsed, ok := parse(raw)
		if !ok {
			unsupported[key] = raw
			continue
		}
		items[key] = parsed
	}
	if len(items) == 0 {
		return types.MapNull(objectType), unsupported, nil
	}
	value, diags := types.MapValueFrom(ctx, objectType, items)
	return value, unsupported, diags
}

func qemuVMDevicePath(slot string) path.Path { return path.Root("disk").AtMapKey(slot) }

func qemuVMHostPciResourceAttribute() schema.MapNestedAttribute {
	return schema.MapNestedAttribute{
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
		MarkdownDescription: "Typed host PCI passthrough keyed by `hostpci0` to `hostpci15`. Configure either `host` (`bus:dev.func`, optionally a semicolon list of virtual functions) or a cluster `mapping` id. `rom_file` names an option ROM in `/usr/share/kvm/`, `legacy_igd` only works with `pc-i440fx` and VGA `none`, and the `*_id` overrides change the ids the guest sees. Passthrough devices usually make the guest non-migratable. Slots removed from configuration are deleted. Unparsable slots stay in `raw.extra_config`.",
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"host":          schema.StringAttribute{Optional: true, Computed: true},
			"mapping":       schema.StringAttribute{Optional: true, Computed: true},
			"mdev":          schema.StringAttribute{Optional: true, Computed: true},
			"pcie":          schema.BoolAttribute{Optional: true, Computed: true},
			"x_vga":         schema.BoolAttribute{Optional: true, Computed: true},
			"rombar":        schema.BoolAttribute{Optional: true, Computed: true},
			"rom_file":      schema.StringAttribute{Optional: true, Computed: true},
			"legacy_igd":    schema.BoolAttribute{Optional: true, Computed: true},
			"vendor_id":     schema.StringAttribute{Optional: true, Computed: true},
			"device_id":     schema.StringAttribute{Optional: true, Computed: true},
			"sub_vendor_id": schema.StringAttribute{Optional: true, Computed: true},
			"sub_device_id": schema.StringAttribute{Optional: true, Computed: true},
			"driver":        schema.StringAttribute{Optional: true, Computed: true},
		}},
	}
}

func qemuVMUSBResourceAttribute() schema.MapNestedAttribute {
	return schema.MapNestedAttribute{
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
		MarkdownDescription: "Typed USB passthrough keyed by `usb0` to `usb13`. Configure either `host` (`bus-port` path, `vendor:product` id, or `spice`) or a cluster `mapping` id. Slots removed from configuration are deleted. Unparsable slots stay in `raw.extra_config`.",
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"host":    schema.StringAttribute{Optional: true, Computed: true},
			"mapping": schema.StringAttribute{Optional: true, Computed: true},
			"usb3":    schema.BoolAttribute{Optional: true, Computed: true},
		}},
	}
}

func qemuVMRNGResourceAttribute() schema.MapNestedAttribute {
	return schema.MapNestedAttribute{
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
		MarkdownDescription: "Typed entropy source at the only slot `rng0`. `source` is required (`/dev/urandom`, `/dev/random`, or `/dev/hwrng`); `max_bytes` and `period` limit the injection rate. Removing the slot deletes it.",
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"source":    schema.StringAttribute{Optional: true, Computed: true},
			"max_bytes": schema.Int64Attribute{Optional: true, Computed: true},
			"period":    schema.Int64Attribute{Optional: true, Computed: true},
		}},
	}
}

func qemuVMVirtiofsResourceAttribute() schema.MapNestedAttribute {
	return schema.MapNestedAttribute{
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
		MarkdownDescription: "Typed virtiofs directory sharing keyed by `virtiofs0` to `virtiofs9`. `dirid` names a cluster directory mapping and doubles as the guest mount tag. Slots removed from configuration are deleted.",
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"dirid":        schema.StringAttribute{Optional: true, Computed: true},
			"cache":        schema.StringAttribute{Optional: true, Computed: true},
			"direct_io":    schema.BoolAttribute{Optional: true, Computed: true},
			"expose_xattr": schema.BoolAttribute{Optional: true, Computed: true},
			"expose_acl":   schema.BoolAttribute{Optional: true, Computed: true},
		}},
	}
}

func qemuVMHostPciDataSourceAttribute() datasourceschema.MapNestedAttribute {
	return qemuVMHostPciResourceAttributeData()
}

func qemuVMUSBDataSourceAttribute() datasourceschema.MapNestedAttribute {
	return qemuVMUSBResourceAttributeData()
}

func qemuVMRNGDataSourceAttribute() datasourceschema.MapNestedAttribute {
	return qemuVMRNGResourceAttributeData()
}

func qemuVMVirtiofsDataSourceAttribute() datasourceschema.MapNestedAttribute {
	return qemuVMVirtiofsResourceAttributeData()
}

func qemuVMHostPciResourceAttributeData() datasourceschema.MapNestedAttribute {
	return datasourceschema.MapNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Typed host PCI passthrough keyed by slot.",
		NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
			"host":          datasourceschema.StringAttribute{Computed: true},
			"mapping":       datasourceschema.StringAttribute{Computed: true},
			"mdev":          datasourceschema.StringAttribute{Computed: true},
			"pcie":          datasourceschema.BoolAttribute{Computed: true},
			"x_vga":         datasourceschema.BoolAttribute{Computed: true},
			"rombar":        datasourceschema.BoolAttribute{Computed: true},
			"rom_file":      datasourceschema.StringAttribute{Computed: true},
			"legacy_igd":    datasourceschema.BoolAttribute{Computed: true},
			"vendor_id":     datasourceschema.StringAttribute{Computed: true},
			"device_id":     datasourceschema.StringAttribute{Computed: true},
			"sub_vendor_id": datasourceschema.StringAttribute{Computed: true},
			"sub_device_id": datasourceschema.StringAttribute{Computed: true},
			"driver":        datasourceschema.StringAttribute{Computed: true},
		}},
	}
}

func qemuVMUSBResourceAttributeData() datasourceschema.MapNestedAttribute {
	return datasourceschema.MapNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Typed USB passthrough keyed by slot.",
		NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
			"host":    datasourceschema.StringAttribute{Computed: true},
			"mapping": datasourceschema.StringAttribute{Computed: true},
			"usb3":    datasourceschema.BoolAttribute{Computed: true},
		}},
	}
}

func qemuVMRNGResourceAttributeData() datasourceschema.MapNestedAttribute {
	return datasourceschema.MapNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Typed entropy source at `rng0`.",
		NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
			"source":    datasourceschema.StringAttribute{Computed: true},
			"max_bytes": datasourceschema.Int64Attribute{Computed: true},
			"period":    datasourceschema.Int64Attribute{Computed: true},
		}},
	}
}

func qemuVMVirtiofsResourceAttributeData() datasourceschema.MapNestedAttribute {
	return datasourceschema.MapNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Typed virtiofs directory shares keyed by slot.",
		NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
			"dirid":        datasourceschema.StringAttribute{Computed: true},
			"cache":        datasourceschema.StringAttribute{Computed: true},
			"direct_io":    datasourceschema.BoolAttribute{Computed: true},
			"expose_xattr": datasourceschema.BoolAttribute{Computed: true},
			"expose_acl":   datasourceschema.BoolAttribute{Computed: true},
		}},
	}
}
