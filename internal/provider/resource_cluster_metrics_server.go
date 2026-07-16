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

var _ resource.Resource = &ClusterMetricsServerResource{}
var _ resource.ResourceWithImportState = &ClusterMetricsServerResource{}
var _ resource.ResourceWithValidateConfig = &ClusterMetricsServerResource{}

const clusterMetricsManagedFieldsKey = "cluster-metrics-managed-fields"

type ClusterMetricsServerResource struct {
	client *Client
}

type clusterMetricsServerModel struct {
	ID                    types.String `tfsdk:"id"`
	ServerID              types.String `tfsdk:"server_id"`
	Type                  types.String `tfsdk:"type"`
	Server                types.String `tfsdk:"server"`
	Port                  types.Int64  `tfsdk:"port"`
	Disable               types.Bool   `tfsdk:"disable"`
	GraphitePath          types.String `tfsdk:"graphite_path"`
	GraphiteProtocol      types.String `tfsdk:"graphite_protocol"`
	GraphiteTimeout       types.Int64  `tfsdk:"graphite_timeout"`
	MTU                   types.Int64  `tfsdk:"mtu"`
	InfluxDBProtocol      types.String `tfsdk:"influxdb_protocol"`
	Organization          types.String `tfsdk:"organization"`
	Bucket                types.String `tfsdk:"bucket"`
	Token                 types.String `tfsdk:"token"`
	VerifyCertificate     types.Bool   `tfsdk:"verify_certificate"`
	MaxBodySize           types.Int64  `tfsdk:"max_body_size"`
	APIPathPrefix         types.String `tfsdk:"api_path_prefix"`
	OpenTelemetryCompress types.String `tfsdk:"opentelemetry_compression"`
	OpenTelemetryHeaders  types.String `tfsdk:"opentelemetry_headers"`
	OpenTelemetryMaxBody  types.Int64  `tfsdk:"opentelemetry_max_body_size"`
	OpenTelemetryPath     types.String `tfsdk:"opentelemetry_path"`
	OpenTelemetryProtocol types.String `tfsdk:"opentelemetry_protocol"`
	OpenTelemetryResource types.String `tfsdk:"opentelemetry_resource_attributes"`
	OpenTelemetryTimeout  types.Int64  `tfsdk:"opentelemetry_timeout"`
	OpenTelemetryVerify   types.Bool   `tfsdk:"opentelemetry_verify_ssl"`
}

func NewClusterMetricsServerResource() resource.Resource {
	return &ClusterMetricsServerResource{}
}

func (r *ClusterMetricsServerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_metrics_server"
}

func (r *ClusterMetricsServerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox VE Graphite, InfluxDB, or OpenTelemetry metrics server through `/cluster/metrics/server/{id}`.",
		Attributes: map[string]schema.Attribute{
			"id":                                schema.StringAttribute{Computed: true, MarkdownDescription: "Metrics server identifier."},
			"server_id":                         schema.StringAttribute{Required: true, MarkdownDescription: "Stable metrics server identifier. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"type":                              schema.StringAttribute{Required: true, MarkdownDescription: "Plugin type: `graphite`, `influxdb`, or `opentelemetry`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"server":                            schema.StringAttribute{Required: true, MarkdownDescription: "Server DNS name or IP address."},
			"port":                              schema.Int64Attribute{Required: true, MarkdownDescription: "Server network port."},
			"disable":                           schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Disable metrics transmission."},
			"graphite_path":                     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Root Graphite path."},
			"graphite_protocol":                 schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Graphite transport protocol: `udp` or `tcp`."},
			"graphite_timeout":                  schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Graphite TCP socket timeout in seconds."},
			"mtu":                               schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "MTU for UDP metrics transmission."},
			"influxdb_protocol":                 schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "InfluxDB protocol: `udp`, `http`, or `https`."},
			"organization":                      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "InfluxDB v2 organization."},
			"bucket":                            schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "InfluxDB v2 bucket or database."},
			"token":                             schema.StringAttribute{Optional: true, Sensitive: true, MarkdownDescription: "InfluxDB access token. The API does not return this value."},
			"verify_certificate":                schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Verify certificates for InfluxDB HTTPS endpoints."},
			"max_body_size":                     schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "InfluxDB maximum batched request body size in bytes."},
			"api_path_prefix":                   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "InfluxDB API path prefix for reverse proxies."},
			"opentelemetry_compression":         schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "OpenTelemetry request compression: `none` or `gzip`."},
			"opentelemetry_headers":             schema.StringAttribute{Optional: true, Computed: true, Sensitive: true, MarkdownDescription: "Base64-encoded JSON OpenTelemetry HTTP headers."},
			"opentelemetry_max_body_size":       schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "OpenTelemetry maximum request body size in bytes."},
			"opentelemetry_path":                schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "OpenTelemetry endpoint path."},
			"opentelemetry_protocol":            schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "OpenTelemetry HTTP protocol: `http` or `https`."},
			"opentelemetry_resource_attributes": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Base64-encoded JSON OpenTelemetry resource attributes."},
			"opentelemetry_timeout":             schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "OpenTelemetry request timeout in seconds."},
			"opentelemetry_verify_ssl":          schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Verify TLS certificates for OpenTelemetry HTTPS endpoints."},
		},
	}
}

