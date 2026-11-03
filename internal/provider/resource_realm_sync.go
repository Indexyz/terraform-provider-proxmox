// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &RealmSyncResource{}
var _ resource.ResourceWithImportState = &RealmSyncResource{}
var _ resource.ResourceWithValidateConfig = &RealmSyncResource{}

type RealmSyncResource struct {
	client *Client
}

type realmSyncModel struct {
	ID             types.String `tfsdk:"id"`
	Realm          types.String `tfsdk:"realm"`
	Scope          types.String `tfsdk:"scope"`
	RemoveVanished types.String `tfsdk:"remove_vanished"`
	EnableNew      types.Bool   `tfsdk:"enable_new"`
	DryRun         types.Bool   `tfsdk:"dry_run"`
	Triggers       types.Map    `tfsdk:"triggers"`
	UPID           types.String `tfsdk:"upid"`
}

var (
	realmSyncScopes       = []string{"users", "groups", "both"}
	realmSyncRemoveTokens = []string{"acl", "properties", "entry"}
)

func NewRealmSyncResource() resource.Resource { return &RealmSyncResource{} }

func (r *RealmSyncResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_realm_sync"
}

func (r *RealmSyncResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Runs an LDAP/AD user and group sync through `POST /access/domains/{realm}/sync` and waits for the worker task. A sync is an imperative command, not stored configuration: every attribute change replaces the instance and re-runs it, and destroying it does nothing. `remove-vanished` makes a sync destructive, deleting users, groups, properties and ACLs that the directory no longer returns.",
		Attributes: map[string]schema.Attribute{
			"id":    schema.StringAttribute{Computed: true, MarkdownDescription: "Sync identifier (the realm name).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"realm": schema.StringAttribute{Required: true, MarkdownDescription: "LDAP or AD realm to sync. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"scope": schema.StringAttribute{Required: true, MarkdownDescription: "What to sync: `users`, `groups`, or `both`. Requires replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"remove_vanished": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("none"),
				MarkdownDescription: "Semicolon-separated list of `acl`, `properties`, `entry` to remove when users or groups vanish from the directory, or `none`. The Proxmox default is `none`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enable_new": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Enable newly synced users immediately. The Proxmox default is `true`.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"dry_run": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Only report what a sync would do without writing anything.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"triggers": schema.MapAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Values that decide when another sync is due; changing any of them re-runs the sync.",
				PlanModifiers:       []planmodifier.Map{mapplanmodifier.RequiresReplace()},
			},
			"upid": schema.StringAttribute{Computed: true, MarkdownDescription: "Worker task UPID of the last sync run."},
		},
	}
}

func (r *RealmSyncResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RealmSyncResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config realmSyncModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateRealmSyncConfig(config)...)
}

func validateRealmSyncConfig(config realmSyncModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.Realm) {
		realm := config.Realm.ValueString()
		switch {
		case realm == "pam" || realm == "pve":
			diags.AddAttributeError(path.Root("realm"), "Unsupported realm type", "Only LDAP and AD realms can be synced.")
		case len(realm) > 32 || !realmIDPattern.MatchString(realm):
			diags.AddAttributeError(path.Root("realm"), "Invalid realm identifier", "realm must be a valid realm identifier.")
		}
	}
	if isConfiguredValue(config.Scope) && !slices.Contains(realmSyncScopes, config.Scope.ValueString()) {
		diags.AddAttributeError(path.Root("scope"), "Invalid sync scope", "scope must be users, groups, or both.")
	}
	if isConfiguredValue(config.RemoveVanished) && config.RemoveVanished.ValueString() != "none" {
		for _, token := range strings.Split(config.RemoveVanished.ValueString(), ";") {
			if !slices.Contains(realmSyncRemoveTokens, token) {
				diags.AddAttributeError(path.Root("remove_vanished"), "Invalid remove-vanished list", "use `none` or a semicolon list of acl, properties, entry.")
				break
			}
		}
	}
	if isConfiguredValue(config.Triggers) && len(config.Triggers.Elements()) == 0 {
		diags.AddAttributeError(path.Root("triggers"), "Missing realm sync triggers", "triggers must contain at least one value so a sync can be re-run.")
	}
	return diags
}

// sync verifies the realm is LDAP/AD, starts the sync and waits for its worker.
func (r *RealmSyncResource) sync(ctx context.Context, plan *realmSyncModel, diags *diag.Diagnostics, state *tfsdk.State) {
	realm := plan.Realm.ValueString()
	current, err := r.client.GetRealm(ctx, realm)
	if err != nil {
		diags.AddError("Unable to Read Proxmox Realm", err.Error())
		return
	}
	if current.Type != "ldap" && current.Type != "ad" {
		diags.AddAttributeError(path.Root("realm"), "Unsupported realm type", fmt.Sprintf("realm %q is of type %q; only LDAP and AD realms can be synced.", realm, current.Type))
		return
	}
	upid, err := r.client.SyncRealm(ctx, RealmSyncRequest{
		Realm:          realm,
		Scope:          plan.Scope.ValueString(),
		RemoveVanished: plan.RemoveVanished.ValueString(),
		EnableNew:      boolPointerValue(plan.EnableNew),
		DryRun:         boolPointerValue(plan.DryRun),
	})
	if err != nil {
		diags.AddError("Unable to Start Proxmox Realm Sync", err.Error())
		return
	}
	owner, err := taskOwnerNode(upid)
	if err != nil {
		diags.AddError("Unable to Start Proxmox Realm Sync", err.Error())
		return
	}
	if err := r.client.waitForNodeTask(ctx, owner, upid); err != nil {
		diags.AddError("Proxmox Realm Sync Failed", fmt.Sprintf("sync task %q did not finish: %s", upid, err))
		return
	}
	next := *plan
	next.ID = types.StringValue(realm)
	next.UPID = types.StringValue(upid)
	diags.Append(state.Set(ctx, &next)...)
}

func (r *RealmSyncResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan realmSyncModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.sync(ctx, &plan, &resp.Diagnostics, &resp.State)
}

func (r *RealmSyncResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state realmSyncModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.GetRealm(ctx, state.Realm.ValueString()); err != nil {
		if errors.Is(err, errNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to Read Proxmox Realm", err.Error())
	}
}

func (r *RealmSyncResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan realmSyncModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.sync(ctx, &plan, &resp.Diagnostics, &resp.State)
}

func (r *RealmSyncResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// A sync is never undone.
}

func (r *RealmSyncResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if len(req.ID) > 32 || !realmIDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected a realm identifier.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm"), req.ID)...)
}
