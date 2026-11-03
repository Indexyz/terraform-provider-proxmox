// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	notificationKindSMTP     = "smtp"
	notificationKindGotify   = "gotify"
	notificationKindSendmail = "sendmail"
	notificationKindWebhook  = "webhook"
)

type notificationEndpointModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Comment    types.String `tfsdk:"comment"`
	Disable    types.Bool   `tfsdk:"disable"`
	Author     types.String `tfsdk:"author"`
	Server     types.String `tfsdk:"server"`
	From       types.String `tfsdk:"from_address"`
	Mode       types.String `tfsdk:"mode"`
	Username   types.String `tfsdk:"username"`
	Password   types.String `tfsdk:"password"`
	Token      types.String `tfsdk:"token"`
	Method     types.String `tfsdk:"method"`
	URL        types.String `tfsdk:"url"`
	Body       types.String `tfsdk:"body"`
	Mailto     types.Set    `tfsdk:"mailto"`
	MailtoUser types.Set    `tfsdk:"mailto_user"`
	Header     types.Set    `tfsdk:"header"`
	Secret     types.Set    `tfsdk:"secret"`
	Port       types.Int64  `tfsdk:"port"`
}

var _ resource.Resource = &NotificationEndpointSMTPResource{}
var _ resource.ResourceWithImportState = &NotificationEndpointSMTPResource{}
var _ resource.ResourceWithValidateConfig = &NotificationEndpointSMTPResource{}
var _ resource.Resource = &NotificationEndpointGotifyResource{}
var _ resource.ResourceWithImportState = &NotificationEndpointGotifyResource{}
var _ resource.ResourceWithValidateConfig = &NotificationEndpointGotifyResource{}
var _ resource.Resource = &NotificationEndpointSendmailResource{}
var _ resource.ResourceWithImportState = &NotificationEndpointSendmailResource{}
var _ resource.ResourceWithValidateConfig = &NotificationEndpointSendmailResource{}
var _ resource.Resource = &NotificationEndpointWebhookResource{}
var _ resource.ResourceWithImportState = &NotificationEndpointWebhookResource{}
var _ resource.ResourceWithValidateConfig = &NotificationEndpointWebhookResource{}

type NotificationEndpointSMTPResource struct{ client *Client }
type NotificationEndpointGotifyResource struct{ client *Client }
type NotificationEndpointSendmailResource struct{ client *Client }
type NotificationEndpointWebhookResource struct{ client *Client }

func NewNotificationEndpointSMTPResource() resource.Resource {
	return &NotificationEndpointSMTPResource{}
}
func NewNotificationEndpointGotifyResource() resource.Resource {
	return &NotificationEndpointGotifyResource{}
}
func NewNotificationEndpointSendmailResource() resource.Resource {
	return &NotificationEndpointSendmailResource{}
}
func NewNotificationEndpointWebhookResource() resource.Resource {
	return &NotificationEndpointWebhookResource{}
}

func (r *NotificationEndpointSMTPResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_endpoint_smtp"
}
func (r *NotificationEndpointGotifyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_endpoint_gotify"
}
func (r *NotificationEndpointSendmailResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_endpoint_sendmail"
}
func (r *NotificationEndpointWebhookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_endpoint_webhook"
}

