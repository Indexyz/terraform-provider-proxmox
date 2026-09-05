// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type qemuVMModel struct {
	ID          types.String  `tfsdk:"id"`
	Node        types.String  `tfsdk:"node"`
	VMID        types.Int64   `tfsdk:"vm_id"`
	VMIDStart   types.Int64   `tfsdk:"vm_id_start"`
	Name        types.String  `tfsdk:"name"`
	Description types.String  `tfsdk:"description"`
	Tags        types.String  `tfsdk:"tags"`
	Template    types.Bool    `tfsdk:"template"`
	Pool        types.String  `tfsdk:"pool"`
	OnBoot      types.Bool    `tfsdk:"onboot"`
	Protection  types.Bool    `tfsdk:"protection"`
	SCSIHW      types.String  `tfsdk:"scsihw"`
	Tablet      types.Bool    `tfsdk:"tablet"`
	Startup     types.String  `tfsdk:"startup"`
	Bios        types.String  `tfsdk:"bios"`
	Machine     types.String  `tfsdk:"machine"`
	Agent       types.String  `tfsdk:"agent"`
	Cores       types.Int64   `tfsdk:"cores"`
	Sockets     types.Int64   `tfsdk:"sockets"`
	Memory      types.Int64   `tfsdk:"memory"`
	NUMA        types.Bool    `tfsdk:"numa"`
	VCPUs       types.Int64   `tfsdk:"vcpus"`
	CPUUnits    types.Int64   `tfsdk:"cpuunits"`
	CPULimit    types.Float64 `tfsdk:"cpulimit"`
	Balloon     types.Int64   `tfsdk:"balloon"`
	Shares      types.Int64   `tfsdk:"shares"`
	Hugepages   types.String  `tfsdk:"hugepages"`
	CPU         types.String  `tfsdk:"cpu"`
	OSType      types.String  `tfsdk:"ostype"`
	Boot        types.String  `tfsdk:"boot"`
	Common      types.Object  `tfsdk:"common"`
	CloudInit   types.Object  `tfsdk:"cloud_init"`
	Network     types.Map     `tfsdk:"network"`
	Disk        types.Map     `tfsdk:"disk"`
	Serial      types.Map     `tfsdk:"serial"`
	EFIDisk     types.Object  `tfsdk:"efi_disk"`
	TPMState    types.Object  `tfsdk:"tpm_state"`
	VGA         types.Object  `tfsdk:"vga"`
	Raw         types.Object  `tfsdk:"raw"`
	Clone       types.Object  `tfsdk:"clone"`
	Status      types.String  `tfsdk:"status"`
	Uptime      types.Int64   `tfsdk:"uptime"`
}

type qemuVMCommonModel struct {
	Hotplug types.String `tfsdk:"hotplug"`
}

type qemuVMIPConfigModel struct {
	IPv4     types.String `tfsdk:"ipv4"`
	Gateway  types.String `tfsdk:"gateway"`
	IPv6     types.String `tfsdk:"ipv6"`
	Gateway6 types.String `tfsdk:"gateway6"`
}

type qemuVMCloudInitModel struct {
	CICustom   types.String `tfsdk:"cicustom"`
	CIPassword types.String `tfsdk:"cipassword"`
	CIType     types.String `tfsdk:"citype"`
	CIUpgrade  types.Bool   `tfsdk:"ciupgrade"`
	CIUser     types.String `tfsdk:"ciuser"`
	IPConfig   types.Map    `tfsdk:"ipconfig"`
	SSHKeys    types.String `tfsdk:"sshkeys"`
}

type qemuVMNetworkModel struct {
	Model    types.String  `tfsdk:"model"`
	Bridge   types.String  `tfsdk:"bridge"`
	MACAddr  types.String  `tfsdk:"macaddr"`
	Tag      types.Int64   `tfsdk:"tag"`
	Trunks   types.String  `tfsdk:"trunks"`
	Firewall types.Bool    `tfsdk:"firewall"`
	LinkDown types.Bool    `tfsdk:"link_down"`
	MTU      types.Int64   `tfsdk:"mtu"`
	Queues   types.Int64   `tfsdk:"queues"`
	Rate     types.Float64 `tfsdk:"rate"`
}

