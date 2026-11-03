// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &BackupNotBackedUpDataSource{}
var _ datasource.DataSource = &BackupJobIncludedVolumesDataSource{}
var _ datasource.DataSourceWithValidateConfig = &BackupJobIncludedVolumesDataSource{}
var _ datasource.DataSource = &ClusterRealmSyncJobsDataSource{}

type BackupNotBackedUpDataSource struct{ client *Client }
type BackupJobIncludedVolumesDataSource struct{ client *Client }
type ClusterRealmSyncJobsDataSource struct{ client *Client }

type backupNotBackedUpModel struct {
	Guests []backupNotBackedUpRecord `tfsdk:"guests"`
}

type backupNotBackedUpRecord struct {
	Name types.String `tfsdk:"name"`
	Type types.String `tfsdk:"type"`
	VMID types.Int64  `tfsdk:"vmid"`
}

type backupJobIncludedVolumesModel struct {
	ID      types.String `tfsdk:"id"`
	Volumes types.List   `tfsdk:"volumes"`
}

type backupVolumeRecord struct {
	Type     types.String `tfsdk:"type"`
	Leaf     types.Bool   `tfsdk:"leaf"`
	Name     types.String `tfsdk:"name"`
	ID       types.Int64  `tfsdk:"id"`
	Children types.List   `tfsdk:"children"`
}

type clusterRealmSyncJobsModel struct {
	Jobs []clusterRealmSyncJobsRecord `tfsdk:"jobs"`
}

