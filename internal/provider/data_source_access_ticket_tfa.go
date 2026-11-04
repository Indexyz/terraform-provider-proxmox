// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource = &AccessTicketDataSource{}
	_ datasource.DataSource = &AccessTFADataSource{}
)

type AccessTicketDataSource struct{ client *Client }
type AccessTFADataSource struct{ client *Client }

type accessTicketDSModel struct {
	ID          types.String `tfsdk:"id"`
	Username    types.String `tfsdk:"username"`
	Password    types.String `tfsdk:"password"`
	OTP         types.String `tfsdk:"otp"`
	Realm       types.String `tfsdk:"realm"`
	Challenge   types.String `tfsdk:"tfa_challenge"`
	NewFormat   types.Bool   `tfsdk:"new_format"`
	Path        types.String `tfsdk:"path"`
	Privs       types.String `tfsdk:"privs"`
	Ticket      types.String `tfsdk:"ticket"`
	CSRF        types.String `tfsdk:"csrf_token"`
	Clustername types.String `tfsdk:"clustername"`
}
type accessTFADSModel struct {
	ID      types.String      `tfsdk:"id"`
	Entries []accessTFARecord `tfsdk:"entries"`
}
type accessTFARecord struct {
	UserID      types.String `tfsdk:"userid"`
	LockedUntil types.Int64  `tfsdk:"tfa_locked_until"`
	TOTPLocked  types.Bool   `tfsdk:"totp_locked"`
	ID          types.String `tfsdk:"id"`
	Type        types.String `tfsdk:"type"`
	Description types.String `tfsdk:"description"`
	Enable      types.Bool   `tfsdk:"enable"`
	Created     types.Int64  `tfsdk:"created"`
}

func NewAccessTicketDataSource() datasource.DataSource { return &AccessTicketDataSource{} }
func NewAccessTFADataSource() datasource.DataSource    { return &AccessTFADataSource{} }

func (d *AccessTicketDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_ticket"
}
func (d *AccessTFADataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_tfa_entries"
}

func (d *AccessTicketDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Authenticates through `POST /access/ticket` and exposes the ticket, CSRF token, and cluster name.", Attributes: map[string]datasourceschema.Attribute{
		"id":       datasourceschema.StringAttribute{Computed: true},
		"username": datasourceschema.StringAttribute{Required: true}, "password": datasourceschema.StringAttribute{Required: true, Sensitive: true},
		"otp": datasourceschema.StringAttribute{Optional: true, Sensitive: true}, "realm": datasourceschema.StringAttribute{Optional: true},
		"tfa_challenge": datasourceschema.StringAttribute{Optional: true, Sensitive: true}, "new_format": datasourceschema.BoolAttribute{Optional: true},
		"path": datasourceschema.StringAttribute{Optional: true}, "privs": datasourceschema.StringAttribute{Optional: true},
		"ticket":      datasourceschema.StringAttribute{Computed: true, Sensitive: true},
		"csrf_token":  datasourceschema.StringAttribute{Computed: true, Sensitive: true},
		"clustername": datasourceschema.StringAttribute{Computed: true},
	}}
}
func (d *AccessTFADataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Lists TFA entries of all users through `GET /access/tfa` (requires `Sys.Audit`).", Attributes: map[string]datasourceschema.Attribute{
		"id": datasourceschema.StringAttribute{Computed: true},
		"entries": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
			"userid": datasourceschema.StringAttribute{Computed: true}, "tfa_locked_until": datasourceschema.Int64Attribute{Computed: true},
			"totp_locked": datasourceschema.BoolAttribute{Computed: true}, "id": datasourceschema.StringAttribute{Computed: true},
			"type": datasourceschema.StringAttribute{Computed: true}, "description": datasourceschema.StringAttribute{Computed: true},
			"enable": datasourceschema.BoolAttribute{Computed: true}, "created": datasourceschema.Int64Attribute{Computed: true},
		}}},
	}}
}

func (d *AccessTicketDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *AccessTFADataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *AccessTicketDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config accessTicketDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	form := url.Values{"username": {config.Username.ValueString()}, "password": {config.Password.ValueString()}}
	for key, value := range map[string]string{
		"otp": stringValue(config.OTP), "realm": stringValue(config.Realm),
		"tfa-challenge": stringValue(config.Challenge), "path": stringValue(config.Path), "privs": stringValue(config.Privs),
	} {
		if value != "" {
			form.Set(key, value)
		}
	}
	if config.NewFormat.ValueBool() {
		form.Set("new-format", "1")
	}
	var result struct {
		CSRF        string `json:"CSRFPreventionToken"`
		Clustername string `json:"clustername"`
		Ticket      string `json:"ticket"`
		Username    string `json:"username"`
	}
	if err := d.client.do(ctx, http.MethodPost, "/access/ticket", nil, form, &result); err != nil {
		resp.Diagnostics.AddError("Unable to Create Access Ticket", err.Error())
		return
	}
	config.ID = types.StringValue(result.Username)
	config.Ticket = stringOrNull(result.Ticket)
	config.CSRF = stringOrNull(result.CSRF)
	config.Clustername = stringOrNull(result.Clustername)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *AccessTFADataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config accessTFADSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var items []struct {
		UserID      string `json:"userid"`
		LockedUntil *int64 `json:"tfa-locked-until"`
		TOTPLocked  bool   `json:"totp-locked"`
		Entries     []struct {
			Created     int64  `json:"created"`
			Description string `json:"description"`
			Enable      bool   `json:"enable"`
			ID          string `json:"id"`
			Type        string `json:"type"`
		} `json:"entries"`
	}
	if err := d.client.do(ctx, http.MethodGet, "/access/tfa", nil, nil, &items); err != nil {
		resp.Diagnostics.AddError("Unable to Read TFA Entries", err.Error())
		return
	}
	records := []accessTFARecord{}
	for _, item := range items {
		for _, entry := range item.Entries {
			records = append(records, accessTFARecord{UserID: types.StringValue(item.UserID), LockedUntil: int64OrNull(item.LockedUntil), TOTPLocked: types.BoolValue(item.TOTPLocked), ID: stringOrNull(entry.ID), Type: stringOrNull(entry.Type), Description: stringOrNull(entry.Description), Enable: types.BoolValue(entry.Enable), Created: types.Int64Value(entry.Created)})
		}
	}
	config.ID = types.StringValue("access_tfa_entries")
	config.Entries = records
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
