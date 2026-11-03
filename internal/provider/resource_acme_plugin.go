// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &ACMEPluginResource{}
var _ resource.ResourceWithImportState = &ACMEPluginResource{}
var _ resource.ResourceWithValidateConfig = &ACMEPluginResource{}

type ACMEPluginResource struct{ client *Client }

type acmePluginModel struct {
	ID              types.String `tfsdk:"id"`
	Plugin          types.String `tfsdk:"plugin"`
	Type            types.String `tfsdk:"type"`
	API             types.String `tfsdk:"api"`
	Data            types.Map    `tfsdk:"data"`
	Nodes           types.Set    `tfsdk:"nodes"`
	ValidationDelay types.Int64  `tfsdk:"validation_delay"`
	Disable         types.Bool   `tfsdk:"disable"`
}

func NewACMEPluginResource() resource.Resource { return &ACMEPluginResource{} }

func (r *ACMEPluginResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acme_plugin"
}

func (r *ACMEPluginResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an ACME DNS challenge plugin through `/cluster/acme/plugins`. The `data` map is sent to Proxmox base64-encoded and holds the DNS API credentials.",
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{Computed: true, MarkdownDescription: "Plugin id."},
			"plugin":           schema.StringAttribute{Required: true, MarkdownDescription: "Unique plugin id. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"type":             schema.StringAttribute{Required: true, MarkdownDescription: "Challenge type, for example `dns`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"api":              schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "DNS API implementation, for example `aws` or `cloudflare`."},
			"data":             schema.MapAttribute{Optional: true, Computed: true, Sensitive: true, ElementType: types.StringType, MarkdownDescription: "DNS API credentials and options."},
			"nodes":            schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Nodes the plugin may be used on. Unset means all nodes."},
			"validation_delay": schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Seconds to wait for DNS propagation before validation."},
			"disable":          schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Disable this plugin."},
		},
	}
}

func (r *ACMEPluginResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *ACMEPluginResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config acmePluginModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if isConfiguredValue(config.Plugin) && !notificationNamePattern.MatchString(config.Plugin.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("plugin"), "Invalid plugin id", "plugin must be a valid configuration id.")
	}
	if isConfiguredValue(config.Type) && !slices.Contains([]string{"dns", "standalone"}, config.Type.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("type"), "Invalid challenge type", "type must be dns or standalone.")
	}
	if isConfiguredValue(config.ValidationDelay) && config.ValidationDelay.ValueInt64() < 0 {
		resp.Diagnostics.AddAttributeError(path.Root("validation_delay"), "Invalid validation delay", "validation_delay must not be negative.")
	}
}

