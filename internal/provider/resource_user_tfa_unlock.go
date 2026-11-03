// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &UserTFAUnlockResource{}
var _ resource.ResourceWithImportState = &UserTFAUnlockResource{}
var _ resource.ResourceWithValidateConfig = &UserTFAUnlockResource{}

type UserTFAUnlockResource struct{ client *Client }

type userTFAUnlockModel struct {
	ID       types.String `tfsdk:"id"`
	UserID   types.String `tfsdk:"user_id"`
	Triggers types.Map    `tfsdk:"triggers"`
	Unlocked types.Bool   `tfsdk:"unlocked"`
}

func NewUserTFAUnlockResource() resource.Resource { return &UserTFAUnlockResource{} }

func (r *UserTFAUnlockResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_tfa_unlock"
}

func (r *UserTFAUnlockResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Clears the TFA lockout of one user through `PUT /access/users/{userid}/unlock-tfa`. Proxmox locks users after too many failed second factors; the lock also clears automatically after a timeout. Every change replaces the instance and unlocks again; destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true, MarkdownDescription: "User identifier.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"user_id":  schema.StringAttribute{Required: true, MarkdownDescription: "User in `name@realm` form. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to unlock again; changing any of them unlocks again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"unlocked": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Proxmox reported the unlock as successful."},
		},
	}
}

func (r *UserTFAUnlockResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *UserTFAUnlockResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config userTFAUnlockModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if isConfiguredValue(config.UserID) && !userIDPattern.MatchString(config.UserID.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("user_id"), "Invalid user id", "user_id must use the name@realm form.")
	}
	if isConfiguredValue(config.Triggers) && len(config.Triggers.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(path.Root("triggers"), "Missing unlock triggers", "triggers must contain at least one value so an unlock can be re-run.")
	}
}

func (r *UserTFAUnlockResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userTFAUnlockModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.unlock(ctx, &plan, &resp.State, &resp.Diagnostics)
}

func (r *UserTFAUnlockResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Unlocking is a command; keep the recorded state.
}

func (r *UserTFAUnlockResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userTFAUnlockModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.unlock(ctx, &plan, &resp.State, &resp.Diagnostics)
}

func (r *UserTFAUnlockResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// A lockout that was cleared stays cleared.
}

func (r *UserTFAUnlockResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	userID := strings.TrimSpace(req.ID)
	if !userIDPattern.MatchString(userID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected a user id in name@realm form.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), userID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), userID)...)
}

func (r *UserTFAUnlockResource) unlock(ctx context.Context, plan *userTFAUnlockModel, state *tfsdk.State, diags *diag.Diagnostics) {
	userID := plan.UserID.ValueString()
	unlocked, err := r.client.UnlockUserTFA(ctx, userID)
	if err != nil {
		// Proxmox answers 500 "no such user" when the user has no TFA entry yet;
		// only a really missing user stays an error.
		if !strings.Contains(err.Error(), "no such user") {
			diags.AddError("Unable to Unlock Proxmox User TFA", fmt.Sprintf("Unable to call `/access/users/%s/unlock-tfa`: %s", userID, err))
			return
		}
		if _, userErr := r.client.GetUser(ctx, userID); userErr != nil {
			diags.AddError("Unable to Unlock Proxmox User TFA", fmt.Sprintf("Unable to call `/access/users/%s/unlock-tfa`: %s", userID, err))
			return
		}
		unlocked = false
	}
	next := *plan
	next.ID = types.StringValue(userID)
	next.Unlocked = types.BoolValue(unlocked)
	diags.Append(state.Set(ctx, &next)...)
}
