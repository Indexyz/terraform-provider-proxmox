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

var _ datasource.DataSource = &UsersDataSource{}

type UsersDataSource struct {
	client *Client
}

type UsersDataSourceModel struct {
	ID    types.String            `tfsdk:"id"`
	Users []UsersDataSourceRecord `tfsdk:"users"`
}

type UsersDataSourceRecord struct {
	UserID    types.String `tfsdk:"user_id"`
	Comment   types.String `tfsdk:"comment"`
	Email     types.String `tfsdk:"email"`
	Enable    types.Bool   `tfsdk:"enable"`
	Expire    types.Int64  `tfsdk:"expire"`
	Firstname types.String `tfsdk:"firstname"`
	Lastname  types.String `tfsdk:"lastname"`
	Groups    types.String `tfsdk:"groups"`
}

func NewUsersDataSource() datasource.DataSource {
	return &UsersDataSource{}
}

func (d *UsersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_users"
}

func (d *UsersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Fetches the list of all Proxmox VE users from `/access/users`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":    datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"users": usersDataSourceAttribute(),
		},
	}
}

func usersDataSourceAttribute() datasourceschema.ListNestedAttribute {
	return datasourceschema.ListNestedAttribute{
		Computed: true,
		NestedObject: datasourceschema.NestedAttributeObject{
			Attributes: map[string]datasourceschema.Attribute{
				"user_id":   datasourceschema.StringAttribute{Computed: true},
				"comment":   datasourceschema.StringAttribute{Computed: true},
				"email":     datasourceschema.StringAttribute{Computed: true},
				"enable":    datasourceschema.BoolAttribute{Computed: true},
				"expire":    datasourceschema.Int64Attribute{Computed: true},
				"firstname": datasourceschema.StringAttribute{Computed: true},
				"lastname":  datasourceschema.StringAttribute{Computed: true},
				"groups":    datasourceschema.StringAttribute{Computed: true},
			},
		},
	}
}

func (d *UsersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *UsersDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	users, err := d.client.Users(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Users", err.Error())
		return
	}
	records := make([]UsersDataSourceRecord, 0, len(users))
	for _, u := range users {
		records = append(records, UsersDataSourceRecord{
			UserID:    stringOrNull(u.UserID),
			Comment:   stringOrNull(u.Comment),
			Email:     stringOrNull(u.Email),
			Enable:    boolOrNull(u.Enable.Ptr()),
			Expire:    int64OrNull(u.Expire.Ptr()),
			Firstname: stringOrNull(u.Firstname),
			Lastname:  stringOrNull(u.Lastname),
			Groups:    stringOrNull(u.Groups),
		})
	}
	state := UsersDataSourceModel{
		ID:    types.StringValue(fmt.Sprintf("users-%d", len(records))),
		Users: records,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
