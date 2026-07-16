// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &ReplicationJobResource{}
var _ resource.ResourceWithImportState = &ReplicationJobResource{}
var _ resource.ResourceWithValidateConfig = &ReplicationJobResource{}

const replicationJobManagedFieldsKey = "replication-job-managed-fields"

var replicationJobIDPattern = regexp.MustCompile(`^[1-9][0-9]{2,8}-[0-9]{1,9}$`)

type ReplicationJobResource struct {
	client *Client
}

type replicationPrivateStateWriter interface {
	SetKey(context.Context, string, []byte) diag.Diagnostics
}

type replicationJobModel struct {
	ID        types.String  `tfsdk:"id"`
	JobID     types.String  `tfsdk:"job_id"`
	Target    types.String  `tfsdk:"target"`
	Comment   types.String  `tfsdk:"comment"`
	Disable   types.Bool    `tfsdk:"disable"`
	Rate      types.Float64 `tfsdk:"rate"`
	Schedule  types.String  `tfsdk:"schedule"`
	Source    types.String  `tfsdk:"source"`
	GuestID   types.Int64   `tfsdk:"guest_id"`
	JobNumber types.Int64   `tfsdk:"job_number"`
	Type      types.String  `tfsdk:"type"`
}

func NewReplicationJobResource() resource.Resource {
	return &ReplicationJobResource{}
}

func (r *ReplicationJobResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_replication_job"
}

func (r *ReplicationJobResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox VE storage replication job configuration through `/cluster/replication`. This resource never runs a replication job. Destroy removes only the job configuration with `force=1`; it does not clean up replication snapshots or target data.",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{Computed: true, MarkdownDescription: "Replication job identifier."},
			"job_id":     schema.StringAttribute{Required: true, MarkdownDescription: "Stable job identifier in `<guest>-<job-number>` form. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"target":     schema.StringAttribute{Required: true, MarkdownDescription: "Target cluster node. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"comment":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Job description."},
			"disable":    schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Disable scheduled replication."},
			"rate":       schema.Float64Attribute{Optional: true, Computed: true, MarkdownDescription: "Rate limit in megabytes per second."},
			"schedule":   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Replication schedule as a Proxmox calendar event. The server default is `*/15`."},
			"source":     schema.StringAttribute{Computed: true, MarkdownDescription: "Observed source node used internally by Proxmox."},
			"guest_id":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Guest ID derived by Proxmox from `job_id`."},
			"job_number": schema.Int64Attribute{Computed: true, MarkdownDescription: "Job number derived by Proxmox from `job_id`."},
			"type":       schema.StringAttribute{Computed: true, MarkdownDescription: "Observed replication section type (`local`)."},
		},
	}
}

func (r *ReplicationJobResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ReplicationJobResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config replicationJobModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateReplicationJobConfig(config)...)
}

func validateReplicationJobConfig(config replicationJobModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if !config.JobID.IsNull() && !config.JobID.IsUnknown() && !replicationJobIDPattern.MatchString(config.JobID.ValueString()) {
		diags.AddAttributeError(path.Root("job_id"), "Invalid replication job identifier", "job_id must use <guest>-<job-number> form, for example 101-0")
	}
	if !config.Target.IsNull() && !config.Target.IsUnknown() && strings.TrimSpace(config.Target.ValueString()) == "" {
		diags.AddAttributeError(path.Root("target"), "Invalid replication target", "target must not be empty")
	}
	if !config.Rate.IsNull() && !config.Rate.IsUnknown() && config.Rate.ValueFloat64() < 1 {
		diags.AddAttributeError(path.Root("rate"), "Invalid replication rate", "rate must be at least 1 megabyte per second")
	}
	if !config.Schedule.IsNull() && !config.Schedule.IsUnknown() && (config.Schedule.ValueString() == "" || len(config.Schedule.ValueString()) > 128) {
		diags.AddAttributeError(path.Root("schedule"), "Invalid replication schedule", "schedule must contain between 1 and 128 characters")
	}
	if !config.Comment.IsNull() && !config.Comment.IsUnknown() && len(config.Comment.ValueString()) > 4096 {
		diags.AddAttributeError(path.Root("comment"), "Invalid replication comment", "comment must not exceed 4096 characters")
	}
	return diags
}

