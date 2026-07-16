// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &RealmResource{}
var _ resource.ResourceWithImportState = &RealmResource{}
var _ resource.ResourceWithValidateConfig = &RealmResource{}

const realmManagedFieldsKey = "realm-managed-fields"

type RealmResource struct {
	client *Client
}

type realmPrivateData interface {
	GetKey(context.Context, string) ([]byte, diag.Diagnostics)
	SetKey(context.Context, string, []byte) diag.Diagnostics
}

func NewRealmResource() resource.Resource {
	return &RealmResource{}
}

func (r *RealmResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_realm"
}

func (r *RealmResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an external Proxmox VE 9 LDAP, Active Directory, or OpenID Connect authentication realm through `/access/domains`.",
		Attributes: map[string]schema.Attribute{
			"id":                    schema.StringAttribute{Computed: true, MarkdownDescription: "Realm identifier (same as `realm`).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"realm":                 schema.StringAttribute{Required: true, MarkdownDescription: "Authentication realm identifier. The built-in `pam` and `pve` realms are not supported. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"type":                  schema.StringAttribute{Required: true, MarkdownDescription: "External realm type: `ldap`, `ad`, or `openid`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"comment":               schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Realm description shown in the Proxmox login interface."},
			"default":               schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Use this realm as the cluster default. Proxmox clears this setting from every other realm when enabled."},
			"server1":               schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Primary LDAP or Active Directory server. Required for `ldap` and `ad`."},
			"server2":               schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Fallback LDAP or Active Directory server."},
			"port":                  schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "LDAP or Active Directory server port."},
			"mode":                  schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "LDAP protocol mode: `ldap`, `ldaps`, or `ldap+starttls`."},
			"verify":                schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Verify the LDAP server TLS certificate."},
			"ca_path":               schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Path on the Proxmox cluster to the LDAP CA certificate file or directory."},
			"base_dn":               schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "LDAP base distinguished name. Required for `ldap` and optional for `ad`."},
			"user_attr":             schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "LDAP attribute containing the username. Required for `ldap` and optional for `ad`."},
			"domain":                schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Active Directory domain name. Required for `ad`."},
			"bind_dn":               schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "LDAP bind distinguished name."},
			"bind_password":         schema.StringAttribute{Optional: true, Sensitive: true, WriteOnly: true, MarkdownDescription: "LDAP bind password. Requires Terraform 1.11 or later and must be paired with `bind_password_version`. The value is never stored in plan or state."},
			"bind_password_version": schema.Int64Attribute{Optional: true, MarkdownDescription: "Version counter for `bind_password`. Increment it to rotate the password; removing it deletes a previously managed remote password."},
			"issuer_url":            schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "OpenID Connect issuer URL. Required for `openid`."},
			"client_id":             schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "OpenID Connect client identifier. Required for `openid`."},
			"client_key":            schema.StringAttribute{Optional: true, Sensitive: true, WriteOnly: true, MarkdownDescription: "OpenID Connect client secret. Requires Terraform 1.11 or later and must be paired with `client_key_version`. The provider discards any value returned by Proxmox and never stores it in plan or state."},
			"client_key_version":    schema.Int64Attribute{Optional: true, MarkdownDescription: "Version counter for `client_key`. Increment it to rotate the secret; removing it deletes a previously managed remote client key."},
			"autocreate":            schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Automatically create Proxmox users after a successful OpenID login."},
			"username_claim":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "OpenID claim used as the username. Proxmox fixes this value at realm creation, so changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"scopes":                schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Space-separated OpenID scopes."},
			"prompt":                schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "OpenID authorization prompt value."},
			"query_userinfo":        schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Query the OpenID userinfo endpoint for claim values."},
			"acr_values":            schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "OpenID Authentication Context Class Reference values."},
			"audiences":             schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Additional OpenID audiences accepted alongside `client_id` (Proxmox VE 9)."},
		},
	}
}

func (r *RealmResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RealmResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config realmModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateRealmConfig(config)...)
}

func (r *RealmResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config realmModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.CreateRealm(ctx, realmRequestFromModels(config, realmModel{}, true)); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Realm", err.Error())
		return
	}
	state, diags := r.readState(ctx, config.Realm.ValueString(), &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *RealmResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state realmModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.Realm.ValueString(), &state)
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

func (r *RealmResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config realmModel
	var prior realmModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManaged, diags := readRealmManagedFields(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateReq := realmRequestFromModels(config, prior, false)
	updateReq.Delete = realmDeleteKeys(config, previouslyManaged)
	if !updateReq.IsEmpty() {
		current, err := r.client.GetRealm(ctx, prior.Realm.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to Read Current Proxmox Realm", err.Error())
			return
		}
		if current.Digest != "" {
			updateReq.Digest = &current.Digest
		}
		if err := r.client.UpdateRealm(ctx, prior.Realm.ValueString(), updateReq); err != nil {
			resp.Diagnostics.AddError("Unable to Update Proxmox Realm", err.Error())
			return
		}
	}
	state, stateDiags := r.readState(ctx, prior.Realm.ValueString(), &config)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *RealmResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state realmModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteRealm(ctx, state.Realm.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Realm", err.Error())
	}
}

func (r *RealmResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	realm := strings.TrimSpace(req.ID)
	if realm == "pve" || realm == "pam" {
		resp.Diagnostics.AddError("Unsupported Import Identifier", "The built-in pve and pam realms cannot be managed by this resource.")
		return
	}
	if len(realm) > 32 || !realmIDPattern.MatchString(realm) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected a valid external Proxmox realm identifier.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), realm)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm"), realm)...)
}

func (r *RealmResource) readState(ctx context.Context, realm string, prior *realmModel) (realmModel, diag.Diagnostics) {
	config, err := r.client.GetRealm(ctx, realm)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return realmModel{ID: types.StringNull()}, nil
		}
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox Realm", fmt.Sprintf("Unable to read realm %q: %s", realm, err))
		return realmModel{}, diags
	}
	return realmStateFromAPI(config, prior), nil
}

func (r *RealmResource) storeManagedFields(ctx context.Context, config realmModel, private realmPrivateData, diags *diag.Diagnostics) {
	managedFields, err := json.Marshal(realmManagedFields(config))
	if err != nil {
		diags.AddError("Unable to Store Realm State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, realmManagedFieldsKey, managedFields)...)
}

func readRealmManagedFields(ctx context.Context, private realmPrivateData) ([]string, diag.Diagnostics) {
	value, diags := private.GetKey(ctx, realmManagedFieldsKey)
	if diags.HasError() || len(value) == 0 {
		return nil, diags
	}
	var fields []string
	if err := json.Unmarshal(value, &fields); err != nil {
		diags.AddError("Unable to Read Realm State", fmt.Sprintf("unable to decode managed fields: %v", err))
	}
	return fields, diags
}
