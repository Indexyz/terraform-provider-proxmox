// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource = &CephCmdSafetyDataSource{}
	_ datasource.DataSource = &CephReleasesDataSource{}
	_ datasource.DataSource = &CephConfigDefaultDataSource{}
)

type CephCmdSafetyDataSource struct{ client *Client }
type CephReleasesDataSource struct{ client *Client }
type CephConfigDefaultDataSource struct{ client *Client }

type cephCmdSafetyDSModel struct {
	ID      types.String `tfsdk:"id"`
	Node    types.String `tfsdk:"node"`
	Service types.String `tfsdk:"service"`
	Action  types.String `tfsdk:"action"`
	IDName  types.String `tfsdk:"name"`
	Safe    types.Bool   `tfsdk:"safe"`
	Status  types.String `tfsdk:"status"`
}
type cephReleasesDSModel struct {
	ID       types.String        `tfsdk:"id"`
	Node     types.String        `tfsdk:"node"`
	Releases []cephReleaseRecord `tfsdk:"releases"`
}
type cephReleaseRecord struct {
	Release     types.String `tfsdk:"release"`
	Version     types.String `tfsdk:"version"`
	Available   types.Bool   `tfsdk:"available"`
	IsDefault   types.Bool   `tfsdk:"is_default"`
	Unsupported types.Bool   `tfsdk:"unsupported"`
}
type cephConfigDefaultDSModel struct {
	ID     types.String `tfsdk:"id"`
	Node   types.String `tfsdk:"node"`
	Config types.String `tfsdk:"config"`
}

func NewCephCmdSafetyDataSource() datasource.DataSource     { return &CephCmdSafetyDataSource{} }
func NewCephReleasesDataSource() datasource.DataSource      { return &CephReleasesDataSource{} }
func NewCephConfigDefaultDataSource() datasource.DataSource { return &CephConfigDefaultDataSource{} }

func (d *CephCmdSafetyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_cmd_safety"
}
func (d *CephReleasesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_releases"
}
func (d *CephConfigDefaultDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_config_index"
}

func (d *CephCmdSafetyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Checks whether a Ceph service action is safe through `GET /nodes/{node}/ceph/cmd-safety`.", Attributes: map[string]datasourceschema.Attribute{
		"id": datasourceschema.StringAttribute{Computed: true}, "node": datasourceschema.StringAttribute{Required: true},
		"service": datasourceschema.StringAttribute{Required: true}, "action": datasourceschema.StringAttribute{Required: true},
		"name": datasourceschema.StringAttribute{Required: true},
		"safe": datasourceschema.BoolAttribute{Computed: true}, "status": datasourceschema.StringAttribute{Computed: true},
	}}
}
func (d *CephReleasesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Lists installable Ceph releases from `GET /nodes/{node}/ceph/releases`. PVE 9.2 returns 501 for this path.", Attributes: map[string]datasourceschema.Attribute{
		"id": datasourceschema.StringAttribute{Computed: true}, "node": datasourceschema.StringAttribute{Required: true},
		"releases": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
			"release": datasourceschema.StringAttribute{Computed: true}, "version": datasourceschema.StringAttribute{Computed: true},
			"available": datasourceschema.BoolAttribute{Computed: true}, "is_default": datasourceschema.BoolAttribute{Computed: true},
			"unsupported": datasourceschema.BoolAttribute{Computed: true},
		}}},
	}}
}
func (d *CephConfigDefaultDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Reads default Ceph configuration entries as JSON from `GET /nodes/{node}/ceph/cfg`.", Attributes: map[string]datasourceschema.Attribute{
		"id": datasourceschema.StringAttribute{Computed: true}, "node": datasourceschema.StringAttribute{Required: true},
		"config": datasourceschema.StringAttribute{Computed: true},
	}}
}

func (d *CephCmdSafetyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephReleasesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CephConfigDefaultDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

type cephCmdSafetyResult struct {
	Safe   bool   `json:"safe"`
	Status string `json:"status"`
}

func (d *CephCmdSafetyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephCmdSafetyDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	query := url.Values{"service": {config.Service.ValueString()}, "action": {config.Action.ValueString()}, "id": {config.IDName.ValueString()}}
	var result cephCmdSafetyResult
	if err := d.client.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(config.Node.ValueString())+"/ceph/cmd-safety", query, nil, &result); err != nil {
		resp.Diagnostics.AddError("Unable to Check Ceph Command Safety", err.Error())
		return
	}
	config.ID = types.StringValue(config.Node.ValueString() + "/" + config.Service.ValueString() + "/" + config.Action.ValueString())
	config.Safe = types.BoolValue(result.Safe)
	config.Status = stringOrNull(result.Status)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *CephReleasesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephReleasesDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var items []struct {
		Release     string `json:"release"`
		Version     string `json:"version"`
		Available   bool   `json:"available"`
		IsDefault   bool   `json:"is-default"`
		Unsupported bool   `json:"unsupported"`
	}
	if err := d.client.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(config.Node.ValueString())+"/ceph/releases", nil, nil, &items); err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph Releases", err.Error())
		return
	}
	records := make([]cephReleaseRecord, 0, len(items))
	for _, item := range items {
		records = append(records, cephReleaseRecord{Release: stringOrNull(item.Release), Version: stringOrNull(item.Version), Available: types.BoolValue(item.Available), IsDefault: types.BoolValue(item.IsDefault), Unsupported: types.BoolValue(item.Unsupported)})
	}
	config.ID = types.StringValue(config.Node.ValueString())
	config.Releases = records
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *CephConfigDefaultDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cephConfigDefaultDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var items []map[string]any
	if err := d.client.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(config.Node.ValueString())+"/ceph/cfg", nil, nil, &items); err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph Config Defaults", err.Error())
		return
	}
	data, err := json.Marshal(items)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Encode Ceph Config Defaults", err.Error())
		return
	}
	config.ID = types.StringValue(config.Node.ValueString())
	config.Config = types.StringValue(string(data))
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
