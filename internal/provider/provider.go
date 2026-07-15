// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	defaultTimeoutSeconds = 30

	envEndpoint       = "PROXMOX_VE_ENDPOINT"
	envUsername       = "PROXMOX_VE_USERNAME"
	envPassword       = "PROXMOX_VE_PASSWORD"
	envOTP            = "PROXMOX_VE_OTP"
	envAPITokenID     = "PROXMOX_VE_API_TOKEN_ID"
	envAPITokenSecret = "PROXMOX_VE_API_TOKEN_SECRET"
	envInsecure       = "PROXMOX_VE_INSECURE"
	envTimeout        = "PROXMOX_VE_TIMEOUT"
)

var _ provider.Provider = &ProxmoxProvider{}

type ProxmoxProvider struct {
	version string
}

type ProxmoxProviderModel struct {
	Endpoint       types.String `tfsdk:"endpoint"`
	Username       types.String `tfsdk:"username"`
	Password       types.String `tfsdk:"password"`
	OTP            types.String `tfsdk:"otp"`
	APITokenID     types.String `tfsdk:"api_token_id"`
	APITokenSecret types.String `tfsdk:"api_token_secret"`
	Insecure       types.Bool   `tfsdk:"insecure"`
	TimeoutSeconds types.Int64  `tfsdk:"timeout_seconds"`
	UserAgent      types.String `tfsdk:"user_agent"`
}

func (p *ProxmoxProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "proxmox"
	resp.Version = p.version
}

func (p *ProxmoxProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Terraform provider for Proxmox VE.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: fmt.Sprintf("Proxmox VE API endpoint. Accepts a base URL such as `https://pve.example.com:8006` and auto-appends `/api2/json` if needed. Can also be set with `%s`.", envEndpoint),
			},
			"username": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: fmt.Sprintf("Proxmox user ID for ticket-based authentication, for example `root@pam`. Can also be set with `%s`.", envUsername),
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: fmt.Sprintf("Password for ticket-based authentication. Can also be set with `%s`.", envPassword),
			},
			"otp": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: fmt.Sprintf("One-time password used with ticket-based authentication. Can also be set with `%s`.", envOTP),
			},
			"api_token_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: fmt.Sprintf("Full Proxmox API token identifier in the form `user@realm!tokenid`. Can also be set with `%s`.", envAPITokenID),
			},
			"api_token_secret": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: fmt.Sprintf("Proxmox API token secret. Can also be set with `%s`.", envAPITokenSecret),
			},
			"insecure": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: fmt.Sprintf("Disable TLS certificate verification for the Proxmox endpoint. Can also be set with `%s`.", envInsecure),
			},
			"timeout_seconds": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: fmt.Sprintf("HTTP request timeout in seconds. Defaults to `%d`. Can also be set with `%s`.", defaultTimeoutSeconds, envTimeout),
			},
			"user_agent": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional custom HTTP user agent string sent to the Proxmox API.",
			},
		},
	}
}

func (p *ProxmoxProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data ProxmoxProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg, diags := providerConfigFromModel(data, p.version)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := NewClient(ctx, cfg)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Configure Proxmox Client",
			err.Error(),
		)
		return
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *ProxmoxProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewACLResource,
		NewClusterFirewallOptionsResource,
		NewFirewallRuleResource,
		NewGroupResource,
		NewGuestFirewallOptionsResource,
		NewLXCContainerResource,
		NewLXCSnapshotResource,
		NewNodeFirewallOptionsResource,
		NewPoolResource,
		NewQemuSnapshotResource,
		NewQemuVMResource,
		NewRoleResource,
		NewStorageResource,
		NewUserResource,
		NewUserTokenResource,
	}
}

