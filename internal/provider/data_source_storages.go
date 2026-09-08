// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &StoragesDataSource{}

type StoragesDataSource struct {
	client *Client
}

type StoragesDataSourceModel struct {
	ID       types.String               `tfsdk:"id"`
	Type     types.String               `tfsdk:"type"`
	Largest  types.Bool                 `tfsdk:"largest"`
	Storages []StoragesDataSourceRecord `tfsdk:"storages"`
}

type StoragesDataSourceRecord struct {
	Storage       types.String `tfsdk:"storage"`
	Type          types.String `tfsdk:"type"`
	Content       types.String `tfsdk:"content"`
	Nodes         types.String `tfsdk:"nodes"`
	Disable       types.Bool   `tfsdk:"disable"`
	Shared        types.Bool   `tfsdk:"shared"`
	CapacityBytes types.Int64  `tfsdk:"capacity_bytes"`
}

func NewStoragesDataSource() datasource.DataSource {
	return &StoragesDataSource{}
}

func (d *StoragesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storages"
}

func (d *StoragesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Fetches the list of all Proxmox VE storage pools from `/storage`, enriched with reported capacity from `/cluster/resources`. The optional `type` and `largest` filters are applied by the provider. Storages the authenticated identity cannot audit (Datastore.Audit) have no reported capacity.",
		Attributes: map[string]datasourceschema.Attribute{
			"id": datasourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique identifier for this data source call.",
			},
			"type": datasourceschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Exact, case-sensitive match on the storage plugin type (`zfs`, `dir`, `nfs`, ...). An empty value disables this filter.",
			},
			"largest": datasourceschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "When true, keep only the storages tied at the highest reported `capacity_bytes`. Storages without reported capacity are excluded, and an empty list is returned when no storage reports capacity. Ties between distinct storages are all returned; assert uniqueness with Terraform's `one()`.",
			},
			"storages": storagesDataSourceAttribute(),
		},
	}
}

func storagesDataSourceAttribute() datasourceschema.ListNestedAttribute {
	return datasourceschema.ListNestedAttribute{
		Computed: true,
		NestedObject: datasourceschema.NestedAttributeObject{
			Attributes: map[string]datasourceschema.Attribute{
				"storage":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Storage identifier."},
				"type":           datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Storage type."},
				"content":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Allowed content types."},
				"nodes":          datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Nodes this storage applies to."},
				"disable":        datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the storage is disabled."},
				"shared":         datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the storage is shared."},
				"capacity_bytes": datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Highest capacity reported for this storage across cluster nodes, in bytes. This is an aggregate hint and does not guarantee the capacity on any specific deployment node. Null while no capacity has been reported (disabled, unauditable, or not yet indexed by Proxmox)."},
			},
		},
	}
}

func (d *StoragesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *StoragesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config StoragesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	storages, err := d.client.Storages(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Storages", err.Error())
		return
	}

	capacities, err := d.storageCapacities(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Storage Capacities", fmt.Sprintf("Unable to call `/cluster/resources`: %s", err))
		return
	}

	filterType := stringValue(config.Type)
	filterLargest := !config.Largest.IsNull() && config.Largest.ValueBool()

	records := make([]StoragesDataSourceRecord, 0, len(storages))
	for _, s := range storages {
		if filterType != "" && s.Type != filterType {
			continue
		}
		records = append(records, StoragesDataSourceRecord{
			Storage:       stringOrNull(s.Storage),
			Type:          stringOrNull(s.Type),
			Content:       stringOrNull(s.Content),
			Nodes:         stringOrNull(s.Nodes),
			Disable:       boolOrNull(s.Disable.Ptr()),
			Shared:        boolOrNull(s.Shared.Ptr()),
			CapacityBytes: int64OrNull(capacities[s.Storage]),
		})
	}

	if filterLargest {
		records = keepLargestStorages(records)
	}

	state := StoragesDataSourceModel{
		ID:       types.StringValue(fmt.Sprintf("storages-%d", len(records))),
		Type:     config.Type,
		Largest:  config.Largest,
		Storages: records,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// storageCapacities returns the highest reported maxdisk per storage across
// its per-node entries in `/cluster/resources`; storages without a reported
// capacity are absent from the map.
func (d *StoragesDataSource) storageCapacities(ctx context.Context) (map[string]*int64, error) {
	resources, err := d.client.ClusterResources(ctx, "storage")
	if err != nil {
		return nil, err
	}

	capacities := make(map[string]*int64, len(resources))
	for _, resource := range resources {
		if resource.MaxDisk == nil {
			continue
		}
		if current, ok := capacities[resource.Storage]; !ok || *current < *resource.MaxDisk {
			value := *resource.MaxDisk
			capacities[resource.Storage] = &value
		}
	}
	return capacities, nil
}

// keepLargestStorages keeps only records tied at the highest reported
// capacity; records without a reported capacity are dropped.
func keepLargestStorages(records []StoragesDataSourceRecord) []StoragesDataSourceRecord {
	var largest *types.Int64
	for _, record := range records {
		if record.CapacityBytes.IsNull() {
			continue
		}
		if largest == nil || record.CapacityBytes.ValueInt64() > largest.ValueInt64() {
			value := record.CapacityBytes
			largest = &value
		}
	}
	if largest == nil {
		return []StoragesDataSourceRecord{}
	}

	filtered := make([]StoragesDataSourceRecord, 0, len(records))
	for _, record := range records {
		if !record.CapacityBytes.IsNull() && record.CapacityBytes.ValueInt64() == largest.ValueInt64() {
			filtered = append(filtered, record)
		}
	}
	return filtered
}
