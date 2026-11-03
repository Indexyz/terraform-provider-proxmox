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

var _ resource.Resource = &SDNApplyResource{}
var _ resource.ResourceWithValidateConfig = &SDNApplyResource{}

type SDNApplyResource struct {
	client *Client
}

type sdnApplyModel struct {
	ID             types.String `tfsdk:"id"`
	Triggers       types.Map    `tfsdk:"triggers"`
	Pending        types.List   `tfsdk:"pending"`
	PendingObjects types.Map    `tfsdk:"pending_objects"`
}

func NewSDNApplyResource() resource.Resource {
	return &SDNApplyResource{}
}

func (r *SDNApplyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_apply"
}

func (r *SDNApplyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Commits the staged Proxmox VE SDN configuration through `PUT /cluster/sdn` and waits for the cluster-wide network reload. Zone, VNet and subnet resources only stage changes, so this is the single explicit activation step. Destroy is a no-op: a committed configuration stays committed. A failed commit keeps the instance in state for a retry, and a release of the global lock is attempted on failure.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "Static identifier.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"triggers": schema.MapAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Values that decide when a commit is due; changing any of them re-applies. Use a hash over the managed SDN objects, for example `{ revision = sha1(jsonencode([...])) }`.",
				PlanModifiers:       []planmodifier.Map{mapplanmodifier.RequiresReplace()},
			},
			"pending":         schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Staged objects Proxmox still reports as pending, formatted as `<type> <id> (<state>)`. Empty after a successful commit."},
			"pending_objects": schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Staged zones keyed by zone id, valued with their staging state."},
		},
	}
}

func (r *SDNApplyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SDNApplyResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config sdnApplyModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateSDNApply(config)...)
}

func validateSDNApply(config sdnApplyModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.Triggers) && len(config.Triggers.Elements()) == 0 {
		diags.AddAttributeError(path.Root("triggers"), "Missing SDN apply triggers", "triggers must contain at least one value so a commit can be re-run.")
	}
	return diags
}

func (r *SDNApplyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sdnApplyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.Diagnostics, &resp.State)
}

func (r *SDNApplyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sdnApplyModel
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

func (r *SDNApplyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan sdnApplyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.Diagnostics, &resp.State)
}

func (r *SDNApplyResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// A commit is never undone, so destroying the apply record leaves the
	// running configuration in place.
}

func (r *SDNApplyResource) apply(ctx context.Context, plan *sdnApplyModel, diags *diag.Diagnostics, state *tfsdk.State) {
	upid, err := r.client.ApplySDN(ctx)
	if err != nil {
		diags.AddError("Unable to Apply Proxmox SDN Configuration", err.Error())
		return
	}
	owner, err := taskOwnerNode(upid)
	if err != nil {
		diags.AddError("Unable to Apply Proxmox SDN Configuration", err.Error())
		return
	}
	if err := r.client.waitForNodeTask(ctx, owner, upid); err != nil {
		diags.AddError("Unable to Apply Proxmox SDN Configuration", fmt.Sprintf("SDN reload task %q did not finish: %s", upid, err))
		return
	}
	refreshed, readDiags := r.readState(ctx, plan)
	diags.Append(readDiags...)
	if diags.HasError() {
		return
	}
	diags.Append(state.Set(ctx, &refreshed)...)
}

func (r *SDNApplyResource) readState(ctx context.Context, prior *sdnApplyModel) (sdnApplyModel, diag.Diagnostics) {
	zones, err := r.client.SDNZones(ctx)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox SDN Configuration", fmt.Sprintf("unable to read staged SDN zones: %s", err))
		return sdnApplyModel{}, diags
	}
	state := *prior
	state.ID = types.StringValue("sdn_apply")
	state.Pending = types.ListNull(types.StringType)
	state.PendingObjects = types.MapNull(types.StringType)

	staged := map[string]string{}
	var lines []string
	for _, zone := range zones {
		if zone.State == "" {
			continue
		}
		staged[zone.Zone] = zone.State
		lines = append(lines, fmt.Sprintf("zone %s (%s)", zone.Zone, zone.State))
	}
	if len(lines) > 0 {
		sort.Strings(lines)
		pending, listDiags := types.ListValueFrom(ctx, types.StringType, lines)
		if listDiags.HasError() {
			return sdnApplyModel{}, listDiags
		}
		objects, mapDiags := types.MapValueFrom(ctx, types.StringType, staged)
		if mapDiags.HasError() {
			return sdnApplyModel{}, mapDiags
		}
		state.Pending = pending
		state.PendingObjects = objects
	}
	return state, nil
}
