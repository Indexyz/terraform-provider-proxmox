// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource = &OpenIDAuthURLDataSource{}
	_ datasource.DataSource = &OpenIDLoginDataSource{}
	_ datasource.DataSource = &GuestMTunnelDataSource{}
)

type OpenIDAuthURLDataSource struct{ client *Client }
type OpenIDLoginDataSource struct{ client *Client }
type GuestMTunnelDataSource struct{ client *Client }

type openIDAuthURLModel struct {
	Realm       types.String `tfsdk:"realm"`
	RedirectURL types.String `tfsdk:"redirect_url"`
	URL         types.String `tfsdk:"url"`
}
type openIDLoginModel struct {
	Code                types.String `tfsdk:"code"`
	RedirectURL         types.String `tfsdk:"redirect_url"`
	State               types.String `tfsdk:"state"`
	Ticket              types.String `tfsdk:"ticket"`
	CSRFPreventionToken types.String `tfsdk:"csrf_prevention_token"`
	Username            types.String `tfsdk:"username"`
	Cap                 types.String `tfsdk:"cap"`
	Clustername         types.String `tfsdk:"clustername"`
}
type guestMTunnelModel struct {
	Type     types.String `tfsdk:"type"`
	Node     types.String `tfsdk:"node"`
	VMID     types.Int64  `tfsdk:"vmid"`
	Bridges  types.List   `tfsdk:"bridges"`
	Storages types.List   `tfsdk:"storages"`
	Socket   types.String `tfsdk:"socket"`
	Ticket   types.String `tfsdk:"ticket"`
	UPID     types.String `tfsdk:"upid"`
}

func NewOpenIDAuthURLDataSource() datasource.DataSource { return &OpenIDAuthURLDataSource{} }
func NewOpenIDLoginDataSource() datasource.DataSource   { return &OpenIDLoginDataSource{} }
func NewGuestMTunnelDataSource() datasource.DataSource  { return &GuestMTunnelDataSource{} }

func (d *OpenIDAuthURLDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_openid_auth_url"
}
func (d *OpenIDLoginDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_openid_login"
}
func (d *GuestMTunnelDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_mtunnel"
}

func (d *OpenIDAuthURLDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Builds an OpenID authorization URL through `/access/openid/auth-url`.", Attributes: map[string]datasourceschema.Attribute{
		"realm": datasourceschema.StringAttribute{Required: true}, "redirect_url": datasourceschema.StringAttribute{Required: true},
		"url": datasourceschema.StringAttribute{Computed: true},
	}}
}
func (d *OpenIDLoginDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Exchanges an OpenID authorization code through `/access/openid/login`.", Attributes: map[string]datasourceschema.Attribute{
		"code": datasourceschema.StringAttribute{Required: true}, "redirect_url": datasourceschema.StringAttribute{Required: true},
		"state":                 datasourceschema.StringAttribute{Required: true},
		"ticket":                datasourceschema.StringAttribute{Computed: true, Sensitive: true},
		"csrf_prevention_token": datasourceschema.StringAttribute{Computed: true, Sensitive: true},
		"username":              datasourceschema.StringAttribute{Computed: true},
		"cap":                   datasourceschema.StringAttribute{Computed: true},
		"clustername":           datasourceschema.StringAttribute{Computed: true},
	}}
}
func (d *GuestMTunnelDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Opens a migration tunnel through `/nodes/{node}/{type}/{vmid}/mtunnel`.", Attributes: map[string]datasourceschema.Attribute{
		"type": datasourceschema.StringAttribute{Required: true}, "node": datasourceschema.StringAttribute{Required: true},
		"vmid":     datasourceschema.Int64Attribute{Required: true},
		"bridges":  datasourceschema.ListAttribute{Optional: true, ElementType: types.StringType},
		"storages": datasourceschema.ListAttribute{Optional: true, ElementType: types.StringType},
		"socket":   datasourceschema.StringAttribute{Computed: true}, "ticket": datasourceschema.StringAttribute{Computed: true, Sensitive: true},
		"upid": datasourceschema.StringAttribute{Computed: true},
	}}
}

func (d *OpenIDAuthURLDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *OpenIDLoginDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *GuestMTunnelDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *OpenIDAuthURLDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config openIDAuthURLModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	form := url.Values{"realm": {config.Realm.ValueString()}, "redirect-url": {config.RedirectURL.ValueString()}}
	var authURL string
	if err := d.client.do(ctx, http.MethodPost, "/access/openid/auth-url", nil, form, &authURL); err != nil {
		resp.Diagnostics.AddError("Unable to Build OpenID Auth URL", err.Error())
		return
	}
	config.URL = types.StringValue(authURL)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *OpenIDLoginDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config openIDLoginModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	form := url.Values{"code": {config.Code.ValueString()}, "redirect-url": {config.RedirectURL.ValueString()}, "state": {config.State.ValueString()}}
	var result struct {
		Ticket      string         `json:"ticket"`
		CSRF        string         `json:"CSRFPreventionToken"`
		Username    string         `json:"username"`
		Cap         map[string]any `json:"cap"`
		Clustername string         `json:"clustername"`
	}
	if err := d.client.do(ctx, http.MethodPost, "/access/openid/login", nil, form, &result); err != nil {
		resp.Diagnostics.AddError("Unable to Complete OpenID Login", err.Error())
		return
	}
	capJSON, _ := json.Marshal(result.Cap)
	config.Ticket = types.StringValue(result.Ticket)
	config.CSRFPreventionToken = types.StringValue(result.CSRF)
	config.Username = types.StringValue(result.Username)
	config.Cap = types.StringValue(string(capJSON))
	config.Clustername = types.StringValue(result.Clustername)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *GuestMTunnelDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config guestMTunnelModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	form := url.Values{}
	if !config.Bridges.IsNull() {
		for _, item := range config.Bridges.Elements() {
			value, ok := item.(types.String)
			if !ok {
				continue
			}
			form.Add("bridges", value.ValueString())
		}
	}
	if !config.Storages.IsNull() {
		for _, item := range config.Storages.Elements() {
			value, ok := item.(types.String)
			if !ok {
				continue
			}
			form.Add("storages", value.ValueString())
		}
	}
	var result struct {
		Socket string `json:"socket"`
		Ticket string `json:"ticket"`
		UPID   string `json:"upid"`
	}
	apiPath := "/nodes/" + url.PathEscape(config.Node.ValueString()) + "/" + config.Type.ValueString() + "/" + strconv.FormatInt(config.VMID.ValueInt64(), 10) + "/mtunnel"
	if err := d.client.do(ctx, http.MethodPost, apiPath, nil, form, &result); err != nil {
		resp.Diagnostics.AddError("Unable to Open Migration Tunnel", err.Error())
		return
	}
	config.Socket = types.StringValue(result.Socket)
	config.Ticket = types.StringValue(result.Ticket)
	config.UPID = types.StringValue(result.UPID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

var _ = strings.TrimSpace
