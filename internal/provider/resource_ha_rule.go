// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &HARuleResource{}
var _ resource.ResourceWithImportState = &HARuleResource{}
var _ resource.ResourceWithValidateConfig = &HARuleResource{}

const haRuleManagedFieldsKey = "ha-rule-managed-fields"

var (
	haRuleIDPattern        = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]+$`)
	haRuleNodePattern      = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`)
	haRuleNodeEntryPattern = regexp.MustCompile(`^([a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?)(?::([0-9]+))?$`)
)

const (
	haRuleTypeNodeAffinity     = "node-affinity"
	haRuleTypeResourceAffinity = "resource-affinity"
)

type HARuleResource struct {
	client *Client
}

type haRulePrivateData interface {
	GetKey(context.Context, string) ([]byte, diag.Diagnostics)
	SetKey(context.Context, string, []byte) diag.Diagnostics
}

type haRuleModel struct {
	ID                types.String `tfsdk:"id"`
	Rule              types.String `tfsdk:"rule"`
	Type              types.String `tfsdk:"type"`
	Resources         types.Set    `tfsdk:"resources"`
	Nodes             types.Map    `tfsdk:"nodes"`
	Affinity          types.String `tfsdk:"affinity"`
	Strict            types.Bool   `tfsdk:"strict"`
	Disable           types.Bool   `tfsdk:"disable"`
	Comment           types.String `tfsdk:"comment"`
	Order             types.Int64  `tfsdk:"order"`
	FeasibilityErrors types.Map    `tfsdk:"feasibility_errors"`
}

func NewHARuleResource() resource.Resource {
	return &HARuleResource{}
}

func (r *HARuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ha_rule"
}

func (r *HARuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox VE 9 high-availability placement rule through `/cluster/ha/rules`. Rules constrain how Proxmox HA places already HA-enrolled guests; they never start, stop, migrate, or wait for a guest. Each `resources` entry must reference a guest that is already HA configured, which the provider expresses by referencing `proxmox_ha_resource.<name>.resource_id`. Destroy removes only the rule.",
		Attributes: map[string]schema.Attribute{
			"id":                 schema.StringAttribute{Computed: true, MarkdownDescription: "HA rule identifier (same as `rule`).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"rule":               schema.StringAttribute{Required: true, MarkdownDescription: "HA rule identifier. Must start with a letter and may contain letters, numbers, `-`, and `_`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"type":               schema.StringAttribute{Required: true, MarkdownDescription: "Rule type: `node-affinity` or `resource-affinity`. The type of an existing rule cannot change, so this requires replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"resources":          schema.SetAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Canonical HA resource identifiers (`vm:<vmid>` or `ct:<vmid>`) the rule applies to. Proxmox requires at least one HA-configured resource, and at least two for `resource-affinity`."},
			"nodes":              schema.MapAttribute{Optional: true, Computed: true, ElementType: types.Int64Type, MarkdownDescription: "Cluster node names mapped to a relative priority, higher being more preferred. Required for `node-affinity` and not allowed for `resource-affinity`. With `affinity = \"negative\"` every priority must stay `0`."},
			"affinity":           schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "`positive` (prefer the given nodes / keep resources together) or `negative` (avoid the given nodes / keep resources apart). Required for `resource-affinity`; for `node-affinity` Proxmox defaults to `positive`, and `negative` needs `pve-ha-manager` 5.2.5 or later."},
			"strict":             schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether `node-affinity` is strict: a strict rule restricts the resources to the given nodes instead of merely preferring them. Not allowed for `resource-affinity`. The Proxmox default is `false`."},
			"disable":            schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Keep the rule configured but exclude it from scheduling and feasibility checks. The Proxmox default is `false`."},
			"comment":            schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "HA rule description."},
			"order":              schema.Int64Attribute{Computed: true, MarkdownDescription: "Server-assigned evaluation ordinal of the rule."},
			"feasibility_errors": schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Feasibility errors Proxmox currently reports for this rule, keyed by the offending property (`resources`, `nodes`). Proxmox only checks the rule that is being written, so other rules can become infeasible without any Terraform change; infeasible rules are dropped by the scheduler and do nothing."},
		},
	}
}

func (r *HARuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", err.Error())
		return
	}
	r.client = client
}

func (r *HARuleResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config haRuleModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateHARuleConfig(config)...)
}

