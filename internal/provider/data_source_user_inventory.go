// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/url"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &UserTokensDataSource{}
var _ datasource.DataSourceWithValidateConfig = &UserTokensDataSource{}
var _ datasource.DataSource = &UserTFAEntriesDataSource{}
var _ datasource.DataSourceWithValidateConfig = &UserTFAEntriesDataSource{}

type UserTokensDataSource struct{ client *Client }
type UserTFAEntriesDataSource struct{ client *Client }

type userTokensModel struct {
	UserID types.String       `tfsdk:"user_id"`
	Tokens []userTokensRecord `tfsdk:"tokens"`
}

type userTokensRecord struct {
	TokenID types.String `tfsdk:"token_id"`
	Comment types.String `tfsdk:"comment"`
	Expire  types.Int64  `tfsdk:"expire"`
	PrivSep types.Bool   `tfsdk:"privsep"`
}

type userTokenEntry struct {
	TokenID string               `json:"tokenid"`
	Comment string               `json:"comment"`
	Expire  proxmoxOptionalInt64 `json:"expire"`
	PrivSep proxmoxOptionalBool  `json:"privsep"`
}

type userTFAEntriesModel struct {
	UserID  types.String           `tfsdk:"user_id"`
	Entries []userTFAEntriesRecord `tfsdk:"entries"`
}

type userTFAEntriesRecord struct {
	EntryID     types.String `tfsdk:"entry_id"`
	Type        types.String `tfsdk:"type"`
	Description types.String `tfsdk:"description"`
	Created     types.Int64  `tfsdk:"created"`
	Enable      types.Bool   `tfsdk:"enable"`
}

type userTFAEntry struct {
	EntryID     string              `json:"id"`
	Type        string              `json:"type"`
	Description string              `json:"description"`
	Created     int64               `json:"created"`
	Enable      proxmoxOptionalBool `json:"enable"`
}

func NewUserTokensDataSource() datasource.DataSource     { return &UserTokensDataSource{} }
func NewUserTFAEntriesDataSource() datasource.DataSource { return &UserTFAEntriesDataSource{} }

func (d *UserTokensDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_tokens"
}

func (d *UserTFAEntriesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_tfa_entries"
}

func (d *UserTokensDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists the API tokens of one user from `/access/users/{userid}/token`, sorted by token id. Token secrets are never exposed.",
		Attributes: map[string]datasourceschema.Attribute{
			"user_id": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "User in `name@realm` form."},
			"tokens": datasourceschema.ListNestedAttribute{
				Computed: true,
				NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
					"token_id": datasourceschema.StringAttribute{Computed: true},
					"comment":  datasourceschema.StringAttribute{Computed: true},
					"expire":   datasourceschema.Int64Attribute{Computed: true},
					"privsep":  datasourceschema.BoolAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *UserTFAEntriesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists the TFA entries of one user from `/access/tfa/{userid}`, sorted by creation time. Secrets are not exposed.",
		Attributes: map[string]datasourceschema.Attribute{
			"user_id": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "User in `name@realm` form."},
			"entries": datasourceschema.ListNestedAttribute{
				Computed: true,
				NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
					"entry_id":    datasourceschema.StringAttribute{Computed: true},
					"type":        datasourceschema.StringAttribute{Computed: true},
					"description": datasourceschema.StringAttribute{Computed: true},
					"created":     datasourceschema.Int64Attribute{Computed: true},
					"enable":      datasourceschema.BoolAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *UserTokensDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *UserTFAEntriesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *UserTokensDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config userTokensModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if !resp.Diagnostics.HasError() {
		validateUserInventoryConfig(config.UserID, resp)
	}
}

func (d *UserTFAEntriesDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config userTFAEntriesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if !resp.Diagnostics.HasError() {
		validateUserInventoryConfig(config.UserID, resp)
	}
}

func validateUserInventoryConfig(userID types.String, resp *datasource.ValidateConfigResponse) {
	if isConfiguredValue(userID) && !userIDPattern.MatchString(userID.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("user_id"), "Invalid user id", "user_id must use the name@realm form.")
	}
}

func (d *UserTokensDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config userTokensModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	userID := config.UserID.ValueString()
	var entries []userTokenEntry
	if err := d.client.do(ctx, "GET", "/access/users/"+url.PathEscape(userID)+"/token", nil, nil, &entries); err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox User Tokens", fmt.Sprintf("Unable to call `/access/users/%s/token`: %s", userID, err))
		return
	}
	records := make([]userTokensRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, userTokensRecord{TokenID: types.StringValue(entry.TokenID), Comment: stringOrNull(entry.Comment), Expire: int64OrNull(entry.Expire.Ptr()), PrivSep: nodesBoolOrNull(entry.PrivSep)})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].TokenID.ValueString() < records[j].TokenID.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &userTokensModel{UserID: config.UserID, Tokens: records})...)
}

func (d *UserTFAEntriesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config userTFAEntriesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	userID := config.UserID.ValueString()
	var entries []userTFAEntry
	if err := d.client.do(ctx, "GET", "/access/tfa/"+url.PathEscape(userID), nil, nil, &entries); err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox User TFA Entries", fmt.Sprintf("Unable to call `/access/tfa/%s`: %s", userID, err))
		return
	}
	records := make([]userTFAEntriesRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, userTFAEntriesRecord{
			EntryID:     types.StringValue(entry.EntryID),
			Type:        types.StringValue(entry.Type),
			Description: stringOrNull(entry.Description),
			Created:     int64OrNull(optionalPositive(entry.Created)),
			Enable:      nodesBoolOrNull(entry.Enable),
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Created.ValueInt64() < records[j].Created.ValueInt64() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &userTFAEntriesModel{UserID: config.UserID, Entries: records})...)
}
