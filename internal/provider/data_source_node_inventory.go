// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/url"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &NodeHardwarePCIDataSource{}
var _ datasource.DataSource = &NodeHardwareUSBDataSource{}
var _ datasource.DataSource = &NodeAPTVersionsDataSource{}
var _ datasource.DataSource = &NodeServicesDataSource{}
var _ datasource.DataSourceWithValidateConfig = &NodeHardwarePCIDataSource{}
var _ datasource.DataSourceWithValidateConfig = &NodeHardwareUSBDataSource{}
var _ datasource.DataSourceWithValidateConfig = &NodeAPTVersionsDataSource{}
var _ datasource.DataSourceWithValidateConfig = &NodeServicesDataSource{}

type NodeHardwarePCIDataSource struct{ client *Client }
type NodeHardwareUSBDataSource struct{ client *Client }
type NodeAPTVersionsDataSource struct{ client *Client }
type NodeServicesDataSource struct{ client *Client }

type nodeHardwarePCIModel struct {
	Node    types.String            `tfsdk:"node"`
	Devices []nodeHardwarePCIRecord `tfsdk:"devices"`
}

type nodeHardwarePCIRecord struct {
	ID                  types.String `tfsdk:"id"`
	Class               types.String `tfsdk:"class"`
	Device              types.String `tfsdk:"device"`
	DeviceName          types.String `tfsdk:"device_name"`
	Vendor              types.String `tfsdk:"vendor"`
	VendorName          types.String `tfsdk:"vendor_name"`
	SubsystemDevice     types.String `tfsdk:"subsystem_device"`
	SubsystemDeviceName types.String `tfsdk:"subsystem_device_name"`
	SubsystemVendor     types.String `tfsdk:"subsystem_vendor"`
	SubsystemVendorName types.String `tfsdk:"subsystem_vendor_name"`
	IOMMUGroup          types.Int64  `tfsdk:"iommugroup"`
	MDev                types.Bool   `tfsdk:"mdev"`
}

type nodeHardwareUSBModel struct {
	Node    types.String            `tfsdk:"node"`
	Devices []nodeHardwareUSBRecord `tfsdk:"devices"`
}

type nodeHardwareUSBRecord struct {
	BusNum       types.Int64  `tfsdk:"busnum"`
	Class        types.String `tfsdk:"class"`
	DevNum       types.Int64  `tfsdk:"devnum"`
	Level        types.Int64  `tfsdk:"level"`
	Manufacturer types.String `tfsdk:"manufacturer"`
	Port         types.Int64  `tfsdk:"port"`
	ProductID    types.String `tfsdk:"prodid"`
	Product      types.String `tfsdk:"product"`
	Serial       types.String `tfsdk:"serial"`
	Speed        types.String `tfsdk:"speed"`
	USBPath      types.String `tfsdk:"usbpath"`
	VendorID     types.String `tfsdk:"vendid"`
}

type nodeAPTVersionsModel struct {
	Node     types.String            `tfsdk:"node"`
	Packages []nodeAPTVersionsRecord `tfsdk:"packages"`
}

type nodeAPTVersionsRecord struct {
	Package        types.String `tfsdk:"package"`
	Version        types.String `tfsdk:"version"`
	OldVersion     types.String `tfsdk:"old_version"`
	Origin         types.String `tfsdk:"origin"`
	CurrentState   types.String `tfsdk:"current_state"`
	Section        types.String `tfsdk:"section"`
	Priority       types.String `tfsdk:"priority"`
	Title          types.String `tfsdk:"title"`
	Description    types.String `tfsdk:"description"`
	Arch           types.String `tfsdk:"arch"`
	ManagerVersion types.String `tfsdk:"manager_version"`
	NotifyStatus   types.String `tfsdk:"notify_status"`
	RunningKernel  types.String `tfsdk:"running_kernel"`
}

type nodeServicesModel struct {
	Node     types.String         `tfsdk:"node"`
	Services []nodeServicesRecord `tfsdk:"services"`
}

type nodeServicesRecord struct {
	Service     types.String `tfsdk:"service"`
	Name        types.String `tfsdk:"name"`
	Desc        types.String `tfsdk:"desc"`
	State       types.String `tfsdk:"state"`
	ActiveState types.String `tfsdk:"active_state"`
	UnitState   types.String `tfsdk:"unit_state"`
}

