// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &RealmDataSource{}

type RealmDataSource struct {
	client *Client
}

type realmDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	Realm         types.String `tfsdk:"realm"`
	Type          types.String `tfsdk:"type"`
	Comment       types.String `tfsdk:"comment"`
	Default       types.Bool   `tfsdk:"default"`
	Server1       types.String `tfsdk:"server1"`
	Server2       types.String `tfsdk:"server2"`
	Port          types.Int64  `tfsdk:"port"`
	Mode          types.String `tfsdk:"mode"`
	Verify        types.Bool   `tfsdk:"verify"`
	CAPath        types.String `tfsdk:"ca_path"`
	BaseDN        types.String `tfsdk:"base_dn"`
	UserAttr      types.String `tfsdk:"user_attr"`
	Domain        types.String `tfsdk:"domain"`
	BindDN        types.String `tfsdk:"bind_dn"`
	IssuerURL     types.String `tfsdk:"issuer_url"`
	ClientID      types.String `tfsdk:"client_id"`
	Autocreate    types.Bool   `tfsdk:"autocreate"`
	UsernameClaim types.String `tfsdk:"username_claim"`
	Scopes        types.String `tfsdk:"scopes"`
	Prompt        types.String `tfsdk:"prompt"`
	QueryUserinfo types.Bool   `tfsdk:"query_userinfo"`
	ACRValues     types.String `tfsdk:"acr_values"`
	Audiences     types.String `tfsdk:"audiences"`
}

func NewRealmDataSource() datasource.DataSource {
	return &RealmDataSource{}
}

func (d *RealmDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_realm"
}

func (d *RealmDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads a Proxmox VE authentication realm from `/access/domains/{realm}`. Built-in `pam` and `pve` realms are supported for read-only lookup.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":             datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Realm identifier (same as `realm`)."},
			"realm":          datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Authentication realm identifier to look up."},
			"type":           datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Realm type: `ldap`, `ad`, `openid`, `pam`, or `pve`."},
			"comment":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Realm description shown in the Proxmox login interface."},
			"default":        datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this is the cluster default realm."},
			"server1":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Primary LDAP or Active Directory server."},
			"server2":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Fallback LDAP or Active Directory server."},
			"port":           datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "LDAP or Active Directory server port."},
			"mode":           datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "LDAP protocol mode."},
			"verify":         datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the LDAP server TLS certificate is verified."},
			"ca_path":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Path on the Proxmox cluster to the LDAP CA certificate file or directory."},
			"base_dn":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "LDAP base distinguished name."},
			"user_attr":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "LDAP attribute containing the username."},
			"domain":         datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Active Directory domain name."},
			"bind_dn":        datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "LDAP bind distinguished name."},
			"issuer_url":     datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "OpenID Connect issuer URL."},
			"client_id":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "OpenID Connect client identifier."},
			"autocreate":     datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether successful OpenID logins automatically create Proxmox users."},
			"username_claim": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "OpenID claim used as the username."},
			"scopes":         datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Space-separated OpenID scopes."},
			"prompt":         datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "OpenID authorization prompt value."},
			"query_userinfo": datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Proxmox queries the OpenID userinfo endpoint for claim values."},
			"acr_values":     datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "OpenID Authentication Context Class Reference values."},
			"audiences":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Additional OpenID audiences accepted alongside `client_id` (Proxmox VE 9)."},
		},
	}
}

func (d *RealmDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RealmDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config realmDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, diags := d.readState(ctx, config.Realm.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (d *RealmDataSource) readState(ctx context.Context, realm string) (realmDataSourceModel, diag.Diagnostics) {
	config, err := d.client.GetRealm(ctx, realm)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox Realm", err.Error())
		return realmDataSourceModel{}, diags
	}
	return realmDataSourceStateFromAPI(config), nil
}

func realmDataSourceStateFromAPI(config Realm) realmDataSourceModel {
	return realmDataSourceModel{
		ID:            types.StringValue(config.Realm),
		Realm:         types.StringValue(config.Realm),
		Type:          types.StringValue(config.Type),
		Comment:       stringOrNull(config.Comment),
		Default:       boolOrNull(config.Default.Ptr()),
		Server1:       stringOrNull(config.Server1),
		Server2:       stringOrNull(config.Server2),
		Port:          int64OrNull(config.Port.Ptr()),
		Mode:          stringOrNull(config.Mode),
		Verify:        boolOrNull(config.Verify.Ptr()),
		CAPath:        stringOrNull(config.CAPath),
		BaseDN:        stringOrNull(config.BaseDN),
		UserAttr:      stringOrNull(config.UserAttr),
		Domain:        stringOrNull(config.Domain),
		BindDN:        stringOrNull(config.BindDN),
		IssuerURL:     stringOrNull(config.IssuerURL),
		ClientID:      stringOrNull(config.ClientID),
		Autocreate:    boolOrNull(config.Autocreate.Ptr()),
		UsernameClaim: stringOrNull(config.UsernameClaim),
		Scopes:        stringOrNull(config.Scopes),
		Prompt:        stringOrNull(config.Prompt),
		QueryUserinfo: boolOrNull(config.QueryUserinfo.Ptr()),
		ACRValues:     stringOrNull(config.ACRValues),
		Audiences:     stringOrNull(config.Audiences),
	}
}