func (r *ClusterMetricsServerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ClusterMetricsServerResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config clusterMetricsServerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateClusterMetricsServerConfig(config)...)
}

func validateClusterMetricsServerConfig(config clusterMetricsServerModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if !config.Type.IsNull() && !config.Type.IsUnknown() && !slices.Contains([]string{"graphite", "influxdb", "opentelemetry"}, config.Type.ValueString()) {
		diags.AddAttributeError(path.Root("type"), "Invalid metrics server type", "type must be graphite, influxdb, or opentelemetry")
	}
	if !config.Server.IsNull() && !config.Server.IsUnknown() && strings.TrimSpace(config.Server.ValueString()) == "" {
		diags.AddAttributeError(path.Root("server"), "Invalid metrics server address", "server must not be empty")
	}
	if !config.Port.IsNull() && !config.Port.IsUnknown() && (config.Port.ValueInt64() < 1 || config.Port.ValueInt64() > 65536) {
		diags.AddAttributeError(path.Root("port"), "Invalid metrics server port", "port must be between 1 and 65536")
	}
	validateMetricsEnum(&diags, path.Root("graphite_protocol"), config.GraphiteProtocol, []string{"udp", "tcp"})
	validateMetricsEnum(&diags, path.Root("influxdb_protocol"), config.InfluxDBProtocol, []string{"udp", "http", "https"})
	validateMetricsEnum(&diags, path.Root("opentelemetry_compression"), config.OpenTelemetryCompress, []string{"none", "gzip"})
	validateMetricsEnum(&diags, path.Root("opentelemetry_protocol"), config.OpenTelemetryProtocol, []string{"http", "https"})
	if !config.Type.IsNull() && !config.Type.IsUnknown() {
		var incompatible []string
		addString := func(name string, value types.String) {
			if !value.IsNull() && !value.IsUnknown() {
				incompatible = append(incompatible, name)
			}
		}
		addInt64 := func(name string, value types.Int64) {
			if !value.IsNull() && !value.IsUnknown() {
				incompatible = append(incompatible, name)
			}
		}
		addBool := func(name string, value types.Bool) {
			if !value.IsNull() && !value.IsUnknown() {
				incompatible = append(incompatible, name)
			}
		}
		switch config.Type.ValueString() {
		case "graphite":
			addString("api_path_prefix", config.APIPathPrefix)
			addString("bucket", config.Bucket)
			addString("influxdb_protocol", config.InfluxDBProtocol)
			addInt64("max_body_size", config.MaxBodySize)
			addString("organization", config.Organization)
			addString("token", config.Token)
			addBool("verify_certificate", config.VerifyCertificate)
			addOpenTelemetryMetricsFields(&incompatible, config)
		case "influxdb":
			addString("graphite_path", config.GraphitePath)
			addString("graphite_protocol", config.GraphiteProtocol)
			addInt64("graphite_timeout", config.GraphiteTimeout)
			addOpenTelemetryMetricsFields(&incompatible, config)
		case "opentelemetry":
			addString("api_path_prefix", config.APIPathPrefix)
			addString("bucket", config.Bucket)
			addString("graphite_path", config.GraphitePath)
			addString("graphite_protocol", config.GraphiteProtocol)
			addInt64("graphite_timeout", config.GraphiteTimeout)
			addString("influxdb_protocol", config.InfluxDBProtocol)
			addInt64("max_body_size", config.MaxBodySize)
			addString("organization", config.Organization)
			addString("token", config.Token)
			addBool("verify_certificate", config.VerifyCertificate)
		}
		if len(incompatible) > 0 {
			slices.Sort(incompatible)
			diags.AddError("Invalid metrics server fields", fmt.Sprintf("type %q does not support: %s", config.Type.ValueString(), strings.Join(incompatible, ", ")))
		}
	}
	return diags
}

