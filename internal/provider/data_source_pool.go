// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &PoolDataSource{}

type PoolDataSource struct {
	client *Client
}

type PoolDataSourceModel struct {
	ID         types.String              `tfsdk:"id"`
	PoolID     types.String              `tfsdk:"pool_id"`
	Comment    types.String              `tfsdk:"comment"`
	VMIDs      types.Set                 `tfsdk:"vm_ids"`
	StorageIDs types.Set                 `tfsdk:"storage_ids"`
	Members    []PoolDataSourceMemberRow `tfsdk:"members"`
}

type PoolDataSourceMemberRow struct {
	ID        types.String `tfsdk:"id"`
	Node      types.String `tfsdk:"node"`
	StorageID types.String `tfsdk:"storage_id"`
	Type      types.String `tfsdk:"type"`
	VMID      types.Int64  `tfsdk:"vm_id"`
}

func NewPoolDataSource() datasource.DataSource {
	return &PoolDataSource{}
}

func (d *PoolDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pool"
}

func (d *PoolDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a Proxmox VE pool from `/pools`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"pool_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Pool identifier.",
			},
			"comment": schema.StringAttribute{Computed: true},
			"vm_ids": schema.SetAttribute{
				Computed:    true,
				ElementType: types.Int64Type,
			},
			"storage_ids": schema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
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
	}
}

func (d *PoolDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PoolDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PoolDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pool, err := d.client.GetPool(ctx, config.PoolID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Pool", fmt.Sprintf("Unable to call `/pools?poolid=%s`: %s", config.PoolID.ValueString(), err))
		return
	}

	vmIDs, storageIDs, diags := poolDataSourceValues(ctx, pool)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state := PoolDataSourceModel{
		ID:         types.StringValue(pool.PoolID),
		PoolID:     types.StringValue(pool.PoolID),
		Comment:    stringOrNull(pool.Comment),
		VMIDs:      vmIDs,
		StorageIDs: storageIDs,
		Members:    poolDataSourceMembers(pool),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func poolDataSourceValues(ctx context.Context, pool Pool) (types.Set, types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	vmIDs, storageIDs, _ := flattenPoolMembers(pool)
	vmIDSet, vmDiags := int64SetValue(ctx, vmIDs)
	storageIDSet, storageDiags := stringSetValue(ctx, storageIDs)
	diags.Append(vmDiags...)
	diags.Append(storageDiags...)
	return vmIDSet, storageIDSet, diags
}

func poolDataSourceMembers(pool Pool) []PoolDataSourceMemberRow {
	members := make([]PoolDataSourceMemberRow, 0, len(pool.Members))
	for _, member := range pool.Members {
		members = append(members, PoolDataSourceMemberRow{
			ID:        stringOrNull(member.ID),
			Node:      stringOrNull(member.Node),
			StorageID: stringOrNull(member.Storage),
			Type:      stringOrNull(member.Type),
			VMID:      int64OrNull(member.VMID),
		})
	}
	return members
}
