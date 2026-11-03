// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ClusterConfigDataSource{}
var _ resource.Resource = &ClusterJoinResource{}
var _ resource.ResourceWithValidateConfig = &ClusterJoinResource{}
var _ resource.Resource = &ClusterNodeRemoveResource{}
var _ resource.ResourceWithImportState = &ClusterNodeRemoveResource{}
var _ resource.ResourceWithValidateConfig = &ClusterNodeRemoveResource{}

type ClusterConfigDataSource struct{ client *Client }
type ClusterJoinResource struct{ client *Client }
type ClusterNodeRemoveResource struct{ client *Client }

type clusterConfigModel struct {
	Totem      types.Map    `tfsdk:"totem"`
	APIVersion types.String `tfsdk:"apiversion"`
	Nodes      types.List   `tfsdk:"nodes"`
}

type clusterJoinModel struct {
	ID          types.String `tfsdk:"id"`
	Hostname    types.String `tfsdk:"hostname"`
	Password    types.String `tfsdk:"password"`
	Fingerprint types.String `tfsdk:"fingerprint"`
	NodeID      types.Int64  `tfsdk:"node_id"`
	Votes       types.Int64  `tfsdk:"votes"`
	Force       types.Bool   `tfsdk:"force"`
	Links       types.Set    `tfsdk:"links"`
	Triggers    types.Map    `tfsdk:"triggers"`
	UPID        types.String `tfsdk:"upid"`
}

type clusterNodeRemoveModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	Triggers types.Map    `tfsdk:"triggers"`
}

func NewClusterConfigDataSource() datasource.DataSource { return &ClusterConfigDataSource{} }
func NewClusterJoinResource() resource.Resource         { return &ClusterJoinResource{} }
func NewClusterNodeRemoveResource() resource.Resource   { return &ClusterNodeRemoveResource{} }

func (d *ClusterConfigDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_config"
}

func (r *ClusterJoinResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_join"
}

func (r *ClusterNodeRemoveResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_node_remove"
}

func (d *ClusterConfigDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the corosync configuration of the cluster: `/cluster/config/totem`, `/cluster/config/apiversion` and the member nodes from `/cluster/config/nodes`. Values are exposed as JSON strings.",
		Attributes: map[string]datasourceschema.Attribute{
			"totem":      datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
			"apiversion": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Corosync API version."},
			"nodes":      datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (r *ClusterJoinResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Joins this node to an existing cluster through `POST /cluster/config/join` and waits for the task. The action is imperative: every change replaces the instance, destroy is a no-op. `fingerprint` must be the SHA-256 fingerprint of the peer node certificate.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, MarkdownDescription: "Join identifier (the peer hostname)."},
			"hostname":    schema.StringAttribute{Required: true, MarkdownDescription: "IP or DNS name of an existing cluster node.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"password":    schema.StringAttribute{Required: true, Sensitive: true, MarkdownDescription: "`root@pam` password of the peer node.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"fingerprint": schema.StringAttribute{Required: true, MarkdownDescription: "SHA-256 certificate fingerprint of the peer node.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"node_id":     schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Explicit node id in the cluster.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"votes":       schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Corosync votes for this node.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"force":       schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Force the join even if the peer reports warnings.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"links":       schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()}},
			"triggers":    schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to join again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"upid":        schema.StringAttribute{Computed: true, MarkdownDescription: "Worker task UPID."},
		},
	}
}

func (r *ClusterNodeRemoveResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Removes a node from the cluster configuration through `DELETE /cluster/config/nodes/{node}`. The local node cannot be removed through its own API: Proxmox answers 500 `Cannot delete myself` and the call becomes a no-op, so apply it against another node. The action is imperative: every change replaces the instance, destroy is a no-op, and the node itself is not powered off.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true, MarkdownDescription: "Node name."},
			"node":     schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node to remove. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to remove again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
		},
	}
}