type qemuVMDiskModel struct {
	Storage   types.String  `tfsdk:"storage"`
	Volume    types.String  `tfsdk:"volume"`
	Size      types.String  `tfsdk:"size"`
	Media     types.String  `tfsdk:"media"`
	Cache     types.String  `tfsdk:"cache"`
	Discard   types.String  `tfsdk:"discard"`
	Iothread  types.Bool    `tfsdk:"iothread"`
	SSD       types.Bool    `tfsdk:"ssd"`
	Replicate types.Bool    `tfsdk:"replicate"`
	Backup    types.Bool    `tfsdk:"backup"`
	Shared    types.Bool    `tfsdk:"shared"`
	Snapshot  types.Bool    `tfsdk:"snapshot"`
	Serial    types.String  `tfsdk:"serial"`
	IOPS      types.Int64   `tfsdk:"iops"`
	IOPSMax   types.Int64   `tfsdk:"iops_max"`
	IOPSRd    types.Int64   `tfsdk:"iops_rd"`
	IOPSRdMax types.Int64   `tfsdk:"iops_rd_max"`
	IOPSWr    types.Int64   `tfsdk:"iops_wr"`
	IOPSWrMax types.Int64   `tfsdk:"iops_wr_max"`
	MBPS      types.Float64 `tfsdk:"mbps"`
	MBPSMax   types.Float64 `tfsdk:"mbps_max"`
	MBPSRd    types.Float64 `tfsdk:"mbps_rd"`
	MBPSRdMax types.Float64 `tfsdk:"mbps_rd_max"`
	MBPSWr    types.Float64 `tfsdk:"mbps_wr"`
	MBPSWrMax types.Float64 `tfsdk:"mbps_wr_max"`
}

type qemuVMEFIDiskModel struct {
	Storage         types.String `tfsdk:"storage"`
	Volume          types.String `tfsdk:"volume"`
	Size            types.String `tfsdk:"size"`
	EFIType         types.String `tfsdk:"efitype"`
	Format          types.String `tfsdk:"format"`
	MSCert          types.String `tfsdk:"ms_cert"`
	PreEnrolledKeys types.Bool   `tfsdk:"pre_enrolled_keys"`
}

type qemuVMTPMStateModel struct {
	Storage types.String `tfsdk:"storage"`
	Volume  types.String `tfsdk:"volume"`
	Size    types.String `tfsdk:"size"`
	Format  types.String `tfsdk:"format"`
	Version types.String `tfsdk:"version"`
}

type qemuVMVGAModel struct {
	Type      types.String `tfsdk:"type"`
	Memory    types.Int64  `tfsdk:"memory"`
	Clipboard types.String `tfsdk:"clipboard"`
}

type qemuVMRawModel struct {
	ExtraConfig types.Map `tfsdk:"extra_config"`
}

type qemuVMCloneModel struct {
	SourceNode   types.String `tfsdk:"source_node"`
	SourceVMID   types.Int64  `tfsdk:"source_vmid"`
	Full         types.Bool   `tfsdk:"full"`
	SnapshotName types.String `tfsdk:"snapshot_name"`
	Storage      types.String `tfsdk:"storage"`
	Format       types.String `tfsdk:"format"`
	BWLimit      types.Int64  `tfsdk:"bwlimit"`
}

func qemuVMCommonAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"hotplug": types.StringType,
	}
}

func qemuVMIPConfigAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"ipv4":     types.StringType,
		"gateway":  types.StringType,
		"ipv6":     types.StringType,
		"gateway6": types.StringType,
	}
}

func qemuVMCloudInitAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"cicustom":   types.StringType,
		"cipassword": types.StringType,
		"citype":     types.StringType,
		"ciupgrade":  types.BoolType,
		"ciuser":     types.StringType,
		"ipconfig":   types.MapType{ElemType: types.ObjectType{AttrTypes: qemuVMIPConfigAttrTypes()}},
		"sshkeys":    types.StringType,
	}
}

func qemuVMNetworkAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"model":     types.StringType,
		"bridge":    types.StringType,
		"macaddr":   types.StringType,
		"tag":       types.Int64Type,
		"trunks":    types.StringType,
		"firewall":  types.BoolType,
		"link_down": types.BoolType,
		"mtu":       types.Int64Type,
		"queues":    types.Int64Type,
		"rate":      types.Float64Type,
	}
}

func qemuVMDiskAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"storage":     types.StringType,
		"volume":      types.StringType,
		"size":        types.StringType,
		"media":       types.StringType,
		"cache":       types.StringType,
		"discard":     types.StringType,
		"iothread":    types.BoolType,
		"ssd":         types.BoolType,
		"replicate":   types.BoolType,
		"backup":      types.BoolType,
		"shared":      types.BoolType,
		"snapshot":    types.BoolType,
		"serial":      types.StringType,
		"iops":        types.Int64Type,
		"iops_max":    types.Int64Type,
		"iops_rd":     types.Int64Type,
		"iops_rd_max": types.Int64Type,
		"iops_wr":     types.Int64Type,
		"iops_wr_max": types.Int64Type,
		"mbps":        types.Float64Type,
		"mbps_max":    types.Float64Type,
		"mbps_rd":     types.Float64Type,
		"mbps_rd_max": types.Float64Type,
		"mbps_wr":     types.Float64Type,
		"mbps_wr_max": types.Float64Type,
	}
}

func qemuVMEFIDiskAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"storage":           types.StringType,
		"volume":            types.StringType,
		"size":              types.StringType,
		"efitype":           types.StringType,
		"format":            types.StringType,
		"ms_cert":           types.StringType,
		"pre_enrolled_keys": types.BoolType,
	}
}

func qemuVMTPMStateAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"storage": types.StringType,
		"volume":  types.StringType,
		"size":    types.StringType,
		"format":  types.StringType,
		"version": types.StringType,
	}
}

func qemuVMVGAAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"type":      types.StringType,
		"memory":    types.Int64Type,
		"clipboard": types.StringType,
	}
}

func qemuVMRawAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"extra_config": types.MapType{ElemType: types.StringType},
	}
}

func qemuVMCloneAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"source_node":   types.StringType,
		"source_vmid":   types.Int64Type,
		"full":          types.BoolType,
		"snapshot_name": types.StringType,
		"storage":       types.StringType,
		"format":        types.StringType,
		"bwlimit":       types.Int64Type,
	}
}

func qemuVMCommonDataSourceAttribute() datasourceschema.SingleNestedAttribute {
	return datasourceschema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Common advanced QEMU configuration surfaced from `/config`.",
		Attributes: map[string]datasourceschema.Attribute{
			"hotplug": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Hotplug feature set from `/config`."},
		},
	}
}

func qemuVMCommonResourceAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		Computed:            true,
		MarkdownDescription: "Common advanced QEMU configuration managed through `/config`.",
		Attributes: map[string]schema.Attribute{
			"hotplug": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Hotplug feature set managed through `/config`."},
		},
	}
}

func qemuVMCloudInitDataSourceAttribute() datasourceschema.SingleNestedAttribute {
	return datasourceschema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Typed cloud-init configuration surfaced from `/config` when supported by this provider version.",
		Attributes: map[string]datasourceschema.Attribute{
			"cicustom":   datasourceschema.StringAttribute{Computed: true},
			"cipassword": datasourceschema.StringAttribute{Computed: true, Sensitive: true},
			"citype":     datasourceschema.StringAttribute{Computed: true},
			"ciupgrade":  datasourceschema.BoolAttribute{Computed: true},
			"ciuser":     datasourceschema.StringAttribute{Computed: true},
			"ipconfig": datasourceschema.MapNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Cloud-init interface IP configuration keyed by Proxmox slot name such as `ipconfig0`.",
				NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
					"ipv4":     datasourceschema.StringAttribute{Computed: true},
					"gateway":  datasourceschema.StringAttribute{Computed: true},
					"ipv6":     datasourceschema.StringAttribute{Computed: true},
					"gateway6": datasourceschema.StringAttribute{Computed: true},
				}},
			},
			"sshkeys": datasourceschema.StringAttribute{Computed: true},
		},
	}
}

func qemuVMCloudInitResourceAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		Computed:            true,
		MarkdownDescription: "Typed cloud-init configuration managed through `/config`.",
		Attributes: map[string]schema.Attribute{
			"cicustom":   schema.StringAttribute{Optional: true, Computed: true},
			"cipassword": schema.StringAttribute{Optional: true, Computed: true, Sensitive: true},
			"citype":     schema.StringAttribute{Optional: true, Computed: true},
			"ciupgrade":  schema.BoolAttribute{Optional: true, Computed: true},
			"ciuser":     schema.StringAttribute{Optional: true, Computed: true},
			"ipconfig": schema.MapNestedAttribute{
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "Cloud-init interface IP configuration keyed by Proxmox slot name such as `ipconfig0`.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"ipv4":     schema.StringAttribute{Optional: true, Computed: true},
					"gateway":  schema.StringAttribute{Optional: true, Computed: true},
					"ipv6":     schema.StringAttribute{Optional: true, Computed: true},
					"gateway6": schema.StringAttribute{Optional: true, Computed: true},
				}},
			},
			"sshkeys": schema.StringAttribute{Optional: true, Computed: true},
		},
	}
}

func qemuVMNetworkDataSourceAttribute() datasourceschema.MapNestedAttribute {
	return datasourceschema.MapNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Typed network devices keyed by Proxmox slot name such as `net0`.",
		NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
			"model":     datasourceschema.StringAttribute{Computed: true},
			"bridge":    datasourceschema.StringAttribute{Computed: true},
			"macaddr":   datasourceschema.StringAttribute{Computed: true},
			"tag":       datasourceschema.Int64Attribute{Computed: true},
			"trunks":    datasourceschema.StringAttribute{Computed: true},
			"firewall":  datasourceschema.BoolAttribute{Computed: true},
			"link_down": datasourceschema.BoolAttribute{Computed: true},
			"mtu":       datasourceschema.Int64Attribute{Computed: true},
			"queues":    datasourceschema.Int64Attribute{Computed: true},
			"rate":      datasourceschema.Float64Attribute{Computed: true},
		}},
	}
}

func qemuVMNetworkResourceAttribute() schema.MapNestedAttribute {
	return schema.MapNestedAttribute{
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
		MarkdownDescription: "Typed network devices keyed by Proxmox slot name such as `net0`.",
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"model":     schema.StringAttribute{Optional: true, Computed: true},
			"bridge":    schema.StringAttribute{Optional: true, Computed: true},
			"macaddr":   schema.StringAttribute{Optional: true, Computed: true},
			"tag":       schema.Int64Attribute{Optional: true, Computed: true},
			"trunks":    schema.StringAttribute{Optional: true, Computed: true},
			"firewall":  schema.BoolAttribute{Optional: true, Computed: true},
			"link_down": schema.BoolAttribute{Optional: true, Computed: true},
			"mtu":       schema.Int64Attribute{Optional: true, Computed: true},
			"queues":    schema.Int64Attribute{Optional: true, Computed: true},
			"rate":      schema.Float64Attribute{Optional: true, Computed: true},
		}},
	}
}

func qemuVMDiskDataSourceAttribute() datasourceschema.MapNestedAttribute {
	return datasourceschema.MapNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Typed disk devices keyed by Proxmox slot name such as `scsi0` or `virtio0` when fully covered by this provider version.",
		NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
			"storage":     datasourceschema.StringAttribute{Computed: true},
			"volume":      datasourceschema.StringAttribute{Computed: true},
			"size":        datasourceschema.StringAttribute{Computed: true},
			"media":       datasourceschema.StringAttribute{Computed: true},
			"cache":       datasourceschema.StringAttribute{Computed: true},
			"discard":     datasourceschema.StringAttribute{Computed: true},
			"iothread":    datasourceschema.BoolAttribute{Computed: true},
			"ssd":         datasourceschema.BoolAttribute{Computed: true},
			"replicate":   datasourceschema.BoolAttribute{Computed: true},
			"backup":      datasourceschema.BoolAttribute{Computed: true},
			"shared":      datasourceschema.BoolAttribute{Computed: true},
			"snapshot":    datasourceschema.BoolAttribute{Computed: true},
			"serial":      datasourceschema.StringAttribute{Computed: true},
			"iops":        datasourceschema.Int64Attribute{Computed: true},
			"iops_max":    datasourceschema.Int64Attribute{Computed: true},
			"iops_rd":     datasourceschema.Int64Attribute{Computed: true},
			"iops_rd_max": datasourceschema.Int64Attribute{Computed: true},
			"iops_wr":     datasourceschema.Int64Attribute{Computed: true},
			"iops_wr_max": datasourceschema.Int64Attribute{Computed: true},
			"mbps":        datasourceschema.Float64Attribute{Computed: true},
			"mbps_max":    datasourceschema.Float64Attribute{Computed: true},
			"mbps_rd":     datasourceschema.Float64Attribute{Computed: true},
			"mbps_rd_max": datasourceschema.Float64Attribute{Computed: true},
			"mbps_wr":     datasourceschema.Float64Attribute{Computed: true},
			"mbps_wr_max": datasourceschema.Float64Attribute{Computed: true},
		}},
	}
}

