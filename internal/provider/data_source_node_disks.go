// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &NodeDisksDataSource{}
var _ datasource.DataSourceWithValidateConfig = &NodeDisksDataSource{}
var _ datasource.DataSource = &NodeDiskSmartDataSource{}
var _ datasource.DataSourceWithValidateConfig = &NodeDiskSmartDataSource{}

type NodeDisksDataSource struct{ client *Client }
type NodeDiskSmartDataSource struct{ client *Client }

type nodeDisksModel struct {
	Node  types.String      `tfsdk:"node"`
	Disks []nodeDisksRecord `tfsdk:"disks"`
}

type nodeDisksRecord struct {
	DevPath types.String `tfsdk:"devpath"`
	GPT     types.Bool   `tfsdk:"gpt"`
	Health  types.String `tfsdk:"health"`
	Model   types.String `tfsdk:"model"`
	Mounted types.Bool   `tfsdk:"mounted"`
	OSDID   types.String `tfsdk:"osdid"`
	Parent  types.String `tfsdk:"parent"`
	Serial  types.String `tfsdk:"serial"`
	Size    types.Int64  `tfsdk:"size"`
	Used    types.String `tfsdk:"used"`
	Vendor  types.String `tfsdk:"vendor"`
	WWN     types.String `tfsdk:"wwn"`
}

type nodeDiskSmartModel struct {
	Node       types.String `tfsdk:"node"`
	Disk       types.String `tfsdk:"disk"`
	Health     types.String `tfsdk:"health"`
	Type       types.String `tfsdk:"type"`
	Text       types.String `tfsdk:"text"`
	Attributes types.Map    `tfsdk:"attributes"`
}

func NewNodeDisksDataSource() datasource.DataSource     { return &NodeDisksDataSource{} }
func NewNodeDiskSmartDataSource() datasource.DataSource { return &NodeDiskSmartDataSource{} }

func (d *NodeDisksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_disks"
}

func (d *NodeDiskSmartDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_disk_smart"
}

func (d *NodeDisksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists the block devices of one node from `/nodes/{node}/disks/list`, sorted by device path. Proxmox wipes devices used by the disk resources.",
		Attributes: map[string]datasourceschema.Attribute{
			"node": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"disks": datasourceschema.ListNestedAttribute{
				Computed: true,
				NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
					"devpath": datasourceschema.StringAttribute{Computed: true},
					"gpt":     datasourceschema.BoolAttribute{Computed: true},
					"health":  datasourceschema.StringAttribute{Computed: true},
					"model":   datasourceschema.StringAttribute{Computed: true},
					"mounted": datasourceschema.BoolAttribute{Computed: true},
					"osdid":   datasourceschema.StringAttribute{Computed: true},
					"parent":  datasourceschema.StringAttribute{Computed: true},
					"serial":  datasourceschema.StringAttribute{Computed: true},
					"size":    datasourceschema.Int64Attribute{Computed: true},
					"used":    datasourceschema.StringAttribute{Computed: true},
					"vendor":  datasourceschema.StringAttribute{Computed: true},
					"wwn":     datasourceschema.StringAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *NodeDiskSmartDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads S.M.A.R.T. data of one node disk from `/nodes/{node}/disks/smart`. `attributes` maps attribute ids to their JSON representation.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":       datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"disk":       datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Block device such as `/dev/disk/by-id/...`."},
			"health":     datasourceschema.StringAttribute{Computed: true},
			"type":       datasourceschema.StringAttribute{Computed: true},
			"text":       datasourceschema.StringAttribute{Computed: true},
			"attributes": datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *NodeDisksDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *NodeDiskSmartDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *NodeDisksDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config nodeDisksModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if !resp.Diagnostics.HasError() {
		validateNodeName(config.Node, resp)
	}
}

func (d *NodeDiskSmartDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config nodeDiskSmartModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if !resp.Diagnostics.HasError() {
		validateNodeName(config.Node, resp)
		if isConfiguredValue(config.Disk) && !nodeDiskDevicePattern.MatchString(config.Disk.ValueString()) {
			resp.Diagnostics.AddAttributeError(pathRoot("disk"), "Invalid device path", "disk must be an absolute `/dev/...` path.")
		}
	}
}

func (d *NodeDisksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeDisksModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	disks, err := d.client.NodeDisks(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Node Disks", err.Error())
		return
	}
	sortDisks(disks)
	records := make([]nodeDisksRecord, 0, len(disks))
	for _, disk := range disks {
		records = append(records, nodeDisksRecord{
			DevPath: types.StringValue(disk.DevPath), GPT: nodesBoolOrNull(disk.GPT), Health: stringOrNull(disk.Health),
			Model: stringOrNull(disk.Model), Mounted: nodesBoolOrNull(disk.Mounted), OSDID: stringOrNull(string(disk.OSDID)),
			Parent: stringOrNull(disk.Parent), Serial: stringOrNull(disk.Serial), Size: int64OrNull(optionalPositive(disk.Size)),
			Used: stringOrNull(disk.Used), Vendor: stringOrNull(disk.Vendor), WWN: stringOrNull(disk.WWN),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeDisksModel{Node: config.Node, Disks: records})...)
}

func (d *NodeDiskSmartDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeDiskSmartModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := d.client.NodeDiskSmart(ctx, config.Node.ValueString(), config.Disk.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Disk S.M.A.R.T. Data", err.Error())
		return
	}
	attributes := map[string]string{}
	for id, raw := range result.Attributes {
		attributes[id] = string(raw)
	}
	value, mapDiags := types.MapValueFrom(ctx, types.StringType, attributes)
	resp.Diagnostics.Append(mapDiags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeDiskSmartModel{
		Node: config.Node, Disk: config.Disk, Health: stringOrNull(result.Health),
		Type: stringOrNull(result.Type), Text: stringOrNull(result.Text), Attributes: value,
	})...)
}

func pathRoot(name string) path.Path { return path.Root(name) }