func notificationEndpointSchema(kind, description string) schema.Schema {
	optionalString := func(text string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: text}
	}
	optionalSet := func(text string) schema.SetAttribute {
		return schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: text}
	}
	attributes := map[string]schema.Attribute{
		"id":      schema.StringAttribute{Computed: true, MarkdownDescription: "Endpoint name."},
		"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Stable endpoint name (`pve-configid`). Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"comment": optionalString("Comment."),
		"disable": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Disable this target.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
		// Union of all endpoint kinds; unused attributes stay null.
		"server":       optionalString("Server hostname, IP or URL."),
		"port":         schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "SMTP port. Proxmox defaults to 25 without TLS and 465 with TLS."},
		"from_address": optionalString("Sender address."),
		"author":       optionalString("Author shown in the mail."),
		"mailto":       optionalSet("Recipient addresses."),
		"mailto_user":  optionalSet("Recipient Proxmox users."),
		"mode":         optionalString("TLS mode: `insecure`, `starttls` or `tls`."),
		"username":     optionalString("SMTP authentication user."),
		"password":     schema.StringAttribute{Optional: true, Sensitive: true, MarkdownDescription: "SMTP authentication password. Not returned by Proxmox."},
		"token":        schema.StringAttribute{Optional: true, Sensitive: true, MarkdownDescription: "Gotify application token. Not returned by Proxmox."},
		"url":          optionalString("Webhook URL (`http://` or `https://`)."),
		"method":       optionalString("HTTP method: `post`, `put` or `get`."),
		"header":       optionalSet("HTTP header property strings such as `Authorization=Bearer x`."),
		"secret":       schema.SetAttribute{Optional: true, Computed: true, Sensitive: true, ElementType: types.StringType, MarkdownDescription: "`name=<header>,value=<base64 value>` entries to redact in the Proxmox UI; Proxmox echoes only the names back."},
		"body":         optionalString("Request body template."),
	}
	switch kind {
	case notificationKindSMTP:
		attributes["server"] = schema.StringAttribute{Required: true, MarkdownDescription: "SMTP server hostname or IP."}
		attributes["from_address"] = schema.StringAttribute{Required: true, MarkdownDescription: "Sender address."}
	case notificationKindGotify:
		attributes["server"] = schema.StringAttribute{Required: true, MarkdownDescription: "Gotify server URL."}
		attributes["token"] = schema.StringAttribute{Required: true, Sensitive: true, MarkdownDescription: "Gotify application token. Not returned by Proxmox."}
	case notificationKindWebhook:
		attributes["url"] = schema.StringAttribute{Required: true, MarkdownDescription: "Webhook URL (`http://` or `https://`)."}
	}
	return schema.Schema{MarkdownDescription: description, Attributes: attributes}
}
func (r *NotificationEndpointSMTPResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = notificationEndpointSchema(notificationKindSMTP, "Manages an SMTP notification endpoint through `/cluster/notifications/endpoints/smtp`.")
}
func (r *NotificationEndpointGotifyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = notificationEndpointSchema(notificationKindGotify, "Manages a Gotify notification endpoint through `/cluster/notifications/endpoints/gotify`.")
}
func (r *NotificationEndpointSendmailResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = notificationEndpointSchema(notificationKindSendmail, "Manages the local sendmail notification target through `/cluster/notifications/endpoints/sendmail`. The built-in `mail-to-root` target can be managed by importing it.")
}
func (r *NotificationEndpointWebhookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = notificationEndpointSchema(notificationKindWebhook, "Manages a webhook notification endpoint through `/cluster/notifications/endpoints/webhook`.")
}

func (r *NotificationEndpointSMTPResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *NotificationEndpointGotifyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *NotificationEndpointSendmailResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *NotificationEndpointWebhookResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func notificationConfigure(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *Client {
	if req.ProviderData == nil {
		return nil
	}
	client, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", err.Error())
		return nil
	}
	return client
}

func notificationEndpointForm(kind string, plan notificationEndpointModel) url.Values {
	form := url.Values{}
	form.Set("name", plan.Name.ValueString())
	if isConfiguredValue(plan.Comment) {
		form.Set("comment", plan.Comment.ValueString())
	}
	if isConfiguredValue(plan.Disable) {
		setOptionalBool(form, "disable", boolPointerValue(plan.Disable))
	}
	addString := func(key string, value types.String) {
		if isConfiguredValue(value) {
			form.Set(key, value.ValueString())
		}
	}
	switch kind {
	case notificationKindSMTP:
		addString("server", plan.Server)
		addString("from-address", plan.From)
		addString("author", plan.Author)
		addString("mode", plan.Mode)
		addString("username", plan.Username)
		if isConfiguredValue(plan.Password) {
			form.Set("password", plan.Password.ValueString())
		}
		if isConfiguredValue(plan.Port) {
			setOptionalInt64(form, "port", int64PointerValue(plan.Port))
		}
		addStringSet(form, "mailto", plan.Mailto)
		addStringSet(form, "mailto-user", plan.MailtoUser)
	case notificationKindGotify:
		addString("server", plan.Server)
		if isConfiguredValue(plan.Token) {
			form.Set("token", plan.Token.ValueString())
		}
	case notificationKindSendmail:
		addString("from-address", plan.From)
		addString("author", plan.Author)
		addStringSet(form, "mailto", plan.Mailto)
		addStringSet(form, "mailto-user", plan.MailtoUser)
	case notificationKindWebhook:
		addString("url", plan.URL)
		addString("method", plan.Method)
		addString("body", plan.Body)
		addStringSet(form, "header", plan.Header)
		addStringSet(form, "secret", plan.Secret)
	}
	return form
}

func addStringSet(form url.Values, key string, value types.Set) {
	if !isConfiguredValue(value) {
		return
	}
	for _, element := range value.Elements() {
		if stringValue, ok := element.(types.String); ok && !stringValue.IsNull() {
			form.Add(key, stringValue.ValueString())
		}
	}
}

func notificationEndpointDeleteKeys(config notificationEndpointModel) []string {
	var deleted []string
	if isConfiguredValue(config.Comment) && config.Comment.ValueString() == "" {
		deleted = append(deleted, "comment")
	}
	if isConfiguredValue(config.Disable) && !config.Disable.ValueBool() {
		deleted = append(deleted, "disable")
	}
	for name, value := range map[string]types.String{"author": config.Author, "username": config.Username, "method": config.Method, "body": config.Body, "from-address": config.From} {
		if isConfiguredValue(value) && value.ValueString() == "" {
			deleted = append(deleted, name)
		}
	}
	slices.Sort(deleted)
	return deleted
}

func notificationEndpointState(ctx context.Context, entry NotificationEndpoint, prior notificationEndpointModel) (notificationEndpointModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	state := notificationEndpointModel{
		ID:         types.StringValue(entry.Name),
		Name:       types.StringValue(entry.Name),
		Comment:    stringOrNull(entry.Comment),
		Disable:    boolOrNull(entry.Disable.Ptr()),
		Author:     stringOrNull(entry.Author),
		Server:     stringOrNull(entry.Server),
		From:       stringOrNull(entry.From),
		Mode:       stringOrNull(entry.Mode),
		Username:   stringOrNull(entry.Username),
		Password:   types.StringNull(),
		Token:      types.StringNull(),
		Method:     stringOrNull(entry.Method),
		URL:        stringOrNull(entry.URL),
		Body:       stringOrNull(entry.Body),
		Port:       types.Int64Null(),
		Mailto:     types.SetNull(types.StringType),
		MailtoUser: types.SetNull(types.StringType),
		Header:     types.SetNull(types.StringType),
		Secret:     types.SetNull(types.StringType),
	}
	if entry.Port > 0 {
		state.Port = types.Int64Value(entry.Port)
	}
	secretRaw := mergeNotificationSecrets(entry.Secret, prior.Secret)
	for name, source := range map[string]struct {
		value types.Set
		raw   []string
	}{"mailto": {prior.Mailto, entry.Mailto}, "mailto-user": {prior.MailtoUser, entry.MailtoUser}, "header": {prior.Header, entry.Header}, "secret": {prior.Secret, secretRaw}} {
		if len(source.raw) == 0 {
			continue
		}
		value, listDiags := types.SetValueFrom(ctx, types.StringType, source.raw)
		diags.Append(listDiags...)
		switch name {
		case "mailto":
			state.Mailto = value
		case "mailto-user":
			state.MailtoUser = value
		case "header":
			state.Header = value
		case "secret":
			state.Secret = value
		}
	}
	// Proxmox never returns the secrets; keep the configured inputs.
	state.Password = prior.Password
	state.Token = prior.Token
	return state, diags
}

func (r *NotificationEndpointSMTPResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	notificationEndpointCreate(ctx, notificationKindSMTP, r.client, req, resp)
}
func (r *NotificationEndpointGotifyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	notificationEndpointCreate(ctx, notificationKindGotify, r.client, req, resp)
}
func (r *NotificationEndpointSendmailResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	notificationEndpointCreate(ctx, notificationKindSendmail, r.client, req, resp)
}
func (r *NotificationEndpointWebhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	notificationEndpointCreate(ctx, notificationKindWebhook, r.client, req, resp)
}

func (r *NotificationEndpointSMTPResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	notificationEndpointRead(ctx, notificationKindSMTP, r.client, req, resp)
}
func (r *NotificationEndpointGotifyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	notificationEndpointRead(ctx, notificationKindGotify, r.client, req, resp)
}
func (r *NotificationEndpointSendmailResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	notificationEndpointRead(ctx, notificationKindSendmail, r.client, req, resp)
}
func (r *NotificationEndpointWebhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	notificationEndpointRead(ctx, notificationKindWebhook, r.client, req, resp)
}

func (r *NotificationEndpointSMTPResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	notificationEndpointUpdate(ctx, notificationKindSMTP, r.client, req, resp)
}
func (r *NotificationEndpointGotifyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	notificationEndpointUpdate(ctx, notificationKindGotify, r.client, req, resp)
}
func (r *NotificationEndpointSendmailResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	notificationEndpointUpdate(ctx, notificationKindSendmail, r.client, req, resp)
}
func (r *NotificationEndpointWebhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	notificationEndpointUpdate(ctx, notificationKindWebhook, r.client, req, resp)
}

func (r *NotificationEndpointSMTPResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	notificationEndpointDelete(ctx, notificationKindSMTP, r.client, req, resp)
}
func (r *NotificationEndpointGotifyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	notificationEndpointDelete(ctx, notificationKindGotify, r.client, req, resp)
}
func (r *NotificationEndpointSendmailResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	notificationEndpointDelete(ctx, notificationKindSendmail, r.client, req, resp)
}
func (r *NotificationEndpointWebhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	notificationEndpointDelete(ctx, notificationKindWebhook, r.client, req, resp)
}

func (r *NotificationEndpointSMTPResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	notificationEndpointValidate(ctx, notificationKindSMTP, req, resp)
}
func (r *NotificationEndpointGotifyResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	notificationEndpointValidate(ctx, notificationKindGotify, req, resp)
}
func (r *NotificationEndpointSendmailResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	notificationEndpointValidate(ctx, notificationKindSendmail, req, resp)
}
func (r *NotificationEndpointWebhookResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	notificationEndpointValidate(ctx, notificationKindWebhook, req, resp)
}

func (r *NotificationEndpointSMTPResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	notificationImport(ctx, req, resp)
}
func (r *NotificationEndpointGotifyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	notificationImport(ctx, req, resp)
}
func (r *NotificationEndpointSendmailResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	notificationImport(ctx, req, resp)
}
func (r *NotificationEndpointWebhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	notificationImport(ctx, req, resp)
}

func notificationEndpointValidate(ctx context.Context, kind string, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config notificationEndpointModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if isConfiguredValue(config.Name) && !notificationNamePattern.MatchString(config.Name.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid endpoint name", "name must be a valid configuration id.")
	}
	if isConfiguredValue(config.Mode) && !slices.Contains([]string{"insecure", "starttls", "tls"}, config.Mode.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("mode"), "Invalid TLS mode", "mode must be insecure, starttls or tls.")
	}
	if isConfiguredValue(config.Method) && !slices.Contains([]string{"post", "put", "get"}, config.Method.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("method"), "Invalid HTTP method", "method must be post, put or get.")
	}
	if kind == notificationKindWebhook && isConfiguredValue(config.URL) && !strings.HasPrefix(config.URL.ValueString(), "http://") && !strings.HasPrefix(config.URL.ValueString(), "https://") {
		resp.Diagnostics.AddAttributeError(path.Root("url"), "Invalid webhook URL", "url must start with http:// or https://.")
	}
	if isConfiguredValue(config.Port) && (config.Port.ValueInt64() < 1 || config.Port.ValueInt64() > 65535) {
		resp.Diagnostics.AddAttributeError(path.Root("port"), "Invalid port", "port must be between 1 and 65535.")
	}
}

