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

var _ datasource.DataSource = &ReplicationJobsDataSource{}

type ReplicationJobsDataSource struct {
	client *Client
}

type replicationJobsDataSourceModel struct {
	ID    types.String                      `tfsdk:"id"`
	Guest types.Int64                       `tfsdk:"guest"`
	Jobs  []replicationJobsDataSourceRecord `tfsdk:"jobs"`
}

type replicationJobsDataSourceRecord struct {
	ID        types.String  `tfsdk:"id"`
	Guest     types.Int64   `tfsdk:"guest"`
	JobNumber types.Int64   `tfsdk:"job_number"`
	Type      types.String  `tfsdk:"type"`
	Source    types.String  `tfsdk:"source"`
	Target    types.String  `tfsdk:"target"`
	Schedule  types.String  `tfsdk:"schedule"`
	Rate      types.Float64 `tfsdk:"rate"`
	Disable   types.Bool    `tfsdk:"disable"`
	Comment   types.String  `tfsdk:"comment"`
}

func NewReplicationJobsDataSource() datasource.DataSource { return &ReplicationJobsDataSource{} }

func (d *ReplicationJobsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_replication_jobs"
}

func (d *ReplicationJobsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists cluster replication jobs from `/cluster/replication`, sorted by job id. An unset `guest` returns all jobs, a set one only that guest's jobs.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":    datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Unique identifier for this data source call."},
			"guest": datasourceschema.Int64Attribute{Optional: true, MarkdownDescription: "Restrict the result to one guest VMID."},
			"jobs": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching replication jobs, sorted by `id`.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"id":         datasourceschema.StringAttribute{Computed: true},
						"guest":      datasourceschema.Int64Attribute{Computed: true},
						"job_number": datasourceschema.Int64Attribute{Computed: true},
						"type":       datasourceschema.StringAttribute{Computed: true},
						"source":     datasourceschema.StringAttribute{Computed: true},
						"target":     datasourceschema.StringAttribute{Computed: true},
						"schedule":   datasourceschema.StringAttribute{Computed: true},
						"rate":       datasourceschema.Float64Attribute{Computed: true},
						"disable":    datasourceschema.BoolAttribute{Computed: true},
						"comment":    datasourceschema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *ReplicationJobsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ReplicationJobsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config replicationJobsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	jobs, err := d.client.ReplicationJobs(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Replication Jobs", fmt.Sprintf("Unable to call `/cluster/replication`: %s", err))
		return
	}
	records := make([]replicationJobsDataSourceRecord, 0, len(jobs))
	for _, job := range jobs {
		if isConfiguredValue(config.Guest) && job.GuestID != config.Guest.ValueInt64() {
			continue
		}
		record := replicationJobsDataSourceRecord{
			ID:        types.StringValue(job.ID),
			Guest:     types.Int64Value(job.GuestID),
			JobNumber: types.Int64Value(job.JobNumber),
			Type:      stringOrNull(job.Type),
			Source:    stringOrNull(job.Source),
			Target:    stringOrNull(job.Target),
			Schedule:  stringOrNull(job.Schedule),
			Rate:      float64OrNull(job.Rate),
			Disable:   nodesBoolOrNull(job.Disable),
			Comment:   stringOrNull(job.Comment),
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID.ValueString() < records[j].ID.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &replicationJobsDataSourceModel{ID: types.StringValue("replication_jobs"), Guest: config.Guest, Jobs: records})...)
}
