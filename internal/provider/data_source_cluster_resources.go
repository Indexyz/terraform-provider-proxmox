// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ClusterResourcesDataSource{}

type ClusterResourcesDataSource struct {
	client *Client
}

type ClusterResourcesDataSourceModel struct {
	ID        types.String                       `tfsdk:"id"`
	Type      types.String                       `tfsdk:"type"`
	Resources []ClusterResourcesDataSourceRecord `tfsdk:"resources"`
}

type ClusterResourcesDataSourceRecord struct {
	CGroupMode  types.Int64   `tfsdk:"cgroup_mode"`
	Content     types.String  `tfsdk:"content"`
	CPU         types.Float64 `tfsdk:"cpu"`
	Disk        types.Int64   `tfsdk:"disk"`
	DiskRead    types.Int64   `tfsdk:"disk_read"`
	DiskWrite   types.Int64   `tfsdk:"disk_write"`
	HAState     types.String  `tfsdk:"ha_state"`
	ID          types.String  `tfsdk:"id"`
	Level       types.String  `tfsdk:"level"`
	Lock        types.String  `tfsdk:"lock"`
	MaxCPU      types.Float64 `tfsdk:"max_cpu"`
	MaxDisk     types.Int64   `tfsdk:"max_disk"`
	MaxMemory   types.Int64   `tfsdk:"max_memory"`
	MemoryHost  types.Int64   `tfsdk:"memory_host"`
	MemoryUsed  types.Int64   `tfsdk:"memory_used"`
	Name        types.String  `tfsdk:"name"`
	NetIn       types.Int64   `tfsdk:"net_in"`
	NetOut      types.Int64   `tfsdk:"net_out"`
	Network     types.String  `tfsdk:"network"`
	NetworkType types.String  `tfsdk:"network_type"`
	Node        types.String  `tfsdk:"node"`
	PluginType  types.String  `tfsdk:"plugin_type"`
	Pool        types.String  `tfsdk:"pool"`
	Protocol    types.String  `tfsdk:"protocol"`
	SDN         types.String  `tfsdk:"sdn"`
	Shared      types.Bool    `tfsdk:"shared"`
	Status      types.String  `tfsdk:"status"`
	Storage     types.String  `tfsdk:"storage"`
	Tags        types.String  `tfsdk:"tags"`
	Template    types.Bool    `tfsdk:"template"`
	Type        types.String  `tfsdk:"resource_type"`
	Uptime      types.Int64   `tfsdk:"uptime"`
	VMID        types.Int64   `tfsdk:"vm_id"`
	ZoneType    types.String  `tfsdk:"zone_type"`
}

func NewClusterResourcesDataSource() datasource.DataSource {
	return &ClusterResourcesDataSource{}
}

func (d *ClusterResourcesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_resources"
}

func (d *ClusterResourcesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists resources visible from `/cluster/resources`. The optional `type` filter maps directly to the Proxmox API and supports `vm`, `storage`, `node`, and `sdn`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional API-side filter for `/cluster/resources`.",
			},
			"resources": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Cluster resources returned by the Proxmox API.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"cgroup_mode":   schema.Int64Attribute{Computed: true},
						"content":       schema.StringAttribute{Computed: true},
						"cpu":           schema.Float64Attribute{Computed: true},
						"disk":          schema.Int64Attribute{Computed: true},
						"disk_read":     schema.Int64Attribute{Computed: true},
						"disk_write":    schema.Int64Attribute{Computed: true},
						"ha_state":      schema.StringAttribute{Computed: true},
						"id":            schema.StringAttribute{Computed: true},
						"level":         schema.StringAttribute{Computed: true},
						"lock":          schema.StringAttribute{Computed: true},
						"max_cpu":       schema.Float64Attribute{Computed: true},
						"max_disk":      schema.Int64Attribute{Computed: true},
						"max_memory":    schema.Int64Attribute{Computed: true},
						"memory_host":   schema.Int64Attribute{Computed: true},
						"memory_used":   schema.Int64Attribute{Computed: true},
						"name":          schema.StringAttribute{Computed: true},
						"net_in":        schema.Int64Attribute{Computed: true},
						"net_out":       schema.Int64Attribute{Computed: true},
						"network":       schema.StringAttribute{Computed: true},
						"network_type":  schema.StringAttribute{Computed: true},
						"node":          schema.StringAttribute{Computed: true},
						"plugin_type":   schema.StringAttribute{Computed: true},
						"pool":          schema.StringAttribute{Computed: true},
						"protocol":      schema.StringAttribute{Computed: true},
						"sdn":           schema.StringAttribute{Computed: true},
						"shared":        schema.BoolAttribute{Computed: true},
						"status":        schema.StringAttribute{Computed: true},
						"storage":       schema.StringAttribute{Computed: true},
						"tags":          schema.StringAttribute{Computed: true},
						"template":      schema.BoolAttribute{Computed: true},
						"resource_type": schema.StringAttribute{Computed: true},
						"uptime":        schema.Int64Attribute{Computed: true},
						"vm_id":         schema.Int64Attribute{Computed: true},
						"zone_type":     schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *ClusterResourcesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ClusterResourcesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ClusterResourcesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resourceType := stringValue(config.Type)
	resources, err := d.client.ClusterResources(ctx, resourceType)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Cluster Resources", fmt.Sprintf("Unable to call `/cluster/resources`: %s", err))
		return
	}

	records := make([]ClusterResourcesDataSourceRecord, 0, len(resources))
	for _, resource := range resources {
		records = append(records, ClusterResourcesDataSourceRecord{
			CGroupMode:  int64OrNull(resource.CGroupMode),
			Content:     stringOrNull(resource.Content),
			CPU:         float64OrNull(resource.CPU),
			Disk:        int64OrNull(resource.Disk),
			DiskRead:    int64OrNull(resource.DiskRead),
			DiskWrite:   int64OrNull(resource.DiskWrite),
			HAState:     stringOrNull(resource.HAState),
			ID:          stringOrNull(resource.ID),
			Level:       stringOrNull(resource.Level),
			Lock:        stringOrNull(resource.Lock),
			MaxCPU:      float64OrNull(resource.MaxCPU),
			MaxDisk:     int64OrNull(resource.MaxDisk),
			MaxMemory:   int64OrNull(resource.MaxMemory),
			MemoryHost:  int64OrNull(resource.MemoryHost),
			MemoryUsed:  int64OrNull(resource.MemoryUsed),
			Name:        stringOrNull(resource.Name),
			NetIn:       int64OrNull(resource.NetIn),
			NetOut:      int64OrNull(resource.NetOut),
			Network:     stringOrNull(resource.Network),
			NetworkType: stringOrNull(resource.NetworkType),
			Node:        stringOrNull(resource.Node),
			PluginType:  stringOrNull(resource.PluginType),
			Pool:        stringOrNull(resource.Pool),
			Protocol:    stringOrNull(resource.Protocol),
			SDN:         stringOrNull(resource.SDN),
			Shared:      boolOrNull(resource.Shared),
			Status:      stringOrNull(resource.Status),
			Storage:     stringOrNull(resource.Storage),
			Tags:        stringOrNull(resource.Tags),
			Template:    boolOrNull(resource.Template),
			Type:        stringOrNull(resource.Type),
			Uptime:      int64OrNull(resource.Uptime),
			VMID:        int64OrNull(resource.VMID),
			ZoneType:    stringOrNull(resource.ZoneType),
		})
	}

	state := ClusterResourcesDataSourceModel{
		ID:        types.StringValue("cluster_resources"),
		Type:      config.Type,
		Resources: records,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