func validateHARuleConfig(config haRuleModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if !config.Rule.IsNull() && !config.Rule.IsUnknown() && !haRuleIDPattern.MatchString(config.Rule.ValueString()) {
		diags.AddAttributeError(path.Root("rule"), "Invalid HA rule identifier", "rule must start with a letter, contain only letters, numbers, hyphens, or underscores, and be at least two characters long.")
	}
	if !config.Comment.IsNull() && !config.Comment.IsUnknown() && len(config.Comment.ValueString()) > 4096 {
		diags.AddAttributeError(path.Root("comment"), "Invalid HA rule comment", "comment must not exceed 4096 characters.")
	}
	if !config.Affinity.IsNull() && !config.Affinity.IsUnknown() && !slices.Contains([]string{"positive", "negative"}, config.Affinity.ValueString()) {
		diags.AddAttributeError(path.Root("affinity"), "Invalid HA rule affinity", "affinity must be positive or negative.")
	}

	resources := haRuleConfiguredResources(config.Resources)
	if len(resources) == 0 && !config.Resources.IsUnknown() {
		diags.AddAttributeError(path.Root("resources"), "Missing HA rule resources", "resources must contain at least one canonical vm:<vmid> or ct:<vmid> identifier.")
	}
	for _, sid := range resources {
		if !haResourceIDPattern.MatchString(sid) {
			diags.AddAttributeError(path.Root("resources"), "Invalid HA resource identifier", fmt.Sprintf("%q must use canonical vm:<vmid> or ct:<vmid> form.", sid))
		}
	}

	nodes := haRuleConfiguredNodes(config.Nodes)
	negativePriorities := false
	for node, priority := range nodes {
		if !haRuleNodePattern.MatchString(node) {
			diags.AddAttributeError(path.Root("nodes"), "Invalid HA node name", fmt.Sprintf("%q must contain only letters, numbers, and inner hyphens.", node))
		}
		if priority < 0 {
			negativePriorities = true
		}
	}
	if negativePriorities {
		diags.AddAttributeError(path.Root("nodes"), "Invalid HA node priority", "node priorities must not be negative.")
	}

	if config.Type.IsNull() || config.Type.IsUnknown() {
		return diags
	}

	switch config.Type.ValueString() {
	case haRuleTypeNodeAffinity:
		if len(nodes) == 0 && !config.Nodes.IsUnknown() {
			diags.AddAttributeError(path.Root("nodes"), "Missing HA node-affinity nodes", "nodes must map at least one cluster node for a node-affinity rule.")
		}
		if !config.Affinity.IsNull() && !config.Affinity.IsUnknown() && config.Affinity.ValueString() == "negative" {
			var prioritized []string
			for node, priority := range nodes {
				if priority != 0 {
					prioritized = append(prioritized, node)
				}
			}
			if len(prioritized) > 0 {
				slices.Sort(prioritized)
				diags.AddAttributeError(path.Root("nodes"), "Invalid negative node-affinity priorities", fmt.Sprintf("a negative node-affinity rule must not set node priorities, but %s do.", strings.Join(prioritized, ", ")))
			}
		}
	case haRuleTypeResourceAffinity:
		if !config.Nodes.IsNull() && !config.Nodes.IsUnknown() {
			diags.AddAttributeError(path.Root("nodes"), "Unexpected HA resource-affinity nodes", "nodes is only valid for a node-affinity rule.")
		}
		if !config.Strict.IsNull() && !config.Strict.IsUnknown() {
			diags.AddAttributeError(path.Root("strict"), "Unexpected HA resource-affinity strict", "strict is only valid for a node-affinity rule.")
		}
		if config.Affinity.IsNull() && !config.Affinity.IsUnknown() {
			diags.AddAttributeError(path.Root("affinity"), "Missing HA resource-affinity affinity", "affinity is required for a resource-affinity rule.")
		}
		if len(resources) == 1 {
			diags.AddAttributeError(path.Root("resources"), "Too few HA resource-affinity resources", "a resource-affinity rule needs at least two resources to have any effect.")
		}
	default:
		diags.AddAttributeError(path.Root("type"), "Invalid HA rule type", "type must be node-affinity or resource-affinity.")
	}
	return diags
}

func haRuleConfiguredResources(value types.Set) []string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	var resources []string
	for _, element := range value.Elements() {
		if element.IsNull() || element.IsUnknown() {
			continue
		}
		stringValue, ok := element.(types.String)
		if !ok {
			continue
		}
		resources = append(resources, stringValue.ValueString())
	}
	return resources
}

