// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
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
	"net/url"
)

var _ resource.Resource = &NotificationMatcherResource{}
var _ resource.ResourceWithImportState = &NotificationMatcherResource{}
var _ resource.ResourceWithValidateConfig = &NotificationMatcherResource{}

type NotificationMatcherResource struct{ client *Client }

type notificationMatcherModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Comment       types.String `tfsdk:"comment"`
	Disable       types.Bool   `tfsdk:"disable"`
	Target        types.Set    `tfsdk:"target"`
	Mode          types.String `tfsdk:"mode"`
	InvertMatch   types.Bool   `tfsdk:"invert_match"`
	MatchField    types.Set    `tfsdk:"match_field"`
	MatchSeverity types.Set    `tfsdk:"match_severity"`
	MatchCalendar types.Set    `tfsdk:"match_calendar"`
}

func NewNotificationMatcherResource() resource.Resource { return &NotificationMatcherResource{} }

func (r *NotificationMatcherResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_matcher"
}

func (r *NotificationMatcherResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a notification matcher through `/cluster/notifications/matchers`. A matcher routes matching notifications to one or more targets; clear all match rules to match everything.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Matcher name."},
			"name":           schema.StringAttribute{Required: true, MarkdownDescription: "Stable matcher name (`pve-configid`). Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"target":         schema.SetAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Target endpoints (built-in names or `proxmox_notification_endpoint_*` names)."},
			"comment":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Comment."},
			"disable":        schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Disable this matcher.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"mode":           schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "`all` requires every rule to match, `any` one of them. The Proxmox default is `all`."},
			"invert_match":   schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Send notifications that do **not** match the rules.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"match_field":    schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Field matchers such as `type=system` or `hostname=node1`."},
			"match_severity": schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Severities: `info`, `notice`, `warning`, `error`, `unknown`."},
			"match_calendar": schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Calendar matchers in systemd calendar syntax."},
		},
	}
}

func (r *NotificationMatcherResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *NotificationMatcherResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config notificationMatcherModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if isConfiguredValue(config.Name) && !notificationNamePattern.MatchString(config.Name.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid matcher name", "name must be a valid configuration id.")
	}
	if isConfiguredValue(config.Mode) && !slices.Contains([]string{"all", "any"}, config.Mode.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("mode"), "Invalid matcher mode", "mode must be all or any.")
	}
	if isConfiguredValue(config.Target) && len(config.Target.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(path.Root("target"), "Missing matcher targets", "target needs at least one endpoint.")
	}
	if isConfiguredValue(config.MatchSeverity) {
		for _, element := range config.MatchSeverity.Elements() {
			value, ok := element.(types.String)
			if ok && isConfiguredValue(value) && !slices.Contains([]string{"info", "notice", "warning", "error", "unknown"}, value.ValueString()) {
				resp.Diagnostics.AddAttributeError(path.Root("match_severity"), "Invalid severity", "match_severity accepts info, notice, warning, error and unknown.")
			}
		}
	}
}