func (r *ACMEPluginResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan acmePluginModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data, diags := acmePluginDataFromModel(plan.Data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plugin := ACMEPlugin{ID: plan.Plugin.ValueString(), Type: plan.Type.ValueString(), API: stringValue(plan.API), Data: data, Nodes: strings.Join(stringSetValues(plan.Nodes), " ")}
	if err := r.client.CreateACMEPlugin(ctx, plugin, boolPointerValue(plan.Disable), int64PointerValue(plan.ValidationDelay)); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox ACME Plugin", err.Error())
		return
	}
	state, stateDiags := r.readState(ctx, plan.Plugin.ValueString())
	resp.Diagnostics.Append(stateDiags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ACMEPluginResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state acmePluginModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.Plugin.ValueString())
	resp.Diagnostics.Append(diags...)
	if refreshed.ID.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *ACMEPluginResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config acmePluginModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetACMEPlugin(ctx, config.Plugin.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox ACME Plugin", err.Error())
		return
	}
	data, diags := acmePluginDataFromModel(config.Data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current.API = stringValue(config.API)
	current.Data = data
	current.Nodes = strings.Join(stringSetValues(config.Nodes), " ")
	current.Disable = proxmoxOptionalBool{value: boolPointerValue(config.Disable)}
	current.ValidationDelay = int64ValueOrZero(config.ValidationDelay)
	var deleted []string
	if !isConfiguredValue(config.API) {
		deleted = append(deleted, "api")
	}
	if !isConfiguredValue(config.Nodes) {
		deleted = append(deleted, "nodes")
	}
	if !isConfiguredValue(config.ValidationDelay) {
		deleted = append(deleted, "validation-delay")
	}
	if isConfiguredValue(config.Disable) && !config.Disable.ValueBool() {
		deleted = append(deleted, "disable")
	}
	if err := r.client.UpdateACMEPlugin(ctx, current.ID, current, boolPointerValue(config.Disable), int64PointerValue(config.ValidationDelay), deleted, current.Digest); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox ACME Plugin", err.Error())
		return
	}
	state, stateDiags := r.readState(ctx, config.Plugin.ValueString())
	resp.Diagnostics.Append(stateDiags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ACMEPluginResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state acmePluginModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteACMEPlugin(ctx, state.Plugin.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox ACME Plugin", err.Error())
	}
}

func (r *ACMEPluginResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := strings.TrimSpace(req.ID)
	if !notificationNamePattern.MatchString(id) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected an ACME plugin id.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("plugin"), id)...)
}

func (r *ACMEPluginResource) readState(ctx context.Context, id string) (acmePluginModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	plugin, err := r.client.GetACMEPlugin(ctx, id)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return acmePluginModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox ACME Plugin", err.Error())
		return acmePluginModel{}, diags
	}
	data, dataDiags := acmePluginDataToModel(ctx, plugin.Data)
	diags.Append(dataDiags...)
	nodes := types.SetNull(types.StringType)
	if strings.TrimSpace(plugin.Nodes) != "" {
		value, setDiags := types.SetValueFrom(ctx, types.StringType, strings.Fields(plugin.Nodes))
		diags.Append(setDiags...)
		nodes = value
	}
	return acmePluginModel{
		ID:              types.StringValue(plugin.ID),
		Plugin:          types.StringValue(plugin.ID),
		Type:            types.StringValue(plugin.Type),
		API:             stringOrNull(plugin.API),
		Data:            data,
		Nodes:           nodes,
		ValidationDelay: int64OrNull(optionalInt64(plugin.ValidationDelay)),
		Disable:         boolOrNull(plugin.Disable.Ptr()),
	}, diags
}

func acmePluginDataFromModel(value types.Map) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !isConfiguredValue(value) {
		return "", diags
	}
	items := map[string]string{}
	diags = value.ElementsAs(context.Background(), &items, false)
	if diags.HasError() {
		return "", diags
	}
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+"="+items[key])
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(pairs, "\n"))), diags
}

func acmePluginDataToModel(ctx context.Context, encoded string) (types.Map, diag.Diagnostics) {
	var diags diag.Diagnostics
	if encoded == "" {
		return types.MapNull(types.StringType), diags
	}
	// Create expects base64 while reads return the plain property string.
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		decoded = []byte(encoded)
	}
	items := map[string]string{}
	for _, line := range strings.FieldsFunc(string(decoded), func(r rune) bool { return r == '\n' || r == '\r' }) {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			items[key] = value
		}
	}
	value, mapDiags := types.MapValueFrom(ctx, types.StringType, items)
	diags.Append(mapDiags...)
	return value, diags
}

func stringSetValues(value types.Set) []string {
	if !isConfiguredValue(value) {
		return nil
	}
	values := make([]string, 0, len(value.Elements()))
	for _, element := range value.Elements() {
		if text, ok := element.(types.String); ok && !text.IsNull() {
			values = append(values, text.ValueString())
		}
	}
	return values
}

func int64ValueOrZero(value types.Int64) int64 {
	if isConfiguredValue(value) {
		return value.ValueInt64()
	}
	return 0
}

func optionalInt64(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}
