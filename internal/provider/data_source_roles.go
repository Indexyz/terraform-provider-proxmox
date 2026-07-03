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

var _ datasource.DataSource = &RolesDataSource{}

type RolesDataSource struct {
	client *Client
}

type RolesDataSourceModel struct {
	ID    types.String            `tfsdk:"id"`
	Roles []RolesDataSourceRecord `tfsdk:"roles"`
}

type RolesDataSourceRecord struct {
	RoleID types.String `tfsdk:"role_id"`
	Privs  types.String `tfsdk:"privs"`
}

func NewRolesDataSource() datasource.DataSource {
	return &RolesDataSource{}
}

func (d *RolesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_roles"
}

func (d *RolesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Fetches the list of all Proxmox VE access roles from `/access/roles`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":    datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"roles": rolesDataSourceAttribute(),
		},
	}
}

func rolesDataSourceAttribute() datasourceschema.ListNestedAttribute {
	return datasourceschema.ListNestedAttribute{
		Computed: true,
		NestedObject: datasourceschema.NestedAttributeObject{
			Attributes: map[string]datasourceschema.Attribute{
				"role_id": datasourceschema.StringAttribute{Computed: true},
				"privs":   datasourceschema.StringAttribute{Computed: true},
			},
		},
	}
}

func (d *RolesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RolesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	roles, err := d.client.Roles(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Roles", err.Error())
		return
	}
	records := make([]RolesDataSourceRecord, 0, len(roles))
	for _, r := range roles {
		records = append(records, RolesDataSourceRecord{
			RoleID: stringOrNull(r.RoleID),
			Privs:  stringOrNull(r.Privs),
		})
	}
	state := RolesDataSourceModel{
		ID:    types.StringValue(fmt.Sprintf("roles-%d", len(records))),
		Roles: records,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
