// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &UserDataSource{}

type UserDataSource struct {
	client *Client
}

type userDataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	UserID    types.String `tfsdk:"user_id"`
	Comment   types.String `tfsdk:"comment"`
	Email     types.String `tfsdk:"email"`
	Enable    types.Bool   `tfsdk:"enable"`
	Expire    types.Int64  `tfsdk:"expire"`
	Firstname types.String `tfsdk:"firstname"`
	Lastname  types.String `tfsdk:"lastname"`
	Groups    types.String `tfsdk:"groups"`
}

func NewUserDataSource() datasource.DataSource {
	return &UserDataSource{}
}

func (d *UserDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *UserDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Fetches details of a single Proxmox VE user from `/access/users/{userid}`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "User identifier."},
			"user_id":   datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Full User ID in `name@realm` format."},
			"comment":   datasourceschema.StringAttribute{Computed: true},
			"email":     datasourceschema.StringAttribute{Computed: true},
			"enable":    datasourceschema.BoolAttribute{Computed: true},
			"expire":    datasourceschema.Int64Attribute{Computed: true},
			"firstname": datasourceschema.StringAttribute{Computed: true},
			"lastname":  datasourceschema.StringAttribute{Computed: true},
			"groups":    datasourceschema.StringAttribute{Computed: true},
		},
	}
}

func (d *UserDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *UserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state userDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	user, err := d.client.GetUser(ctx, state.UserID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox User", err.Error())
		return
	}
	state.ID = types.StringValue(user.UserID)
	state.Comment = stringOrNull(user.Comment)
	state.Email = stringOrNull(user.Email)
	state.Enable = boolOrNull(user.Enable.Ptr())
	state.Expire = int64OrNull(user.Expire.Ptr())
	state.Firstname = stringOrNull(user.Firstname)
	state.Lastname = stringOrNull(user.Lastname)
	state.Groups = stringOrNull(user.Groups)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
