// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &UserTFAResource{}
var _ resource.ResourceWithImportState = &UserTFAResource{}
var _ resource.ResourceWithValidateConfig = &UserTFAResource{}

type UserTFAResource struct {
	client *Client
}

type userTFAModel struct {
	ID            types.String `tfsdk:"id"`
	UserID        types.String `tfsdk:"user_id"`
	EntryID       types.String `tfsdk:"entry_id"`
	Type          types.String `tfsdk:"type"`
	Description   types.String `tfsdk:"description"`
	Enable        types.Bool   `tfsdk:"enable"`
	TOTP          types.String `tfsdk:"totp"`
	Value         types.String `tfsdk:"value"`
	Password      types.String `tfsdk:"password"`
	RecoveryCodes types.List   `tfsdk:"recovery_codes"`
}

var (
	userIDPattern   = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z][A-Za-z0-9._-]*$`)
	userTFATypes    = []string{"totp", "recovery", "yubico"}
	userTFATOTPKind = "otpauth://"
)

func NewUserTFAResource() resource.Resource { return &UserTFAResource{} }

func (r *UserTFAResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_tfa"
}

func (r *UserTFAResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one second-factor entry of a Proxmox VE user through `/access/tfa/{userid}`. Only the non-interactive types are supported: `totp`, `recovery` and `yubico` (`webauthn`/`u2f` need a browser registration ceremony). Proxmox rejects API tokens on these endpoints, so the provider must authenticate with a ticket; a non-`root@pam` identity additionally needs `password`.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `user_id/entry_id` form.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"user_id":  schema.StringAttribute{Required: true, MarkdownDescription: "User the factor belongs to, in `name@realm` form. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"entry_id": schema.StringAttribute{Computed: true, MarkdownDescription: "Server-assigned factor id.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"type":     schema.StringAttribute{Required: true, MarkdownDescription: "Factor type: `totp`, `recovery`, or `yubico`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"description": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Description distinguishing multiple factors.",
			},
			"enable": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Whether the factor is enabled for login. The Proxmox default is `true`.",
			},
			"totp": schema.StringAttribute{
				Optional: true, Sensitive: true, MarkdownDescription: "`otpauth://` provisioning URI. Required for `totp`; Proxmox stores the shared secret, so the URI is kept from state and never read back. Changes require replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"value": schema.StringAttribute{
				Optional: true, Sensitive: true, MarkdownDescription: "Current OTP value. Required for `yubico` (Proxmox only keeps its public key prefix) and optional for `totp` as a verification code. Changes require replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"password": schema.StringAttribute{
				Optional: true, Sensitive: true, MarkdownDescription: "Password of the authenticating identity. Required when the provider does not authenticate as `root@pam`.",
			},
			"recovery_codes": schema.ListAttribute{
				Computed: true, Sensitive: true, ElementType: types.StringType, MarkdownDescription: "Generated recovery codes, only for `recovery` entries and only known right after creation.",
			},
		},
	}
}

func (r *UserTFAResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *UserTFAResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config userTFAModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateUserTFAConfig(config)...)
}

func validateUserTFAConfig(config userTFAModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.UserID) && !userIDPattern.MatchString(config.UserID.ValueString()) {
		diags.AddAttributeError(path.Root("user_id"), "Invalid user id", "user_id must use the name@realm form.")
	}
	if isConfiguredValue(config.Type) && !slices.Contains(userTFATypes, config.Type.ValueString()) {
		diags.AddAttributeError(path.Root("type"), "Unsupported factor type", "type must be totp, recovery, or yubico.")
	}
	if isConfiguredValue(config.Description) && len(config.Description.ValueString()) > 255 {
		diags.AddAttributeError(path.Root("description"), "Invalid description", "description must not exceed 255 characters.")
	}
	if !isConfiguredValue(config.Type) {
		return diags
	}
	switch config.Type.ValueString() {
	case "totp":
		if !isConfiguredValue(config.TOTP) {
			diags.AddAttributeError(path.Root("totp"), "Missing TOTP URI", "totp is required for totp factors.")
		} else if !strings.HasPrefix(config.TOTP.ValueString(), userTFATOTPKind) {
			diags.AddAttributeError(path.Root("totp"), "Invalid TOTP URI", "totp must be an otpauth:// URI.")
		}
	case "yubico":
		if !isConfiguredValue(config.Value) {
			diags.AddAttributeError(path.Root("value"), "Missing Yubico OTP", "value is required for yubico factors.")
		}
		if isConfiguredValue(config.TOTP) {
			diags.AddAttributeError(path.Root("totp"), "Unexpected TOTP URI", "totp is only used by totp factors.")
		}
	case "recovery":
		if isConfiguredValue(config.TOTP) {
			diags.AddAttributeError(path.Root("totp"), "Unexpected TOTP URI", "totp is only used by totp factors.")
		}
		if isConfiguredValue(config.Value) {
			diags.AddAttributeError(path.Root("value"), "Unexpected OTP value", "value is only used by yubico factors.")
		}
	}
	return diags
}

