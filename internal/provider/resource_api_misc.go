// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &NodeHostsResource{}
	_ resource.Resource                = &CustomCPUModelResource{}
	_ resource.ResourceWithImportState = &CustomCPUModelResource{}
	_ resource.Resource                = &HAArmedResource{}
)

type NodeHostsResource struct{ client *Client }
type CustomCPUModelResource struct{ client *Client }
type HAArmedResource struct{ client *Client }

type nodeHostsModel struct {
	ID     types.String `tfsdk:"id"`
	Node   types.String `tfsdk:"node"`
	Data   types.String `tfsdk:"data"`
	Digest types.String `tfsdk:"digest"`
}
type customCPUModelModel struct {
	ID            types.String `tfsdk:"id"`
	CPUType       types.String `tfsdk:"cputype"`
	Flags         types.String `tfsdk:"flags"`
	GuestPhysBits types.Int64  `tfsdk:"guest_phys_bits"`
	Hidden        types.Bool   `tfsdk:"hidden"`
	HVVendorID    types.String `tfsdk:"hv_vendor_id"`
	Level         types.Int64  `tfsdk:"level"`
	PhysBits      types.String `tfsdk:"phys_bits"`
	ReportedModel types.String `tfsdk:"reported_model"`
	Digest        types.String `tfsdk:"digest"`
}
type haArmedModel struct {
	ID       types.String `tfsdk:"id"`
	Triggers types.Map    `tfsdk:"triggers"`
	Armed    types.Bool   `tfsdk:"armed"`
}

func NewNodeHostsResource() resource.Resource      { return &NodeHostsResource{} }
func NewCustomCPUModelResource() resource.Resource { return &CustomCPUModelResource{} }
func NewHAArmedResource() resource.Resource        { return &HAArmedResource{} }

func (r *NodeHostsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_hosts"
}
func (r *CustomCPUModelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_cpu_model"
}
func (r *HAArmedResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ha_armed"
}

func (r *NodeHostsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Manages `/etc/hosts` of a node through `GET|POST /nodes/{node}/hosts`.", Attributes: map[string]schema.Attribute{
		"id":     schema.StringAttribute{Computed: true},
		"node":   schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"data":   schema.StringAttribute{Required: true},
		"digest": schema.StringAttribute{Computed: true},
	}}
}
func (r *CustomCPUModelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Manages a custom QEMU CPU model through `/cluster/qemu/custom-cpu-models`. Import by `cputype`.", Attributes: map[string]schema.Attribute{
		"id":              schema.StringAttribute{Computed: true},
		"cputype":         schema.StringAttribute{Required: true, MarkdownDescription: "Model name, must start with `custom-`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"flags":           schema.StringAttribute{Optional: true},
		"guest_phys_bits": schema.Int64Attribute{Optional: true},
		"hidden":          schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
		"hv_vendor_id":    schema.StringAttribute{Optional: true},
		"level":           schema.Int64Attribute{Optional: true},
		"phys_bits":       schema.StringAttribute{Optional: true},
		"reported_model":  schema.StringAttribute{Required: true},
		"digest":          schema.StringAttribute{Computed: true},
	}}
}
func (r *HAArmedResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Arms HA fencing on create (`POST /cluster/ha/status/arm-ha`) and disarms it on destroy.", Attributes: map[string]schema.Attribute{
		"id":       schema.StringAttribute{Computed: true},
		"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifierRequiresReplace()}},
		"armed":    schema.BoolAttribute{Computed: true},
	}}
}

