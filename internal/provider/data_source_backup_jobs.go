// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &BackupJobsDataSource{}

type BackupJobsDataSource struct {
	client *Client
}

type backupJobsDataSourceModel struct {
	ID   types.String                 `tfsdk:"id"`
	Node types.String                 `tfsdk:"node"`
	Jobs []backupJobsDataSourceRecord `tfsdk:"jobs"`
}

type backupJobsDataSourceRecord struct {
	ID          types.String `tfsdk:"id"`
	All         types.Bool   `tfsdk:"all"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	Node        types.String `tfsdk:"node"`
	Storage     types.String `tfsdk:"storage"`
	Schedule    types.String `tfsdk:"schedule"`
	Mode        types.String `tfsdk:"mode"`
	Compress    types.String `tfsdk:"compress"`
	Comment     types.String `tfsdk:"comment"`
	Pool        types.String `tfsdk:"pool"`
	VMIDs       types.String `tfsdk:"vmids"`
	ExcludeVMID types.String `tfsdk:"exclude_vmids"`
	NextRun     types.Int64  `tfsdk:"next_run"`
}

func NewBackupJobsDataSource() datasource.DataSource { return &BackupJobsDataSource{} }

func (d *BackupJobsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backup_jobs"
}

func (d *BackupJobsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists cluster backup jobs from `/cluster/backup`, sorted by job id. An unset `node` returns all jobs, a set one only the jobs of that node.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"node": datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Restrict the result to one node."},
			"jobs": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching backup jobs, sorted by `id`.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"id":            datasourceschema.StringAttribute{Computed: true},
						"all":           datasourceschema.BoolAttribute{Computed: true},
						"enabled":       datasourceschema.BoolAttribute{Computed: true},
						"node":          datasourceschema.StringAttribute{Computed: true},
						"storage":       datasourceschema.StringAttribute{Computed: true},
						"schedule":      datasourceschema.StringAttribute{Computed: true},
						"mode":          datasourceschema.StringAttribute{Computed: true},
						"compress":      datasourceschema.StringAttribute{Computed: true},
						"comment":       datasourceschema.StringAttribute{Computed: true},
						"pool":          datasourceschema.StringAttribute{Computed: true},
						"vmids":         datasourceschema.StringAttribute{Computed: true},
						"exclude_vmids": datasourceschema.StringAttribute{Computed: true},
						"next_run":      datasourceschema.Int64Attribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *BackupJobsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", err.Error())
		return
	}
	d.client = client
}

func (d *BackupJobsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config backupJobsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	jobs, err := d.client.BackupJobs(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Backup Jobs", fmt.Sprintf("Unable to call `/cluster/backup`: %s", err))
		return
	}
	filter := stringValue(config.Node)
	records := make([]backupJobsDataSourceRecord, 0, len(jobs))
	for _, job := range jobs {
		if filter != "" && job.Node != filter {
			continue
		}
		records = append(records, backupJobsDataSourceRecord{
			ID:          types.StringValue(job.ID),
			All:         nodesBoolOrNull(job.All),
			Enabled:     nodesBoolOrNull(job.Enabled),
			Node:        stringOrNull(job.Node),
			Storage:     stringOrNull(job.Storage),
			Schedule:    stringOrNull(job.Schedule),
			Mode:        stringOrNull(job.Mode),
			Compress:    stringOrNull(job.Compress),
			Comment:     stringOrNull(job.Comment),
			Pool:        stringOrNull(job.Pool),
			VMIDs:       stringOrNull(job.VMIDs),
			ExcludeVMID: stringOrNull(job.ExcludeVMIDs),
			NextRun:     int64OrNull(job.NextRun.Ptr()),
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID.ValueString() < records[j].ID.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &backupJobsDataSourceModel{ID: types.StringValue("backup_jobs"), Node: config.Node, Jobs: records})...)
}