type nodeHardwarePCIEntry struct {
	ID                  string               `json:"id"`
	Class               string               `json:"class"`
	Device              string               `json:"device"`
	DeviceName          string               `json:"device_name"`
	Vendor              string               `json:"vendor"`
	VendorName          string               `json:"vendor_name"`
	SubsystemDevice     string               `json:"subsystem_device"`
	SubsystemDeviceName string               `json:"subsystem_device_name"`
	SubsystemVendor     string               `json:"subsystem_vendor"`
	SubsystemVendorName string               `json:"subsystem_vendor_name"`
	IOMMUGroup          proxmoxOptionalInt64 `json:"iommugroup"`
	MDev                proxmoxOptionalBool  `json:"mdev"`
}

type nodeHardwareUSBEntry struct {
	BusNum       int64  `json:"busnum"`
	Class        string `json:"class"`
	DevNum       int64  `json:"devnum"`
	Level        int64  `json:"level"`
	Manufacturer string `json:"manufacturer"`
	Port         int64  `json:"port"`
	ProductID    string `json:"prodid"`
	Product      string `json:"product"`
	Serial       string `json:"serial"`
	Speed        string `json:"speed"`
	USBPath      string `json:"usbpath"`
	VendorID     string `json:"vendid"`
}

type nodeAPTVersionsEntry struct {
	Package        string `json:"Package"`
	Version        string `json:"Version"`
	OldVersion     string `json:"OldVersion"`
	Origin         string `json:"Origin"`
	CurrentState   string `json:"CurrentState"`
	Section        string `json:"Section"`
	Priority       string `json:"Priority"`
	Title          string `json:"Title"`
	Description    string `json:"Description"`
	Arch           string `json:"Arch"`
	ManagerVersion string `json:"ManagerVersion"`
	NotifyStatus   string `json:"NotifyStatus"`
	RunningKernel  string `json:"RunningKernel"`
}

type nodeServicesEntry struct {
	Service     string `json:"service"`
	Name        string `json:"name"`
	Desc        string `json:"desc"`
	State       string `json:"state"`
	ActiveState string `json:"active-state"`
	UnitState   string `json:"unit-state"`
}

func NewNodeHardwarePCIDataSource() datasource.DataSource { return &NodeHardwarePCIDataSource{} }
func NewNodeHardwareUSBDataSource() datasource.DataSource { return &NodeHardwareUSBDataSource{} }
func NewNodeAPTVersionsDataSource() datasource.DataSource { return &NodeAPTVersionsDataSource{} }
func NewNodeServicesDataSource() datasource.DataSource    { return &NodeServicesDataSource{} }

func (d *NodeHardwarePCIDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_hardware_pci"
}

func (d *NodeHardwareUSBDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_hardware_usb"
}

func (d *NodeAPTVersionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_apt_versions"
}

func (d *NodeServicesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_services"
}

func (d *NodeHardwarePCIDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists the PCI devices of one node from `/nodes/{node}/hardware/pci`, sorted by address. Use `id` plus `vendor`/`device` for `proxmox_cluster_mapping_pci` entries.",
		Attributes: map[string]datasourceschema.Attribute{
			"node": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"devices": datasourceschema.ListNestedAttribute{
				Computed: true,
				NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
					"id":                    datasourceschema.StringAttribute{Computed: true},
					"class":                 datasourceschema.StringAttribute{Computed: true},
					"device":                datasourceschema.StringAttribute{Computed: true},
					"device_name":           datasourceschema.StringAttribute{Computed: true},
					"vendor":                datasourceschema.StringAttribute{Computed: true},
					"vendor_name":           datasourceschema.StringAttribute{Computed: true},
					"subsystem_device":      datasourceschema.StringAttribute{Computed: true},
					"subsystem_device_name": datasourceschema.StringAttribute{Computed: true},
					"subsystem_vendor":      datasourceschema.StringAttribute{Computed: true},
					"subsystem_vendor_name": datasourceschema.StringAttribute{Computed: true},
					"iommugroup":            datasourceschema.Int64Attribute{Computed: true},
					"mdev":                  datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the device can create mediated devices."},
				}},
			},
		},
	}
}

