// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type lxcContainerModel struct {
	ID           types.String  `tfsdk:"id"`
	Node         types.String  `tfsdk:"node"`
	VMID         types.Int64   `tfsdk:"vm_id"`
	OSTemplate   types.String  `tfsdk:"ostemplate"`
	Hostname     types.String  `tfsdk:"hostname"`
	Description  types.String  `tfsdk:"description"`
	Tags         types.String  `tfsdk:"tags"`
	Arch         types.String  `tfsdk:"arch"`
	Cores        types.Int64   `tfsdk:"cores"`
	CPULimit     types.Float64 `tfsdk:"cpulimit"`
	CPUUnits     types.Int64   `tfsdk:"cpuunits"`
	Memory       types.Int64   `tfsdk:"memory"`
	Swap         types.Int64   `tfsdk:"swap"`
	OnBoot       types.Bool    `tfsdk:"onboot"`
	Protection   types.Bool    `tfsdk:"protection"`
	Startup      types.String  `tfsdk:"startup"`
	Unprivileged types.Bool    `tfsdk:"unprivileged"`
	Features     types.String  `tfsdk:"features"`
	Console      types.Bool    `tfsdk:"console"`
	TTY          types.Int64   `tfsdk:"tty"`
	CMode        types.String  `tfsdk:"cmode"`
	Hookscript   types.String  `tfsdk:"hookscript"`
	OSType       types.String  `tfsdk:"ostype"`
	RootFS       types.String  `tfsdk:"rootfs"`
	Nameserver   types.String  `tfsdk:"nameserver"`
	Searchdomain types.String  `tfsdk:"searchdomain"`
	Timezone     types.String  `tfsdk:"timezone"`
	Network      types.Map     `tfsdk:"network"`
	MountPoint   types.Map     `tfsdk:"mount_point"`
	Raw          types.Object  `tfsdk:"raw"`
	Clone        types.Object  `tfsdk:"clone"`
	Status       types.String  `tfsdk:"status"`
	Uptime       types.Int64   `tfsdk:"uptime"`
}

type lxcContainerRawModel struct {
	ExtraConfig types.Map `tfsdk:"extra_config"`
}

func lxcContainerRawAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"extra_config": types.MapType{ElemType: types.StringType},
	}
}

type lxcContainerNetworkModel struct {
	Name     types.String  `tfsdk:"name"`
	Bridge   types.String  `tfsdk:"bridge"`
	IP       types.String  `tfsdk:"ip"`
	Gateway  types.String  `tfsdk:"gateway"`
	IPv6     types.String  `tfsdk:"ip6"`
	Gateway6 types.String  `tfsdk:"gateway6"`
	HWAddr   types.String  `tfsdk:"hwaddr"`
	Type     types.String  `tfsdk:"type"`
	Tag      types.Int64   `tfsdk:"tag"`
	Trunks   types.String  `tfsdk:"trunks"`
	Rate     types.Float64 `tfsdk:"rate"`
	MTU      types.Int64   `tfsdk:"mtu"`
	Firewall types.Bool    `tfsdk:"firewall"`
	LinkDown types.Bool    `tfsdk:"link_down"`
}

func lxcContainerNetworkAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":      types.StringType,
		"bridge":    types.StringType,
		"ip":        types.StringType,
		"gateway":   types.StringType,
		"ip6":       types.StringType,
		"gateway6":  types.StringType,
		"hwaddr":    types.StringType,
		"type":      types.StringType,
		"tag":       types.Int64Type,
		"trunks":    types.StringType,
		"rate":      types.Float64Type,
		"mtu":       types.Int64Type,
		"firewall":  types.BoolType,
		"link_down": types.BoolType,
	}
}

type lxcContainerMountPointModel struct {
	Volume     types.String `tfsdk:"volume"`
	MountPoint types.String `tfsdk:"mountpoint"`
	Size       types.String `tfsdk:"size"`
	Backup     types.Bool   `tfsdk:"backup"`
	ReadOnly   types.Bool   `tfsdk:"read_only"`
	Quota      types.Bool   `tfsdk:"quota"`
	Replicate  types.Bool   `tfsdk:"replicate"`
	Shared     types.Bool   `tfsdk:"shared"`
	ACL        types.Bool   `tfsdk:"acl"`
}

func lxcContainerMountPointAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"volume":     types.StringType,
		"mountpoint": types.StringType,
		"size":       types.StringType,
		"backup":     types.BoolType,
		"read_only":  types.BoolType,
		"quota":      types.BoolType,
		"replicate":  types.BoolType,
		"shared":     types.BoolType,
		"acl":        types.BoolType,
	}
}

