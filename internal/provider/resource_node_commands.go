// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource = &NodeWakeOnLANResource{}
	_ resource.Resource = &NodeStartAllResource{}
	_ resource.Resource = &NodeMigrateAllResource{}
	_ resource.Resource = &NodeAPLDownloadResource{}
	_ resource.Resource = &NodeOCIPullResource{}
)

type NodeWakeOnLANResource struct{ client *Client }
type NodeStartAllResource struct{ client *Client }
type NodeMigrateAllResource struct{ client *Client }
type NodeAPLDownloadResource struct{ client *Client }
type NodeOCIPullResource struct{ client *Client }

type wakeOnLANModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	Triggers types.Map    `tfsdk:"triggers"`
	MAC      types.String `tfsdk:"mac"`
}
type nodeStartAllModel struct {
	ID         types.String `tfsdk:"id"`
	Node       types.String `tfsdk:"node"`
	VMs        types.Set    `tfsdk:"vms"`
	Force      types.Bool   `tfsdk:"force"`
	MaxWorkers types.Int64  `tfsdk:"max_workers"`
	Triggers   types.Map    `tfsdk:"triggers"`
	UPID       types.String `tfsdk:"upid"`
}
type nodeMigrateAllModel struct {
	ID             types.String `tfsdk:"id"`
	Node           types.String `tfsdk:"node"`
	Target         types.String `tfsdk:"target"`
	VMs            types.Set    `tfsdk:"vms"`
	WithLocalDisks types.Bool   `tfsdk:"with_local_disks"`
	MaxWorkers     types.Int64  `tfsdk:"max_workers"`
	Triggers       types.Map    `tfsdk:"triggers"`
	UPID           types.String `tfsdk:"upid"`
}
type aplDownloadModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	Storage  types.String `tfsdk:"storage"`
	Template types.String `tfsdk:"template"`
	Triggers types.Map    `tfsdk:"triggers"`
	UPID     types.String `tfsdk:"upid"`
}
type ociPullModel struct {
	ID        types.String `tfsdk:"id"`
	Node      types.String `tfsdk:"node"`
	Storage   types.String `tfsdk:"storage"`
	Reference types.String `tfsdk:"reference"`
	Filename  types.String `tfsdk:"filename"`
	Triggers  types.Map    `tfsdk:"triggers"`
	UPID      types.String `tfsdk:"upid"`
}

func NewNodeWakeOnLANResource() resource.Resource   { return &NodeWakeOnLANResource{} }
func NewNodeStartAllResource() resource.Resource    { return &NodeStartAllResource{} }
func NewNodeMigrateAllResource() resource.Resource  { return &NodeMigrateAllResource{} }
func NewNodeAPLDownloadResource() resource.Resource { return &NodeAPLDownloadResource{} }
func NewNodeOCIPullResource() resource.Resource     { return &NodeOCIPullResource{} }

func (r *NodeWakeOnLANResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_wakeonlan"
}
func (r *NodeStartAllResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_startall"
}
func (r *NodeMigrateAllResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_migrateall"
}
func (r *NodeAPLDownloadResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_apl_download"
}
func (r *NodeOCIPullResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_oci_pull"
}

func (r *NodeWakeOnLANResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Sends a Wake-on-LAN packet through `POST /nodes/{node}/wakeonlan`.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true},
			"node":     schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"mac":      schema.StringAttribute{Computed: true, MarkdownDescription: "MAC address that was woken."},
		},
	}
}
func (r *NodeStartAllResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Starts all guests of a node through `POST /nodes/{node}/startall` and waits for the worker.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true},
			"node":        schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"vms":         schema.SetAttribute{Optional: true, ElementType: types.Int64Type, PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()}},
			"force":       schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
			"max_workers": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
			"triggers":    schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"upid":        schema.StringAttribute{Computed: true},
		},
	}
}
func (r *NodeMigrateAllResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Migrates all guests of a node through `POST /nodes/{node}/migrateall` and waits for the worker.",
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{Computed: true},
			"node":             schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"target":           schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"vms":              schema.SetAttribute{Optional: true, ElementType: types.Int64Type, PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()}},
			"with_local_disks": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
			"max_workers":      schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
			"triggers":         schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"upid":             schema.StringAttribute{Computed: true},
		},
	}
}
func (r *NodeAPLDownloadResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Downloads an appliance template through `POST /nodes/{node}/aplinfo` and waits for the worker.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true},
			"node":     schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"storage":  schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"template": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"upid":     schema.StringAttribute{Computed: true},
		},
	}
}
func (r *NodeOCIPullResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Pulls an OCI image into a storage through `/nodes/{node}/storage/{storage}/oci-registry-pull` and waits for the worker.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true},
			"node":      schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"storage":   schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"reference": schema.StringAttribute{Required: true, MarkdownDescription: "OCI reference such as `docker.io/library/alpine:latest`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"filename":  schema.StringAttribute{Optional: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"triggers":  schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"upid":      schema.StringAttribute{Computed: true},
		},
	}
}