func addOpenTelemetryMetricsFields(fields *[]string, config clusterMetricsServerModel) {
	values := []struct {
		name    string
		present bool
	}{
		{"opentelemetry_compression", !config.OpenTelemetryCompress.IsNull() && !config.OpenTelemetryCompress.IsUnknown()},
		{"opentelemetry_headers", !config.OpenTelemetryHeaders.IsNull() && !config.OpenTelemetryHeaders.IsUnknown()},
		{"opentelemetry_max_body_size", !config.OpenTelemetryMaxBody.IsNull() && !config.OpenTelemetryMaxBody.IsUnknown()},
		{"opentelemetry_path", !config.OpenTelemetryPath.IsNull() && !config.OpenTelemetryPath.IsUnknown()},
		{"opentelemetry_protocol", !config.OpenTelemetryProtocol.IsNull() && !config.OpenTelemetryProtocol.IsUnknown()},
		{"opentelemetry_resource_attributes", !config.OpenTelemetryResource.IsNull() && !config.OpenTelemetryResource.IsUnknown()},
		{"opentelemetry_timeout", !config.OpenTelemetryTimeout.IsNull() && !config.OpenTelemetryTimeout.IsUnknown()},
		{"opentelemetry_verify_ssl", !config.OpenTelemetryVerify.IsNull() && !config.OpenTelemetryVerify.IsUnknown()},
	}
	for _, value := range values {
		if value.present {
			*fields = append(*fields, value.name)
		}
	}
}

func validateMetricsEnum(diags *diag.Diagnostics, attribute path.Path, value types.String, allowed []string) {
	if value.IsNull() || value.IsUnknown() || slices.Contains(allowed, value.ValueString()) {
		return
	}
	diags.AddAttributeError(attribute, "Invalid metrics server value", fmt.Sprintf("value must be one of %s", strings.Join(allowed, ", ")))
}

func clusterMetricsServerRequestFromModel(model clusterMetricsServerModel) ClusterMetricsServerRequest {
	return ClusterMetricsServerRequest{
		APIPathPrefix:         stringPointer(model.APIPathPrefix),
		Bucket:                stringPointer(model.Bucket),
		Disable:               boolPointerValue(model.Disable),
		InfluxDBProtocol:      stringPointer(model.InfluxDBProtocol),
		MaxBodySize:           int64PointerValue(model.MaxBodySize),
		MTU:                   int64PointerValue(model.MTU),
		OpenTelemetryCompress: stringPointer(model.OpenTelemetryCompress),
		OpenTelemetryHeaders:  stringPointer(model.OpenTelemetryHeaders),
		OpenTelemetryMaxBody:  int64PointerValue(model.OpenTelemetryMaxBody),
		OpenTelemetryPath:     stringPointer(model.OpenTelemetryPath),
		OpenTelemetryProtocol: stringPointer(model.OpenTelemetryProtocol),
		OpenTelemetryResource: stringPointer(model.OpenTelemetryResource),
		OpenTelemetryTimeout:  int64PointerValue(model.OpenTelemetryTimeout),
		OpenTelemetryVerify:   boolPointerValue(model.OpenTelemetryVerify),
		Organization:          stringPointer(model.Organization),
		Path:                  stringPointer(model.GraphitePath),
		Port:                  model.Port.ValueInt64(),
		Protocol:              stringPointer(model.GraphiteProtocol),
		Server:                model.Server.ValueString(),
		Timeout:               int64PointerValue(model.GraphiteTimeout),
		Token:                 stringPointer(model.Token),
		Type:                  model.Type.ValueString(),
		VerifyCertificate:     boolPointerValue(model.VerifyCertificate),
	}
}

func clusterMetricsManagedFields(config clusterMetricsServerModel) []string {
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
	addString("api-path-prefix", config.APIPathPrefix)
	addString("bucket", config.Bucket)
	addBool("disable", config.Disable)
	addString("influxdbproto", config.InfluxDBProtocol)
	addInt64("max-body-size", config.MaxBodySize)
	addInt64("mtu", config.MTU)
	addString("otel-compression", config.OpenTelemetryCompress)
	addString("otel-headers", config.OpenTelemetryHeaders)
	addInt64("otel-max-body-size", config.OpenTelemetryMaxBody)
	addString("otel-path", config.OpenTelemetryPath)
	addString("otel-protocol", config.OpenTelemetryProtocol)
	addString("otel-resource-attributes", config.OpenTelemetryResource)
	addInt64("otel-timeout", config.OpenTelemetryTimeout)
	addBool("otel-verify-ssl", config.OpenTelemetryVerify)
	addString("organization", config.Organization)
	addString("path", config.GraphitePath)
	addString("proto", config.GraphiteProtocol)
	addInt64("timeout", config.GraphiteTimeout)
	addString("token", config.Token)
	addBool("verify-certificate", config.VerifyCertificate)
	slices.Sort(fields)
	return fields
}

func clusterMetricsDeleteKeys(config clusterMetricsServerModel, previouslyManaged []string) []string {
	current := clusterMetricsManagedFields(config)
	currentSet := make(map[string]struct{}, len(current))
	for _, key := range current {
		currentSet[key] = struct{}{}
	}
	var deleted []string
	for _, key := range previouslyManaged {
		if _, ok := currentSet[key]; !ok {
			deleted = append(deleted, key)
		}
	}
	slices.Sort(deleted)
	return deleted
}

