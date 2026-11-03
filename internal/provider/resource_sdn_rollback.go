// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"sort"

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

var _ resource.Resource = &SDNRollbackResource{}
var _ resource.ResourceWithValidateConfig = &SDNRollbackResource{}

type SDNRollbackResource struct {
	client *Client
}

type sdnRollbackModel struct {
	ID             types.String `tfsdk:"id"`
	Triggers       types.Map    `tfsdk:"triggers"`
	Remaining      types.List   `tfsdk:"remaining_pending"`
	RemainingZones types.Map    `tfsdk:"remaining_pending_zones"`
}

func NewSDNRollbackResource() resource.Resource { return &SDNRollbackResource{} }

func (r *SDNRollbackResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_rollback"
}

func (r *SDNRollbackResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Rolls the staged Proxmox VE SDN configuration back to the running snapshot through `POST /cluster/sdn/rollback`. This discards every uncommitted change on the cluster, so it is a separate opt-in step next to `proxmox_sdn_apply` (which commits). Destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "Static identifier.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"triggers": schema.MapAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Values that decide when a rollback is due; changing any of them re-runs it.",
				PlanModifiers:       []planmodifier.Map{mapplanmodifier.RequiresReplace()},
			},
			"remaining_pending":       schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Pending objects Proxmox still reports after the rollback, formatted as `<type> <id> (<state>)`."},
			"remaining_pending_zones": schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Pending zones keyed by zone id after the rollback."},
		},
	}
}

func (r *SDNRollbackResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SDNRollbackResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config sdnRollbackModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateSDNRollback(config)...)
}

func validateSDNRollback(config sdnRollbackModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.Triggers) && len(config.Triggers.Elements()) == 0 {
		diags.AddAttributeError(path.Root("triggers"), "Missing SDN rollback triggers", "triggers must contain at least one value so a rollback can be re-run.")
	}
	return diags
}

func (r *SDNRollbackResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sdnRollbackModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.rollback(ctx, &plan, &resp.Diagnostics, &resp.State)
}

func (r *SDNRollbackResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sdnRollbackModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *SDNRollbackResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan sdnRollbackModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.rollback(ctx, &plan, &resp.Diagnostics, &resp.State)
}

func (r *SDNRollbackResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *SDNRollbackResource) rollback(ctx context.Context, plan *sdnRollbackModel, diags *diag.Diagnostics, state *tfsdk.State) {
	token, err := r.client.LockSDN(ctx)
	if err != nil {
		diags.AddError("Unable to Lock Proxmox SDN Configuration", err.Error())
		return
	}
	rollbackErr := r.client.RollbackSDN(ctx, token)
	if unlockErr := r.client.UnlockSDN(ctx, token); unlockErr != nil {
		diags.AddWarning("Unable to Release Proxmox SDN Lock", fmt.Sprintf("the SDN configuration lock may be stale: %s", unlockErr))
	}
	if rollbackErr != nil {
		diags.AddError("Unable to Roll Back Proxmox SDN Configuration", rollbackErr.Error())
		return
	}
	refreshed, readDiags := r.readState(ctx, plan)
	diags.Append(readDiags...)
	if diags.HasError() {
		return
	}
	diags.Append(state.Set(ctx, &refreshed)...)
}

func (r *SDNRollbackResource) readState(ctx context.Context, prior *sdnRollbackModel) (sdnRollbackModel, diag.Diagnostics) {
	zones, err := r.client.SDNZones(ctx)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox SDN Configuration", fmt.Sprintf("unable to read staged SDN zones: %s", err))
		return sdnRollbackModel{}, diags
	}
	state := *prior
	state.ID = types.StringValue("sdn_rollback")
	state.Remaining = types.ListNull(types.StringType)
	state.RemainingZones = types.MapNull(types.StringType)

	pending := map[string]string{}
	var lines []string
	for _, zone := range zones {
		if zone.State == "" {
			continue
		}
		pending[zone.Zone] = zone.State
		lines = append(lines, fmt.Sprintf("zone %s (%s)", zone.Zone, zone.State))
	}
	if len(lines) > 0 {
		sort.Strings(lines)
		list, listDiags := types.ListValueFrom(ctx, types.StringType, lines)
		if listDiags.HasError() {
			return sdnRollbackModel{}, listDiags
		}
		objects, mapDiags := types.MapValueFrom(ctx, types.StringType, pending)
		if mapDiags.HasError() {
			return sdnRollbackModel{}, mapDiags
		}
		state.Remaining = list
		state.RemainingZones = objects
	}
	return state, nil
}
