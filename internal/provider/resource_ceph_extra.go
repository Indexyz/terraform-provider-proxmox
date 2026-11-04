// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource     = &CephFSResource{}
	_ resource.Resource     = &CephMDSResource{}
	_ resource.Resource     = &CephMGRResource{}
	_ resource.Resource     = &CephInitResource{}
	_ resource.Resource     = &CephRestartResource{}
	_ resource.Resource     = &CephRestartBulkResource{}
	_ resource.Resource     = &CephOSDScrubResource{}
	_ datasource.DataSource = &CephFSDataSource{}
	_ datasource.DataSource = &CephMDSDataSource{}
	_ datasource.DataSource = &CephMGRDataSource{}
	_ datasource.DataSource = &CephCrushDataSource{}
	_ datasource.DataSource = &CephRulesDataSource{}
	_ datasource.DataSource = &CephConfigDBDataSource{}
	_ datasource.DataSource = &CephConfigRawDataSource{}
	_ datasource.DataSource = &CephConfigValueDataSource{}
	_ datasource.DataSource = &CephLogDataSource{}
	_ datasource.DataSource = &CephOSDLVInfoDataSource{}
	_ datasource.DataSource = &CephOSDMetadataDataSource{}
)

type CephFSResource struct{ client *Client }
type CephMDSResource struct{ client *Client }
type CephMGRResource struct{ client *Client }
type CephInitResource struct{ client *Client }
type CephRestartResource struct{ client *Client }
type CephRestartBulkResource struct{ client *Client }
type CephOSDScrubResource struct{ client *Client }

type CephFSDataSource struct{ client *Client }
type CephMDSDataSource struct{ client *Client }
type CephMGRDataSource struct{ client *Client }
type CephCrushDataSource struct{ client *Client }
type CephRulesDataSource struct{ client *Client }
type CephConfigDBDataSource struct{ client *Client }
type CephConfigRawDataSource struct{ client *Client }
type CephConfigValueDataSource struct{ client *Client }
type CephLogDataSource struct{ client *Client }
type CephOSDLVInfoDataSource struct{ client *Client }
type CephOSDMetadataDataSource struct{ client *Client }