func replicationJobRequestFromModel(model replicationJobModel) ReplicationJobRequest {
	return ReplicationJobRequest{
		Comment:  stringPointer(model.Comment),
		Disable:  boolPointerValue(model.Disable),
		Rate:     float64PointerValue(model.Rate),
		Schedule: stringPointer(model.Schedule),
	}
}

func replicationJobManagedFields(model replicationJobModel) []string {
	var fields []string
	if !model.Comment.IsNull() && !model.Comment.IsUnknown() {
		fields = append(fields, "comment")
	}
	if !model.Disable.IsNull() && !model.Disable.IsUnknown() {
		fields = append(fields, "disable")
	}
	if !model.Rate.IsNull() && !model.Rate.IsUnknown() {
		fields = append(fields, "rate")
	}
	if !model.Schedule.IsNull() && !model.Schedule.IsUnknown() {
		fields = append(fields, "schedule")
	}
	slices.Sort(fields)
	return fields
}

func replicationJobDeleteKeys(model replicationJobModel, previouslyManaged []string) []string {
	current := replicationJobManagedFields(model)
	var deleted []string
	for _, key := range previouslyManaged {
		if !slices.Contains(current, key) {
			deleted = append(deleted, key)
		}
	}
	slices.Sort(deleted)
	return deleted
}

func (r *ReplicationJobResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan replicationJobModel
	var config replicationJobModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.CreateReplicationJob(ctx, plan.JobID.ValueString(), plan.Target.ValueString(), replicationJobRequestFromModel(plan)); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Replication Job", err.Error())
		return
	}
	state, diags := r.readState(ctx, plan.JobID.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *ReplicationJobResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state replicationJobModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.JobID.ValueString())
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

func (r *ReplicationJobResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config replicationJobModel
	var prior replicationJobModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	managedJSON, privateDiags := req.Private.GetKey(ctx, replicationJobManagedFieldsKey)
	resp.Diagnostics.Append(privateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	var previouslyManaged []string
	if len(managedJSON) > 0 {
		if err := json.Unmarshal(managedJSON, &previouslyManaged); err != nil {
			resp.Diagnostics.AddError("Unable to Read Replication Job State", fmt.Sprintf("unable to decode managed fields: %v", err))
			return
		}
	}
	current, err := r.client.GetReplicationJob(ctx, prior.JobID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Replication Job", err.Error())
		return
	}
	updateReq := replicationJobRequestFromModel(config)
	updateReq.Delete = replicationJobDeleteKeys(config, previouslyManaged)
	if current.Digest != "" {
		updateReq.Digest = &current.Digest
	}
	if err := r.client.UpdateReplicationJob(ctx, prior.JobID.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Replication Job", err.Error())
		return
	}
	state, diags := r.readState(ctx, prior.JobID.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *ReplicationJobResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state replicationJobModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteReplicationJob(ctx, state.JobID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Replication Job", err.Error())
	}
}

func (r *ReplicationJobResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !replicationJobIDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "expected a replication job identifier in <guest>-<job-number> form")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("job_id"), req.ID)...)
}

func (r *ReplicationJobResource) readState(ctx context.Context, id string) (replicationJobModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	job, err := r.client.GetReplicationJob(ctx, id)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return replicationJobModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Replication Job", err.Error())
		return replicationJobModel{}, diags
	}
	jobType := job.Type
	if jobType == "" {
		jobType = "local"
	}
	return replicationJobModel{
		ID:        types.StringValue(job.ID),
		JobID:     types.StringValue(job.ID),
		Target:    types.StringValue(job.Target),
		Comment:   stringOrNull(job.Comment),
		Disable:   boolOrNull(job.Disable.Ptr()),
		Rate:      float64OrNull(job.Rate),
		Schedule:  stringOrNull(job.Schedule),
		Source:    stringOrNull(job.Source),
		GuestID:   types.Int64Value(job.GuestID),
		JobNumber: types.Int64Value(job.JobNumber),
		Type:      types.StringValue(jobType),
	}, diags
}

func (r *ReplicationJobResource) storeManagedFields(ctx context.Context, model replicationJobModel, private replicationPrivateStateWriter, diags *diag.Diagnostics) {
	managed, err := json.Marshal(replicationJobManagedFields(model))
	if err != nil {
		diags.AddError("Unable to Store Replication Job State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, replicationJobManagedFieldsKey, managed)...)
}