type clusterRealmSyncJobsRecord struct {
	ID             types.String `tfsdk:"id"`
	Realm          types.String `tfsdk:"realm"`
	Scope          types.String `tfsdk:"scope"`
	Schedule       types.String `tfsdk:"schedule"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	Comment        types.String `tfsdk:"comment"`
	RemoveVanished types.String `tfsdk:"remove_vanished"`
	LastRun        types.Int64  `tfsdk:"last_run"`
	NextRun        types.Int64  `tfsdk:"next_run"`
}

func NewBackupNotBackedUpDataSource() datasource.DataSource {
	return &BackupNotBackedUpDataSource{}
}

func NewBackupJobIncludedVolumesDataSource() datasource.DataSource {
	return &BackupJobIncludedVolumesDataSource{}
}

func NewClusterRealmSyncJobsDataSource() datasource.DataSource {
	return &ClusterRealmSyncJobsDataSource{}
}

func (d *BackupNotBackedUpDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backup_not_backed_up"
}

func (d *BackupJobIncludedVolumesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backup_job_included_volumes"
}

func (d *ClusterRealmSyncJobsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_realm_sync_jobs"
}

func (d *BackupNotBackedUpDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists the guests that have no backup yet according to `/cluster/backup-info/not-backed-up`, sorted by VMID.",
		Attributes: map[string]datasourceschema.Attribute{
			"guests": datasourceschema.ListNestedAttribute{
				Computed: true,
				NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
					"name": datasourceschema.StringAttribute{Computed: true},
					"type": datasourceschema.StringAttribute{Computed: true},
					"vmid": datasourceschema.Int64Attribute{Computed: true},
				}},
			},
		},
	}
}

func (d *BackupJobIncludedVolumesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists the volumes a backup job includes, from `/cluster/backup/{id}/included_volumes`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Backup job id."},
			"volumes": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"type":     datasourceschema.StringAttribute{Computed: true},
				"leaf":     datasourceschema.BoolAttribute{Computed: true},
				"name":     datasourceschema.StringAttribute{Computed: true},
				"id":       datasourceschema.Int64Attribute{Computed: true},
				"children": datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Nested entries as `type:id`."},
			}}},
		},
	}
}

func (d *ClusterRealmSyncJobsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists the scheduled realm sync jobs of the cluster from `/cluster/jobs/realm-sync`, sorted by realm.",
		Attributes: map[string]datasourceschema.Attribute{
			"jobs": datasourceschema.ListNestedAttribute{
				Computed: true,
				NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
					"id":              datasourceschema.StringAttribute{Computed: true},
					"realm":           datasourceschema.StringAttribute{Computed: true},
					"scope":           datasourceschema.StringAttribute{Computed: true},
					"schedule":        datasourceschema.StringAttribute{Computed: true},
					"enabled":         datasourceschema.BoolAttribute{Computed: true},
					"comment":         datasourceschema.StringAttribute{Computed: true},
					"remove_vanished": datasourceschema.StringAttribute{Computed: true},
					"last_run":        datasourceschema.Int64Attribute{Computed: true},
					"next_run":        datasourceschema.Int64Attribute{Computed: true},
				}},
			},
		},
	}
}

func (d *BackupNotBackedUpDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *BackupJobIncludedVolumesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *ClusterRealmSyncJobsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *BackupJobIncludedVolumesDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config backupJobIncludedVolumesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !isConfiguredValue(config.ID) && !config.ID.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("id"), "Missing backup job id", "id must be set.")
	}
}

func (d *BackupNotBackedUpDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config backupNotBackedUpModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	guests, err := d.client.NotBackedUpGuests(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Backup Coverage", err.Error())
		return
	}
	sort.Slice(guests, func(i, j int) bool { return guests[i].VMID < guests[j].VMID })
	records := make([]backupNotBackedUpRecord, 0, len(guests))
	for _, guest := range guests {
		records = append(records, backupNotBackedUpRecord{Name: stringOrNull(guest.Name), Type: stringOrNull(guest.Type), VMID: types.Int64Value(guest.VMID)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &backupNotBackedUpModel{Guests: records})...)
}

func (d *BackupJobIncludedVolumesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config backupJobIncludedVolumesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	volumes, err := d.client.BackupJobIncludedVolumes(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Backup Job Volumes", err.Error())
		return
	}
	records := make([]backupVolumeRecord, 0, len(volumes))
	for _, volume := range volumes {
		records = append(records, backupVolumeRecord{
			Type: stringOrNull(volume.Type), Leaf: nodesBoolOrNull(volume.Leaf), Name: stringOrNull(volume.Name),
			ID: types.Int64Value(volume.ID), Children: flattenVolumeChildren(ctx, volume.Children),
		})
	}
	elementType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"type": types.StringType, "leaf": types.BoolType, "name": types.StringType, "id": types.Int64Type,
		"children": types.ListType{ElemType: types.StringType},
	}}
	value, valueDiags := types.ListValueFrom(ctx, elementType, records)
	resp.Diagnostics.Append(valueDiags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &backupJobIncludedVolumesModel{ID: config.ID, Volumes: value})...)
}

func flattenVolumeChildren(ctx context.Context, nodes []BackupVolumeNode) types.List {
	children := make([]string, 0, len(nodes))
	for _, node := range nodes {
		children = append(children, fmt.Sprintf("%s:%d", node.Type, node.ID))
	}
	value, _ := types.ListValueFrom(ctx, types.StringType, children)
	return value
}

func (d *ClusterRealmSyncJobsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config clusterRealmSyncJobsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	jobs, err := d.client.RealmSyncJobs(ctx)
	if err != nil {
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		resp.Diagnostics.AddError("Unable to Read Proxmox Realm Sync Jobs", err.Error())
		return
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Realm < jobs[j].Realm })
	records := make([]clusterRealmSyncJobsRecord, 0, len(jobs))
	for _, job := range jobs {
		records = append(records, clusterRealmSyncJobsRecord{
			ID: types.StringValue(job.ID), Realm: types.StringValue(job.Realm), Scope: stringOrNull(job.Scope),
			Schedule: stringOrNull(job.Schedule), Enabled: nodesBoolOrNull(job.Enabled), Comment: stringOrNull(job.Comment),
			RemoveVanished: stringOrNull(job.RemoveVanished), LastRun: int64OrNull(job.LastRun.Ptr()), NextRun: int64OrNull(job.NextRun.Ptr()),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &clusterRealmSyncJobsModel{Jobs: records})...)
}