func (r *ClusterMetricsServerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterMetricsServerModel
	var config clusterMetricsServerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.CreateClusterMetricsServer(ctx, plan.ServerID.ValueString(), clusterMetricsServerRequestFromModel(plan)); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Cluster Metrics Server", err.Error())
		return
	}
	state, diags := r.readState(ctx, plan.ServerID.ValueString(), plan.Type.ValueString(), plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	managedFields, err := json.Marshal(clusterMetricsManagedFields(config))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Store Metrics Server State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, clusterMetricsManagedFieldsKey, managedFields)...)
}

func (r *ClusterMetricsServerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterMetricsServerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.ServerID.ValueString(), state.Type.ValueString(), state)
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

func (r *ClusterMetricsServerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config clusterMetricsServerModel
	var prior clusterMetricsServerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManagedJSON, privateDiags := req.Private.GetKey(ctx, clusterMetricsManagedFieldsKey)
	resp.Diagnostics.Append(privateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	var previouslyManaged []string
	if len(previouslyManagedJSON) > 0 {
		if err := json.Unmarshal(previouslyManagedJSON, &previouslyManaged); err != nil {
			resp.Diagnostics.AddError("Unable to Read Metrics Server State", fmt.Sprintf("unable to decode managed fields: %v", err))
			return
		}
	}
	current, err := r.client.GetClusterMetricsServer(ctx, prior.ServerID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Cluster Metrics Server", err.Error())
		return
	}
	updateReq := clusterMetricsServerRequestFromModel(config)
	updateReq.Delete = clusterMetricsDeleteKeys(config, previouslyManaged)
	if current.Digest != "" {
		updateReq.Digest = &current.Digest
	}
	if err := r.client.UpdateClusterMetricsServer(ctx, prior.ServerID.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Cluster Metrics Server", err.Error())
		return
	}
	refreshed, diags := r.readState(ctx, prior.ServerID.ValueString(), prior.Type.ValueString(), config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
	managedFields, err := json.Marshal(clusterMetricsManagedFields(config))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Store Metrics Server State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, clusterMetricsManagedFieldsKey, managedFields)...)
}

func (r *ClusterMetricsServerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterMetricsServerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteClusterMetricsServer(ctx, state.ServerID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Cluster Metrics Server", err.Error())
	}
}

func (r *ClusterMetricsServerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if strings.TrimSpace(req.ID) == "" {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "expected a non-empty metrics server identifier")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *ClusterMetricsServerResource) readState(ctx context.Context, id, serverType string, state clusterMetricsServerModel) (clusterMetricsServerModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	server, err := r.client.GetClusterMetricsServer(ctx, id)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return clusterMetricsServerModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Cluster Metrics Server", err.Error())
		return clusterMetricsServerModel{}, diags
	}
	if server.ID != "" {
		id = server.ID
	}
	if server.Type != "" {
		serverType = server.Type
	}
	state.ID = types.StringValue(id)
	state.ServerID = types.StringValue(id)
	state.Type = types.StringValue(serverType)
	state.Server = stringOrNull(server.Server)
	state.Port = int64OrNull(server.Port)
	state.Disable = boolOrNull(server.Disable.Ptr())
	state.GraphitePath = stringOrNull(server.Path)
	state.GraphiteProtocol = stringOrNull(server.Protocol)
	state.GraphiteTimeout = int64OrNull(server.Timeout)
	state.MTU = int64OrNull(server.MTU)
	state.InfluxDBProtocol = stringOrNull(server.InfluxDBProtocol)
	state.Organization = stringOrNull(server.Organization)
	state.Bucket = stringOrNull(server.Bucket)
	if server.Token != "" {
		state.Token = types.StringValue(server.Token)
	}
	state.VerifyCertificate = boolOrNull(server.VerifyCertificate.Ptr())
	state.MaxBodySize = int64OrNull(server.MaxBodySize)
	state.APIPathPrefix = stringOrNull(server.APIPathPrefix)
	state.OpenTelemetryCompress = stringOrNull(server.OpenTelemetryCompress)
	if server.OpenTelemetryHeaders != "" {
		state.OpenTelemetryHeaders = types.StringValue(server.OpenTelemetryHeaders)
	}
	state.OpenTelemetryMaxBody = int64OrNull(server.OpenTelemetryMaxBody)
	state.OpenTelemetryPath = stringOrNull(server.OpenTelemetryPath)
	state.OpenTelemetryProtocol = stringOrNull(server.OpenTelemetryProtocol)
	state.OpenTelemetryResource = stringOrNull(server.OpenTelemetryResource)
	state.OpenTelemetryTimeout = int64OrNull(server.OpenTelemetryTimeout)
	state.OpenTelemetryVerify = boolOrNull(server.OpenTelemetryVerify.Ptr())
	return state, diags
}
