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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SDNPrefixListResource{}
var _ resource.ResourceWithImportState = &SDNPrefixListResource{}
var _ resource.ResourceWithValidateConfig = &SDNPrefixListResource{}

const sdnPrefixListManagedFieldsKey = "sdn-prefix-list-managed-fields"

var sdnPrefixListIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-_]{0,30}[a-zA-Z0-9]?$`)
var sdnPrefixListReservedIDs = []string{"only_default", "only_default_v6", "loopbacks_ips"}

type SDNPrefixListResource struct {
	client *Client
}

type sdnPrefixListModel struct {
	ID      types.String `tfsdk:"id"`
	ListID  types.String `tfsdk:"prefix_list_id"`
	Entries types.List   `tfsdk:"entries"`
	State   types.String `tfsdk:"state"`
	Pending types.Map    `tfsdk:"pending"`
}

func NewSDNPrefixListResource() resource.Resource {
	return &SDNPrefixListResource{}
}

func (r *SDNPrefixListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_prefix_list"
}

func (r *SDNPrefixListResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox VE SDN prefix list through `/cluster/sdn/prefix-lists`. Entries stay Proxmox property strings such as `action=permit,prefix=10.0.0.0/24`. Prefix lists are referenced by fabrics and route maps and stage like the other SDN objects until `proxmox_sdn_apply` commits.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Prefix list identifier (same as `prefix_list_id`).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"prefix_list_id": schema.StringAttribute{Required: true, MarkdownDescription: "Prefix list identifier: letters, digits, `-`, `_`, at most 32 characters; the reserved Proxmox ids are rejected. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"entries": schema.ListAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				MarkdownDescription: "Prefix list entries as Proxmox property strings, for example `action=permit,prefix=10.0.0.0/24,le=32`.",
			},
			"state":   schema.StringAttribute{Computed: true, MarkdownDescription: "Proxmox staging state: `new` or `changed` while the list is not committed yet."},
			"pending": schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Properties Proxmox currently reports as staged but not committed, with `deleted` for removals."},
		},
	}
}

func (r *SDNPrefixListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SDNPrefixListResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config sdnPrefixListModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateSDNPrefixListConfig(config)...)
}

func validateSDNPrefixListConfig(config sdnPrefixListModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.ListID) {
		id := config.ListID.ValueString()
		switch {
		case !sdnPrefixListIDPattern.MatchString(id):
			diags.AddAttributeError(path.Root("prefix_list_id"), "Invalid prefix list identifier", "prefix_list_id may contain letters, digits, `-`, and `_` and be at most 32 characters long.")
		case slices.Contains(sdnPrefixListReservedIDs, id):
			diags.AddAttributeError(path.Root("prefix_list_id"), "Reserved prefix list identifier", fmt.Sprintf("prefix_list_id %q is reserved by Proxmox.", id))
		}
	}
	for index, entry := range sdnListValues(config.Entries) {
		if !strings.Contains(entry, "action=permit") && !strings.Contains(entry, "action=deny") {
			diags.AddAttributeError(path.Root("entries").AtListIndex(index), "Invalid prefix list entry", "each entry needs `action=permit` or `action=deny`.")
		}
		if !strings.Contains(entry, "prefix=") {
			diags.AddAttributeError(path.Root("entries").AtListIndex(index), "Invalid prefix list entry", "each entry needs a `prefix=` CIDR.")
		}
	}
	return diags
}

func sdnPrefixListManagedFields(config sdnPrefixListModel) []string {
	if isConfiguredValue(config.Entries) {
		return []string{"entries"}
	}
	return nil
}

func (r *SDNPrefixListResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config sdnPrefixListModel
	var plan sdnPrefixListModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	createReq := SDNPrefixListRequest{Entries: sdnListValues(plan.Entries)}
	id := plan.ListID.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		createReq.LockToken = token
		return r.client.CreateSDNPrefixList(ctx, id, createReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, id)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNPrefixListResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sdnPrefixListModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.ListID.ValueString())
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

func (r *SDNPrefixListResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config sdnPrefixListModel
	var plan sdnPrefixListModel
	var prior sdnPrefixListModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManaged, diags := readSDNPrefixListManagedFields(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetSDNPrefixList(ctx, prior.ListID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN Prefix List", err.Error())
		return
	}
	updateReq := SDNPrefixListRequest{
		Entries: sdnListValues(plan.Entries),
		Delete:  sdnDeleteKeys(sdnPrefixListManagedFields(config), previouslyManaged),
	}
	id := prior.ListID.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		updateReq.LockToken = token
		if current.Digest != "" {
			updateReq.Digest = &current.Digest
		}
		return r.client.UpdateSDNPrefixList(ctx, id, updateReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, id)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNPrefixListResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sdnPrefixListModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.GetSDNPrefixList(ctx, state.ListID.ValueString()); err != nil {
		if errors.Is(err, errNotFound) {
			return
		}
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN Prefix List", err.Error())
		return
	}
	id := state.ListID.ValueString()
	withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		return r.client.DeleteSDNPrefixList(ctx, id, token)
	})
}

func (r *SDNPrefixListResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !sdnPrefixListIDPattern.MatchString(req.ID) || slices.Contains(sdnPrefixListReservedIDs, req.ID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected a non-reserved SDN prefix list identifier.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("prefix_list_id"), req.ID)...)
}

func (r *SDNPrefixListResource) readState(ctx context.Context, id string) (sdnPrefixListModel, diag.Diagnostics) {
	entry, err := r.client.GetSDNPrefixList(ctx, id)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return sdnPrefixListModel{ID: types.StringNull()}, nil
		}
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox SDN Prefix List", fmt.Sprintf("Unable to read SDN prefix list %q: %s", id, err))
		return sdnPrefixListModel{}, diags
	}
	if entry.State == "deleted" {
		return sdnPrefixListModel{ID: types.StringNull()}, nil
	}
	return sdnPrefixListStateFromAPI(ctx, entry)
}

func sdnPrefixListStateFromAPI(ctx context.Context, entry SDNPrefixList) (sdnPrefixListModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	staged := sdnPendingMap(entry.Pending)
	entries, listDiags := types.ListValueFrom(ctx, types.StringType, sdnStagedRawList(entry.Entries, staged["entries"]))
	diags.Append(listDiags...)
	state := sdnPrefixListModel{
		ID:      types.StringValue(entry.ID),
		ListID:  types.StringValue(entry.ID),
		Entries: entries,
		State:   stringOrNull(entry.State),
		Pending: types.MapNull(types.StringType),
	}
	if len(staged) > 0 {
		pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		diags.Append(pendingDiags...)
		state.Pending = pending
	}
	return state, diags
}

func (r *SDNPrefixListResource) storeManagedFields(ctx context.Context, config sdnPrefixListModel, private nodeNetworkPrivateData, diags *diag.Diagnostics) {
	managedFields, err := json.Marshal(sdnPrefixListManagedFields(config))
	if err != nil {
		diags.AddError("Unable to Store SDN Prefix List State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, sdnPrefixListManagedFieldsKey, managedFields)...)
}

func readSDNPrefixListManagedFields(ctx context.Context, private nodeNetworkPrivateData) ([]string, diag.Diagnostics) {
	value, diags := private.GetKey(ctx, sdnPrefixListManagedFieldsKey)
	if diags.HasError() || len(value) == 0 {
		return nil, diags
	}
	var fields []string
	if err := json.Unmarshal(value, &fields); err != nil {
		diags.AddError("Unable to Read SDN Prefix List State", fmt.Sprintf("unable to decode managed fields: %v", err))
	}
	return fields, diags
}
