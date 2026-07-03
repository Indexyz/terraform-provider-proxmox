// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &RoleDataSource{}

type RoleDataSource struct {
	client *Client
}

type roleDataSourceModel struct {
	ID     types.String `tfsdk:"id"`
	RoleID types.String `tfsdk:"role_id"`
	Privs  types.String `tfsdk:"privs"`
}

func NewRoleDataSource() datasource.DataSource {
	return &RoleDataSource{}
}

func (d *RoleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (d *RoleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Fetches details of a single Proxmox VE access role from `/access/roles/{roleid}`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Role identifier."},
			"role_id": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Proxmox role identifier."},
			"privs":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Comma-separated list of privileges."},
		},
	}
}

func (d *RoleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RoleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state roleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	role, err := d.client.GetRole(ctx, state.RoleID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Role", err.Error())
		return
	}
	state.ID = types.StringValue(role.RoleID)
	state.Privs = stringOrNull(role.Privs)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
