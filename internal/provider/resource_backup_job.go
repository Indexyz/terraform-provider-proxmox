// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

var _ resource.Resource = &BackupJobResource{}
var _ resource.ResourceWithImportState = &BackupJobResource{}
var _ resource.ResourceWithValidateConfig = &BackupJobResource{}

const backupJobManagedFieldsKey = "backup-job-managed-fields"

type canonicalBackupPrunePlanModifier struct{}

func (canonicalBackupPrunePlanModifier) Description(context.Context) string {
	return "Canonicalizes Proxmox prune-backups property strings."
}

func (canonicalBackupPrunePlanModifier) MarkdownDescription(context.Context) string {
	return "Canonicalizes Proxmox `prune-backups` property strings."
}

func (canonicalBackupPrunePlanModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	resp.PlanValue = types.StringValue(canonicalBackupPruneString(req.ConfigValue.ValueString()))
}

type BackupJobResource struct {
	client *Client
}

type backupJobModel struct {
	ID               types.String `tfsdk:"id"`
	JobID            types.String `tfsdk:"job_id"`
	All              types.Bool   `tfsdk:"all"`
	BWLimit          types.Int64  `tfsdk:"bandwidth_limit"`
	Comment          types.String `tfsdk:"comment"`
	Compress         types.String `tfsdk:"compression"`
	Enabled          types.Bool   `tfsdk:"enabled"`
	ExcludeVMIDs     types.String `tfsdk:"exclude_vm_ids"`
	Mode             types.String `tfsdk:"mode"`
	NextRun          types.Int64  `tfsdk:"next_run"`
	Node             types.String `tfsdk:"node"`
	NotesTemplate    types.String `tfsdk:"notes_template"`
	NotificationMode types.String `tfsdk:"notification_mode"`
	Pool             types.String `tfsdk:"pool"`
	Protected        types.Bool   `tfsdk:"protected"`
	PruneBackups     types.String `tfsdk:"prune_backups"`
	Remove           types.Bool   `tfsdk:"remove"`
	RepeatMissed     types.Bool   `tfsdk:"repeat_missed"`
	Schedule         types.String `tfsdk:"schedule"`
	Storage          types.String `tfsdk:"storage"`
	VMIDs            types.String `tfsdk:"vm_ids"`
}

func NewBackupJobResource() resource.Resource {
	return &BackupJobResource{}
}

func (r *BackupJobResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backup_job"
}

func (r *BackupJobResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox VE scheduled vzdump backup job through `/cluster/backup`. CRUD changes the schedule only and never starts a backup.",
		Attributes: map[string]schema.Attribute{
			"id":                schema.StringAttribute{Computed: true, MarkdownDescription: "Backup job identifier.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"job_id":            schema.StringAttribute{Required: true, MarkdownDescription: "Stable Proxmox backup job identifier. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"all":               schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Back up all guests. Exactly one of `all=true`, `pool`, or `vm_ids` must select guests."},
			"bandwidth_limit":   schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "I/O bandwidth limit in KiB/s."},
			"comment":           schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Backup job description."},
			"compression":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Compression mode: `0`, `1`, `gzip`, `lzo`, or `zstd`."},
			"enabled":           schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable the scheduled job."},
			"exclude_vm_ids":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Comma-separated guest IDs excluded when `all` is true."},
			"mode":              schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Backup mode: `snapshot`, `suspend`, or `stop`."},
			"next_run":          schema.Int64Attribute{Computed: true, MarkdownDescription: "Unix timestamp of the next scheduled run."},
			"node":              schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Only run the job on this node."},
			"notes_template":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Template used to generate backup notes."},
			"notification_mode": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Notification mode: `auto`, `legacy-sendmail`, or `notification-system`."},
			"pool":              schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Back up all guests in this pool."},
			"protected":         schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Mark generated backups as protected."},
			"prune_backups":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Proxmox retention property string, for example `keep-daily=7,keep-last=3`.", PlanModifiers: []planmodifier.String{canonicalBackupPrunePlanModifier{}}},
			"remove":            schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Prune older backups according to `prune_backups`."},
			"repeat_missed":     schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Run the job as soon as possible after a missed schedule."},
			"schedule":          schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Backup schedule in Proxmox calendar-event syntax."},
			"storage":           schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Target Proxmox storage identifier."},
			"vm_ids":            schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Comma-separated guest IDs to back up."},
		},
	}
}

func (r *BackupJobResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *BackupJobResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config backupJobModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateBackupJobConfig(config)...)
}