func notificationEndpointCreate(ctx context.Context, kind string, client *Client, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan notificationEndpointModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	form := notificationEndpointForm(kind, plan)
	name := plan.Name.ValueString()
	if err := client.CreateNotificationEndpoint(ctx, kind, form); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Notification Endpoint", err.Error())
		return
	}
	entry, err := client.GetNotificationEndpoint(ctx, kind, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Notification Endpoint", err.Error())
		return
	}
	state, stateDiags := notificationEndpointState(ctx, entry, plan)
	resp.Diagnostics.Append(stateDiags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func notificationEndpointRead(ctx context.Context, kind string, client *Client, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state notificationEndpointModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entry, err := client.GetNotificationEndpoint(ctx, kind, state.Name.ValueString())
	if errors.Is(err, errNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Notification Endpoint", err.Error())
		return
	}
	refreshed, diags := notificationEndpointState(ctx, entry, state)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func notificationEndpointUpdate(ctx context.Context, kind string, client *Client, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config notificationEndpointModel
	var prior notificationEndpointModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	form := notificationEndpointForm(kind, config)
	entry, err := client.GetNotificationEndpoint(ctx, kind, config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Notification Endpoint", err.Error())
		return
	}
	if deleted := notificationEndpointDeleteKeys(config); len(deleted) > 0 {
		form.Set("delete", strings.Join(deleted, ","))
	}
	if err := client.UpdateNotificationEndpoint(ctx, kind, config.Name.ValueString(), form, entry.Digest); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Notification Endpoint", err.Error())
		return
	}
	refreshed, err := client.GetNotificationEndpoint(ctx, kind, config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Notification Endpoint", err.Error())
		return
	}
	state, stateDiags := notificationEndpointState(ctx, refreshed, config)
	resp.Diagnostics.Append(stateDiags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func notificationEndpointDelete(ctx context.Context, kind string, client *Client, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state notificationEndpointModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := client.DeleteNotificationEndpoint(ctx, kind, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Notification Endpoint", err.Error())
	}
}

func notificationImport(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	name := strings.TrimSpace(req.ID)
	if !notificationNamePattern.MatchString(name) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected a notification endpoint name.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}

var notificationNamePattern = regexpMustCompile(`^[A-Za-z][A-Za-z0-9_-]+$`)

// mergeNotificationSecrets restores values Proxmox redacts on read: the server
// returns `name=<NAME>` only, while the state carries `name=<NAME>,value=<b64>`.
func mergeNotificationSecrets(server []string, prior types.Set) []string {
	if len(server) == 0 {
		return nil
	}
	priorValues := map[string]string{}
	if !prior.IsNull() && !prior.IsUnknown() {
		for _, element := range prior.Elements() {
			value, ok := element.(types.String)
			if !ok {
				continue
			}
			key, rest, found := strings.Cut(value.ValueString(), ",")
			if found {
				priorValues[key] = key + "," + rest
			}
		}
	}
	merged := make([]string, 0, len(server))
	for _, entry := range server {
		key, _, _ := strings.Cut(entry, ",")
		if value, ok := priorValues[key]; ok {
			merged = append(merged, value)
			continue
		}
		merged = append(merged, entry)
	}
	return merged
}
