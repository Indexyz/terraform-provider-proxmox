// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource = &HAStatusSummaryDataSource{}
	_ datasource.DataSource = &CephHealthMutesDataSource{}
	_ datasource.DataSource = &CephMetadataDataSource{}
)

type HAStatusSummaryDataSource struct{ client *Client }
type CephHealthMutesDataSource struct{ client *Client }
type CephMetadataDataSource struct{ client *Client }

type haStatusSummaryDSModel struct {
	ID      types.String `tfsdk:"id"`
	Summary types.String `tfsdk:"summary"`
}
type cephHealthMutesDSModel struct {
	ID    types.String           `tfsdk:"id"`
	Mutes []cephHealthMuteRecord `tfsdk:"mutes"`
}
type cephHealthMuteRecord struct {
	Code    types.String `tfsdk:"code"`
	Summary types.String `tfsdk:"summary"`
	TTL     types.String `tfsdk:"ttl"`
	Sticky  types.Bool   `tfsdk:"sticky"`
}
type cephMetadataDSModel struct {
	ID       types.String `tfsdk:"id"`
	Scope    types.String `tfsdk:"scope"`
	Metadata types.String `tfsdk:"metadata"`
}

func NewHAStatusSummaryDataSource() datasource.DataSource { return &HAStatusSummaryDataSource{} }
func NewCephHealthMutesDataSource() datasource.DataSource { return &CephHealthMutesDataSource{} }
func NewCephMetadataDataSource() datasource.DataSource    { return &CephMetadataDataSource{} }

func (d *HAStatusSummaryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ha_status_summary"
}
func (d *CephHealthMutesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_ceph_health_mutes"
}
func (d *CephMetadataDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_ceph_metadata"
}

func (d *HAStatusSummaryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Reads the cluster-wide HA status summary from `/cluster/ha/status` as JSON.", Attributes: map[string]datasourceschema.Attribute{
		"id": datasourceschema.StringAttribute{Computed: true}, "summary": datasourceschema.StringAttribute{Computed: true},
	}}
}
func (d *CephHealthMutesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Lists currently muted Ceph health checks from `/cluster/ceph/health-mute`.", Attributes: map[string]datasourceschema.Attribute{
		"id": datasourceschema.StringAttribute{Computed: true},
		"mutes": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
			"code": datasourceschema.StringAttribute{Computed: true}, "summary": datasourceschema.StringAttribute{Computed: true},
			"ttl": datasourceschema.StringAttribute{Computed: true}, "sticky": datasourceschema.BoolAttribute{Computed: true},
		}}},
	}}
}
func (d *CephMetadataDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Reads Ceph daemon metadata from `/cluster/ceph/metadata` as JSON.", Attributes: map[string]datasourceschema.Attribute{
		"id": datasourceschema.StringAttribute{Computed: true}, "scope": datasourceschema.StringAttribute{Optional: true, Computed: true},
		"metadata": datasourceschema.StringAttribute{Computed: true},
	}}
}

func (d *HAStatusSummaryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephHealthMutesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephMetadataDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *HAStatusSummaryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config haStatusSummaryDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	summary, err := d.client.HAStatusSummary(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read HA Status", err.Error())
		return
	}
	config.ID = types.StringValue("ha_status_summary")
	config.Summary = types.StringValue(summary)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *CephHealthMutesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephHealthMutesDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	items, err := d.client.CephHealthMutes(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph Health Mutes", err.Error())
		return
	}
	records := make([]cephHealthMuteRecord, 0, len(items))
	for _, item := range items {
		records = append(records, cephHealthMuteRecord{Code: stringOrNull(item.Code), Summary: stringOrNull(item.Summary), TTL: stringOrNull(item.TTL), Sticky: boolOrNull(item.Sticky.Ptr())})
	}
	config.ID = types.StringValue("cluster_ceph_health_mutes")
	config.Mutes = records
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *CephMetadataDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephMetadataDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	scope := stringValue(config.Scope)
	if scope == "" {
		scope = "all"
	}
	metadata, err := d.client.CephMetadata(ctx, scope)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph Metadata", err.Error())
		return
	}
	config.ID = types.StringValue(scope)
	config.Scope = types.StringValue(scope)
	config.Metadata = types.StringValue(metadata)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
