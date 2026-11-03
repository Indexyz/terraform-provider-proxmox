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

var _ datasource.DataSource = &SDNIPAMDataSource{}

type SDNIPAMDataSource struct {
	client *Client
}

type sdnIPAMDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	IPAM        types.String `tfsdk:"ipam"`
	Type        types.String `tfsdk:"type"`
	URL         types.String `tfsdk:"url"`
	Section     types.Int64  `tfsdk:"section"`
	Fingerprint types.String `tfsdk:"fingerprint"`
	State       types.String `tfsdk:"state"`
	Pending     types.Map    `tfsdk:"pending"`
}

func NewSDNIPAMDataSource() datasource.DataSource { return &SDNIPAMDataSource{} }

func (d *SDNIPAMDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_ipam"
}

func (d *SDNIPAMDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads one staged SDN IPAM from `/cluster/sdn/ipams`. The API token is not exposed, only its fingerprint.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":          datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "IPAM identifier."},
			"ipam":        datasourceschema.StringAttribute{Required: true, MarkdownDescription: "IPAM to read."},
			"type":        datasourceschema.StringAttribute{Computed: true},
			"url":         datasourceschema.StringAttribute{Computed: true},
			"section":     datasourceschema.Int64Attribute{Computed: true},
			"fingerprint": datasourceschema.StringAttribute{Computed: true},
			"state":       datasourceschema.StringAttribute{Computed: true},
			"pending":     datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *SDNIPAMDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SDNIPAMDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnIPAMDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := config.IPAM.ValueString()
	entry, err := d.client.GetSDNIPAM(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN IPAM", fmt.Sprintf("Unable to call `/cluster/sdn/ipams`: %s", err))
		return
	}
	staged := sdnPendingMap(entry.Pending)
	pending := types.MapNull(types.StringType)
	if len(staged) > 0 {
		value, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		resp.Diagnostics.Append(pendingDiags...)
		pending = value
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnIPAMDataSourceModel{
		ID:          types.StringValue(name),
		IPAM:        types.StringValue(entry.IPAM),
		Type:        sdnStagedString(entry.Type, staged["type"]),
		URL:         sdnStagedString(entry.URL, staged["url"]),
		Section:     sdnStagedInt64(entry.Section.Ptr(), staged["section"]),
		Fingerprint: sdnStagedString(entry.Fingerprint, staged["fingerprint"]),
		State:       stringOrNull(entry.State),
		Pending:     pending,
	})...)
}
