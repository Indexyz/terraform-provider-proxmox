// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &ACLResource{}
var _ resource.ResourceWithImportState = &ACLResource{}

type ACLResource struct {
	client *Client
}

type aclResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Path      types.String `tfsdk:"path"`
	Propagate types.Bool   `tfsdk:"propagate"`
	Roles     types.List   `tfsdk:"roles"`
	Users     types.List   `tfsdk:"users"`
	Groups    types.List   `tfsdk:"groups"`
}

func NewACLResource() resource.Resource {
	return &ACLResource{}
}

func (r *ACLResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acl"
}

func (r *ACLResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Proxmox VE ACL permissions for a given path through `/access/acl`. This resource binds roles to users and/or groups on the given path. At least one of `users` or `groups` must be set, and `roles` must have at least one entry.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier derived from the ACL path.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"path": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Proxmox object path (e.g. `/`, `/vms/101`, `/storage/local-lvm`, `/pool/my-pool`). Changes require replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"propagate": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Allow permissions to propagate (inherit) to child objects. Defaults to true.",
			},
			"roles": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "List of role IDs to assign.",
			},
			"users": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "List of user IDs (e.g. `admin@pam`) to assign the roles to. At least one of `users` or `groups` must be set.",
			},
			"groups": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "List of group IDs to assign the roles to. At least one of `users` or `groups` must be set.",
			},
		},
	}
}

func (r *ACLResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ACLResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan aclResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.applyACL(ctx, plan, false); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox ACL", err.Error())
		return
	}
	state, diags := r.readACLState(ctx, plan.Path.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ACLResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state aclResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readACLState(ctx, state.Path.ValueString())
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

func (r *ACLResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan aclResourceModel
	var state aclResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Determine removed bindings and delete them, then add all current bindings.
	removed := r.diffRemovedBindings(ctx, plan, state)
	for _, rb := range removed {
		if err := r.deleteBinding(ctx, rb); err != nil {
			resp.Diagnostics.AddError("Unable to Remove Proxmox ACL Binding", err.Error())
			return
		}
	}
	if err := r.applyACL(ctx, plan, false); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox ACL", err.Error())
		return
	}
	refreshed, diags := r.readACLState(ctx, plan.Path.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *ACLResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state aclResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.applyACL(ctx, state, true); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox ACL", err.Error())
	}
}

