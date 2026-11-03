// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &RealmsDataSource{}

type RealmsDataSource struct {
	client *Client
}

type RealmsDataSourceModel struct {
	ID     types.String             `tfsdk:"id"`
	Realms []RealmsDataSourceRecord `tfsdk:"realms"`
}

type RealmsDataSourceRecord struct {
	Realm   types.String `tfsdk:"realm"`
	Type    types.String `tfsdk:"type"`
	Comment types.String `tfsdk:"comment"`
	Default types.Bool   `tfsdk:"default"`
	TFA     types.String `tfsdk:"tfa"`
}

func NewRealmsDataSource() datasource.DataSource {
	return &RealmsDataSource{}
}

func (d *RealmsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_realms"
}

func (d *RealmsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists authentication realms from the world-readable `/access/domains` login index, sorted by `realm`. The index only carries public login metadata: no bind password, client key, client certificate or digest. Use the `proxmox_realm` data source for the full configuration of one realm.",
		Attributes: map[string]datasourceschema.Attribute{
			"id": datasourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique identifier for this data source call.",
			},
			"realms": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "All configured realms, including the built-in `pam` and `pve` realms.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"realm":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Realm identifier."},
						"type":    datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Realm type such as `ldap`, `ad`, `openid`, `pam`, or `pve`."},
						"comment": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Login box description, when set."},
						"default": datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the realm is the cluster default. Proxmox reports this flag in the index but does not document it there; treat it as a hint."},
						"tfa":     datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Forced second factor (`oath` or `yubico`), when configured. The index only exposes the factor type, not its parameters."},
					},
				},
			},
		},
	}
}

func (d *RealmsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RealmsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	realms, err := d.client.Realms(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Realms", fmt.Sprintf("Unable to call `/access/domains`: %s", err))
		return
	}

	records := make([]RealmsDataSourceRecord, 0, len(realms))
	for _, realm := range realms {
		records = append(records, RealmsDataSourceRecord{
			Realm:   types.StringValue(realm.Realm),
			Type:    stringOrNull(realm.Type),
			Comment: stringOrNull(realm.Comment),
			Default: boolOrNull(realm.Default.Ptr()),
			TFA:     stringOrNull(realm.TFA),
		})
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Realm.ValueString() < records[j].Realm.ValueString()
	})

	state := RealmsDataSourceModel{
		ID:     types.StringValue("realms"),
		Realms: records,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
