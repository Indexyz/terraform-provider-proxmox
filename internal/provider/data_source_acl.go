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

var _ datasource.DataSource = &ACLDataSource{}

type ACLDataSource struct {
	client *Client
}

type aclDataSourceModel struct {
	ID      types.String          `tfsdk:"id"`
	Path    types.String          `tfsdk:"path"`
	Type    types.String          `tfsdk:"type"`
	Entries []aclDataSourceRecord `tfsdk:"entries"`
}

type aclDataSourceRecord struct {
	Path      types.String `tfsdk:"path"`
	Type      types.String `tfsdk:"type"`
	UGID      types.String `tfsdk:"ugid"`
	RoleID    types.String `tfsdk:"role_id"`
	Propagate types.Bool   `tfsdk:"propagate"`
}

func NewACLDataSource() datasource.DataSource { return &ACLDataSource{} }

func (d *ACLDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acl"
}

func (d *ACLDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists the effective ACL entries from `/access/acl`, sorted by path, type and identifier. Filter by `path` and/or `type`; both are prefix/exact matches on single fields, not a path prefix match.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"path": datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Restrict the result to one ACL path such as `/vms/100`."},
			"type": datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Restrict the result to `user` or `group` entries."},
			"entries": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching ACL entries, sorted by path, type and identifier.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"path":      datasourceschema.StringAttribute{Computed: true},
						"type":      datasourceschema.StringAttribute{Computed: true},
						"ugid":      datasourceschema.StringAttribute{Computed: true},
						"role_id":   datasourceschema.StringAttribute{Computed: true},
						"propagate": datasourceschema.BoolAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *ACLDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ACLDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config aclDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entries, err := d.client.GetACL(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox ACL", fmt.Sprintf("Unable to call `/access/acl`: %s", err))
		return
	}
	pathFilter := stringValue(config.Path)
	typeFilter := stringValue(config.Type)
	records := make([]aclDataSourceRecord, 0, len(entries))
	for _, entry := range entries {
		if pathFilter != "" && entry.Path != pathFilter {
			continue
		}
		if typeFilter != "" && entry.Type != typeFilter {
			continue
		}
		records = append(records, aclDataSourceRecord{
			Path:      types.StringValue(entry.Path),
			Type:      types.StringValue(entry.Type),
			UGID:      types.StringValue(entry.UGID),
			RoleID:    types.StringValue(entry.RoleID),
			Propagate: nodesBoolOrNull(entry.Propagate),
		})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Path.ValueString() != records[j].Path.ValueString() {
			return records[i].Path.ValueString() < records[j].Path.ValueString()
		}
		if records[i].Type.ValueString() != records[j].Type.ValueString() {
			return records[i].Type.ValueString() < records[j].Type.ValueString()
		}
		return records[i].UGID.ValueString() < records[j].UGID.ValueString()
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &aclDataSourceModel{ID: types.StringValue("acl"), Path: config.Path, Type: config.Type, Entries: records})...)
}
