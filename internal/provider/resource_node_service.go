// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &NodeServiceResource{}
var _ resource.ResourceWithImportState = &NodeServiceResource{}
var _ resource.ResourceWithValidateConfig = &NodeServiceResource{}
var _ resource.Resource = &NodeAPTUpdateResource{}
var _ resource.ResourceWithValidateConfig = &NodeAPTUpdateResource{}
var _ resource.Resource = &NodeAPTRepositoryResource{}
var _ resource.ResourceWithImportState = &NodeAPTRepositoryResource{}

type NodeServiceResource struct{ client *Client }
type NodeAPTUpdateResource struct{ client *Client }
type NodeAPTRepositoryResource struct{ client *Client }

type nodeServiceModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	Service  types.String `tfsdk:"service"`
	State    types.String `tfsdk:"state"`
	Triggers types.Map    `tfsdk:"triggers"`
}

type nodeAPTUpdateModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	Quiet    types.Bool   `tfsdk:"quiet"`
	Notify   types.Bool   `tfsdk:"notify"`
	Triggers types.Map    `tfsdk:"triggers"`
	UPID     types.String `tfsdk:"upid"`
}

type nodeAPTRepositoryModel struct {
	ID      types.String `tfsdk:"id"`
	Node    types.String `tfsdk:"node"`
	Path    types.String `tfsdk:"path"`
	Index   types.Int64  `tfsdk:"index"`
	Enabled types.Bool   `tfsdk:"enabled"`
}

func NewNodeServiceResource() resource.Resource       { return &NodeServiceResource{} }
func NewNodeAPTUpdateResource() resource.Resource     { return &NodeAPTUpdateResource{} }
func NewNodeAPTRepositoryResource() resource.Resource { return &NodeAPTRepositoryResource{} }

func (r *NodeServiceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_service"
}

func (r *NodeAPTUpdateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_apt_update"
}

func (r *NodeAPTRepositoryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_apt_repository"
}

func (r *NodeServiceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the run state of one system service through `/nodes/{node}/services/{service}/{start|stop}`. Destroy leaves the service running. Changing `triggers` re-applies the state.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<node>/<service>` form."},
			"node":     schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"service":  schema.StringAttribute{Required: true, MarkdownDescription: "Service name such as `pveproxy`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"state":    schema.StringAttribute{Required: true, MarkdownDescription: "Desired state: `running` or `stopped`."},
			"triggers": schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Optional values that force a re-apply when changed, for example to restart a service.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace(), mapplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *NodeAPTUpdateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Runs `apt update` on one node through `POST /nodes/{node}/apt/update` and waits for the worker. The action is imperative: every change replaces the instance, destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier (the node name)."},
			"node":     schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"quiet":    schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Suppress progress output. The Proxmox default is `true`.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"notify":   schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Send a notification with the result.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to update again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"upid":     schema.StringAttribute{Computed: true, MarkdownDescription: "Worker task UPID."},
		},
	}
}

func (r *NodeAPTRepositoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Enables or disables one repository entry of an APT source file through `POST /nodes/{node}/apt/repositories`. The entry itself must already exist. Destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<node>/<path>/<index>` form."},
			"node":    schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"path":    schema.StringAttribute{Required: true, MarkdownDescription: "Source file path such as `/etc/apt/sources.list`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"index":   schema.Int64Attribute{Required: true, MarkdownDescription: "Zero-based index of the repository inside the file. Changes require replacement.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"enabled": schema.BoolAttribute{Required: true, MarkdownDescription: "Whether the repository is enabled."},
		},
	}
}

func (r *NodeServiceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *NodeAPTUpdateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *NodeAPTRepositoryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *NodeServiceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeServiceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateNodeNameResource(config.Node, resp)
	if isConfiguredValue(config.Service) && strings.TrimSpace(config.Service.ValueString()) == "" {
		resp.Diagnostics.AddAttributeError(path.Root("service"), "Invalid service", "service must not be empty.")
	}
	if isConfiguredValue(config.State) && !slices.Contains([]string{"running", "stopped"}, config.State.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("state"), "Invalid service state", "state must be running or stopped.")
	}
}

func (r *NodeAPTUpdateResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeAPTUpdateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateNodeNameResource(config.Node, resp)
	if isConfiguredValue(config.Triggers) && len(config.Triggers.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(path.Root("triggers"), "Missing update triggers", "triggers must contain at least one value.")
	}
}

