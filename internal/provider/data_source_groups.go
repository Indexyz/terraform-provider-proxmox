package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &GroupsDataSource{}

type GroupsDataSource struct {
	client *Client
}

type GroupsDataSourceModel struct {
	ID     types.String               `tfsdk:"id"`
	Groups []GroupsDataSourceGroupRow `tfsdk:"groups"`
}

type GroupsDataSourceGroupRow struct {
	GroupID types.String `tfsdk:"group_id"`
	Comment types.String `tfsdk:"comment"`
	Users   types.List   `tfsdk:"users"`
}

func NewGroupsDataSource() datasource.DataSource {
	return &GroupsDataSource{}
}

func (d *GroupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_groups"
}

func (d *GroupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists Proxmox VE access groups from `/access/groups`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"groups": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"group_id": schema.StringAttribute{Computed: true},
						"comment":  schema.StringAttribute{Computed: true},
						"users": schema.ListAttribute{
							Computed:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
		},
	}
}

func (d *GroupsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *GroupsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	groups, err := d.client.Groups(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Groups", fmt.Sprintf("Unable to call `/access/groups`: %s", err))
		return
	}

	items := make([]GroupsDataSourceGroupRow, 0, len(groups))
	for _, group := range groups {
		users, diags := stringListValue(ctx, group.Members)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		items = append(items, GroupsDataSourceGroupRow{
			GroupID: types.StringValue(group.GroupID),
			Comment: stringOrNull(group.Comment),
			Users:   users,
		})
	}

	state := GroupsDataSourceModel{
		ID:     types.StringValue("groups"),
		Groups: items,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