func validateBackupJobConfig(config backupJobModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if !config.JobID.IsUnknown() && strings.TrimSpace(config.JobID.ValueString()) == "" {
		diags.AddAttributeError(path.Root("job_id"), "Invalid backup job identifier", "job_id must not be empty")
	}
	if config.All.IsUnknown() || config.Pool.IsUnknown() || config.VMIDs.IsUnknown() {
		return diags
	}
	all := !config.All.IsNull() && config.All.ValueBool()
	pool := !config.Pool.IsNull() && strings.TrimSpace(config.Pool.ValueString()) != ""
	vmIDs := !config.VMIDs.IsNull() && strings.TrimSpace(config.VMIDs.ValueString()) != ""
	selected := 0
	for _, value := range []bool{all, pool, vmIDs} {
		if value {
			selected++
		}
	}
	if selected != 1 {
		diags.AddError("Invalid backup guest selection", "exactly one of all=true, pool, or vm_ids must select guests")
	}
	if !config.ExcludeVMIDs.IsNull() && !config.ExcludeVMIDs.IsUnknown() && strings.TrimSpace(config.ExcludeVMIDs.ValueString()) != "" && !all {
		diags.AddAttributeError(path.Root("exclude_vm_ids"), "Invalid excluded guest selection", "exclude_vm_ids can only be set when all is true")
	}
	validateBackupEnum(&diags, path.Root("mode"), config.Mode, []string{"snapshot", "suspend", "stop"})
	validateBackupEnum(&diags, path.Root("compression"), config.Compress, []string{"0", "1", "gzip", "lzo", "zstd"})
	validateBackupEnum(&diags, path.Root("notification_mode"), config.NotificationMode, []string{"auto", "legacy-sendmail", "notification-system"})
	if !config.BWLimit.IsNull() && !config.BWLimit.IsUnknown() && config.BWLimit.ValueInt64() < 0 {
		diags.AddAttributeError(path.Root("bandwidth_limit"), "Invalid bandwidth limit", "bandwidth_limit must be at least zero")
	}
	return diags
}

func validateBackupEnum(diags *diag.Diagnostics, attribute path.Path, value types.String, allowed []string) {
	if value.IsNull() || value.IsUnknown() || slices.Contains(allowed, value.ValueString()) {
		return
	}
	diags.AddAttributeError(attribute, "Invalid backup job value", fmt.Sprintf("value must be one of %s", strings.Join(allowed, ", ")))
}

func backupJobRequestFromModel(model backupJobModel) BackupJobRequest {
	return BackupJobRequest{
		All:              boolPointerValue(model.All),
		BWLimit:          int64PointerValue(model.BWLimit),
		Comment:          stringPointer(model.Comment),
		Compress:         stringPointer(model.Compress),
		Enabled:          boolPointerValue(model.Enabled),
		ExcludeVMIDs:     stringPointer(model.ExcludeVMIDs),
		Mode:             stringPointer(model.Mode),
		Node:             stringPointer(model.Node),
		NotesTemplate:    stringPointer(model.NotesTemplate),
		NotificationMode: stringPointer(model.NotificationMode),
		Pool:             stringPointer(model.Pool),
		Protected:        boolPointerValue(model.Protected),
		PruneBackups:     stringPointer(model.PruneBackups),
		Remove:           boolPointerValue(model.Remove),
		RepeatMissed:     boolPointerValue(model.RepeatMissed),
		Schedule:         stringPointer(model.Schedule),
		Storage:          stringPointer(model.Storage),
		VMIDs:            stringPointer(model.VMIDs),
	}
}

func backupJobManagedFields(config backupJobModel) []string {
	var fields []string
	addBool := func(key string, value types.Bool) {
		if !value.IsNull() && !value.IsUnknown() {
			fields = append(fields, key)
		}
	}
	addInt64 := func(key string, value types.Int64) {
		if !value.IsNull() && !value.IsUnknown() {
			fields = append(fields, key)
		}
	}
	addString := func(key string, value types.String) {
		if !value.IsNull() && !value.IsUnknown() {
			fields = append(fields, key)
		}
	}
	addBool("all", config.All)
	addInt64("bwlimit", config.BWLimit)
	addString("comment", config.Comment)
	addString("compress", config.Compress)
	addBool("enabled", config.Enabled)
	addString("exclude", config.ExcludeVMIDs)
	addString("mode", config.Mode)
	addString("node", config.Node)
	addString("notes-template", config.NotesTemplate)
	addString("notification-mode", config.NotificationMode)
	addString("pool", config.Pool)
	addBool("protected", config.Protected)
	addString("prune-backups", config.PruneBackups)
	addBool("remove", config.Remove)
	addBool("repeat-missed", config.RepeatMissed)
	addString("schedule", config.Schedule)
	addString("storage", config.Storage)
	addString("vmid", config.VMIDs)
	slices.Sort(fields)
	return fields
}

