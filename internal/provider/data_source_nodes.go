package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &NodesDataSource{}

type NodesDataSource struct {
	client *Client
}

type NodesDataSourceModel struct {
	ID    types.String              `tfsdk:"id"`
	Nodes []NodesDataSourceNodeItem `tfsdk:"nodes"`
}

type NodesDataSourceNodeItem struct {
	CPU            types.Float64 `tfsdk:"cpu"`
	Level          types.String  `tfsdk:"level"`
	MaxCPU         types.Int64   `tfsdk:"max_cpu"`
	MaxMemory      types.Int64   `tfsdk:"max_memory"`
	MemoryUsed     types.Int64   `tfsdk:"memory_used"`
	Name           types.String  `tfsdk:"name"`
	SSLFingerprint types.String  `tfsdk:"ssl_fingerprint"`
	Status         types.String  `tfsdk:"status"`
	Uptime         types.Int64   `tfsdk:"uptime"`
}

func NewNodesDataSource() datasource.DataSource {
	return &NodesDataSource{}
}

func (d *NodesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nodes"
}

func (d *NodesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists cluster nodes from `/nodes`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier for this data source.",
			},
			"nodes": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Cluster nodes visible to the authenticated user.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"cpu":             schema.Float64Attribute{Computed: true},
						"level":           schema.StringAttribute{Computed: true},
						"max_cpu":         schema.Int64Attribute{Computed: true},
						"max_memory":      schema.Int64Attribute{Computed: true},
						"memory_used":     schema.Int64Attribute{Computed: true},
						"name":            schema.StringAttribute{Computed: true},
						"ssl_fingerprint": schema.StringAttribute{Computed: true},
						"status":          schema.StringAttribute{Computed: true},
						"uptime":          schema.Int64Attribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *NodesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *NodesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	nodes, err := d.client.Nodes(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Nodes", fmt.Sprintf("Unable to call `/nodes`: %s", err))
		return
	}

	items := make([]NodesDataSourceNodeItem, 0, len(nodes))
	for _, node := range nodes {
		items = append(items, NodesDataSourceNodeItem{
			CPU:            types.Float64Value(node.CPU),
			Level:          stringOrNull(node.Level),
			MaxCPU:         types.Int64Value(node.MaxCPU),
			MaxMemory:      types.Int64Value(node.MaxMemory),
			MemoryUsed:     types.Int64Value(node.MemoryUsed),
			Name:           stringOrNull(node.Name),
			SSLFingerprint: stringOrNull(node.SSLFingerprint),
			Status:         stringOrNull(node.Status),
			Uptime:         types.Int64Value(node.Uptime),
		})
	}

	state := NodesDataSourceModel{
		ID:    types.StringValue("nodes"),
		Nodes: items,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