func (d *ClusterConfigDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (r *ClusterJoinResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *ClusterNodeRemoveResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *ClusterJoinResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config clusterJoinModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if isConfiguredValue(config.Hostname) && strings.TrimSpace(config.Hostname.ValueString()) == "" {
		resp.Diagnostics.AddAttributeError(path.Root("hostname"), "Invalid hostname", "hostname must be set.")
	}
	if isConfiguredValue(config.Fingerprint) && !strings.Contains(config.Fingerprint.ValueString(), ":") {
		resp.Diagnostics.AddAttributeError(path.Root("fingerprint"), "Invalid fingerprint", "fingerprint must be a colon separated SHA-256 fingerprint.")
	}
	if isConfiguredValue(config.Triggers) && len(config.Triggers.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(path.Root("triggers"), "Missing join triggers", "triggers must contain at least one value.")
	}
}

func (r *ClusterNodeRemoveResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config clusterNodeRemoveModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if isConfiguredValue(config.Node) && !proxmoxNodeNamePattern.MatchString(config.Node.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("node"), "Invalid node name", "node must be an existing cluster node name.")
	}
	if isConfiguredValue(config.Triggers) && len(config.Triggers.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(path.Root("triggers"), "Missing remove triggers", "triggers must contain at least one value.")
	}
}

func (d *ClusterConfigDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config clusterConfigModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	raw, err := d.client.ClusterConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Cluster Config", err.Error())
		return
	}
	totem := map[string]string{}
	for key, value := range raw.Totem {
		totem[key] = unquoteJSON(value)
	}
	totemMap, totemDiags := types.MapValueFrom(ctx, types.StringType, totem)
	resp.Diagnostics.Append(totemDiags...)
	apiVersion := strings.Trim(string(raw.APIVersion), "\"")
	nodes := append([]string{}, raw.Nodes...)
	sort.Strings(nodes)
	nodeList, nodeDiags := types.ListValueFrom(ctx, types.StringType, nodes)
	resp.Diagnostics.Append(nodeDiags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &clusterConfigModel{Totem: totemMap, APIVersion: stringOrNull(apiVersion), Nodes: nodeList})...)
}

func (r *ClusterJoinResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.join(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *ClusterJoinResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.join(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *ClusterJoinResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *ClusterJoinResource) join(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan clusterJoinModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	upid, err := r.client.JoinCluster(
		ctx,
		plan.Hostname.ValueString(),
		plan.Password.ValueString(),
		plan.Fingerprint.ValueString(),
		int64PointerValue(plan.NodeID),
		int64PointerValue(plan.Votes),
		plan.Force.ValueBool(),
		stringSetValues(plan.Links),
	)
	if err != nil {
		diags.AddError("Unable to Join Proxmox Cluster", err.Error())
		return
	}
	owner, err := taskOwnerNode(upid)
	if err != nil {
		diags.AddError("Unable to Join Proxmox Cluster", err.Error())
		return
	}
	if err := r.client.waitForNodeTask(ctx, owner, upid); err != nil {
		diags.AddError("Proxmox Cluster Join Failed", fmt.Sprintf("join task %q did not finish: %s", upid, err))
		return
	}
	next := plan
	next.ID = types.StringValue(plan.Hostname.ValueString())
	next.UPID = types.StringValue(upid)
	diags.Append(state.Set(ctx, &next)...)
}

func (r *ClusterNodeRemoveResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.remove(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *ClusterNodeRemoveResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.remove(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *ClusterNodeRemoveResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *ClusterNodeRemoveResource) remove(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan clusterNodeRemoveModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	if err := r.client.RemoveClusterNode(ctx, plan.Node.ValueString()); err != nil {
		// The local node cannot be removed through its own API and answers 500;
		// that is the documented Proxmox behaviour, not a failure.
		if !strings.Contains(err.Error(), "Cannot delete myself") {
			diags.AddError("Unable to Remove Proxmox Cluster Node", err.Error())
			return
		}
	}
	next := plan
	next.ID = types.StringValue(plan.Node.ValueString())
	diags.Append(state.Set(ctx, &next)...)
}

func (r *ClusterNodeRemoveResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node := strings.TrimSpace(req.ID)
	if !proxmoxNodeNamePattern.MatchString(node) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected a node name.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
}

var _ = json.Marshal

func (r *ClusterJoinResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
	// Imperative action; state is kept.
}

func (r *ClusterNodeRemoveResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
	// Imperative action; state is kept.
}

func unquoteJSON(raw json.RawMessage) string {
	if len(raw) > 0 && raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			return text
		}
	}
	return string(raw)
}
