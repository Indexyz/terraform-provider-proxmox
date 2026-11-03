// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"net/url"
)

type clusterOptionsModel struct {
	ID                types.String `tfsdk:"id"`
	Description       types.String `tfsdk:"description"`
	Keyboard          types.String `tfsdk:"keyboard"`
	Console           types.String `tfsdk:"console"`
	Language          types.String `tfsdk:"language"`
	EmailFrom         types.String `tfsdk:"email_from"`
	Fencing           types.String `tfsdk:"fencing"`
	Migration         types.String `tfsdk:"migration"`
	MigrationUnsecure types.Bool   `tfsdk:"migration_unsecure"`
	MaxWorkers        types.Int64  `tfsdk:"max_workers"`
	TagStyle          types.String `tfsdk:"tag_style"`
	HA                types.String `tfsdk:"ha"`
	HTTPProxy         types.String `tfsdk:"http_proxy"`
	NextID            types.String `tfsdk:"next_id"`
	MACPrefix         types.String `tfsdk:"mac_prefix"`
}

var _ resource.Resource = &ClusterOptionsResource{}
var _ resource.ResourceWithValidateConfig = &ClusterOptionsResource{}
var _ datasource.DataSource = &ClusterOptionsDataSource{}

type ClusterOptionsResource struct{ client *Client }
type ClusterOptionsDataSource struct{ client *Client }

func NewClusterOptionsResource() resource.Resource       { return &ClusterOptionsResource{} }
func NewClusterOptionsDataSource() datasource.DataSource { return &ClusterOptionsDataSource{} }

func (r *ClusterOptionsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_options"
}

func (d *ClusterOptionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_options"
}

func clusterOptionsAttributes() map[string]schema.Attribute {
	optional := func(text string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: text, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}
	}
	return map[string]schema.Attribute{
		"id":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Always `cluster`.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"description":        optional("Cluster description shown in the GUI."),
		"keyboard":           optional("Default keyboard layout for guests."),
		"console":            optional("Default console viewer: `applet`, `vv`, `html5` or `xtermjs`."),
		"language":           optional("Default GUI language."),
		"email_from":         optional("Sender address for notification mails."),
		"fencing":            optional("Fencing mode: `watchdog`, `hardware` or `both`."),
		"migration":          optional("Migration network and type, for example `secure` or `insecure`."),
		"migration_unsecure": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Allow unencrypted migrations.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
		"max_workers":        schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Maximum concurrent worker tasks.", PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
		"tag_style":          optional("Tag display style for the GUI."),
		"ha":                 optional("HA settings such as the shutdown policy."),
		"http_proxy":         optional("HTTP proxy for downloads and updates."),
		"next_id":            optional("Lower bound for automatically assigned guest VMIDs."),
		"mac_prefix":         optional("MAC address prefix for new guests."),
	}
}

func (r *ClusterOptionsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Manages the cluster-wide options through `/cluster/options`. Only explicitly configured attributes are sent; clearing an attribute removes it from the Proxmox config. Destroy resets nothing.", Attributes: clusterOptionsAttributes()}
}

func (d *ClusterOptionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := map[string]datasourceschema.Attribute{}
	for name, attribute := range clusterOptionsAttributes() {
		if stringAttribute, ok := attribute.(schema.StringAttribute); ok {
			attributes[name] = datasourceschema.StringAttribute{Computed: true, MarkdownDescription: stringAttribute.MarkdownDescription}
		}
	}
	attributes["migration_unsecure"] = datasourceschema.BoolAttribute{Computed: true}
	attributes["max_workers"] = datasourceschema.Int64Attribute{Computed: true}
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Reads the cluster-wide options from `/cluster/options`.", Attributes: attributes}
}