func qemuVMDiskResourceAttribute() schema.MapNestedAttribute {
	return schema.MapNestedAttribute{
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
		MarkdownDescription: "Typed disk devices keyed by Proxmox slot name such as `scsi0` or `virtio0`.",
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"storage":     schema.StringAttribute{Optional: true, Computed: true},
			"volume":      schema.StringAttribute{Optional: true, Computed: true},
			"size":        schema.StringAttribute{Optional: true, Computed: true},
			"media":       schema.StringAttribute{Optional: true, Computed: true},
			"cache":       schema.StringAttribute{Optional: true, Computed: true},
			"discard":     schema.StringAttribute{Optional: true, Computed: true},
			"iothread":    schema.BoolAttribute{Optional: true, Computed: true},
			"ssd":         schema.BoolAttribute{Optional: true, Computed: true},
			"replicate":   schema.BoolAttribute{Optional: true, Computed: true},
			"backup":      schema.BoolAttribute{Optional: true, Computed: true},
			"shared":      schema.BoolAttribute{Optional: true, Computed: true},
			"snapshot":    schema.BoolAttribute{Optional: true, Computed: true},
			"serial":      schema.StringAttribute{Optional: true, Computed: true},
			"iops":        schema.Int64Attribute{Optional: true, Computed: true},
			"iops_max":    schema.Int64Attribute{Optional: true, Computed: true},
			"iops_rd":     schema.Int64Attribute{Optional: true, Computed: true},
			"iops_rd_max": schema.Int64Attribute{Optional: true, Computed: true},
			"iops_wr":     schema.Int64Attribute{Optional: true, Computed: true},
			"iops_wr_max": schema.Int64Attribute{Optional: true, Computed: true},
			"mbps":        schema.Float64Attribute{Optional: true, Computed: true},
			"mbps_max":    schema.Float64Attribute{Optional: true, Computed: true},
			"mbps_rd":     schema.Float64Attribute{Optional: true, Computed: true},
			"mbps_rd_max": schema.Float64Attribute{Optional: true, Computed: true},
			"mbps_wr":     schema.Float64Attribute{Optional: true, Computed: true},
			"mbps_wr_max": schema.Float64Attribute{Optional: true, Computed: true},
		}},
	}
}

func qemuVMEFIDiskDataSourceAttribute() datasourceschema.SingleNestedAttribute {
	return datasourceschema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Typed `efidisk0` firmware storage when the provider fully understands the current grammar; unsupported variants remain in `raw.extra_config`.",
		Attributes: map[string]datasourceschema.Attribute{
			"storage":           datasourceschema.StringAttribute{Computed: true},
			"volume":            datasourceschema.StringAttribute{Computed: true},
			"size":              datasourceschema.StringAttribute{Computed: true},
			"efitype":           datasourceschema.StringAttribute{Computed: true},
			"format":            datasourceschema.StringAttribute{Computed: true},
			"ms_cert":           datasourceschema.StringAttribute{Computed: true},
			"pre_enrolled_keys": datasourceschema.BoolAttribute{Computed: true},
		},
	}
}

func qemuVMEFIDiskResourceAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		Computed:            true,
		MarkdownDescription: "Typed `efidisk0` firmware storage. Unsupported grammar remains available through `raw.extra_config[\"efidisk0\"]`.",
		Attributes: map[string]schema.Attribute{
			"storage":           schema.StringAttribute{Optional: true, Computed: true},
			"volume":            schema.StringAttribute{Optional: true, Computed: true},
			"size":              schema.StringAttribute{Optional: true, Computed: true},
			"efitype":           schema.StringAttribute{Optional: true, Computed: true},
			"format":            schema.StringAttribute{Optional: true, Computed: true},
			"ms_cert":           schema.StringAttribute{Optional: true, Computed: true},
			"pre_enrolled_keys": schema.BoolAttribute{Optional: true, Computed: true},
		},
	}
}

func qemuVMTPMStateDataSourceAttribute() datasourceschema.SingleNestedAttribute {
	return datasourceschema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Typed `tpmstate0` storage when the provider fully understands the current grammar; unsupported variants remain in `raw.extra_config`.",
		Attributes: map[string]datasourceschema.Attribute{
			"storage": datasourceschema.StringAttribute{Computed: true},
			"volume":  datasourceschema.StringAttribute{Computed: true},
			"size":    datasourceschema.StringAttribute{Computed: true},
			"format":  datasourceschema.StringAttribute{Computed: true},
			"version": datasourceschema.StringAttribute{Computed: true},
		},
	}
}

func qemuVMTPMStateResourceAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		Computed:            true,
		MarkdownDescription: "Typed `tpmstate0` storage. Unsupported grammar remains available through `raw.extra_config[\"tpmstate0\"]`.",
		Attributes: map[string]schema.Attribute{
			"storage": schema.StringAttribute{Optional: true, Computed: true},
			"volume":  schema.StringAttribute{Optional: true, Computed: true},
			"size":    schema.StringAttribute{Optional: true, Computed: true},
			"format":  schema.StringAttribute{Optional: true, Computed: true},
			"version": schema.StringAttribute{Optional: true, Computed: true},
		},
	}
}

func qemuVMVGADataSourceAttribute() datasourceschema.SingleNestedAttribute {
	return datasourceschema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Typed VGA hardware configuration from `/config`. Unsupported grammar remains available through `raw.extra_config[\"vga\"]`.",
		Attributes: map[string]datasourceschema.Attribute{
			"type":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "VGA hardware type such as `std`, `virtio`, `qxl`, or `serial0` from `/config`."},
			"memory":    datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "VGA memory in MiB from `/config`."},
			"clipboard": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Clipboard selection such as `vnc` from `/config`."},
		},
	}
}

func qemuVMVGAResourceAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		Computed:            true,
		MarkdownDescription: "Typed VGA hardware configuration managed through `/config`. Unsupported grammar remains available through `raw.extra_config[\"vga\"]`.",
		Attributes: map[string]schema.Attribute{
			"type":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "VGA hardware type such as `std`, `virtio`, `qxl`, or `serial0`. The primary positional part of the Proxmox `vga` value; the block is emitted only when `type` is set."},
			"memory":    schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "VGA memory in MiB managed through `/config`."},
			"clipboard": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Clipboard selection such as `vnc` managed through `/config`."},
		},
	}
}

func qemuVMRawDataSourceAttribute() datasourceschema.SingleNestedAttribute {
	return datasourceschema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Escape hatch for advanced `/config` keys that this provider version does not type yet.",
		Attributes: map[string]datasourceschema.Attribute{
			"extra_config": datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Raw Proxmox config entries keyed by their exact `/config` key such as `hostpci0` or unsupported disk/network slots."},
		},
	}
}

func qemuVMRawResourceAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		Computed:            true,
		MarkdownDescription: "Escape hatch for advanced `/config` keys that this provider version does not type yet.",
		Attributes: map[string]schema.Attribute{
			"extra_config": schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Raw Proxmox config entries keyed by their exact `/config` key such as `hostpci0` or unsupported disk/network slots."},
		},
	}
}

func qemuVMCloneDataSourceAttribute() datasourceschema.SingleNestedAttribute {
	return datasourceschema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Create-time clone inputs. This provider cannot infer clone provenance for existing VMs, so this remains null for imported or data source reads.",
		Attributes: map[string]datasourceschema.Attribute{
			"source_node":   datasourceschema.StringAttribute{Computed: true},
			"source_vmid":   datasourceschema.Int64Attribute{Computed: true},
			"full":          datasourceschema.BoolAttribute{Computed: true},
			"snapshot_name": datasourceschema.StringAttribute{Computed: true},
			"storage":       datasourceschema.StringAttribute{Computed: true},
			"format":        datasourceschema.StringAttribute{Computed: true},
			"bwlimit":       datasourceschema.Int64Attribute{Computed: true},
		},
	}
}

func qemuVMCloneResourceAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: "Create-time clone mode. When configured, the provider clones from `source_vmid` instead of using the plain create path. Changes require replacement. The provider cannot infer clone provenance for imported resources or refreshes without prior state, so this block reads back as null in those cases.",
		PlanModifiers:       []planmodifier.Object{objectplanmodifier.RequiresReplaceIfConfigured()},
		Attributes: map[string]schema.Attribute{
			"source_node":   schema.StringAttribute{Optional: true, MarkdownDescription: "Source node that owns `source_vmid`. Defaults to the managed `node` when omitted."},
			"source_vmid":   schema.Int64Attribute{Required: true, MarkdownDescription: "Source VMID to clone from.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"full":          schema.BoolAttribute{Optional: true, MarkdownDescription: "Whether to request a full clone."},
			"snapshot_name": schema.StringAttribute{Optional: true, MarkdownDescription: "Optional source snapshot name to clone from."},
			"storage":       schema.StringAttribute{Optional: true, MarkdownDescription: "Optional target storage override for full clones."},
			"format":        schema.StringAttribute{Optional: true, MarkdownDescription: "Optional target disk format for full clones."},
			"bwlimit":       schema.Int64Attribute{Optional: true, MarkdownDescription: "Optional clone bandwidth limit in KiB/s."},
		},
	}
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
		"vm_id_start": datasourceschema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Create-time allocation floor for `vm_id` in the `proxmox_qemu_vm` resource. Proxmox does not expose allocation provenance, so data source reads always return null.",
		},
		"name":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Virtual machine name from `/nodes/{node}/qemu/{vmid}/config`."},
		"description": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Optional VM description from `/config`."},
		"tags":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Comma-separated Proxmox tags from `/config`."},
		"template":    datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the guest is a template, as reported by `/config`."},
		"pool":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Pool assignment from `/config`."},
		"onboot":      datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the guest should start automatically on boot."},
		"protection":  datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Proxmox protection is enabled for this VM, disabling remove VM and remove disk operations."},
		"scsihw":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "SCSI controller hardware type from `/config`."},
		"tablet":      datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the USB tablet device is enabled for this VM from `/config`."},
		"startup":     datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Startup ordering string from `/config`."},
		"bios":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Configured BIOS type from `/config`."},
		"machine":     datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Configured machine type from `/config`."},
		"agent":       datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Raw QEMU guest agent configuration string from `/config`."},
		"cores":       datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Configured vCPU cores from `/config`."},
		"sockets":     datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Configured CPU sockets from `/config`."},
		"memory":      datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Configured memory in MiB from `/config`."},
		"numa":        datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether NUMA is enabled for this VM from `/config`."},
		"vcpus":       datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Number of hotplugged vCPUs from `/config`."},
		"cpuunits":    datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "CPU weight for this VM from `/config`."},
		"cpulimit":    datasourceschema.Float64Attribute{Computed: true, MarkdownDescription: "CPU usage limit from `/config`. Value 0 indicates no limit."},
		"balloon":     datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Target balloon memory in MiB from `/config`. 0 disables ballooning."},
		"shares":      datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Memory shares for auto-ballooning from `/config`."},
		"hugepages":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Hugepages size in MiB (`2`, `1024`, or `any`) from `/config`."},
		"cpu":         datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Configured CPU model from `/config`."},
		"ostype":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Configured guest operating system type from `/config`."},
		"boot":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Boot order string from `/config`."},
		"common":      qemuVMCommonDataSourceAttribute(),
		"cloud_init":  qemuVMCloudInitDataSourceAttribute(),
		"network":     qemuVMNetworkDataSourceAttribute(),
		"disk":        qemuVMDiskDataSourceAttribute(),
		"serial":      datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Typed serial devices keyed by Proxmox slot name such as `serial0`, with values like `socket` or `/dev/ttyS0`."},
		"efi_disk":    qemuVMEFIDiskDataSourceAttribute(),
		"tpm_state":   qemuVMTPMStateDataSourceAttribute(),
		"vga":         qemuVMVGADataSourceAttribute(),
		"raw":         qemuVMRawDataSourceAttribute(),
		"clone":       qemuVMCloneDataSourceAttribute(),
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
			Optional:            true,
			Computed:            true,
			MarkdownDescription: "Numeric VMID of the QEMU virtual machine. When omitted, the provider allocates the next free cluster VMID through `GET /cluster/nextid` before the create or clone task starts.",
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
				int64planmodifier.RequiresReplace(),
			},
		},
		"vm_id_start": schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: "Lower bound for automatic `vm_id` allocation. Only valid when `vm_id` is omitted: the provider allocates the first free cluster VMID greater than or equal to this value through `GET /cluster/nextid`. Conflicts with `vm_id`.",
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.RequiresReplace(),
			},
		},
		"name":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Virtual machine name managed through `/nodes/{node}/qemu`, clone mode, and `/config`."},
		"description": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Optional VM description managed through clone mode and `/config`."},
		"tags":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Comma-separated Proxmox tags managed through `/config`."},
		"template":    schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the guest is a template, as observed from `/config`. Terraform does not manage template conversion."},
		"pool":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Pool assignment managed through clone mode and `/config`."},
		"onboot":      schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether the guest should start automatically on boot."},
		"protection":  schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether Proxmox protection is enabled for this VM, disabling remove VM and remove disk operations."},
		"scsihw":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "SCSI controller hardware type managed through `/config`."},
		"tablet":      schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether the USB tablet device is enabled for this VM, usually needed for absolute mouse positioning with VNC."},
		"startup":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Startup ordering string managed through `/config`."},
		"bios":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Configured BIOS type managed through `/config`."},
		"machine":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Configured machine type managed through `/config`."},
		"agent":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Raw QEMU guest agent configuration string managed through `/config`."},
		"cores":       schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Configured vCPU cores managed through `/config`."},
		"sockets":     schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Configured CPU sockets managed through `/config`."},
		"memory":      schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Configured memory in MiB managed through `/config`."},
		"numa":        schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether NUMA is enabled for this VM, managed through `/config`."},
		"vcpus":       schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Number of hotplugged vCPUs managed through `/config`."},
		"cpuunits":    schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "CPU weight for this VM managed through `/config`."},
		"cpulimit":    schema.Float64Attribute{Optional: true, Computed: true, MarkdownDescription: "CPU usage limit managed through `/config`. Value 0 indicates no limit."},
		"balloon":     schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Target balloon memory in MiB managed through `/config`. 0 disables ballooning."},
		"shares":      schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Memory shares for auto-ballooning managed through `/config`."},
		"hugepages":   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Hugepages size in MiB (`2`, `1024`, or `any`) managed through `/config`."},
		"cpu":         schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Configured CPU model managed through `/config`."},
		"ostype":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Configured guest operating system type managed through `/config`."},
		"boot":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Boot order string managed through `/config`."},
		"common":      qemuVMCommonResourceAttribute(),
		"cloud_init":  qemuVMCloudInitResourceAttribute(),
		"network":     qemuVMNetworkResourceAttribute(),
		"disk":        qemuVMDiskResourceAttribute(),
		"serial":      schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Typed serial devices keyed by Proxmox slot name such as `serial0`, with values like `socket` (unix socket for `qm terminal`) or `/dev/ttyS0` (host device passthrough)."},
		"efi_disk":    qemuVMEFIDiskResourceAttribute(),
		"tpm_state":   qemuVMTPMStateResourceAttribute(),
		"vga":         qemuVMVGAResourceAttribute(),
		"raw":         qemuVMRawResourceAttribute(),
		"clone":       qemuVMCloneResourceAttribute(),
		"status":      schema.StringAttribute{Computed: true, MarkdownDescription: "Observed runtime status from `/nodes/{node}/qemu/{vmid}/status/current`. Terraform does not manage power state."},
		"uptime":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Observed guest uptime in seconds from `/status/current`."},
	}
}