type cephFSModel struct {
	ID           types.String `tfsdk:"id"`
	Node         types.String `tfsdk:"node"`
	Name         types.String `tfsdk:"name"`
	PGNum        types.Int64  `tfsdk:"pg_num"`
	AddStorage   types.Bool   `tfsdk:"add_storage"`
	DataPool     types.String `tfsdk:"data_pool"`
	MetadataPool types.String `tfsdk:"metadata_pool"`
	Triggers     types.Map    `tfsdk:"triggers"`
}
type cephMDSModel struct {
	ID         types.String `tfsdk:"id"`
	Node       types.String `tfsdk:"node"`
	Name       types.String `tfsdk:"name"`
	HotStandby types.Bool   `tfsdk:"hotstandby"`
	Host       types.String `tfsdk:"host"`
	FSName     types.String `tfsdk:"fs_name"`
	State      types.String `tfsdk:"state"`
	Triggers   types.Map    `tfsdk:"triggers"`
}
type cephMGRModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	Name     types.String `tfsdk:"name"`
	Host     types.String `tfsdk:"host"`
	State    types.String `tfsdk:"state"`
	Triggers types.Map    `tfsdk:"triggers"`
}
type cephInitModel struct {
	ID             types.String `tfsdk:"id"`
	Node           types.String `tfsdk:"node"`
	Network        types.String `tfsdk:"network"`
	ClusterNetwork types.String `tfsdk:"cluster_network"`
	Size           types.Int64  `tfsdk:"size"`
	MinSize        types.Int64  `tfsdk:"min_size"`
	PGBits         types.Int64  `tfsdk:"pg_bits"`
	DisableCephx   types.Bool   `tfsdk:"disable_cephx"`
	UPID           types.String `tfsdk:"upid"`
}
type cephRestartModel struct {
	ID      types.String `tfsdk:"id"`
	Node    types.String `tfsdk:"node"`
	Service types.String `tfsdk:"service"`
	UPID    types.String `tfsdk:"upid"`
}
type cephRestartBulkModel struct {
	ID           types.String `tfsdk:"id"`
	Node         types.String `tfsdk:"node"`
	ServiceType  types.String `tfsdk:"service_type"`
	Force        types.Bool   `tfsdk:"force"`
	OnlyOutdated types.Bool   `tfsdk:"only_outdated"`
	SetNoout     types.Bool   `tfsdk:"set_noout"`
	Timeout      types.Int64  `tfsdk:"timeout"`
	UPID         types.String `tfsdk:"upid"`
}
type cephOSDScrubModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	OSDID    types.Int64  `tfsdk:"osdid"`
	Deep     types.Bool   `tfsdk:"deep"`
	UPID     types.String `tfsdk:"upid"`
	Triggers types.Map    `tfsdk:"triggers"`
}
type cephFSDSModel struct {
	Node        types.String   `tfsdk:"node"`
	Filesystems []cephFSRecord `tfsdk:"filesystems"`
}
type cephFSRecord struct {
	Name         types.String `tfsdk:"name"`
	DataPool     types.String `tfsdk:"data_pool"`
	MetadataPool types.String `tfsdk:"metadata_pool"`
}
type cephMDSDSModel struct {
	Node    types.String    `tfsdk:"node"`
	Daemons []cephMDSRecord `tfsdk:"daemons"`
}
type cephMDSRecord struct {
	Name   types.String `tfsdk:"name"`
	Host   types.String `tfsdk:"host"`
	FSName types.String `tfsdk:"fs_name"`
	State  types.String `tfsdk:"state"`
}
type cephMGRDSModel struct {
	Node    types.String    `tfsdk:"node"`
	Daemons []cephMGRRecord `tfsdk:"daemons"`
}
type cephMGRRecord struct {
	Name  types.String `tfsdk:"name"`
	Host  types.String `tfsdk:"host"`
	State types.String `tfsdk:"state"`
}
type cephCrushDSModel struct {
	Node  types.String `tfsdk:"node"`
	Crush types.String `tfsdk:"crush"`
}
type cephRulesDSModel struct {
	Node  types.String `tfsdk:"node"`
	Rules types.List   `tfsdk:"rules"`
}
type cephConfigDBDSModel struct {
	Node    types.String       `tfsdk:"node"`
	Entries []cephConfigRecord `tfsdk:"entries"`
}
type cephConfigRecord struct {
	Name      types.String `tfsdk:"name"`
	Section   types.String `tfsdk:"section"`
	Value     types.String `tfsdk:"value"`
	Level     types.String `tfsdk:"level"`
	Mask      types.String `tfsdk:"mask"`
	CanUpdate types.Bool   `tfsdk:"can_update_at_runtime"`
}
type cephConfigRawDSModel struct {
	Node types.String `tfsdk:"node"`
	Raw  types.String `tfsdk:"raw"`
}
type cephConfigValueDSModel struct {
	Node  types.String `tfsdk:"node"`
	Keys  types.List   `tfsdk:"keys"`
	Value types.String `tfsdk:"value"`
}
type cephLogDSModel struct {
	Node    types.String    `tfsdk:"node"`
	Limit   types.Int64     `tfsdk:"limit"`
	Entries []cephLogRecord `tfsdk:"entries"`
}
type cephLogRecord struct {
	Line types.Int64  `tfsdk:"line"`
	Text types.String `tfsdk:"text"`
}
type cephOSDLVInfoDSModel struct {
	Node  types.String `tfsdk:"node"`
	OSDID types.Int64  `tfsdk:"osdid"`
	Type  types.String `tfsdk:"type"`
	Info  types.String `tfsdk:"info"`
}
type cephOSDMetadataDSModel struct {
	Node     types.String `tfsdk:"node"`
	OSDID    types.Int64  `tfsdk:"osdid"`
	Metadata types.String `tfsdk:"metadata"`
}