func (r *NodeWakeOnLANResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *NodeStartAllResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *NodeMigrateAllResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *NodeAPLDownloadResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *NodeOCIPullResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *NodeWakeOnLANResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan wakeOnLANModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mac, err := r.client.WakeOnLAN(ctx, plan.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Wake Node", err.Error())
		return
	}
	plan.ID = types.StringValue(plan.Node.ValueString())
	plan.MAC = types.StringValue(mac)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *NodeWakeOnLANResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *NodeWakeOnLANResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *NodeWakeOnLANResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func setToInt64s(value types.Set, diags *diag.Diagnostics, attr string) []int64 {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	values := make([]int64, 0, len(value.Elements()))
	for _, element := range value.Elements() {
		number, ok := element.(types.Int64)
		if !ok {
			diags.AddAttributeError(path.Root(attr), "Invalid VMID", "vms must contain only integers.")
			return nil
		}
		values = append(values, number.ValueInt64())
	}
	return values
}

func (r *NodeStartAllResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *NodeStartAllResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *NodeStartAllResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *NodeStartAllResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
func (r *NodeStartAllResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan nodeStartAllModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	node := plan.Node.ValueString()
	upid, err := r.client.StartAll(ctx, node, setToInt64s(plan.VMs, diags, "vms"), plan.Force.ValueBool(), plan.MaxWorkers.ValueInt64())
	if diags.HasError() {
		return
	}
	if err != nil {
		diags.AddError("Unable to Start All Guests", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, diags) {
		return
	}
	plan.ID = types.StringValue(node)
	plan.UPID = types.StringValue(upid)
	diags.Append(state.Set(ctx, &plan)...)
}

func (r *NodeMigrateAllResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *NodeMigrateAllResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *NodeMigrateAllResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *NodeMigrateAllResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
func (r *NodeMigrateAllResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan nodeMigrateAllModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	node, target := plan.Node.ValueString(), plan.Target.ValueString()
	upid, err := r.client.MigrateAll(ctx, node, target, setToInt64s(plan.VMs, diags, "vms"), plan.WithLocalDisks.ValueBool(), plan.MaxWorkers.ValueInt64())
	if diags.HasError() {
		return
	}
	if err != nil {
		diags.AddError("Unable to Migrate All Guests", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, diags) {
		return
	}
	plan.ID = types.StringValue(node + "->" + target)
	plan.UPID = types.StringValue(upid)
	diags.Append(state.Set(ctx, &plan)...)
}

func (r *NodeAPLDownloadResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan aplDownloadModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.DownloadAPLTemplate(ctx, plan.Node.ValueString(), plan.Storage.ValueString(), plan.Template.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Download Appliance Template", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	plan.ID = types.StringValue(plan.Node.ValueString() + "/" + plan.Template.ValueString())
	plan.UPID = types.StringValue(upid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *NodeAPLDownloadResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *NodeAPLDownloadResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *NodeAPLDownloadResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *NodeOCIPullResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ociPullModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.OCIPull(ctx, plan.Node.ValueString(), plan.Storage.ValueString(), plan.Reference.ValueString(), stringValue(plan.Filename))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Pull OCI Image", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	plan.ID = types.StringValue(plan.Node.ValueString() + "/" + plan.Reference.ValueString())
	plan.UPID = types.StringValue(upid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *NodeOCIPullResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}
func (r *NodeOCIPullResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *NodeOCIPullResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

var _ = fmt.Sprintf
var _ = strconv.Itoa