func (r *NotificationMatcherResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan notificationMatcherModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	form := notificationMatcherForm(plan)
	form.Set("name", plan.Name.ValueString())
	if err := r.client.CreateNotificationMatcher(ctx, form); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Notification Matcher", err.Error())
		return
	}
	entry, err := r.client.GetNotificationMatcher(ctx, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Notification Matcher", err.Error())
		return
	}
	state, stateDiags := notificationMatcherState(ctx, entry)
	resp.Diagnostics.Append(stateDiags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NotificationMatcherResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state notificationMatcherModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entry, err := r.client.GetNotificationMatcher(ctx, state.Name.ValueString())
	if errors.Is(err, errNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Notification Matcher", err.Error())
		return
	}
	refreshed, diags := notificationMatcherState(ctx, entry)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *NotificationMatcherResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config notificationMatcherModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entry, err := r.client.GetNotificationMatcher(ctx, config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Notification Matcher", err.Error())
		return
	}
	form := notificationMatcherForm(config)
	form.Set("name", config.Name.ValueString())
	var deleted []string
	for name, value := range map[string]types.String{"comment": config.Comment} {
		if isConfiguredValue(value) && value.ValueString() == "" {
			deleted = append(deleted, name)
		}
	}
	for _, pair := range []struct {
		key   string
		value types.Set
	}{{"match-field", config.MatchField}, {"match-severity", config.MatchSeverity}, {"match-calendar", config.MatchCalendar}} {
		if isConfiguredValue(pair.value) && len(pair.value.Elements()) == 0 {
			deleted = append(deleted, pair.key)
		}
	}
	for _, pair := range []struct {
		key   string
		value types.Bool
	}{{"disable", config.Disable}, {"invert-match", config.InvertMatch}} {
		if isConfiguredValue(pair.value) && !pair.value.ValueBool() {
			deleted = append(deleted, pair.key)
		}
	}
	if len(deleted) > 0 {
		form.Set("delete", strings.Join(deleted, ","))
	}
	if err := r.client.UpdateNotificationMatcher(ctx, config.Name.ValueString(), form, entry.Digest); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Notification Matcher", err.Error())
		return
	}
	refreshed, err := r.client.GetNotificationMatcher(ctx, config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Notification Matcher", err.Error())
		return
	}
	state, stateDiags := notificationMatcherState(ctx, refreshed)
	resp.Diagnostics.Append(stateDiags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NotificationMatcherResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state notificationMatcherModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteNotificationMatcher(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Notification Matcher", err.Error())
	}
}

func (r *NotificationMatcherResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	notificationImport(ctx, req, resp)
}

func notificationMatcherForm(model notificationMatcherModel) url.Values {
	form := url.Values{}
	if isConfiguredValue(model.Comment) {
		form.Set("comment", model.Comment.ValueString())
	}
	if isConfiguredValue(model.Disable) {
		setOptionalBool(form, "disable", boolPointerValue(model.Disable))
	}
	if isConfiguredValue(model.Mode) {
		form.Set("mode", model.Mode.ValueString())
	}
	if isConfiguredValue(model.InvertMatch) {
		setOptionalBool(form, "invert-match", boolPointerValue(model.InvertMatch))
	}
	addStringSet(form, "target", model.Target)
	addStringSet(form, "match-field", model.MatchField)
	addStringSet(form, "match-severity", model.MatchSeverity)
	addStringSet(form, "match-calendar", model.MatchCalendar)
	return form
}

func notificationMatcherState(ctx context.Context, entry NotificationMatcher) (notificationMatcherModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	state := notificationMatcherModel{
		ID:            types.StringValue(entry.Name),
		Name:          types.StringValue(entry.Name),
		Comment:       stringOrNull(entry.Comment),
		Disable:       boolOrNull(entry.Disable.Ptr()),
		Mode:          stringOrNull(entry.Mode),
		InvertMatch:   boolOrNull(entry.InvertMatch.Ptr()),
		Target:        types.SetNull(types.StringType),
		MatchField:    types.SetNull(types.StringType),
		MatchSeverity: types.SetNull(types.StringType),
		MatchCalendar: types.SetNull(types.StringType),
	}
	for name, raw := range map[string][]string{"target": entry.Target, "match-field": entry.MatchField, "match-severity": entry.MatchSeverity, "match-calendar": entry.MatchCalendar} {
		if len(raw) == 0 {
			continue
		}
		value, setDiags := types.SetValueFrom(ctx, types.StringType, raw)
		diags.Append(setDiags...)
		switch name {
		case "target":
			state.Target = value
		case "match-field":
			state.MatchField = value
		case "match-severity":
			state.MatchSeverity = value
		case "match-calendar":
			state.MatchCalendar = value
		}
	}
	return state, diags
}