func NewCephFSResource() resource.Resource                { return &CephFSResource{} }
func NewCephMDSResource() resource.Resource               { return &CephMDSResource{} }
func NewCephMGRResource() resource.Resource               { return &CephMGRResource{} }
func NewCephInitResource() resource.Resource              { return &CephInitResource{} }
func NewCephRestartResource() resource.Resource           { return &CephRestartResource{} }
func NewCephRestartBulkResource() resource.Resource       { return &CephRestartBulkResource{} }
func NewCephOSDScrubResource() resource.Resource          { return &CephOSDScrubResource{} }
func NewCephFSDataSource() datasource.DataSource          { return &CephFSDataSource{} }
func NewCephMDSDataSource() datasource.DataSource         { return &CephMDSDataSource{} }
func NewCephMGRDataSource() datasource.DataSource         { return &CephMGRDataSource{} }
func NewCephCrushDataSource() datasource.DataSource       { return &CephCrushDataSource{} }
func NewCephRulesDataSource() datasource.DataSource       { return &CephRulesDataSource{} }
func NewCephConfigDBDataSource() datasource.DataSource    { return &CephConfigDBDataSource{} }
func NewCephConfigRawDataSource() datasource.DataSource   { return &CephConfigRawDataSource{} }
func NewCephConfigValueDataSource() datasource.DataSource { return &CephConfigValueDataSource{} }
func NewCephLogDataSource() datasource.DataSource         { return &CephLogDataSource{} }
func NewCephOSDLVInfoDataSource() datasource.DataSource   { return &CephOSDLVInfoDataSource{} }
func NewCephOSDMetadataDataSource() datasource.DataSource { return &CephOSDMetadataDataSource{} }

func (r *CephFSResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_fs"
}
func (r *CephMDSResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_mds"
}
func (r *CephMGRResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_mgr"
}
func (r *CephInitResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_init"
}
func (r *CephRestartResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_restart"
}
func (r *CephRestartBulkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_restart_bulk"
}
func (r *CephOSDScrubResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_osd_scrub"
}
func (d *CephFSDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_filesystems"
}
func (d *CephMDSDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_mds_daemons"
}
func (d *CephMGRDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_mgr_daemons"
}
func (d *CephCrushDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_crush"
}
func (d *CephRulesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_rules"
}
func (d *CephConfigDBDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_config"
}
func (d *CephConfigRawDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_config_raw"
}
func (d *CephConfigValueDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_config_value"
}
func (d *CephLogDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_log"
}
func (d *CephOSDLVInfoDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_osd_lv_info"
}
func (d *CephOSDMetadataDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_osd_metadata"
}

func (r *CephFSResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Creates and destroys a CephFS through `/nodes/{node}/ceph/fs/{name}`.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true}, "node": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"name":        schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"pg_num":      schema.Int64Attribute{Optional: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"add_storage": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"data_pool":   schema.StringAttribute{Computed: true}, "metadata_pool": schema.StringAttribute{Computed: true},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifierRequiresReplace()}},
	}}
}
func (r *CephMDSResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Creates and destroys a Ceph MDS through `/nodes/{node}/ceph/mds/{name}`.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true}, "node": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"name":       schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"hotstandby": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"host":       schema.StringAttribute{Computed: true}, "fs_name": schema.StringAttribute{Computed: true}, "state": schema.StringAttribute{Computed: true},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifierRequiresReplace()}},
	}}
}
func (r *CephMGRResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Creates and destroys a Ceph MGR through `/nodes/{node}/ceph/mgr/{id}`.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true}, "node": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"name": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"host": schema.StringAttribute{Computed: true}, "state": schema.StringAttribute{Computed: true},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifierRequiresReplace()}},
	}}
}
func (r *CephInitResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Initializes Ceph on a node through `POST /nodes/{node}/ceph/init`.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true}, "node": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"network":         schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"cluster_network": schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"size":            schema.Int64Attribute{Optional: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"min_size":        schema.Int64Attribute{Optional: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"pg_bits":         schema.Int64Attribute{Optional: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"disable_cephx":   schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"upid":            schema.StringAttribute{Computed: true},
	}}
}
func (r *CephRestartResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Restarts one Ceph service through `POST /nodes/{node}/ceph/restart`.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true}, "node": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"service": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}, "upid": schema.StringAttribute{Computed: true},
	}}
}
func (r *CephRestartBulkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Restarts Ceph daemons in bulk through `POST /nodes/{node}/ceph/restart-bulk`.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true}, "node": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"service_type":  schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"force":         schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"only_outdated": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"set_noout":     schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"timeout":       schema.Int64Attribute{Optional: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"upid":          schema.StringAttribute{Computed: true},
	}}
}
func (r *CephOSDScrubResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Scrubs one OSD through `POST /nodes/{node}/ceph/osd/{osdid}/scrub`.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true}, "node": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"osdid":    schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
		"deep":     schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifierRequiresReplace()}},
		"upid":     schema.StringAttribute{Computed: true},
	}}
}