func (r *NodeServiceResource) apply(ctx context.Context, plan *nodeServiceModel, state *tfsdk.State, diags *diag.Diagnostics) {
	node, service := plan.Node.ValueString(), plan.Service.ValueString()
	action := "start"
	if plan.State.ValueString() == "stopped" {
		action = "stop"
	}
	upid, err := r.client.NodeServiceAction(ctx, node, service, action)
	if err != nil {
		diags.AddError("Unable to Change Proxmox Service State", err.Error())
		return
	}
	if upid != "" {
		owner, ownerErr := taskOwnerNode(upid)
		if ownerErr != nil {
			diags.AddError("Unexpected Proxmox Task UPID", ownerErr.Error())
			return
		}
		if err := r.client.waitForNodeTask(ctx, owner, upid); err != nil {
			diags.AddError("Proxmox Service Task Failed", fmt.Sprintf("task %q did not finish: %s", upid, err))
			return
		}
	}
	next := *plan
	next.ID = types.StringValue(node + "/" + service)
	if next.Triggers.IsUnknown() {
		next.Triggers = types.MapNull(types.StringType)
	}
	diags.Append(state.Set(ctx, &next)...)
}

func (r *NodeServiceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nodeServiceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeServiceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nodeServiceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.NodeServiceState(ctx, state.Node.ValueString(), state.Service.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Service State", err.Error())
		return
	}
	if current.ActiveState == "active" {
		state.State = types.StringValue("running")
	} else {
		state.State = types.StringValue("stopped")
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NodeServiceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan nodeServiceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeServiceResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *NodeServiceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, service, ok := strings.Cut(req.ID, "/")
	if !ok || node == "" || service == "" {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected `<node>/<service>`.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service"), service)...)
}

func (r *NodeAPTUpdateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeAPTUpdateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeAPTUpdateResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *NodeAPTUpdateResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan nodeAPTUpdateModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	node := plan.Node.ValueString()
	upid, err := r.client.NodeAPTUpdate(ctx, node, plan.Quiet.ValueBool(), plan.Notify.ValueBool())
	if err != nil {
		diags.AddError("Unable to Run Proxmox APT Update", err.Error())
		return
	}
	owner, err := taskOwnerNode(upid)
	if err != nil {
		diags.AddError("Unexpected Proxmox Task UPID", err.Error())
		return
	}
	if err := r.client.waitForNodeTask(ctx, owner, upid); err != nil {
		diags.AddError("Proxmox APT Update Failed", fmt.Sprintf("task %q did not finish: %s", upid, err))
		return
	}
	next := plan
	next.ID = types.StringValue(node)
	next.UPID = types.StringValue(upid)
	diags.Append(state.Set(ctx, &next)...)
}

func (r *NodeAPTRepositoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.toggle(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeAPTRepositoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.toggle(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *NodeAPTRepositoryResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *NodeAPTRepositoryResource) toggle(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan nodeAPTRepositoryModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	node, apiPath, index := plan.Node.ValueString(), plan.Path.ValueString(), plan.Index.ValueInt64()
	// The toggle is guarded by the digest of the whole APT configuration, which
	// changes with every edit; parallel toggles therefore retry on conflict.
	for attempt := 0; ; attempt++ {
		repositories, err := r.client.NodeAPTRepositories(ctx, node)
		if err != nil {
			diags.AddError("Unable to Read Proxmox APT Repositories", err.Error())
			return
		}
		found := false
		for _, file := range repositories.Files {
			if file.Path == apiPath {
				found = true
				break
			}
		}
		if !found {
			diags.AddAttributeError(path.Root("path"), "Unknown APT source file", fmt.Sprintf("%q is not reported by Proxmox for node %q.", apiPath, node))
			return
		}
		err = r.client.ToggleAPTRepository(ctx, node, apiPath, index, plan.Enabled.ValueBool(), repositories.Digest)
		if err == nil {
			break
		}
		if attempt >= 5 || !strings.Contains(err.Error(), "detected modified configuration") {
			diags.AddError("Unable to Toggle Proxmox APT Repository", err.Error())
			return
		}
	}
	next := plan
	next.ID = types.StringValue(fmt.Sprintf("%s/%s/%d", node, apiPath, index))
	diags.Append(state.Set(ctx, &next)...)
}

func (r *NodeAPTRepositoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nodeAPTRepositoryModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	repositories, err := r.client.NodeAPTRepositories(ctx, state.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox APT Repositories", err.Error())
		return
	}
	for _, file := range repositories.Files {
		if file.Path != state.Path.ValueString() {
			continue
		}
		index := int(state.Index.ValueInt64())
		if index >= len(file.Repositories) {
			continue
		}
		state.Enabled = types.BoolValue(file.Repositories[index].Enabled.Ptr() != nil && *file.Repositories[index].Enabled.Ptr())
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	resp.State.RemoveResource(ctx)
}

func (r *NodeAPTRepositoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, rest, ok := strings.Cut(req.ID, "/")
	separator := strings.LastIndex(rest, "/")
	if !ok || separator <= 0 {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected `<node>/<path>/<index>`.")
		return
	}
	apiPath, index := rest[:separator], rest[separator+1:]
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("path"), apiPath)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("index"), index)...)
}

func validateNodeNameResource(node types.String, resp *resource.ValidateConfigResponse) {
	if isConfiguredValue(node) && !proxmoxNodeNamePattern.MatchString(node.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("node"), "Invalid node name", "node must be an existing cluster node name.")
	}
}

func (r *NodeAPTUpdateResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
	// Imperative action; state is kept.
}
