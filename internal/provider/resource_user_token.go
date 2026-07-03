// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &UserTokenResource{}
var _ resource.ResourceWithImportState = &UserTokenResource{}

type UserTokenResource struct {
	client *Client
}

type userTokenResourceModel struct {
	ID          types.String `tfsdk:"id"`
	UserID      types.String `tfsdk:"user_id"`
	TokenID     types.String `tfsdk:"token_id"`
	FullTokenID types.String `tfsdk:"full_token_id"`
	Value       types.String `tfsdk:"value"`
	Comment     types.String `tfsdk:"comment"`
	Expire      types.Int64  `tfsdk:"expire"`
	Privsep     types.Bool   `tfsdk:"privsep"`
}

func NewUserTokenResource() resource.Resource {
	return &UserTokenResource{}
}

func (r *UserTokenResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_token"
}

func (r *UserTokenResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox VE API token through `/access/users/{userid}/token/{tokenid}`. The token `value` (secret) is only returned once at creation time and is preserved in state; it cannot be read back from Proxmox.",
		Attributes: map[string]schema.Attribute{
			"id":            schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `userid/tokenid` form.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"user_id":       schema.StringAttribute{Required: true, MarkdownDescription: "Full User ID in `name@realm` format. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"token_id":      schema.StringAttribute{Required: true, MarkdownDescription: "User-specific token identifier. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"full_token_id": schema.StringAttribute{Computed: true, MarkdownDescription: "Full token ID in `userid!tokenid` format.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"value":         schema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "API token value (secret). Only available at creation time.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"comment":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Optional token comment."},
			"expire":        schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Token expiration date (seconds since epoch). 0 means no expiration."},
			"privsep":       schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Restrict API token privileges with separate ACLs (default true)."},
		},
	}
}

func (r *UserTokenResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *UserTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userTokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	token, err := r.client.CreateUserToken(ctx, plan.UserID.ValueString(), plan.TokenID.ValueString(), UserTokenRequest{
		Comment: stringPointer(plan.Comment),
		Expire:  int64PointerValue(plan.Expire),
		Privsep: boolPointerValue(plan.Privsep),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox User Token", err.Error())
		return
	}
	state, diags := r.readTokenState(ctx, plan.UserID.ValueString(), plan.TokenID.ValueString(), &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.FullTokenID = stringOrNull(token.FullTokenID)
	state.Value = stringOrNull(token.Value)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *UserTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userTokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readTokenState(ctx, state.UserID.ValueString(), state.TokenID.ValueString(), &state)
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

func (r *UserTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userTokenResourceModel
	var state userTokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateUserToken(ctx, plan.UserID.ValueString(), plan.TokenID.ValueString(), UserTokenRequest{
		Comment: stringPointer(plan.Comment),
		Expire:  int64PointerValue(plan.Expire),
		Privsep: boolPointerValue(plan.Privsep),
	}); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox User Token", err.Error())
		return
	}
	refreshed, diags := r.readTokenState(ctx, plan.UserID.ValueString(), plan.TokenID.ValueString(), &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *UserTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userTokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteUserToken(ctx, state.UserID.ValueString(), state.TokenID.ValueString()); err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Unable to Delete Proxmox User Token", err.Error())
	}
}

func (r *UserTokenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "expected identifier in userid/tokenid form")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("token_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *UserTokenResource) readTokenState(ctx context.Context, userID, tokenID string, prior *userTokenResourceModel) (userTokenResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	token, err := r.client.GetUserToken(ctx, userID, tokenID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return userTokenResourceModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox User Token", err.Error())
		return userTokenResourceModel{}, diags
	}
	id := userID + "/" + tokenID
	value := types.StringNull()
	fullTokenID := types.StringNull()
	if prior != nil {
		if !prior.Value.IsNull() && !prior.Value.IsUnknown() {
			value = prior.Value
		}
		if !prior.FullTokenID.IsNull() && !prior.FullTokenID.IsUnknown() {
			fullTokenID = prior.FullTokenID
		}
	}
	return userTokenResourceModel{
		ID:          types.StringValue(id),
		UserID:      types.StringValue(userID),
		TokenID:     types.StringValue(tokenID),
		FullTokenID: fullTokenID,
		Value:       value,
		Comment:     stringOrNull(token.Comment),
		Expire:      int64OrNull(token.Expire.Ptr()),
		Privsep:     boolOrNull(token.Privsep.Ptr()),
	}, diags
}