func (r *ACLResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("path"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// applyACL sets all role/user/group bindings for the path in a single API call.
// Proxmox PUT /access/acl adds (delete=false) or removes (delete=true) all
// role×user/group combinations for the path.
func (r *ACLResource) applyACL(ctx context.Context, model aclResourceModel, remove bool) error {
	roles, diags := listToStringSlice(ctx, model.Roles)
	if diags.HasError() {
		return fmt.Errorf("unable to read roles: %v", diags)
	}
	users, diags := listToStringSlice(ctx, model.Users)
	if diags.HasError() {
		return fmt.Errorf("unable to read users: %v", diags)
	}
	groups, diags := listToStringSlice(ctx, model.Groups)
	if diags.HasError() {
		return fmt.Errorf("unable to read groups: %v", diags)
	}
	if len(roles) == 0 {
		return fmt.Errorf("at least one role is required")
	}
	if len(users) == 0 && len(groups) == 0 {
		return fmt.Errorf("at least one of users or groups must be set")
	}
	propagate := true
	if !model.Propagate.IsNull() && !model.Propagate.IsUnknown() {
		propagate = model.Propagate.ValueBool()
	}
	req := ACLRequest{
		Path:      model.Path.ValueString(),
		Roles:     strings.Join(sortedStrings(roles), ","),
		Users:     strings.Join(sortedStrings(users), ","),
		Groups:    strings.Join(sortedStrings(groups), ","),
		Propagate: &propagate,
		Delete:    remove,
	}
	return r.client.SetACL(ctx, req)
}

// diffRemovedBindings returns bindings present in state but no longer in plan.
func (r *ACLResource) diffRemovedBindings(ctx context.Context, plan aclResourceModel, state aclResourceModel) []ACLEntry {
	planKeys := map[string]bool{}
	planRoles, _ := listToStringSlice(ctx, plan.Roles)
	planUsers, _ := listToStringSlice(ctx, plan.Users)
	planGroups, _ := listToStringSlice(ctx, plan.Groups)
	for _, role := range planRoles {
		for _, user := range planUsers {
			planKeys[fmt.Sprintf("user|%s|%s", user, role)] = true
		}
		for _, group := range planGroups {
			planKeys[fmt.Sprintf("group|%s|%s", group, role)] = true
		}
	}

	stateRoles, _ := listToStringSlice(ctx, state.Roles)
	stateUsers, _ := listToStringSlice(ctx, state.Users)
	stateGroups, _ := listToStringSlice(ctx, state.Groups)
	var removed []ACLEntry
	for _, role := range stateRoles {
		for _, user := range stateUsers {
			key := fmt.Sprintf("user|%s|%s", user, role)
			if !planKeys[key] {
				removed = append(removed, ACLEntry{Path: state.Path.ValueString(), RoleID: role, Type: "user", UGID: user})
			}
		}
		for _, group := range stateGroups {
			key := fmt.Sprintf("group|%s|%s", group, role)
			if !planKeys[key] {
				removed = append(removed, ACLEntry{Path: state.Path.ValueString(), RoleID: role, Type: "group", UGID: group})
			}
		}
	}
	return removed
}

func (r *ACLResource) deleteBinding(ctx context.Context, binding ACLEntry) error {
	req := ACLRequest{
		Path:      binding.Path,
		Roles:     binding.RoleID,
		Propagate: boolPointerValue(types.BoolValue(true)),
		Delete:    true,
	}
	if binding.Type == "user" {
		req.Users = binding.UGID
	} else {
		req.Groups = binding.UGID
	}
	return r.client.SetACL(ctx, req)
}

func (r *ACLResource) readACLState(ctx context.Context, aclPath string) (aclResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	entries, err := r.client.GetACL(ctx)
	if err != nil {
		diags.AddError("Unable to Read Proxmox ACL", err.Error())
		return aclResourceModel{}, diags
	}

	pathEntries := []ACLEntry{}
	for _, e := range entries {
		if e.Path == aclPath {
			pathEntries = append(pathEntries, e)
		}
	}
	if len(pathEntries) == 0 {
		return aclResourceModel{ID: types.StringNull()}, diags
	}

	roleSet := map[string]bool{}
	userSet := map[string]bool{}
	groupSet := map[string]bool{}
	propagate := true
	for _, e := range pathEntries {
		roleSet[e.RoleID] = true
		switch e.Type {
		case "user":
			userSet[e.UGID] = true
		case "group":
			groupSet[e.UGID] = true
		}
		if e.Propagate.Ptr() != nil {
			propagate = *e.Propagate.Ptr()
		}
	}

	roles := sortedSetKeys(roleSet)
	users := sortedSetKeys(userSet)
	groups := sortedSetKeys(groupSet)

	roleList, roleDiags := types.ListValueFrom(ctx, types.StringType, roles)
	diags.Append(roleDiags...)
	userList, userDiags := types.ListValueFrom(ctx, types.StringType, users)
	diags.Append(userDiags...)
	groupList, groupDiags := types.ListValueFrom(ctx, types.StringType, groups)
	diags.Append(groupDiags...)
	if diags.HasError() {
		return aclResourceModel{}, diags
	}

	return aclResourceModel{
		ID:        types.StringValue(aclPath),
		Path:      types.StringValue(aclPath),
		Propagate: types.BoolValue(propagate),
		Roles:     roleList,
		Users:     userList,
		Groups:    groupList,
	}, diags
}

func sortedSetKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func listToStringSlice(ctx context.Context, list types.List) ([]string, diag.Diagnostics) {
	if list.IsNull() || list.IsUnknown() {
		return nil, nil
	}
	var result []string
	diags := list.ElementsAs(ctx, &result, false)
	return result, diags
}