func (r *ClusterOptionsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (d *ClusterOptionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func clusterOptionsFromAPI(entry ClusterOptions) clusterOptionsModel {
	return clusterOptionsModel{
		ID: types.StringValue("cluster"), Description: stringOrNull(strings.TrimRight(entry.Description, "\n")), Keyboard: stringOrNull(entry.Keyboard),
		Console: stringOrNull(entry.Console), Language: stringOrNull(entry.Language), EmailFrom: stringOrNull(entry.EmailFrom),
		Fencing: stringOrNull(entry.Fencing), Migration: stringOrNull(entry.Migration), MigrationUnsecure: nodesBoolOrNull(entry.MigrationUnsecure),
		MaxWorkers: int64OrNull(entry.MaxWorkers.Ptr()), TagStyle: stringOrNull(entry.TagStyle), HA: stringOrNull(entry.HA),
		HTTPProxy: stringOrNull(entry.HTTPProxy), NextID: stringOrNull(entry.NextID), MACPrefix: stringOrNull(entry.MACPrefix),
	}
}

func (r *ClusterOptionsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterOptionsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entry, err := r.client.ClusterOptions(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Cluster Options", err.Error())
		return
	}
	refreshed := clusterOptionsFromAPI(entry)
	for _, pair := range []struct {
		prior  types.String
		target *types.String
	}{
		{state.Description, &refreshed.Description}, {state.Keyboard, &refreshed.Keyboard}, {state.Console, &refreshed.Console},
		{state.Language, &refreshed.Language}, {state.EmailFrom, &refreshed.EmailFrom}, {state.Fencing, &refreshed.Fencing},
		{state.Migration, &refreshed.Migration}, {state.TagStyle, &refreshed.TagStyle}, {state.HA, &refreshed.HA},
		{state.HTTPProxy, &refreshed.HTTPProxy}, {state.NextID, &refreshed.NextID}, {state.MACPrefix, &refreshed.MACPrefix},
	} {
		if refreshed.Description.IsNull() || pair.target.IsNull() {
			if !pair.prior.IsNull() && !pair.prior.IsUnknown() {
				*pair.target = pair.prior
			}
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *ClusterOptionsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterOptionsModel
	var config clusterOptionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, config, &resp.State, &resp.Diagnostics)
}

func (r *ClusterOptionsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan clusterOptionsModel
	var config clusterOptionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, config, &resp.State, &resp.Diagnostics)
}

func (r *ClusterOptionsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Cluster options cannot be reset as a whole.
}

func (r *ClusterOptionsResource) apply(ctx context.Context, config clusterOptionsModel, state *tfsdk.State, diags *diag.Diagnostics) {
	form, deleted := r.buildForm(config)
	if len(deleted) > 0 {
		form.Set("delete", strings.Join(deleted, ","))
	}
	if err := r.client.UpdateClusterOptions(ctx, form); err != nil {
		diags.AddError("Unable to Update Proxmox Cluster Options", err.Error())
		return
	}
	entry, err := r.client.ClusterOptions(ctx)
	if err != nil {
		diags.AddError("Unable to Read Proxmox Cluster Options", err.Error())
		return
	}
	refreshed := clusterOptionsFromAPI(entry)
	for _, pair := range []struct {
		config types.String
		target *types.String
	}{
		{config.Description, &refreshed.Description}, {config.Keyboard, &refreshed.Keyboard}, {config.Console, &refreshed.Console},
		{config.Language, &refreshed.Language}, {config.EmailFrom, &refreshed.EmailFrom}, {config.Fencing, &refreshed.Fencing},
		{config.Migration, &refreshed.Migration}, {config.TagStyle, &refreshed.TagStyle}, {config.HA, &refreshed.HA},
		{config.HTTPProxy, &refreshed.HTTPProxy}, {config.NextID, &refreshed.NextID}, {config.MACPrefix, &refreshed.MACPrefix},
	} {
		if isConfiguredValue(pair.config) && pair.config.ValueString() == "" {
			*pair.target = types.StringValue("")
		}
	}
	diags.Append(state.Set(ctx, &refreshed)...)
}

func (r *ClusterOptionsResource) buildForm(config clusterOptionsModel) (url.Values, []string) {
	form := url.Values{}
	var deleted []string
	setString := func(key string, value types.String) {
		if !isConfiguredValue(value) {
			return
		}
		if value.ValueString() == "" {
			deleted = append(deleted, key)
			return
		}
		form.Set(key, value.ValueString())
	}
	setString("description", config.Description)
	setString("keyboard", config.Keyboard)
	setString("console", config.Console)
	setString("language", config.Language)
	setString("email_from", config.EmailFrom)
	setString("fencing", config.Fencing)
	setString("migration", config.Migration)
	setString("tag-style", config.TagStyle)
	setString("ha", config.HA)
	setString("http_proxy", config.HTTPProxy)
	setString("next-id", config.NextID)
	setString("mac_prefix", config.MACPrefix)
	if isConfiguredValue(config.MigrationUnsecure) {
		setOptionalBool(form, "migration_unsecure", boolPointerValue(config.MigrationUnsecure))
	}
	if isConfiguredValue(config.MaxWorkers) {
		setOptionalInt64(form, "max_workers", int64PointerValue(config.MaxWorkers))
	}
	return form, deleted
}

var _ = fmt.Sprintf
var _ = strings.Join

func slicesContains(values []string, value string) bool {
	for _, entry := range values {
		if entry == value {
			return true
		}
	}
	return false
}

var (
	clusterConsoleValues = []string{"applet", "vv", "html5", "xtermjs"}
	clusterFencingValues = []string{"watchdog", "hardware", "both"}
)

func (r *ClusterOptionsResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config clusterOptionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if isConfiguredValue(config.Console) && !slicesContains(clusterConsoleValues, config.Console.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("console"), "Invalid console", "console must be applet, vv, html5 or xtermjs.")
	}
	if isConfiguredValue(config.Fencing) && !slicesContains(clusterFencingValues, config.Fencing.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("fencing"), "Invalid fencing mode", "fencing must be watchdog, hardware or both.")
	}
	if isConfiguredValue(config.MaxWorkers) && config.MaxWorkers.ValueInt64() < 1 {
		resp.Diagnostics.AddAttributeError(path.Root("max_workers"), "Invalid max workers", "max_workers must be at least 1.")
	}
}

func (d *ClusterOptionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config clusterOptionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entry, err := d.client.ClusterOptions(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Cluster Options", err.Error())
		return
	}
	state := clusterOptionsFromAPI(entry)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
