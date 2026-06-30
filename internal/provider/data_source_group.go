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

var _ datasource.DataSource = &GroupDataSource{}

type GroupDataSource struct {
	client *Client
}

type GroupDataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	GroupID types.String `tfsdk:"group_id"`
	Comment types.String `tfsdk:"comment"`
	Members types.List   `tfsdk:"members"`
}

func NewGroupDataSource() datasource.DataSource {
	return &GroupDataSource{}
}

func (d *GroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (d *GroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a Proxmox VE access group from `/access/groups/{groupid}`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Proxmox group identifier.",
			},
			"comment": schema.StringAttribute{Computed: true},
			"members": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *GroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *GroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config GroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := d.client.GetGroup(ctx, config.GroupID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Group", fmt.Sprintf("Unable to call `/access/groups/%s`: %s", config.GroupID.ValueString(), err))
		return
	}

	members, diags := stringListValue(ctx, group.Members)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state := GroupDataSourceModel{
		ID:      types.StringValue(group.GroupID),
		GroupID: types.StringValue(group.GroupID),
		Comment: stringOrNull(group.Comment),
		Members: members,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
