package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &NodeDataSource{}

type NodeDataSource struct {
	client *Client
}

type NodeDataSourceModel struct {
	ID                types.String  `tfsdk:"id"`
	Node              types.String  `tfsdk:"node"`
	BootMode          types.String  `tfsdk:"boot_mode"`
	CPU               types.Float64 `tfsdk:"cpu"`
	CPUCores          types.Int64   `tfsdk:"cpu_cores"`
	CPUCount          types.Int64   `tfsdk:"cpu_count"`
	CPUModel          types.String  `tfsdk:"cpu_model"`
	CPUSockets        types.Int64   `tfsdk:"cpu_sockets"`
	KernelMachine     types.String  `tfsdk:"kernel_machine"`
	KernelRelease     types.String  `tfsdk:"kernel_release"`
	KernelSysname     types.String  `tfsdk:"kernel_sysname"`
	KernelVersion     types.String  `tfsdk:"kernel_version"`
	LoadAverage       types.List    `tfsdk:"load_average"`
	MemoryAvailable   types.Int64   `tfsdk:"memory_available"`
	MemoryFree        types.Int64   `tfsdk:"memory_free"`
	MemoryTotal       types.Int64   `tfsdk:"memory_total"`
	MemoryUsed        types.Int64   `tfsdk:"memory_used"`
	PVEVersion        types.String  `tfsdk:"pve_version"`
	RootFSAvailable   types.Int64   `tfsdk:"rootfs_available"`
	RootFSFree        types.Int64   `tfsdk:"rootfs_free"`
	RootFSTotal       types.Int64   `tfsdk:"rootfs_total"`
	RootFSUsed        types.Int64   `tfsdk:"rootfs_used"`
	SecureBootEnabled types.Bool    `tfsdk:"secure_boot_enabled"`
}

func NewNodeDataSource() datasource.DataSource {
	return &NodeDataSource{}
}

func (d *NodeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node"
}

func (d *NodeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads detailed status for a specific node from `/nodes/{node}/status`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster node name.",
			},
			"boot_mode":      schema.StringAttribute{Computed: true},
			"cpu":            schema.Float64Attribute{Computed: true},
			"cpu_cores":      schema.Int64Attribute{Computed: true},
			"cpu_count":      schema.Int64Attribute{Computed: true},
			"cpu_model":      schema.StringAttribute{Computed: true},
			"cpu_sockets":    schema.Int64Attribute{Computed: true},
			"kernel_machine": schema.StringAttribute{Computed: true},
			"kernel_release": schema.StringAttribute{Computed: true},
			"kernel_sysname": schema.StringAttribute{Computed: true},
			"kernel_version": schema.StringAttribute{Computed: true},
			"load_average": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
			"memory_available":    schema.Int64Attribute{Computed: true},
			"memory_free":         schema.Int64Attribute{Computed: true},
			"memory_total":        schema.Int64Attribute{Computed: true},
			"memory_used":         schema.Int64Attribute{Computed: true},
			"pve_version":         schema.StringAttribute{Computed: true},
			"rootfs_available":    schema.Int64Attribute{Computed: true},
			"rootfs_free":         schema.Int64Attribute{Computed: true},
			"rootfs_total":        schema.Int64Attribute{Computed: true},
			"rootfs_used":         schema.Int64Attribute{Computed: true},
			"secure_boot_enabled": schema.BoolAttribute{Computed: true},
		},
	}
}

func (d *NodeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", err.Error())
		return
	}

	d.client = client
}

func (d *NodeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NodeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	status, err := d.client.NodeStatus(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Node Status", fmt.Sprintf("Unable to call `/nodes/%s/status`: %s", config.Node.ValueString(), err))
		return
	}

	loadAverage, diags := stringListValue(ctx, status.LoadAverage)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state := NodeDataSourceModel{
		ID:                config.Node,
		Node:              config.Node,
		BootMode:          stringOrNull(status.BootInfo.Mode),
		CPU:               types.Float64Value(status.CPU),
		CPUCores:          types.Int64Value(status.CPUInfo.Cores),
		CPUCount:          types.Int64Value(status.CPUInfo.CPUs),
		CPUModel:          stringOrNull(status.CPUInfo.Model),
		CPUSockets:        types.Int64Value(status.CPUInfo.Sockets),
		KernelMachine:     stringOrNull(status.CurrentKernel.Machine),
		KernelRelease:     stringOrNull(status.CurrentKernel.Release),
		KernelSysname:     stringOrNull(status.CurrentKernel.Sysname),
		KernelVersion:     stringOrNull(status.CurrentKernel.Version),
		LoadAverage:       loadAverage,
		MemoryAvailable:   types.Int64Value(status.Memory.Available),
		MemoryFree:        types.Int64Value(status.Memory.Free),
		MemoryTotal:       types.Int64Value(status.Memory.Total),
		MemoryUsed:        types.Int64Value(status.Memory.Used),
		PVEVersion:        stringOrNull(status.PVEVersion),
		RootFSAvailable:   types.Int64Value(status.RootFS.Available),
		RootFSFree:        types.Int64Value(status.RootFS.Free),
		RootFSTotal:       types.Int64Value(status.RootFS.Total),
		RootFSUsed:        types.Int64Value(status.RootFS.Used),
		SecureBootEnabled: boolOrNull(status.BootInfo.SecureBoot),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