func haRuleConfiguredNodes(value types.Map) map[string]int64 {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	nodes := make(map[string]int64, len(value.Elements()))
	for node, element := range value.Elements() {
		if element.IsNull() || element.IsUnknown() {
			continue
		}
		intValue, ok := element.(types.Int64)
		if !ok {
			continue
		}
		nodes[node] = intValue.ValueInt64()
	}
	return nodes
}

func haRuleRequestFromModels(ctx context.Context, config, plan haRuleModel) (HARuleRequest, diag.Diagnostics) {
	request := HARuleRequest{Type: plan.Type.ValueString()}
	resources, diags := expandStringSet(ctx, plan.Resources)
	request.Resources = resources
	if !config.Nodes.IsNull() && !config.Nodes.IsUnknown() {
		nodes := make([]HANode, 0, len(plan.Nodes.Elements()))
		for node, element := range plan.Nodes.Elements() {
			priority, ok := element.(types.Int64)
			if !ok {
				continue
			}
			nodes = append(nodes, HANode{Node: node, Priority: priority.ValueInt64()})
		}
		request.Nodes = nodes
	}
	if !config.Affinity.IsNull() && !config.Affinity.IsUnknown() {
		request.Affinity = stringPointer(plan.Affinity)
	}
	if !config.Strict.IsNull() && !config.Strict.IsUnknown() {
		request.Strict = boolPointerValue(plan.Strict)
	}
	if !config.Disable.IsNull() && !config.Disable.IsUnknown() {
		request.Disable = boolPointerValue(plan.Disable)
	}
	if !config.Comment.IsNull() && !config.Comment.IsUnknown() {
		request.Comment = stringPointer(plan.Comment)
	}
	return request, diags
}

func haRuleManagedFields(config haRuleModel) []string {
	var fields []string
	if !config.Comment.IsNull() && !config.Comment.IsUnknown() {
		fields = append(fields, "comment")
	}
	if !config.Disable.IsNull() && !config.Disable.IsUnknown() {
		fields = append(fields, "disable")
	}
	if !config.Nodes.IsNull() && !config.Nodes.IsUnknown() {
		fields = append(fields, "nodes")
	}
	if !config.Strict.IsNull() && !config.Strict.IsUnknown() {
		fields = append(fields, "strict")
	}
	if !config.Affinity.IsNull() && !config.Affinity.IsUnknown() {
		fields = append(fields, "affinity")
	}
	slices.Sort(fields)
	return fields
}

func haRuleDeleteKeys(config haRuleModel, previouslyManaged []string) []string {
	current := haRuleManagedFields(config)
	var deleted []string
	for _, key := range previouslyManaged {
		if !slices.Contains(current, key) {
			deleted = append(deleted, key)
		}
	}
	slices.Sort(deleted)
	return deleted
}

func (r *HARuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config haRuleModel
	var plan haRuleModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	createReq, diags := haRuleRequestFromModels(ctx, config, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.CreateHARule(ctx, plan.Rule.ValueString(), createReq); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox HA Rule", err.Error())
		return
	}
	state, stateDiags := r.readState(ctx, plan.Rule.ValueString())
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *HARuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state haRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.Rule.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if refreshed.ID.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *HARuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config haRuleModel
	var plan haRuleModel
	var prior haRuleModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManaged, diags := readHARuleManagedFields(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetHARule(ctx, prior.Rule.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Current Proxmox HA Rule", err.Error())
		return
	}
	updateReq, requestDiags := haRuleRequestFromModels(ctx, config, plan)
	resp.Diagnostics.Append(requestDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateReq.Delete = haRuleDeleteKeys(config, previouslyManaged)
	if current.Digest != "" {
		updateReq.Digest = &current.Digest
	}
	if err := r.client.UpdateHARule(ctx, prior.Rule.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox HA Rule", err.Error())
		return
	}
	state, stateDiags := r.readState(ctx, prior.Rule.ValueString())
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *HARuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state haRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteHARule(ctx, state.Rule.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox HA Rule", err.Error())
	}
}

func (r *HARuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !haRuleIDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected a Proxmox HA rule identifier that starts with a letter and contains only letters, numbers, hyphens, or underscores.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("rule"), req.ID)...)
}