func backupJobDeleteKeys(config, prior backupJobModel, previouslyManaged []string) []string {
	current := backupJobManagedFields(config)
	currentSet := make(map[string]struct{}, len(current))
	for _, key := range current {
		currentSet[key] = struct{}{}
	}
	deleted := make(map[string]struct{})
	for _, key := range previouslyManaged {
		if _, ok := currentSet[key]; !ok {
			deleted[key] = struct{}{}
		}
	}
	addIfUnmanaged := func(key string, present bool) {
		if _, managed := currentSet[key]; !managed && present {
			deleted[key] = struct{}{}
		}
	}
	allSelected := !config.All.IsNull() && !config.All.IsUnknown() && config.All.ValueBool()
	poolSelected := !config.Pool.IsNull() && !config.Pool.IsUnknown() && config.Pool.ValueString() != ""
	vmIDsSelected := !config.VMIDs.IsNull() && !config.VMIDs.IsUnknown() && config.VMIDs.ValueString() != ""
	switch {
	case allSelected:
		addIfUnmanaged("pool", !prior.Pool.IsNull())
		addIfUnmanaged("vmid", !prior.VMIDs.IsNull())
	case poolSelected:
		addIfUnmanaged("all", !prior.All.IsNull())
		addIfUnmanaged("vmid", !prior.VMIDs.IsNull())
	case vmIDsSelected:
		addIfUnmanaged("all", !prior.All.IsNull())
		addIfUnmanaged("pool", !prior.Pool.IsNull())
	}
	keys := make([]string, 0, len(deleted))
	for key := range deleted {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func (r *BackupJobResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan backupJobModel
	var config backupJobModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.CreateBackupJob(ctx, plan.JobID.ValueString(), backupJobRequestFromModel(plan)); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Backup Job", err.Error())
		return
	}
	state, diags := r.readState(ctx, plan.JobID.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	managedFields, err := json.Marshal(backupJobManagedFields(config))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Store Backup Job State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, backupJobManagedFieldsKey, managedFields)...)
}

func (r *BackupJobResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state backupJobModel
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

func (r *BackupJobResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config backupJobModel
	var prior backupJobModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManagedJSON, privateDiags := req.Private.GetKey(ctx, backupJobManagedFieldsKey)
	resp.Diagnostics.Append(privateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	var previouslyManaged []string
	if len(previouslyManagedJSON) > 0 {
		if err := json.Unmarshal(previouslyManagedJSON, &previouslyManaged); err != nil {
			resp.Diagnostics.AddError("Unable to Read Backup Job State", fmt.Sprintf("unable to decode managed fields: %v", err))
			return
		}
	}
	updateReq := backupJobRequestFromModel(config)
	updateReq.Delete = backupJobDeleteKeys(config, prior, previouslyManaged)
	if err := r.client.UpdateBackupJob(ctx, prior.JobID.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Backup Job", err.Error())
		return
	}
	refreshed, diags := r.readState(ctx, prior.JobID.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
	managedFields, err := json.Marshal(backupJobManagedFields(config))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Store Backup Job State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, backupJobManagedFieldsKey, managedFields)...)
}

func (r *BackupJobResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state backupJobModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteBackupJob(ctx, state.JobID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Backup Job", err.Error())
	}
}

func (r *BackupJobResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if strings.TrimSpace(req.ID) == "" {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "expected a non-empty backup job identifier")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("job_id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *BackupJobResource) readState(ctx context.Context, id string) (backupJobModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	job, err := r.client.GetBackupJob(ctx, id)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return backupJobModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Backup Job", err.Error())
		return backupJobModel{}, diags
	}
	pruneBackups, err := canonicalBackupPruneOptions(job.PruneBackups)
	if err != nil {
		diags.AddError("Unable to Decode Proxmox Backup Job", err.Error())
		return backupJobModel{}, diags
	}
	return backupJobModel{
		ID:               types.StringValue(id),
		JobID:            types.StringValue(id),
		All:              boolOrNull(job.All.Ptr()),
		BWLimit:          int64OrNull(job.BWLimit.Ptr()),
		Comment:          stringOrNull(job.Comment),
		Compress:         stringOrNull(job.Compress),
		Enabled:          boolOrNull(job.Enabled.Ptr()),
		ExcludeVMIDs:     stringOrNull(job.ExcludeVMIDs),
		Mode:             stringOrNull(job.Mode),
		NextRun:          int64OrNull(job.NextRun.Ptr()),
		Node:             stringOrNull(job.Node),
		NotesTemplate:    stringOrNull(job.NotesTemplate),
		NotificationMode: stringOrNull(job.NotificationMode),
		Pool:             stringOrNull(job.Pool),
		Protected:        boolOrNull(job.Protected.Ptr()),
		PruneBackups:     stringOrNull(pruneBackups),
		Remove:           boolOrNull(job.Remove.Ptr()),
		RepeatMissed:     boolOrNull(job.RepeatMissed.Ptr()),
		Schedule:         stringOrNull(job.Schedule),
		Storage:          stringOrNull(job.Storage),
		VMIDs:            stringOrNull(job.VMIDs),
	}, diags
}