func (d *CephFSDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Lists Ceph filesystems from `/nodes/{node}/ceph/fs`.", Attributes: map[string]datasourceschema.Attribute{
		"node": datasourceschema.StringAttribute{Required: true},
		"filesystems": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
			"name": datasourceschema.StringAttribute{Computed: true}, "data_pool": datasourceschema.StringAttribute{Computed: true}, "metadata_pool": datasourceschema.StringAttribute{Computed: true},
		}}},
	}}
}
func (d *CephMDSDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Lists Ceph MDS daemons from `/nodes/{node}/ceph/mds`.", Attributes: map[string]datasourceschema.Attribute{
		"node": datasourceschema.StringAttribute{Required: true},
		"daemons": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
			"name": datasourceschema.StringAttribute{Computed: true}, "host": datasourceschema.StringAttribute{Computed: true},
			"fs_name": datasourceschema.StringAttribute{Computed: true}, "state": datasourceschema.StringAttribute{Computed: true},
		}}},
	}}
}
func (d *CephMGRDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Lists Ceph MGR daemons from `/nodes/{node}/ceph/mgr`.", Attributes: map[string]datasourceschema.Attribute{
		"node": datasourceschema.StringAttribute{Required: true},
		"daemons": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
			"name": datasourceschema.StringAttribute{Computed: true}, "host": datasourceschema.StringAttribute{Computed: true}, "state": datasourceschema.StringAttribute{Computed: true},
		}}},
	}}
}
func (d *CephCrushDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Reads the raw Ceph CRUSH map JSON from `/nodes/{node}/ceph/crush`.", Attributes: map[string]datasourceschema.Attribute{
		"node": datasourceschema.StringAttribute{Required: true}, "crush": datasourceschema.StringAttribute{Computed: true},
	}}
}
func (d *CephRulesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Lists CRUSH rule names from `/nodes/{node}/ceph/rules`.", Attributes: map[string]datasourceschema.Attribute{
		"node": datasourceschema.StringAttribute{Required: true}, "rules": datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
	}}
}
func (d *CephConfigDBDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Lists Ceph configuration database entries from `/nodes/{node}/ceph/cfg/db`.", Attributes: map[string]datasourceschema.Attribute{
		"node": datasourceschema.StringAttribute{Required: true},
		"entries": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
			"name": datasourceschema.StringAttribute{Computed: true}, "section": datasourceschema.StringAttribute{Computed: true}, "value": datasourceschema.StringAttribute{Computed: true},
			"level": datasourceschema.StringAttribute{Computed: true}, "mask": datasourceschema.StringAttribute{Computed: true}, "can_update_at_runtime": datasourceschema.BoolAttribute{Computed: true},
		}}},
	}}
}
func (d *CephConfigRawDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Reads the raw Ceph config from `/nodes/{node}/ceph/cfg/raw`.", Attributes: map[string]datasourceschema.Attribute{
		"node": datasourceschema.StringAttribute{Required: true}, "raw": datasourceschema.StringAttribute{Computed: true},
	}}
}
func (d *CephConfigValueDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Reads specific Ceph config values from `/nodes/{node}/ceph/cfg/value`.", Attributes: map[string]datasourceschema.Attribute{
		"node": datasourceschema.StringAttribute{Required: true}, "keys": datasourceschema.ListAttribute{Required: true, ElementType: types.StringType}, "value": datasourceschema.StringAttribute{Computed: true},
	}}
}
func (d *CephLogDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Lists Ceph daemon log lines from `/nodes/{node}/ceph/log`.", Attributes: map[string]datasourceschema.Attribute{
		"node": datasourceschema.StringAttribute{Required: true}, "limit": datasourceschema.Int64Attribute{Optional: true, Computed: true},
		"entries": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
			"line": datasourceschema.Int64Attribute{Computed: true}, "text": datasourceschema.StringAttribute{Computed: true},
		}}},
	}}
}
func (d *CephOSDLVInfoDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Reads OSD LV info from `/nodes/{node}/ceph/osd/{osdid}/lv-info`.", Attributes: map[string]datasourceschema.Attribute{
		"node": datasourceschema.StringAttribute{Required: true}, "osdid": datasourceschema.Int64Attribute{Required: true},
		"type": datasourceschema.StringAttribute{Required: true}, "info": datasourceschema.StringAttribute{Computed: true},
	}}
}
func (d *CephOSDMetadataDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Reads OSD metadata from `/nodes/{node}/ceph/osd/{osdid}/metadata`.", Attributes: map[string]datasourceschema.Attribute{
		"node": datasourceschema.StringAttribute{Required: true}, "osdid": datasourceschema.Int64Attribute{Required: true}, "metadata": datasourceschema.StringAttribute{Computed: true},
	}}
}