func (d *NodeHardwareUSBDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists the USB devices of one node from `/nodes/{node}/hardware/usb`, sorted by bus/port. Device ids are the `vendid:prodid` pair used by `proxmox_cluster_mapping_usb`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"devices": datasourceschema.ListNestedAttribute{
				Computed: true,
				NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
					"busnum":       datasourceschema.Int64Attribute{Computed: true},
					"class":        datasourceschema.StringAttribute{Computed: true},
					"devnum":       datasourceschema.Int64Attribute{Computed: true},
					"level":        datasourceschema.Int64Attribute{Computed: true},
					"manufacturer": datasourceschema.StringAttribute{Computed: true},
					"port":         datasourceschema.Int64Attribute{Computed: true},
					"prodid":       datasourceschema.StringAttribute{Computed: true},
					"product":      datasourceschema.StringAttribute{Computed: true},
					"serial":       datasourceschema.StringAttribute{Computed: true},
					"speed":        datasourceschema.StringAttribute{Computed: true},
					"usbpath":      datasourceschema.StringAttribute{Computed: true},
					"vendid":       datasourceschema.StringAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *NodeAPTVersionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists installable package versions of one node from `/nodes/{node}/apt/versions`, sorted by package. `old_version` is set when an upgrade is available.",
		Attributes: map[string]datasourceschema.Attribute{
			"node": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"packages": datasourceschema.ListNestedAttribute{
				Computed: true,
				NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
					"package":         datasourceschema.StringAttribute{Computed: true},
					"version":         datasourceschema.StringAttribute{Computed: true},
					"old_version":     datasourceschema.StringAttribute{Computed: true},
					"origin":          datasourceschema.StringAttribute{Computed: true},
					"current_state":   datasourceschema.StringAttribute{Computed: true},
					"section":         datasourceschema.StringAttribute{Computed: true},
					"priority":        datasourceschema.StringAttribute{Computed: true},
					"title":           datasourceschema.StringAttribute{Computed: true},
					"description":     datasourceschema.StringAttribute{Computed: true},
					"arch":            datasourceschema.StringAttribute{Computed: true},
					"manager_version": datasourceschema.StringAttribute{Computed: true},
					"notify_status":   datasourceschema.StringAttribute{Computed: true},
					"running_kernel":  datasourceschema.StringAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *NodeServicesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists the known system services of one node from `/nodes/{node}/services`, sorted by service name.",
		Attributes: map[string]datasourceschema.Attribute{
			"node": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"services": datasourceschema.ListNestedAttribute{
				Computed: true,
				NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
					"service":      datasourceschema.StringAttribute{Computed: true},
					"name":         datasourceschema.StringAttribute{Computed: true},
					"desc":         datasourceschema.StringAttribute{Computed: true},
					"state":        datasourceschema.StringAttribute{Computed: true},
					"active_state": datasourceschema.StringAttribute{Computed: true},
					"unit_state":   datasourceschema.StringAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *NodeHardwarePCIDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *NodeHardwareUSBDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *NodeAPTVersionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *NodeServicesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *NodeHardwarePCIDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config nodeHardwarePCIModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if !resp.Diagnostics.HasError() {
		validateNodeName(config.Node, resp)
	}
}
func (d *NodeHardwareUSBDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config nodeHardwareUSBModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if !resp.Diagnostics.HasError() {
		validateNodeName(config.Node, resp)
	}
}
func (d *NodeAPTVersionsDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config nodeAPTVersionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if !resp.Diagnostics.HasError() {
		validateNodeName(config.Node, resp)
	}
}
func (d *NodeServicesDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config nodeServicesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if !resp.Diagnostics.HasError() {
		validateNodeName(config.Node, resp)
	}
}

func (d *NodeHardwarePCIDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeHardwarePCIModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := config.Node.ValueString()
	var entries []nodeHardwarePCIEntry
	if err := d.client.do(ctx, "GET", "/nodes/"+url.PathEscape(node)+"/hardware/pci", nil, nil, &entries); err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Node PCI Hardware", err.Error())
		return
	}
	records := make([]nodeHardwarePCIRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, nodeHardwarePCIRecord{
			ID: types.StringValue(entry.ID), Class: stringOrNull(entry.Class), Device: stringOrNull(entry.Device),
			DeviceName: stringOrNull(entry.DeviceName), Vendor: stringOrNull(entry.Vendor), VendorName: stringOrNull(entry.VendorName),
			SubsystemDevice: stringOrNull(entry.SubsystemDevice), SubsystemDeviceName: stringOrNull(entry.SubsystemDeviceName),
			SubsystemVendor: stringOrNull(entry.SubsystemVendor), SubsystemVendorName: stringOrNull(entry.SubsystemVendorName),
			IOMMUGroup: int64OrNull(entry.IOMMUGroup.Ptr()), MDev: nodesBoolOrNull(entry.MDev),
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID.ValueString() < records[j].ID.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeHardwarePCIModel{Node: config.Node, Devices: records})...)
}

func (d *NodeHardwareUSBDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeHardwareUSBModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := config.Node.ValueString()
	var entries []nodeHardwareUSBEntry
	if err := d.client.do(ctx, "GET", "/nodes/"+url.PathEscape(node)+"/hardware/usb", nil, nil, &entries); err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Node USB Hardware", err.Error())
		return
	}
	records := make([]nodeHardwareUSBRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, nodeHardwareUSBRecord{
			BusNum: types.Int64Value(entry.BusNum), Class: stringOrNull(entry.Class), DevNum: types.Int64Value(entry.DevNum),
			Level: types.Int64Value(entry.Level), Manufacturer: stringOrNull(entry.Manufacturer), Port: types.Int64Value(entry.Port),
			ProductID: types.StringValue(entry.ProductID), Product: stringOrNull(entry.Product), Serial: stringOrNull(entry.Serial),
			Speed: stringOrNull(entry.Speed), USBPath: stringOrNull(entry.USBPath), VendorID: types.StringValue(entry.VendorID),
		})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].BusNum.ValueInt64() != records[j].BusNum.ValueInt64() {
			return records[i].BusNum.ValueInt64() < records[j].BusNum.ValueInt64()
		}
		return records[i].Port.ValueInt64() < records[j].Port.ValueInt64()
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeHardwareUSBModel{Node: config.Node, Devices: records})...)
}

