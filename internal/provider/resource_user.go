// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &UserResource{}
var _ resource.ResourceWithImportState = &UserResource{}

type UserResource struct {
	client *Client
}

type userResourceModel struct {
	ID        types.String `tfsdk:"id"`
	UserID    types.String `tfsdk:"user_id"`
	Comment   types.String `tfsdk:"comment"`
	Email     types.String `tfsdk:"email"`
	Enable    types.Bool   `tfsdk:"enable"`
	Expire    types.Int64  `tfsdk:"expire"`
	Firstname types.String `tfsdk:"firstname"`
	Lastname  types.String `tfsdk:"lastname"`
	Groups    types.String `tfsdk:"groups"`
	Keys      types.String `tfsdk:"keys"`
	Password  types.String `tfsdk:"password"`
}

func NewUserResource() resource.Resource {
	return &UserResource{}
}

func (r *UserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox VE user through `/access/users`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "User identifier (e.g. `name@pam`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Full User ID in `name@realm` format. Changes require replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comment":   schema.StringAttribute{Optional: true, MarkdownDescription: "Optional user comment."},
			"email":     schema.StringAttribute{Optional: true, MarkdownDescription: "Email address."},
			"enable":    schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable the account (default)."},
			"expire":    schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Account expiration date (seconds since epoch). 0 means no expiration."},
			"firstname": schema.StringAttribute{Optional: true, MarkdownDescription: "First name."},
			"lastname":  schema.StringAttribute{Optional: true, MarkdownDescription: "Last name."},
			"groups":    schema.StringAttribute{Optional: true, MarkdownDescription: "Comma-separated list of group IDs."},
			"keys":      schema.StringAttribute{Optional: true, MarkdownDescription: "Keys for two-factor auth (yubico)."},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Initial password. Proxmox does not report this, so it is not read back.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *UserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func userRequestFromModel(model userResourceModel) UserRequest {
	return UserRequest{
		Comment:   stringPointer(model.Comment),
		Email:     stringPointer(model.Email),
		Enable:    boolPointerValue(model.Enable),
		Expire:    int64PointerValue(model.Expire),
		Firstname: stringPointer(model.Firstname),
		Lastname:  stringPointer(model.Lastname),
		Groups:    stringPointer(model.Groups),
		Keys:      stringPointer(model.Keys),
		Password:  stringPointer(model.Password),
	}
}

func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.CreateUser(ctx, plan.UserID.ValueString(), userRequestFromModel(plan)); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox User", err.Error())
		return
	}
	state, diags := r.readUserState(ctx, plan.UserID.ValueString(), &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readUserState(ctx, state.UserID.ValueString(), &state)
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

func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userResourceModel
	var state userResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateReq := userRequestFromModel(plan)
	updateReq.Password = nil
	if err := r.client.UpdateUser(ctx, plan.UserID.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox User", err.Error())
		return
	}
	refreshed, diags := r.readUserState(ctx, plan.UserID.ValueString(), &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteUser(ctx, state.UserID.ValueString()); err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Unable to Delete Proxmox User", err.Error())
	}
}

func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("user_id"), req, resp)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *UserResource) readUserState(ctx context.Context, userID string, prior *userResourceModel) (userResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	user, err := r.client.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return userResourceModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox User", fmt.Sprintf("Unable to read user %q: %s", userID, err))
		return userResourceModel{}, diags
	}
	password := types.StringNull()
	if prior != nil && !prior.Password.IsNull() && !prior.Password.IsUnknown() {
		password = prior.Password
	}
	return userResourceModel{
		ID:        types.StringValue(user.UserID),
		UserID:    types.StringValue(user.UserID),
		Comment:   stringOrNull(user.Comment),
		Email:     stringOrNull(user.Email),
		Enable:    boolOrNull(user.Enable.Ptr()),
		Expire:    int64OrNull(user.Expire.Ptr()),
		Firstname: stringOrNull(user.Firstname),
		Lastname:  stringOrNull(user.Lastname),
		Groups:    stringOrNull(user.Groups),
		Keys:      stringOrNull(user.Keys),
		Password:  password,
	}, diags
}