func (r *HARuleResource) readState(ctx context.Context, rule string) (haRuleModel, diag.Diagnostics) {
	entry, err := r.client.GetHARule(ctx, rule)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return haRuleModel{ID: types.StringNull()}, nil
		}
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox HA Rule", fmt.Sprintf("Unable to read HA rule %q: %s", rule, err))
		return haRuleModel{}, diags
	}
	return haRuleStateFromAPI(ctx, entry)
}

func haRuleStateFromAPI(ctx context.Context, entry HARule) (haRuleModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	resources, resourceDiags := stringSetValue(ctx, haRuleResourceList(entry.Resources))
	diags.Append(resourceDiags...)

	state := haRuleModel{
		ID:                types.StringValue(entry.Rule),
		Rule:              types.StringValue(entry.Rule),
		Type:              types.StringValue(entry.Type),
		Resources:         resources,
		Nodes:             types.MapNull(types.Int64Type),
		Affinity:          types.StringNull(),
		Strict:            types.BoolNull(),
		Disable:           types.BoolValue(entry.Disable.Ptr() != nil && *entry.Disable.Ptr()),
		Comment:           stringOrNull(entry.Comment),
		Order:             types.Int64Value(0),
		FeasibilityErrors: types.MapNull(types.StringType),
	}
	if entry.Order.Ptr() != nil {
		state.Order = types.Int64Value(*entry.Order.Ptr())
	}
	if len(entry.Errors) > 0 {
		errorsMap, errorDiags := types.MapValueFrom(ctx, types.StringType, entry.Errors)
		diags.Append(errorDiags...)
		state.FeasibilityErrors = errorsMap
	}

	switch entry.Type {
	case haRuleTypeNodeAffinity:
		nodes, nodeDiags := haRuleNodeValues(ctx, entry.Nodes)
		diags.Append(nodeDiags...)
		state.Nodes = nodes
		affinity := entry.Affinity
		if affinity == "" {
			affinity = "positive"
		}
		state.Affinity = types.StringValue(affinity)
		state.Strict = types.BoolValue(entry.Strict.Ptr() != nil && *entry.Strict.Ptr())
	case haRuleTypeResourceAffinity:
		state.Affinity = stringOrNull(entry.Affinity)
	}
	return state, diags
}

func haRuleResourceList(value string) []string {
	if value == "" {
		return nil
	}
	resources := strings.Split(value, ",")
	for i := range resources {
		resources[i] = strings.TrimSpace(resources[i])
	}
	return slices.DeleteFunc(resources, func(sid string) bool { return sid == "" })
}

func haRuleNodeValues(ctx context.Context, value string) (types.Map, diag.Diagnostics) {
	nodes := map[string]int64{}
	if value != "" {
		for _, entry := range strings.Split(value, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			match := haRuleNodeEntryPattern.FindStringSubmatch(entry)
			if match == nil {
				return types.MapNull(types.Int64Type), diag.Diagnostics{diag.NewErrorDiagnostic(
					"Unable to Decode Proxmox HA Rule Nodes",
					fmt.Sprintf("unexpected Proxmox HA node entry %q.", entry),
				)}
			}
			priority := int64(0)
			if match[2] != "" {
				parsed, err := strconv.ParseInt(match[2], 10, 64)
				if err != nil {
					return types.MapNull(types.Int64Type), diag.Diagnostics{diag.NewErrorDiagnostic(
						"Unable to Decode Proxmox HA Rule Nodes",
						fmt.Sprintf("unable to decode Proxmox HA node priority %q: %s", entry, err),
					)}
				}
				priority = parsed
			}
			nodes[match[1]] = priority
		}
	}
	nodesValue, diags := types.MapValueFrom(ctx, types.Int64Type, nodes)
	return nodesValue, diags
}

func (r *HARuleResource) storeManagedFields(ctx context.Context, config haRuleModel, private haRulePrivateData, diags *diag.Diagnostics) {
	managedFields, err := json.Marshal(haRuleManagedFields(config))
	if err != nil {
		diags.AddError("Unable to Store HA Rule State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, haRuleManagedFieldsKey, managedFields)...)
}

func readHARuleManagedFields(ctx context.Context, private haRulePrivateData) ([]string, diag.Diagnostics) {
	value, diags := private.GetKey(ctx, haRuleManagedFieldsKey)
	if diags.HasError() || len(value) == 0 {
		return nil, diags
	}
	var fields []string
	if err := json.Unmarshal(value, &fields); err != nil {
		diags.AddError("Unable to Read HA Rule State", fmt.Sprintf("unable to decode managed fields: %v", err))
	}
	return fields, diags
}
