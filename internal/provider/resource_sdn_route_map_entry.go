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
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SDNRouteMapEntryResource{}
var _ resource.ResourceWithImportState = &SDNRouteMapEntryResource{}
var _ resource.ResourceWithValidateConfig = &SDNRouteMapEntryResource{}

const sdnRouteMapEntryManagedFieldsKey = "sdn-route-map-entry-managed-fields"

var (
	sdnRouteMapIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-_]{0,30}[a-zA-Z0-9]?$`)
	// sdnRouteMapReservedPrefix marks ids Proxmox reserves for generated maps.
	sdnRouteMapReservedPrefix = "pve_"
	sdnRouteMapReservedIDs    = []string{"MAP_VTEP_IN", "MAP_VTEP_OUT", "correct_src"}
	sdnRouteMapActions        = []string{"permit", "deny"}
)

type SDNRouteMapEntryResource struct {
	client *Client
}

type sdnRouteMapEntryModel struct {
	ID         types.String `tfsdk:"id"`
	RouteMapID types.String `tfsdk:"route_map_id"`
	Order      types.Int64  `tfsdk:"order"`
	Action     types.String `tfsdk:"action"`
	Set        types.List   `tfsdk:"set"`
	Match      types.List   `tfsdk:"match"`
	ExitAction types.String `tfsdk:"exit_action"`
	Call       types.String `tfsdk:"call"`
	State      types.String `tfsdk:"state"`
	Pending    types.Map    `tfsdk:"pending"`
}

func NewSDNRouteMapEntryResource() resource.Resource {
	return &SDNRouteMapEntryResource{}
}

func (r *SDNRouteMapEntryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_route_map_entry"
}

func (r *SDNRouteMapEntryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one entry of a Proxmox VE SDN route map through `/cluster/sdn/route-maps/entries`. Route maps exist implicitly through their entries; the entry is identified by `route_map_id` plus `order`. `set`, `match` and `exit_action` stay Proxmox property strings. Entries stage like the other SDN objects until `proxmox_sdn_apply` commits.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `route_map_id/order` form.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"route_map_id": schema.StringAttribute{Required: true, MarkdownDescription: "Route map identifier: letters, digits, `-`, `_`, at most 32 characters; the reserved Proxmox ids are rejected. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"order":        schema.Int64Attribute{Required: true, MarkdownDescription: "Entry index between 0 and 65535. Changes require replacement.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"action":       schema.StringAttribute{Required: true, MarkdownDescription: "Matching policy: `permit` or `deny`."},
			"set": schema.ListAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				MarkdownDescription: "`set` statements as Proxmox property strings, for example `local-preference=100`.",
			},
			"match": schema.ListAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				MarkdownDescription: "`match` statements as Proxmox property strings.",
			},
			"exit_action": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Exit action such as `on-match-next`, `on-match-goto=<order>`, or `continue`."},
			"call":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Route map called from this entry."},
			"state":       schema.StringAttribute{Computed: true, MarkdownDescription: "Proxmox staging state: `new` or `changed` while the entry is not committed yet."},
			"pending":     schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Properties Proxmox currently reports as staged but not committed, with `deleted` for removals."},
		},
	}
}

func (r *SDNRouteMapEntryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SDNRouteMapEntryResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config sdnRouteMapEntryModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateSDNRouteMapEntryConfig(config)...)
}

func validateSDNRouteMapEntryConfig(config sdnRouteMapEntryModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.RouteMapID) {
		id := config.RouteMapID.ValueString()
		switch {
		case !sdnRouteMapIDPattern.MatchString(id):
			diags.AddAttributeError(path.Root("route_map_id"), "Invalid route map identifier", "route_map_id may contain letters, digits, `-`, and `_` and be at most 32 characters long.")
		case strings.HasPrefix(id, sdnRouteMapReservedPrefix) || slices.Contains(sdnRouteMapReservedIDs, id):
			diags.AddAttributeError(path.Root("route_map_id"), "Reserved route map identifier", fmt.Sprintf("route_map_id %q is reserved by Proxmox.", id))
		}
	}
	if isConfiguredValue(config.Order) && (config.Order.ValueInt64() < 0 || config.Order.ValueInt64() > 65535) {
		diags.AddAttributeError(path.Root("order"), "Invalid route map order", "order must be between 0 and 65535.")
	}
	if isConfiguredValue(config.Action) && !slices.Contains(sdnRouteMapActions, config.Action.ValueString()) {
		diags.AddAttributeError(path.Root("action"), "Invalid route map action", "action must be permit or deny.")
	}
	return diags
}

func sdnRouteMapEntryRequestFromModels(config, plan sdnRouteMapEntryModel) SDNRouteMapEntryRequest {
	request := SDNRouteMapEntryRequest{
		RouteMapID: plan.RouteMapID.ValueString(),
		Order:      plan.Order.ValueInt64(),
		Action:     plan.Action.ValueString(),
	}
	if isConfiguredValue(config.Set) {
		request.Set = sdnListValues(plan.Set)
	}
	if isConfiguredValue(config.Match) {
		request.Match = sdnListValues(plan.Match)
	}
	if isConfiguredValue(config.ExitAction) {
		request.ExitAction = stringPointer(plan.ExitAction)
	}
	if isConfiguredValue(config.Call) {
		request.Call = stringPointer(plan.Call)
	}
	return request
}

func sdnRouteMapEntryManagedFields(config sdnRouteMapEntryModel) []string {
	var fields []string
	for _, field := range []struct {
		name       string
		configured bool
	}{
		{"set", isConfiguredValue(config.Set)},
		{"match", isConfiguredValue(config.Match)},
		{"exit-action", isConfiguredValue(config.ExitAction)},
		{"call", isConfiguredValue(config.Call)},
	} {
		if field.configured {
			fields = append(fields, field.name)
		}
	}
	slices.Sort(fields)
	return fields
}

func (r *SDNRouteMapEntryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config sdnRouteMapEntryModel
	var plan sdnRouteMapEntryModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	createReq := sdnRouteMapEntryRequestFromModels(config, plan)
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		createReq.LockToken = token
		return r.client.CreateSDNRouteMapEntry(ctx, createReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, createReq.RouteMapID, createReq.Order)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNRouteMapEntryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sdnRouteMapEntryModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.RouteMapID.ValueString(), state.Order.ValueInt64())
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

func (r *SDNRouteMapEntryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config sdnRouteMapEntryModel
	var plan sdnRouteMapEntryModel
	var prior sdnRouteMapEntryModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManaged, diags := readSDNRouteMapEntryManagedFields(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetSDNRouteMapEntry(ctx, prior.RouteMapID.ValueString(), prior.Order.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN Route Map Entry", err.Error())
		return
	}
	updateReq := sdnRouteMapEntryRequestFromModels(config, plan)
	updateReq.Delete = sdnDeleteKeys(sdnRouteMapEntryManagedFields(config), previouslyManaged)
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		updateReq.LockToken = token
		if current.Digest != "" {
			updateReq.Digest = &current.Digest
		}
		return r.client.UpdateSDNRouteMapEntry(ctx, updateReq.RouteMapID, updateReq.Order, updateReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, updateReq.RouteMapID, updateReq.Order)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNRouteMapEntryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sdnRouteMapEntryModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.GetSDNRouteMapEntry(ctx, state.RouteMapID.ValueString(), state.Order.ValueInt64()); err != nil {
		if errors.Is(err, errNotFound) {
			return
		}
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN Route Map Entry", err.Error())
		return
	}
	routeMapID := state.RouteMapID.ValueString()
	order := state.Order.ValueInt64()
	withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		return r.client.DeleteSDNRouteMapEntry(ctx, routeMapID, order, token)
	})
}

func (r *SDNRouteMapEntryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	routeMapID, orderText, ok := strings.Cut(req.ID, "/")
	if !ok || !sdnRouteMapIDPattern.MatchString(routeMapID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected an import identifier in <route_map_id>/<order> form.")
		return
	}
	var order int64
	if _, err := fmt.Sscanf(orderText, "%d", &order); err != nil || order < 0 || order > 65535 {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "order must be an integer between 0 and 65535.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), fmt.Sprintf("%s/%d", routeMapID, order))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("route_map_id"), routeMapID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("order"), order)...)
}

func (r *SDNRouteMapEntryResource) readState(ctx context.Context, routeMapID string, order int64) (sdnRouteMapEntryModel, diag.Diagnostics) {
	entry, err := r.client.GetSDNRouteMapEntry(ctx, routeMapID, order)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return sdnRouteMapEntryModel{ID: types.StringNull()}, nil
		}
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox SDN Route Map Entry", fmt.Sprintf("Unable to read route map entry %d of %q: %s", order, routeMapID, err))
		return sdnRouteMapEntryModel{}, diags
	}
	if entry.State == "deleted" {
		return sdnRouteMapEntryModel{ID: types.StringNull()}, nil
	}
	return sdnRouteMapEntryStateFromAPI(ctx, entry)
}

func sdnRouteMapEntryStateFromAPI(ctx context.Context, entry SDNRouteMapEntry) (sdnRouteMapEntryModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	staged := sdnPendingMap(entry.Pending)
	setValues, setDiags := types.ListValueFrom(ctx, types.StringType, sdnStagedRawList(entry.Set, staged["set"]))
	diags.Append(setDiags...)
	matchValues, matchDiags := types.ListValueFrom(ctx, types.StringType, sdnStagedRawList(entry.Match, staged["match"]))
	diags.Append(matchDiags...)
	order := int64(0)
	if entry.Order.Ptr() != nil {
		order = *entry.Order.Ptr()
	}
	state := sdnRouteMapEntryModel{
		ID:         types.StringValue(fmt.Sprintf("%s/%d", entry.RouteMapID, order)),
		RouteMapID: types.StringValue(entry.RouteMapID),
		Order:      types.Int64Value(order),
		Action:     types.StringValue(entry.Action),
		Set:        setValues,
		Match:      matchValues,
		ExitAction: sdnStagedString(entry.ExitAction, staged["exit-action"]),
		Call:       sdnStagedString(entry.Call, staged["call"]),
		State:      stringOrNull(entry.State),
		Pending:    types.MapNull(types.StringType),
	}
	if len(staged) > 0 {
		pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		diags.Append(pendingDiags...)
		state.Pending = pending
	}
	return state, diags
}

func (r *SDNRouteMapEntryResource) storeManagedFields(ctx context.Context, config sdnRouteMapEntryModel, private nodeNetworkPrivateData, diags *diag.Diagnostics) {
	managedFields, err := json.Marshal(sdnRouteMapEntryManagedFields(config))
	if err != nil {
		diags.AddError("Unable to Store SDN Route Map Entry State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, sdnRouteMapEntryManagedFieldsKey, managedFields)...)
}

func readSDNRouteMapEntryManagedFields(ctx context.Context, private nodeNetworkPrivateData) ([]string, diag.Diagnostics) {
	value, diags := private.GetKey(ctx, sdnRouteMapEntryManagedFieldsKey)
	if diags.HasError() || len(value) == 0 {
		return nil, diags
	}
	var fields []string
	if err := json.Unmarshal(value, &fields); err != nil {
		diags.AddError("Unable to Read SDN Route Map Entry State", fmt.Sprintf("unable to decode managed fields: %v", err))
	}
	return fields, diags
}