func (r *NodeHostsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *CustomCPUModelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *HAArmedResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *NodeHostsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *NodeHostsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *NodeHostsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nodeHostsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.refresh(ctx, state.Node.ValueString(), &resp.State, &resp.Diagnostics)
}
func (r *NodeHostsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
func (r *NodeHostsResource) apply(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan nodeHostsModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	node := plan.Node.ValueString()
	if err := r.client.SetNodeHosts(ctx, node, plan.Data.ValueString(), stringValue(plan.Digest)); err != nil {
		diags.AddError("Unable to Update Node Hosts", err.Error())
		return
	}
	r.refresh(ctx, node, state, diags)
}
func (r *NodeHostsResource) refresh(ctx context.Context, node string, state *tfsdk.State, diags *diag.Diagnostics) {
	hosts, err := r.client.NodeHosts(ctx, node)
	if err != nil {
		diags.AddError("Unable to Read Node Hosts", err.Error())
		return
	}
	diags.Append(state.Set(ctx, &nodeHostsModel{ID: types.StringValue(node), Node: types.StringValue(node), Data: types.StringValue(hosts.Data), Digest: types.StringValue(hosts.Digest)})...)
}

func (r *CustomCPUModelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan customCPUModelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	model := customCPUModelFromPlan(plan)
	if err := r.client.CreateCustomCPUModel(ctx, model); err != nil {
		resp.Diagnostics.AddError("Unable to Create Custom CPU Model", err.Error())
		return
	}
	r.refresh(ctx, plan.CPUType.ValueString(), &resp.State, &resp.Diagnostics)
}
func (r *CustomCPUModelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state customCPUModelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.refresh(ctx, state.CPUType.ValueString(), &resp.State, &resp.Diagnostics)
}
func (r *CustomCPUModelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan customCPUModelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var prior customCPUModelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	model := customCPUModelFromPlan(plan)
	model.Digest = prior.Digest.ValueString()
	deleteKeys := []string{}
	if plan.Flags.IsNull() {
		deleteKeys = append(deleteKeys, "flags")
	}
	if plan.GuestPhysBits.IsNull() {
		deleteKeys = append(deleteKeys, "guest-phys-bits")
	}
	if plan.HVVendorID.IsNull() {
		deleteKeys = append(deleteKeys, "hv-vendor-id")
	}
	if plan.Level.IsNull() {
		deleteKeys = append(deleteKeys, "level")
	}
	if plan.PhysBits.IsNull() {
		deleteKeys = append(deleteKeys, "phys-bits")
	}
	if plan.ReportedModel.IsNull() {
		deleteKeys = append(deleteKeys, "reported-model")
	}
	if err := r.client.UpdateCustomCPUModel(ctx, plan.CPUType.ValueString(), model, deleteKeys); err != nil {
		resp.Diagnostics.AddError("Unable to Update Custom CPU Model", err.Error())
		return
	}
	r.refresh(ctx, plan.CPUType.ValueString(), &resp.State, &resp.Diagnostics)
}
func (r *CustomCPUModelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state customCPUModelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteCustomCPUModel(ctx, state.CPUType.ValueString()); err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Unable to Delete Custom CPU Model", err.Error())
	}
}
func (r *CustomCPUModelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cputype"), req.ID)...)
}
func (r *CustomCPUModelResource) refresh(ctx context.Context, name string, state *tfsdk.State, diags *diag.Diagnostics) {
	model, err := r.client.CustomCPUModel(ctx, name)
	if err != nil {
		if errors.Is(err, errNotFound) {
			state.RemoveResource(ctx)
			return
		}
		diags.AddError("Unable to Read Custom CPU Model", err.Error())
		return
	}
	hidden := types.BoolValue(false)
	if model.Hidden.Ptr() != nil {
		hidden = types.BoolValue(*model.Hidden.Ptr())
	}
	diags.Append(state.Set(ctx, &customCPUModelModel{
		ID: types.StringValue(name), CPUType: types.StringValue(model.CPUType), Flags: stringOrNull(model.Flags),
		GuestPhysBits: int64ValueOrNull(model.GuestPhysBits), Hidden: hidden, HVVendorID: stringOrNull(model.HVVendorID),
		Level: int64ValueOrNull(model.Level), PhysBits: stringOrNull(model.PhysBits), ReportedModel: stringOrNull(model.ReportedModel),
		Digest: stringOrNull(model.Digest),
	})...)
}
func customCPUModelFromPlan(plan customCPUModelModel) CustomCPUModel {
	model := CustomCPUModel{CPUType: plan.CPUType.ValueString(), Flags: stringValue(plan.Flags), GuestPhysBits: plan.GuestPhysBits.ValueInt64(), HVVendorID: stringValue(plan.HVVendorID), Level: plan.Level.ValueInt64(), PhysBits: stringValue(plan.PhysBits), ReportedModel: stringValue(plan.ReportedModel)}
	if !plan.Hidden.IsNull() && !plan.Hidden.IsUnknown() {
		value := plan.Hidden.ValueBool()
		model.Hidden.value = &value
	}
	return model
}

func (r *HAArmedResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan haArmedModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.do(ctx, "POST", "/cluster/ha/status/arm-ha", nil, nil, nil); err != nil {
		resp.Diagnostics.AddError("Unable to Arm HA", err.Error())
		return
	}
	plan.ID = types.StringValue("ha-armed")
	plan.Armed = types.BoolValue(true)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *HAArmedResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {}
func (r *HAArmedResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *HAArmedResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state haArmedModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.do(ctx, "POST", "/cluster/ha/status/disarm-ha", nil, nil, nil); err != nil {
		resp.Diagnostics.AddError("Unable to Disarm HA", err.Error())
	}
}

var _ = fmt.Sprintf

func mapplanmodifierRequiresReplace() planmodifier.Map {
	return mapplanmodifier.RequiresReplace()
}

func int64ValueOrNull(value int64) types.Int64 {
	if value == 0 {
		return types.Int64Null()
	}
	return types.Int64Value(value)
}
