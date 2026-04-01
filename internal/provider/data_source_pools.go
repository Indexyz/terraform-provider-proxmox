package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &PoolsDataSource{}

type PoolsDataSource struct {
	client *Client
}

type PoolsDataSourceModel struct {
	ID    types.String            `tfsdk:"id"`
	Pools []PoolsDataSourceRecord `tfsdk:"pools"`
}

type PoolsDataSourceRecord struct {
	PoolID     types.String               `tfsdk:"pool_id"`
	Comment    types.String               `tfsdk:"comment"`
	VMIDs      types.Set                  `tfsdk:"vm_ids"`
	StorageIDs types.Set                  `tfsdk:"storage_ids"`
	Members    []PoolsDataSourceMemberRow `tfsdk:"members"`
}

type PoolsDataSourceMemberRow struct {
	ID        types.String `tfsdk:"id"`
	Node      types.String `tfsdk:"node"`
	StorageID types.String `tfsdk:"storage_id"`
	Type      types.String `tfsdk:"type"`
	VMID      types.Int64  `tfsdk:"vm_id"`
}

func NewPoolsDataSource() datasource.DataSource {
	return &PoolsDataSource{}
}

func (d *PoolsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pools"
}

func (d *PoolsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists Proxmox VE pools from `/pools`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"pools": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"pool_id":     schema.StringAttribute{Computed: true},
						"comment":     schema.StringAttribute{Computed: true},
						"vm_ids":      schema.SetAttribute{Computed: true, ElementType: types.Int64Type},
						"storage_ids": schema.SetAttribute{Computed: true, ElementType: types.StringType},
						"members": schema.ListNestedAttribute{
							Computed: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"id":         schema.StringAttribute{Computed: true},
									"node":       schema.StringAttribute{Computed: true},
									"storage_id": schema.StringAttribute{Computed: true},
									"type":       schema.StringAttribute{Computed: true},
									"vm_id":      schema.Int64Attribute{Computed: true},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (d *PoolsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PoolsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	pools, err := d.client.Pools(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Pools", fmt.Sprintf("Unable to call `/pools`: %s", err))
		return
	}

	items := make([]PoolsDataSourceRecord, 0, len(pools))
	for _, pool := range pools {
		vmIDs, storageIDs, diags := poolDataSourceValues(ctx, pool)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		members := make([]PoolsDataSourceMemberRow, 0, len(pool.Members))
		for _, member := range pool.Members {
			members = append(members, PoolsDataSourceMemberRow{
				ID:        stringOrNull(member.ID),
				Node:      stringOrNull(member.Node),
				StorageID: stringOrNull(member.Storage),
				Type:      stringOrNull(member.Type),
				VMID:      int64OrNull(member.VMID),
			})
		}

		items = append(items, PoolsDataSourceRecord{
			PoolID:     types.StringValue(pool.PoolID),
			Comment:    stringOrNull(pool.Comment),
			VMIDs:      vmIDs,
			StorageIDs: storageIDs,
			Members:    members,
		})
	}

	state := PoolsDataSourceModel{
		ID:    types.StringValue("pools"),
		Pools: items,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
