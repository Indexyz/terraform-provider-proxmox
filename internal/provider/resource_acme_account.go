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

var _ resource.Resource = &ACMEAccountResource{}
var _ resource.ResourceWithImportState = &ACMEAccountResource{}
var _ resource.ResourceWithValidateConfig = &ACMEAccountResource{}

type ACMEAccountResource struct{ client *Client }

type acmeAccountModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Contact   types.String `tfsdk:"contact"`
	Directory types.String `tfsdk:"directory"`
	TOS       types.String `tfsdk:"tos_url"`
	EABKID    types.String `tfsdk:"eab_kid"`
	EABHMAC   types.String `tfsdk:"eab_hmac_key"`
	Location  types.String `tfsdk:"location"`
}

func NewACMEAccountResource() resource.Resource { return &ACMEAccountResource{} }

func (r *ACMEAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acme_account"
}

func (r *ACMEAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Registers an ACME account with a certificate authority through `/cluster/acme/account`. The account is the prerequisite for ACME certificate orders on nodes.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Account name."},
			"name":         schema.StringAttribute{Required: true, MarkdownDescription: "Account name. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"contact":      schema.StringAttribute{Required: true, MarkdownDescription: "Contact address, usually `mailto:admin@example.com`."},
			"directory":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "ACME directory URL. Proxmox defaults to Let's Encrypt production.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"tos_url":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Override the terms-of-service URL if the CA does not advertise one.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"eab_kid":      schema.StringAttribute{Optional: true, MarkdownDescription: "External account binding key id, for CAs that require it.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"eab_hmac_key": schema.StringAttribute{Optional: true, Sensitive: true, MarkdownDescription: "External account binding HMAC key (base64url).", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"location":     schema.StringAttribute{Computed: true, MarkdownDescription: "Account URL reported by the CA."},
		},
	}
}

func (r *ACMEAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *ACMEAccountResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config acmeAccountModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if isConfiguredValue(config.Name) && !notificationNamePattern.MatchString(config.Name.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid account name", "name must be a valid configuration id.")
	}
	if isConfiguredValue(config.Contact) && !strings.Contains(config.Contact.ValueString(), "@") {
		resp.Diagnostics.AddAttributeError(path.Root("contact"), "Invalid contact", "contact must be an email address.")
	}
	hasKID := isConfiguredValue(config.EABKID)
	hasHMAC := isConfiguredValue(config.EABHMAC)
	if hasKID != hasHMAC {
		resp.Diagnostics.AddAttributeError(path.Root("eab_hmac_key"), "Incomplete external account binding", "eab_kid and eab_hmac_key must be configured together.")
	}
}

func (r *ACMEAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan acmeAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	account := ACMEAccount{Name: plan.Name.ValueString(), Directory: stringValue(plan.Directory), TOS: stringValue(plan.TOS)}
	account.Details.Contact = []string{plan.Contact.ValueString()}
	upid, err := r.client.CreateACMEAccount(ctx, account, stringValue(plan.EABKID), stringValue(plan.EABHMAC))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox ACME Account", err.Error())
		return
	}
	if !r.wait(ctx, upid, &resp.Diagnostics) {
		return
	}
	state, diags := r.readState(ctx, plan.Name.ValueString(), plan)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ACMEAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state acmeAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.Name.ValueString(), state)
	resp.Diagnostics.Append(diags...)
	if refreshed.ID.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *ACMEAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan acmeAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.UpdateACMEAccount(ctx, plan.Name.ValueString(), plan.Contact.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox ACME Account", err.Error())
		return
	}
	if !r.wait(ctx, upid, &resp.Diagnostics) {
		return
	}
	state, diags := r.readState(ctx, plan.Name.ValueString(), plan)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ACMEAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state acmeAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.DeleteACMEAccount(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox ACME Account", err.Error())
		return
	}
	r.wait(ctx, upid, &resp.Diagnostics)
}

func (r *ACMEAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	notificationImport(ctx, req, resp)
}

func (r *ACMEAccountResource) readState(ctx context.Context, name string, prior acmeAccountModel) (acmeAccountModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	account, err := r.client.GetACMEAccount(ctx, name)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return acmeAccountModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox ACME Account", err.Error())
		return acmeAccountModel{}, diags
	}
	return acmeAccountModel{
		ID:        types.StringValue(account.Name),
		Name:      types.StringValue(account.Name),
		Contact:   acmeAccountContact(account, prior.Contact),
		Directory: stringOrNull(account.Directory),
		TOS:       stringOrNull(account.TOS),
		EABKID:    prior.EABKID,
		EABHMAC:   prior.EABHMAC,
		Location:  stringOrNull(account.Location),
	}, diags
}

// wait polls an account task; ACME account mutations are asynchronous.
func (r *ACMEAccountResource) wait(ctx context.Context, upid string, diags *diag.Diagnostics) bool {
	if upid == "" {
		return true
	}
	owner, err := taskOwnerNode(upid)
	if err != nil {
		diags.AddError("Unable to Start Proxmox ACME Account Task", err.Error())
		return false
	}
	if err := r.client.waitForNodeTask(ctx, owner, upid); err != nil {
		diags.AddError("Proxmox ACME Account Task Failed", err.Error())
		return false
	}
	return true
}

func acmeAccountContact(account ACMEAccount, prior types.String) types.String {
	if len(account.Details.Contact) > 0 {
		return types.StringValue(strings.Join(account.Details.Contact, ","))
	}
	return prior
}