func (d *NodeAPTVersionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeAPTVersionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := config.Node.ValueString()
	var entries []nodeAPTVersionsEntry
	if err := d.client.do(ctx, "GET", "/nodes/"+url.PathEscape(node)+"/apt/versions", nil, nil, &entries); err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Node APT Versions", err.Error())
		return
	}
	records := make([]nodeAPTVersionsRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, nodeAPTVersionsRecord{
			Package: types.StringValue(entry.Package), Version: stringOrNull(entry.Version), OldVersion: stringOrNull(entry.OldVersion),
			Origin: stringOrNull(entry.Origin), CurrentState: stringOrNull(entry.CurrentState), Section: stringOrNull(entry.Section),
			Priority: stringOrNull(entry.Priority), Title: stringOrNull(entry.Title), Description: stringOrNull(entry.Description),
			Arch: stringOrNull(entry.Arch), ManagerVersion: stringOrNull(entry.ManagerVersion), NotifyStatus: stringOrNull(entry.NotifyStatus),
			RunningKernel: stringOrNull(entry.RunningKernel),
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Package.ValueString() < records[j].Package.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeAPTVersionsModel{Node: config.Node, Packages: records})...)
}

func (d *NodeServicesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeServicesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := config.Node.ValueString()
	var entries []nodeServicesEntry
	if err := d.client.do(ctx, "GET", "/nodes/"+url.PathEscape(node)+"/services", nil, nil, &entries); err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Node Services", err.Error())
		return
	}
	records := make([]nodeServicesRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, nodeServicesRecord{
			Service: types.StringValue(entry.Service), Name: stringOrNull(entry.Name), Desc: stringOrNull(entry.Desc),
			State: stringOrNull(entry.State), ActiveState: stringOrNull(entry.ActiveState), UnitState: stringOrNull(entry.UnitState),
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Service.ValueString() < records[j].Service.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeServicesModel{Node: config.Node, Services: records})...)
}