func (r *CephFSResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *CephMDSResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *CephMGRResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *CephInitResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *CephRestartResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *CephRestartBulkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *CephOSDScrubResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (d *CephFSDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephMDSDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephMGRDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephCrushDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephRulesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephConfigDBDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephConfigRawDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephConfigValueDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephLogDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephOSDLVInfoDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephOSDMetadataDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func waitTask(ctx context.Context, client *Client, upid string, diags *diag.Diagnostics) bool {
	return nodeDiskWait(ctx, client, upid, diags)
}

func (r *CephFSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cephFSModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.CreateCephFS(ctx, plan.Node.ValueString(), plan.Name.ValueString(), plan.PGNum.ValueInt64(), plan.AddStorage.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create CephFS", err.Error())
		return
	}
	if !waitTask(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	r.refresh(ctx, plan.Node.ValueString(), plan.Name.ValueString(), plan.PGNum, plan.AddStorage, plan.Triggers, &resp.State, &resp.Diagnostics)
}
func (r *CephFSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cephFSModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.refresh(ctx, state.Node.ValueString(), state.Name.ValueString(), state.PGNum, state.AddStorage, state.Triggers, &resp.State, &resp.Diagnostics)
}
func (r *CephFSResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *CephFSResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cephFSModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.DeleteCephFS(ctx, state.Node.ValueString(), state.Name.ValueString())
	if err != nil {
		if !isNotFoundError(err) {
			resp.Diagnostics.AddError("Unable to Delete CephFS", err.Error())
		}
		return
	}
	waitTask(ctx, r.client, upid, &resp.Diagnostics)
}
func (r *CephFSResource) refresh(ctx context.Context, node, name string, pgNum types.Int64, addStorage types.Bool, triggers types.Map, state *tfsdk.State, diags *diag.Diagnostics) {
	items, err := r.client.CephFS(ctx, node)
	if err != nil {
		diags.AddError("Unable to Read Ceph Filesystems", err.Error())
		return
	}
	for _, item := range items {
		if item.Name == name {
			diags.Append(state.Set(ctx, &cephFSModel{ID: types.StringValue(node + "/" + name), Node: types.StringValue(node), Name: types.StringValue(name), DataPool: stringOrNull(item.DataPool), MetadataPool: stringOrNull(item.MetadataPool), PGNum: pgNum, AddStorage: addStorage, Triggers: triggers})...)
			return
		}
	}
	state.RemoveResource(ctx)
}

func (r *CephMDSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cephMDSModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if existing, err := r.client.CephMDS(ctx, plan.Node.ValueString()); err == nil {
		for _, item := range existing {
			if item.Name == plan.Name.ValueString() {
				r.refresh(ctx, plan.Node.ValueString(), plan.Name.ValueString(), plan.Triggers, &resp.State, &resp.Diagnostics)
				return
			}
		}
	}
	upid, err := r.client.CreateCephMDS(ctx, plan.Node.ValueString(), plan.Name.ValueString(), plan.HotStandby.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Ceph MDS", err.Error())
		return
	}
	if !waitTask(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	r.refresh(ctx, plan.Node.ValueString(), plan.Name.ValueString(), plan.Triggers, &resp.State, &resp.Diagnostics)
}
func (r *CephMDSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cephMDSModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.refresh(ctx, state.Node.ValueString(), state.Name.ValueString(), state.Triggers, &resp.State, &resp.Diagnostics)
}
func (r *CephMDSResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *CephMDSResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cephMDSModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.DeleteCephMDS(ctx, state.Node.ValueString(), state.Name.ValueString())
	if err != nil {
		if !isNotFoundError(err) {
			resp.Diagnostics.AddError("Unable to Delete Ceph MDS", err.Error())
		}
		return
	}
	waitTask(ctx, r.client, upid, &resp.Diagnostics)
}
func (r *CephMDSResource) refresh(ctx context.Context, node, name string, triggers types.Map, state *tfsdk.State, diags *diag.Diagnostics) {
	items, err := r.client.CephMDS(ctx, node)
	if err != nil {
		diags.AddError("Unable to Read Ceph MDS", err.Error())
		return
	}
	for _, item := range items {
		if item.Name == name {
			diags.Append(state.Set(ctx, &cephMDSModel{ID: types.StringValue(node + "/" + name), Node: types.StringValue(node), Name: types.StringValue(name), Host: stringOrNull(item.Host), FSName: stringOrNull(item.FSName), State: stringOrNull(item.State), HotStandby: types.BoolValue(false), Triggers: triggers})...)
			return
		}
	}
	state.RemoveResource(ctx)
}

func (r *CephMGRResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cephMGRModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if existing, err := r.client.CephMGRs(ctx, plan.Node.ValueString()); err == nil {
		for _, item := range existing {
			if item.Name == plan.Name.ValueString() {
				r.refresh(ctx, plan.Node.ValueString(), plan.Name.ValueString(), plan.Triggers, &resp.State, &resp.Diagnostics)
				return
			}
		}
	}
	upid, err := r.client.CreateCephMGR(ctx, plan.Node.ValueString(), plan.Name.ValueString())
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			resp.Diagnostics.AddError("Unable to Create Ceph MGR", err.Error())
			return
		}
		r.refresh(ctx, plan.Node.ValueString(), plan.Name.ValueString(), plan.Triggers, &resp.State, &resp.Diagnostics)
		return
	}
	if err := r.client.waitForNodeTask(ctx, plan.Node.ValueString(), upid); err != nil && !strings.Contains(err.Error(), "already exists") {
		resp.Diagnostics.AddError("Unable to Create Ceph MGR", err.Error())
		return
	}
	for attempt := 0; ; attempt++ {
		items, listErr := r.client.CephMGRs(ctx, plan.Node.ValueString())
		if listErr == nil {
			for _, item := range items {
				if item.Name == plan.Name.ValueString() {
					r.refresh(ctx, plan.Node.ValueString(), plan.Name.ValueString(), plan.Triggers, &resp.State, &resp.Diagnostics)
					return
				}
			}
		}
		if attempt > 30 {
			resp.Diagnostics.AddError("Unable to Read Ceph MGR", fmt.Sprint(listErr))
			return
		}
		time.Sleep(time.Second)
	}
}
func (r *CephMGRResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cephMGRModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.refresh(ctx, state.Node.ValueString(), state.Name.ValueString(), state.Triggers, &resp.State, &resp.Diagnostics)
}
func (r *CephMGRResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *CephMGRResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cephMGRModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.DeleteCephMGR(ctx, state.Node.ValueString(), state.Name.ValueString())
	if err != nil {
		if !isNotFoundError(err) {
			resp.Diagnostics.AddError("Unable to Delete Ceph MGR", err.Error())
		}
		return
	}
	waitTask(ctx, r.client, upid, &resp.Diagnostics)
}
func (r *CephMGRResource) refresh(ctx context.Context, node, name string, triggers types.Map, state *tfsdk.State, diags *diag.Diagnostics) {
	items, err := r.client.CephMGRs(ctx, node)
	if err != nil {
		diags.AddError("Unable to Read Ceph MGR", err.Error())
		return
	}
	for _, item := range items {
		if item.Name == name {
			diags.Append(state.Set(ctx, &cephMGRModel{ID: types.StringValue(node + "/" + name), Node: types.StringValue(node), Name: types.StringValue(name), Host: stringOrNull(item.Host), State: stringOrNull(item.State), Triggers: triggers})...)
			return
		}
	}
	state.RemoveResource(ctx)
}

func (r *CephInitResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cephInitModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.CephInit(ctx, plan.Node.ValueString(), stringValue(plan.Network), stringValue(plan.ClusterNetwork), plan.Size.ValueInt64(), plan.MinSize.ValueInt64(), plan.PGBits.ValueInt64(), plan.DisableCephx.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Initialize Ceph", err.Error())
		return
	}
	if !waitTask(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	plan.ID = types.StringValue(plan.Node.ValueString())
	plan.UPID = types.StringValue(upid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *CephInitResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *CephInitResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *CephInitResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *CephRestartResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cephRestartModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.CephRestart(ctx, plan.Node.ValueString(), plan.Service.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Restart Ceph Service", err.Error())
		return
	}
	if !waitTask(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	plan.ID = types.StringValue(plan.Node.ValueString() + "/" + plan.Service.ValueString())
	plan.UPID = types.StringValue(upid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *CephRestartResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *CephRestartResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *CephRestartResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *CephRestartBulkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cephRestartBulkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.CephRestartBulk(ctx, plan.Node.ValueString(), stringValue(plan.ServiceType), plan.Force.ValueBool(), plan.OnlyOutdated.ValueBool(), plan.SetNoout.ValueBool(), plan.Timeout.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Restart Ceph Services", err.Error())
		return
	}
	if !waitTask(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	plan.ID = types.StringValue(plan.Node.ValueString())
	plan.UPID = types.StringValue(upid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *CephRestartBulkResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *CephRestartBulkResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *CephRestartBulkResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *CephOSDScrubResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cephOSDScrubModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.ScrubCephOSD(ctx, plan.Node.ValueString(), plan.OSDID.ValueInt64(), plan.Deep.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Scrub Ceph OSD", err.Error())
		return
	}
	if upid != "" && !waitTask(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	plan.ID = types.StringValue(fmt.Sprintf("%s/%d", plan.Node.ValueString(), plan.OSDID.ValueInt64()))
	plan.UPID = types.StringValue(upid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *CephOSDScrubResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *CephOSDScrubResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *CephOSDScrubResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (d *CephFSDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephFSDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	items, err := d.client.CephFS(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph Filesystems", err.Error())
		return
	}
	records := make([]cephFSRecord, 0, len(items))
	for _, item := range items {
		records = append(records, cephFSRecord{Name: stringOrNull(item.Name), DataPool: stringOrNull(item.DataPool), MetadataPool: stringOrNull(item.MetadataPool)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cephFSDSModel{Node: config.Node, Filesystems: records})...)
}
func (d *CephMDSDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephMDSDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	items, err := d.client.CephMDS(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph MDS", err.Error())
		return
	}
	records := make([]cephMDSRecord, 0, len(items))
	for _, item := range items {
		records = append(records, cephMDSRecord{Name: stringOrNull(item.Name), Host: stringOrNull(item.Host), FSName: stringOrNull(item.FSName), State: stringOrNull(item.State)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cephMDSDSModel{Node: config.Node, Daemons: records})...)
}
func (d *CephMGRDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephMGRDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	items, err := d.client.CephMGRs(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph MGR", err.Error())
		return
	}
	records := make([]cephMGRRecord, 0, len(items))
	for _, item := range items {
		records = append(records, cephMGRRecord{Name: stringOrNull(item.Name), Host: stringOrNull(item.Host), State: stringOrNull(item.State)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cephMGRDSModel{Node: config.Node, Daemons: records})...)
}
func (d *CephCrushDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephCrushDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	crush, err := d.client.CephCrush(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph CRUSH", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cephCrushDSModel{Node: config.Node, Crush: types.StringValue(crush)})...)
}
func (d *CephRulesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephRulesDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rules, err := d.client.CephRules(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph Rules", err.Error())
		return
	}
	value, diags := types.ListValueFrom(ctx, types.StringType, rules)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cephRulesDSModel{Node: config.Node, Rules: value})...)
}
func (d *CephConfigDBDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephConfigDBDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	items, err := d.client.CephConfigDB(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph Config", err.Error())
		return
	}
	records := make([]cephConfigRecord, 0, len(items))
	for _, item := range items {
		records = append(records, cephConfigRecord{Name: stringOrNull(item.Name), Section: stringOrNull(item.Section), Value: stringOrNull(item.Value), Level: stringOrNull(item.Level), Mask: stringOrNull(item.Mask), CanUpdate: nodesBoolOrNull(item.CanUpdateAtRuntime)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cephConfigDBDSModel{Node: config.Node, Entries: records})...)
}
func (d *CephConfigRawDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephConfigRawDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	raw, err := d.client.CephConfigRaw(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph Config", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cephConfigRawDSModel{Node: config.Node, Raw: types.StringValue(raw)})...)
}
func (d *CephConfigValueDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephConfigValueDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	keys := listStrings(config.Keys)
	value, err := d.client.CephConfigValue(ctx, config.Node.ValueString(), keys)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph Config Value", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cephConfigValueDSModel{Node: config.Node, Keys: config.Keys, Value: types.StringValue(value)})...)
}
func (d *CephLogDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephLogDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	limit := int64(100)
	if !config.Limit.IsNull() && !config.Limit.IsUnknown() {
		limit = config.Limit.ValueInt64()
	}
	items, err := d.client.CephLog(ctx, config.Node.ValueString(), limit)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph Log", err.Error())
		return
	}
	records := make([]cephLogRecord, 0, len(items))
	for _, item := range items {
		records = append(records, cephLogRecord{Line: types.Int64Value(item.Line), Text: stringOrNull(item.Text)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cephLogDSModel{Node: config.Node, Limit: types.Int64Value(limit), Entries: records})...)
}
func (d *CephOSDLVInfoDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephOSDLVInfoDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	info, err := d.client.CephOSDLVInfo(ctx, config.Node.ValueString(), config.OSDID.ValueInt64(), config.Type.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read OSD LV Info", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cephOSDLVInfoDSModel{Node: config.Node, OSDID: config.OSDID, Type: config.Type, Info: types.StringValue(info)})...)
}
func (d *CephOSDMetadataDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephOSDMetadataDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	metadata, err := d.client.CephOSDMetadata(ctx, config.Node.ValueString(), config.OSDID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read OSD Metadata", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cephOSDMetadataDSModel{Node: config.Node, OSDID: config.OSDID, Metadata: types.StringValue(metadata)})...)
}

func listStrings(list types.List) []string {
	elements := list.Elements()
	values := make([]string, 0, len(elements))
	for _, element := range elements {
		value, ok := element.(types.String)
		if !ok {
			continue
		}
		values = append(values, value.ValueString())
	}
	return values
}

var _ = path.Root
var _ = stringdefault.StaticString
var _ = int64default.StaticInt64