func (p *ProxmoxProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewClusterMetricsServersDataSource,
		NewGroupDataSource,
		NewGroupsDataSource,
		NewLXCContainerDataSource,
		NewPoolDataSource,
		NewPoolsDataSource,
		NewQemuVMDataSource,
		NewRoleDataSource,
		NewRolesDataSource,
		NewNodeDNSDataSource,
		NewNodeTimeDataSource,
		NewStorageDataSource,
		NewStoragesDataSource,
		NewUserDataSource,
		NewUsersDataSource,
		NewVersionDataSource,
		NewNodesDataSource,
		NewNodeDataSource,
		NewClusterResourcesDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &ProxmoxProvider{version: version}
	}
}

func providerConfigFromModel(data ProxmoxProviderModel, version string) (ClientConfig, diag.Diagnostics) {
	var diags diag.Diagnostics

	cfg := ClientConfig{
		Endpoint:       firstNonEmpty(stringValue(data.Endpoint), os.Getenv(envEndpoint)),
		Username:       firstNonEmpty(stringValue(data.Username), os.Getenv(envUsername)),
		Password:       firstNonEmpty(stringValue(data.Password), os.Getenv(envPassword)),
		OTP:            firstNonEmpty(stringValue(data.OTP), os.Getenv(envOTP)),
		APITokenID:     firstNonEmpty(stringValue(data.APITokenID), os.Getenv(envAPITokenID)),
		APITokenSecret: firstNonEmpty(stringValue(data.APITokenSecret), os.Getenv(envAPITokenSecret)),
		Insecure:       boolValue(data.Insecure, envInsecure),
		Timeout:        time.Duration(int64Value(data.TimeoutSeconds, envTimeout, defaultTimeoutSeconds)) * time.Second,
		UserAgent:      firstNonEmpty(stringValue(data.UserAgent), fmt.Sprintf("terraform-provider-proxmox/%s", version)),
	}

	if cfg.Endpoint == "" {
		diags.AddError(
			"Missing Proxmox Endpoint",
			fmt.Sprintf("Set `endpoint` in the provider configuration or export `%s`.", envEndpoint),
		)
	}

	tokenAuthConfigured := cfg.APITokenID != "" || cfg.APITokenSecret != ""
	passwordAuthConfigured := cfg.Username != "" || cfg.Password != ""

	switch {
	case tokenAuthConfigured && passwordAuthConfigured:
		diags.AddError(
			"Conflicting Authentication Settings",
			"Configure either `username`/`password` authentication or `api_token_id`/`api_token_secret` authentication, not both.",
		)
	case tokenAuthConfigured:
		if cfg.APITokenID == "" || cfg.APITokenSecret == "" {
			diags.AddError(
				"Incomplete API Token Authentication Settings",
				"Both `api_token_id` and `api_token_secret` must be set when using API token authentication.",
			)
		}
	case passwordAuthConfigured:
		if cfg.Username == "" || cfg.Password == "" {
			diags.AddError(
				"Incomplete Ticket Authentication Settings",
				"Both `username` and `password` must be set when using ticket-based authentication.",
			)
		}
	default:
		diags.AddError(
			"Missing Authentication Settings",
			fmt.Sprintf("Set either `username`/`password` or `api_token_id`/`api_token_secret`. Environment variables `%s`, `%s`, `%s`, and `%s` are also supported.", envUsername, envPassword, envAPITokenID, envAPITokenSecret),
		)
	}

	return cfg, diags
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringValue(value types.String) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return value.ValueString()
}

func boolValue(value types.Bool, envName string) bool {
	if !value.IsNull() && !value.IsUnknown() {
		return value.ValueBool()
	}

	envValue := strings.TrimSpace(os.Getenv(envName))
	if envValue == "" {
		return false
	}

	parsed, err := strconv.ParseBool(envValue)
	if err != nil {
		return false
	}

	return parsed
}

func int64Value(value types.Int64, envName string, fallback int64) int64 {
	if !value.IsNull() && !value.IsUnknown() {
		return value.ValueInt64()
	}

	envValue := strings.TrimSpace(os.Getenv(envName))
	if envValue == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(envValue, 10, 64)
	if err != nil {
		return fallback
	}

	return parsed
}