func (r *UserTFAResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config userTFAModel
	var plan userTFAModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	userID := plan.UserID.ValueString()
	result, err := r.client.CreateUserTFA(ctx, userID, UserTFACreateRequest{
		Type:        plan.Type.ValueString(),
		Description: stringPointerValue(config.Description),
		TOTP:        stringPointerValue(config.TOTP),
		Value:       stringPointerValue(config.Value),
		Password:    stringPointerValue(config.Password),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox User TFA Entry", err.Error())
		return
	}
	state, stateDiags := r.readState(ctx, userID, result.ID, &plan)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(result.Recovery) > 0 {
		codes, codeDiags := types.ListValueFrom(ctx, types.StringType, result.Recovery)
		resp.Diagnostics.Append(codeDiags...)
		state.RecoveryCodes = codes
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *UserTFAResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userTFAModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.UserID.ValueString(), state.EntryID.ValueString(), &state)
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

func (r *UserTFAResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config userTFAModel
	var plan userTFAModel
	var prior userTFAModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateReq := UserTFAUpdateRequest{Password: stringPointerValue(config.Password)}
	if isConfiguredValue(config.Description) {
		updateReq.Description = stringPointer(plan.Description)
	}
	if isConfiguredValue(config.Enable) {
		updateReq.Enable = boolPointerValue(plan.Enable)
	}
	if err := r.client.UpdateUserTFA(ctx, prior.UserID.ValueString(), prior.EntryID.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox User TFA Entry", err.Error())
		return
	}
	state, stateDiags := r.readState(ctx, prior.UserID.ValueString(), prior.EntryID.ValueString(), &prior)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Password = plan.Password
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *UserTFAResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userTFAModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteUserTFA(ctx, state.UserID.ValueString(), state.EntryID.ValueString(), stringPointerValue(state.Password)); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox User TFA Entry", err.Error())
	}
}

func (r *UserTFAResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	userID, entryID, ok := strings.Cut(req.ID, "/")
	if !ok || !userIDPattern.MatchString(userID) || entryID == "" {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected an identifier in user_id/entry_id form.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), userID+"/"+entryID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), userID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("entry_id"), entryID)...)
}

func (r *UserTFAResource) readState(ctx context.Context, userID, entryID string, prior *userTFAModel) (userTFAModel, diag.Diagnostics) {
	entry, err := r.client.GetUserTFA(ctx, userID, entryID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return userTFAModel{ID: types.StringNull()}, nil
		}
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox User TFA Entry", fmt.Sprintf("Unable to read TFA entry %q of user %q: %s", entryID, userID, err))
		return userTFAModel{}, diags
	}
	state := userTFAModel{
		ID:            types.StringValue(userID + "/" + entryID),
		UserID:        types.StringValue(userID),
		EntryID:       types.StringValue(entryID),
		Type:          stringOrNull(entry.Type),
		Description:   stringOrNull(entry.Description),
		Enable:        boolOrNull(entry.Enable.Ptr()),
		TOTP:          types.StringNull(),
		Value:         types.StringNull(),
		Password:      types.StringNull(),
		RecoveryCodes: types.ListNull(types.StringType),
	}
	if state.Enable.IsNull() {
		state.Enable = types.BoolValue(true)
	}
	// Secrets are never returned by Proxmox; keep the configured inputs.
	state.TOTP = prior.TOTP
	state.Value = prior.Value
	state.Password = prior.Password
	state.RecoveryCodes = prior.RecoveryCodes
	return state, nil
}
