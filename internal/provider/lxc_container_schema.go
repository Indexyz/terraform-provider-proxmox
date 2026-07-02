// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type lxcContainerModel struct {
	ID           types.String `tfsdk:"id"`
	Node         types.String `tfsdk:"node"`
	VMID         types.Int64  `tfsdk:"vm_id"`
	OSTemplate   types.String `tfsdk:"ostemplate"`
	Hostname     types.String `tfsdk:"hostname"`
	Description  types.String `tfsdk:"description"`
	Tags         types.String `tfsdk:"tags"`
	Arch         types.String `tfsdk:"arch"`
	Cores        types.Int64  `tfsdk:"cores"`
	Memory       types.Int64  `tfsdk:"memory"`
	Swap         types.Int64  `tfsdk:"swap"`
	OnBoot       types.Bool   `tfsdk:"onboot"`
	Protection   types.Bool   `tfsdk:"protection"`
	Startup      types.String `tfsdk:"startup"`
	Unprivileged types.Bool   `tfsdk:"unprivileged"`
	Features     types.String `tfsdk:"features"`
	OSType       types.String `tfsdk:"ostype"`
	RootFS       types.String `tfsdk:"rootfs"`
	Nameserver   types.String `tfsdk:"nameserver"`
	Searchdomain types.String `tfsdk:"searchdomain"`
	Timezone     types.String `tfsdk:"timezone"`
	Network      types.Map    `tfsdk:"network"`
	MountPoint   types.Map    `tfsdk:"mount_point"`
	Raw          types.Object `tfsdk:"raw"`
	Status       types.String `tfsdk:"status"`
	Uptime       types.Int64  `tfsdk:"uptime"`
}

type lxcContainerRawModel struct {
	ExtraConfig types.Map `tfsdk:"extra_config"`
}

func lxcContainerRawAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"extra_config": types.MapType{ElemType: types.StringType},
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
		"memory":       datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Configured memory in MiB from `/config`."},
		"swap":         datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Configured swap in MiB from `/config`."},
		"onboot":       datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the container should start automatically on boot."},
		"protection":   datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Proxmox protection is enabled for this container."},
		"startup":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Startup ordering string from `/config`."},
		"unprivileged": datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the container is unprivileged."},
		"features":     datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Raw LXC features string from `/config`."},
		"ostype":       datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Configured container operating system type from `/config`."},
		"rootfs":       datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Root filesystem configuration from `/config`."},
		"nameserver":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "DNS nameserver configuration from `/config`."},
		"searchdomain": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "DNS search domain configuration from `/config`."},
		"timezone":     datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Timezone configuration from `/config`."},
		"network":      datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Raw LXC network entries keyed by Proxmox slot name such as `net0`."},
		"mount_point":  datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Raw LXC mount-point entries keyed by Proxmox slot name such as `mp0`."},
		"raw":          lxcContainerRawDataSourceAttribute(),
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
		"ostype":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Configured container operating system type managed through `/config`."},
		"rootfs":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Root filesystem configuration. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"nameserver":   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "DNS nameserver configuration managed through `/config`."},
		"searchdomain": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "DNS search domain configuration managed through `/config`."},
		"timezone":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Timezone configuration managed through `/config`."},
		"network":      schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Raw LXC network entries keyed by Proxmox slot name such as `net0`."},
		"mount_point":  schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Raw LXC mount-point entries keyed by Proxmox slot name such as `mp0`."},
		"raw":          lxcContainerRawResourceAttribute(),
		"status":       schema.StringAttribute{Computed: true, MarkdownDescription: "Observed runtime status from `/nodes/{node}/lxc/{vmid}/status/current`. Terraform does not manage power state."},
		"uptime":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Observed container uptime in seconds from `/status/current`."},
	}
}