type lxcContainerCloneModel struct {
	SourceNode   types.String `tfsdk:"source_node"`
	SourceVMID   types.Int64  `tfsdk:"source_vmid"`
	Full         types.Bool   `tfsdk:"full"`
	SnapshotName types.String `tfsdk:"snapshot_name"`
	Storage      types.String `tfsdk:"storage"`
	BWLimit      types.Int64  `tfsdk:"bwlimit"`
}

func lxcContainerCloneAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"source_node":   types.StringType,
		"source_vmid":   types.Int64Type,
		"full":          types.BoolType,
		"snapshot_name": types.StringType,
		"storage":       types.StringType,
		"bwlimit":       types.Int64Type,
	}
}

func lxcContainerCloneDataSourceAttribute() datasourceschema.SingleNestedAttribute {
	return datasourceschema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Create-time clone inputs. This provider cannot infer clone provenance for existing containers, so this remains null for imported or data source reads.",
		Attributes: map[string]datasourceschema.Attribute{
			"source_node":   datasourceschema.StringAttribute{Computed: true},
			"source_vmid":   datasourceschema.Int64Attribute{Computed: true},
			"full":          datasourceschema.BoolAttribute{Computed: true},
			"snapshot_name": datasourceschema.StringAttribute{Computed: true},
			"storage":       datasourceschema.StringAttribute{Computed: true},
			"bwlimit":       datasourceschema.Int64Attribute{Computed: true},
		},
	}
}

func lxcContainerCloneResourceAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: "Create-time clone mode. When configured, the provider clones from `source_vmid` instead of using the plain create (ostemplate) path. Changes require replacement. The provider cannot infer clone provenance for imported resources or refreshes without prior state, so this block reads back as null in those cases.",
		PlanModifiers:       []planmodifier.Object{objectplanmodifier.RequiresReplaceIfConfigured()},
		Attributes: map[string]schema.Attribute{
			"source_node":   schema.StringAttribute{Optional: true, MarkdownDescription: "Source node that owns `source_vmid`. Defaults to the managed `node` when omitted."},
			"source_vmid":   schema.Int64Attribute{Required: true, MarkdownDescription: "Source VMID to clone from.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"full":          schema.BoolAttribute{Optional: true, MarkdownDescription: "Whether to request a full clone."},
			"snapshot_name": schema.StringAttribute{Optional: true, MarkdownDescription: "Optional source snapshot name to clone from."},
			"storage":       schema.StringAttribute{Optional: true, MarkdownDescription: "Optional target storage override for full clones."},
			"bwlimit":       schema.Int64Attribute{Optional: true, MarkdownDescription: "Optional clone bandwidth limit in KiB/s."},
		},
	}
}

func lxcContainerRawDataSourceAttribute() datasourceschema.SingleNestedAttribute {
	return datasourceschema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Escape hatch for LXC `/config` keys that this provider version does not type yet.",
		Attributes: map[string]datasourceschema.Attribute{
			"extra_config": datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Raw Proxmox LXC config entries keyed by their exact `/config` key."},
		},
	}
}

func lxcContainerRawResourceAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		Computed:            true,
		MarkdownDescription: "Escape hatch for LXC `/config` keys that this provider version does not type yet.",
		Attributes: map[string]schema.Attribute{
			"extra_config": schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Raw Proxmox LXC config entries keyed by their exact `/config` key."},
		},
	}
}

func lxcContainerDataSourceAttributes() map[string]datasourceschema.Attribute {
	return map[string]datasourceschema.Attribute{
		"id":           datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Terraform identifier in `node/vm_id` form."},
		"node":         datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Proxmox node that owns the LXC container."},
		"vm_id":        datasourceschema.Int64Attribute{Required: true, MarkdownDescription: "Numeric VMID of the LXC container."},
		"ostemplate":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Create-time OS template. Proxmox does not report this from `/config`, so data source reads leave it null."},
		"hostname":     datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Container hostname from `/config`."},
		"description":  datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Optional container description from `/config`."},
		"tags":         datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Comma-separated Proxmox tags from `/config`."},
		"arch":         datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Container architecture from `/config`."},
		"cores":        datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Configured vCPU cores from `/config`."},
		"cpulimit":     datasourceschema.Float64Attribute{Computed: true, MarkdownDescription: "CPU usage limit from `/config`. Value 0 indicates no limit."},
		"cpuunits":     datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "CPU weight for this container from `/config`."},
		"memory":       datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Configured memory in MiB from `/config`."},
		"swap":         datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Configured swap in MiB from `/config`."},
		"onboot":       datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the container should start automatically on boot."},
		"protection":   datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Proxmox protection is enabled for this container."},
		"startup":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Startup ordering string from `/config`."},
		"unprivileged": datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the container is unprivileged."},
		"features":     datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Raw LXC features string from `/config`."},
		"console":      datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether a console device is attached to the container from `/config`."},
		"tty":          datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Number of TTYs available to the container from `/config`."},
		"cmode":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Console mode (`shell` or `console`) from `/config`."},
		"hookscript":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Hookscript path from `/config`."},
		"ostype":       datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Configured container operating system type from `/config`."},
		"rootfs":       datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Root filesystem configuration from `/config`."},
		"nameserver":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "DNS nameserver configuration from `/config`."},
		"searchdomain": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "DNS search domain configuration from `/config`."},
		"timezone":     datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Timezone configuration from `/config`."},
		"network":      datasourceschema.MapAttribute{Computed: true, ElementType: types.ObjectType{AttrTypes: lxcContainerNetworkAttrTypes()}, MarkdownDescription: "Typed LXC network devices keyed by Proxmox slot name such as `net0`. Unsupported grammar remains available through `raw.extra_config[\"netN\"]`."},
		"mount_point":  datasourceschema.MapAttribute{Computed: true, ElementType: types.ObjectType{AttrTypes: lxcContainerMountPointAttrTypes()}, MarkdownDescription: "Typed LXC mount points keyed by Proxmox slot name such as `mp0`. Unsupported grammar remains available through `raw.extra_config[\"mpN\"]`."},
		"raw":          lxcContainerRawDataSourceAttribute(),
		"clone":        lxcContainerCloneDataSourceAttribute(),
		"status":       datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Observed runtime status from `/nodes/{node}/lxc/{vmid}/status/current`."},
		"uptime":       datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Observed container uptime in seconds from `/status/current`."},
	}
}

func lxcContainerResourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Terraform identifier in `node/vm_id` form.",
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"node": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Proxmox node that owns the LXC container.",
			PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"vm_id": schema.Int64Attribute{
			Required:            true,
			MarkdownDescription: "Numeric VMID of the LXC container.",
			PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
		},
		"ostemplate":  schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Create-time OS template such as `local:vztmpl/debian-12-standard.tar.zst`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"hostname":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Container hostname managed through `/config`."},
		"description": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Optional container description managed through `/config`."},
		"tags":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Comma-separated Proxmox tags managed through `/config`."},
		"arch":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Container architecture. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"cores":       schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Configured vCPU cores managed through `/config`."},
		"cpulimit":    schema.Float64Attribute{Optional: true, Computed: true, MarkdownDescription: "CPU usage limit managed through `/config`. Value 0 indicates no limit."},
		"cpuunits":    schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "CPU weight for this container managed through `/config`."},
		"memory":      schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Configured memory in MiB managed through `/config`."},
		"swap":        schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Configured swap in MiB managed through `/config`."},
		"onboot":      schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether the container should start automatically on boot."},
		"protection":  schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether Proxmox protection is enabled for this container."},
		"startup":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Startup ordering string managed through `/config`."},
		"unprivileged": schema.BoolAttribute{
			Optional:            true,
			Computed:            true,
			MarkdownDescription: "Whether the container is unprivileged. Changes require replacement.",
			PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
		},
		"features":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Raw LXC features string managed through `/config`."},
		"console":      schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether a console device is attached to the container, managed through `/config`."},
		"tty":          schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Number of TTYs available to the container, managed through `/config`."},
		"cmode":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Console mode (`shell` or `console`) managed through `/config`."},
		"hookscript":   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Hookscript path managed through `/config`."},
		"ostype":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Configured container operating system type managed through `/config`."},
		"rootfs":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Root filesystem configuration. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"nameserver":   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "DNS nameserver configuration managed through `/config`."},
		"searchdomain": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "DNS search domain configuration managed through `/config`."},
		"timezone":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Timezone configuration managed through `/config`."},
		"network":      schema.MapAttribute{Optional: true, Computed: true, ElementType: types.ObjectType{AttrTypes: lxcContainerNetworkAttrTypes()}, MarkdownDescription: "Typed LXC network devices keyed by Proxmox slot name such as `net0`. Unsupported grammar remains available through `raw.extra_config[\"netN\"]`."},
		"mount_point":  schema.MapAttribute{Optional: true, Computed: true, ElementType: types.ObjectType{AttrTypes: lxcContainerMountPointAttrTypes()}, MarkdownDescription: "Typed LXC mount points keyed by Proxmox slot name such as `mp0`. Unsupported grammar remains available through `raw.extra_config[\"mpN\"]`."},
		"raw":          lxcContainerRawResourceAttribute(),
		"clone":        lxcContainerCloneResourceAttribute(),
		"status":       schema.StringAttribute{Computed: true, MarkdownDescription: "Observed runtime status from `/nodes/{node}/lxc/{vmid}/status/current`. Terraform does not manage power state."},
		"uptime":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Observed container uptime in seconds from `/status/current`."},
	}
}
