package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &VersionDataSource{}

type VersionDataSource struct {
	client *Client
}

type VersionDataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	Console types.String `tfsdk:"console"`
	Release types.String `tfsdk:"release"`
	RepoID  types.String `tfsdk:"repoid"`
	Version types.String `tfsdk:"version"`
}

func NewVersionDataSource() datasource.DataSource {
	return &VersionDataSource{}
}

func (d *VersionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_version"
}

func (d *VersionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches Proxmox VE cluster version details from `/version`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier for this data source.",
			},
			"console": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Default console viewer configured in Proxmox VE.",
			},
			"release": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current Proxmox VE point release in `x.y` format.",
			},
			"repoid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Short git revision used to build this Proxmox VE version.",
			},
			"version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Full `pve-manager` package version.",
			},
		},
	}
}

func (d *VersionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *VersionDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	version, err := d.client.Version(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Version", fmt.Sprintf("Unable to call `/version`: %s", err))
		return
	}

	state := VersionDataSourceModel{
		ID:      types.StringValue("version"),
		Console: stringOrNull(version.Console),
		Release: stringOrNull(version.Release),
		RepoID:  stringOrNull(version.RepoID),
		Version: stringOrNull(version.Version),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
