// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type qemuVMModel struct {
	ID          types.String `tfsdk:"id"`
	Node        types.String `tfsdk:"node"`
	VMID        types.Int64  `tfsdk:"vm_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Tags        types.String `tfsdk:"tags"`
	Template    types.Bool   `tfsdk:"template"`
	Pool        types.String `tfsdk:"pool"`
	OnBoot      types.Bool   `tfsdk:"onboot"`
	Startup     types.String `tfsdk:"startup"`
	Bios        types.String `tfsdk:"bios"`
	Machine     types.String `tfsdk:"machine"`
	Agent       types.String `tfsdk:"agent"`
	Cores       types.Int64  `tfsdk:"cores"`
	Sockets     types.Int64  `tfsdk:"sockets"`
	Memory      types.Int64  `tfsdk:"memory"`
	CPU         types.String `tfsdk:"cpu"`
	OSType      types.String `tfsdk:"ostype"`
	Boot        types.String `tfsdk:"boot"`
	Status      types.String `tfsdk:"status"`
	Uptime      types.Int64  `tfsdk:"uptime"`
}

func qemuVMDataSourceAttributes() map[string]datasourceschema.Attribute {
	return map[string]datasourceschema.Attribute{
		"id": datasourceschema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Terraform identifier in `node/vm_id` form.",
		},
		"node": datasourceschema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Proxmox node that owns the QEMU virtual machine.",
		},
		"vm_id": datasourceschema.Int64Attribute{
			Required:            true,
			MarkdownDescription: "Numeric VMID of the QEMU virtual machine.",
		},
		"name":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Virtual machine name from `/nodes/{node}/qemu/{vmid}/config`."},
		"description": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Optional VM description from `/config`."},
		"tags":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Comma-separated Proxmox tags from `/config`."},
		"template":    datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the guest is a template, as reported by `/config`."},
		"pool":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Pool assignment from `/config`."},
		"onboot":      datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the guest should start automatically on boot."},
		"startup":     datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Startup ordering string from `/config`."},
		"bios":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Configured BIOS type from `/config`."},
		"machine":     datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Configured machine type from `/config`."},
		"agent":       datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Raw QEMU guest agent configuration string from `/config`."},
		"cores":       datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Configured vCPU cores from `/config`."},
		"sockets":     datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Configured CPU sockets from `/config`."},
		"memory":      datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Configured memory in MiB from `/config`."},
		"cpu":         datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Configured CPU model from `/config`."},
		"ostype":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Configured guest operating system type from `/config`."},
		"boot":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Boot order string from `/config`."},
		"status":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Observed runtime status from `/nodes/{node}/qemu/{vmid}/status/current`."},
		"uptime":      datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Observed guest uptime in seconds from `/status/current`."},
	}
}

func qemuVMResourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Terraform identifier in `node/vm_id` form.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"node": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Proxmox node that owns the QEMU virtual machine.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"vm_id": schema.Int64Attribute{
			Required:            true,
			MarkdownDescription: "Numeric VMID of the QEMU virtual machine.",
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.RequiresReplace(),
			},
		},
		"name":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Virtual machine name managed through `/nodes/{node}/qemu` and `/config`."},
		"description": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Optional VM description managed through `/config`."},
		"tags":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Comma-separated Proxmox tags managed through `/config`."},
		"template":    schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the guest is a template, as observed from `/config`. Terraform does not manage template conversion."},
		"pool":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Pool assignment managed through `/config`."},
		"onboot":      schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether the guest should start automatically on boot."},
		"startup":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Startup ordering string managed through `/config`."},
		"bios":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Configured BIOS type managed through `/config`."},
		"machine":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Configured machine type managed through `/config`."},
		"agent":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Raw QEMU guest agent configuration string managed through `/config`."},
		"cores":       schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Configured vCPU cores managed through `/config`."},
		"sockets":     schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Configured CPU sockets managed through `/config`."},
		"memory":      schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Configured memory in MiB managed through `/config`."},
		"cpu":         schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Configured CPU model managed through `/config`."},
		"ostype":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Configured guest operating system type managed through `/config`."},
		"boot":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Boot order string managed through `/config`."},
		"status":      schema.StringAttribute{Computed: true, MarkdownDescription: "Observed runtime status from `/nodes/{node}/qemu/{vmid}/status/current`. Terraform does not manage power state."},
		"uptime":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Observed guest uptime in seconds from `/status/current`."},
	}
}
