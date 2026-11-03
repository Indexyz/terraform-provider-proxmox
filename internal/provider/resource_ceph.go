// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &CephFlagResource{}
var _ resource.ResourceWithImportState = &CephFlagResource{}
var _ resource.Resource = &CephMonitorResource{}
var _ resource.ResourceWithImportState = &CephMonitorResource{}
var _ resource.Resource = &CephPoolResource{}
var _ resource.ResourceWithImportState = &CephPoolResource{}
var _ resource.Resource = &CephOSDResource{}
var _ resource.ResourceWithImportState = &CephOSDResource{}

type CephFlagResource struct{ client *Client }
type CephMonitorResource struct{ client *Client }
type CephPoolResource struct{ client *Client }
type CephOSDResource struct{ client *Client }

type cephFlagModel struct {
	ID      types.String `tfsdk:"id"`
	Flag    types.String `tfsdk:"flag"`
	Enabled types.Bool   `tfsdk:"enabled"`
}

type cephMonitorModel struct {
	ID          types.String `tfsdk:"id"`
	Node        types.String `tfsdk:"node"`
	MonID       types.String `tfsdk:"monid"`
	MonAddress  types.String `tfsdk:"mon_address"`
	Quorum      types.Bool   `tfsdk:"quorum"`
	State       types.String `tfsdk:"state"`
	CephVersion types.String `tfsdk:"ceph_version"`
}

type cephPoolModel struct {
	ID              types.String  `tfsdk:"id"`
	Node            types.String  `tfsdk:"node"`
	Name            types.String  `tfsdk:"name"`
	Size            types.Int64   `tfsdk:"size"`
	MinSize         types.Int64   `tfsdk:"min_size"`
	PGNum           types.Int64   `tfsdk:"pg_num"`
	PGNumMin        types.Int64   `tfsdk:"pg_num_min"`
	PGAutoscaleMode types.String  `tfsdk:"pg_autoscale_mode"`
	CrushRuleName   types.String  `tfsdk:"crush_rule_name"`
	PercentUsed     types.Float64 `tfsdk:"percent_used"`
}

type cephOSDModel struct {
	ID               types.String `tfsdk:"id"`
	Node             types.String `tfsdk:"node"`
	Dev              types.String `tfsdk:"dev"`
	OSDsPerDevice    types.Int64  `tfsdk:"osds_per_device"`
	DBDev            types.String `tfsdk:"db_dev"`
	WALDev           types.String `tfsdk:"wal_dev"`
	DeviceClass      types.String `tfsdk:"device_class"`
	Encrypted        types.Bool   `tfsdk:"encrypted"`
	CleanupOnDestroy types.Bool   `tfsdk:"cleanup_on_destroy"`
	OSDID            types.Int64  `tfsdk:"osdid"`
}

func NewCephFlagResource() resource.Resource    { return &CephFlagResource{} }
func NewCephMonitorResource() resource.Resource { return &CephMonitorResource{} }
func NewCephPoolResource() resource.Resource    { return &CephPoolResource{} }
func NewCephOSDResource() resource.Resource     { return &CephOSDResource{} }

func (r *CephFlagResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_ceph_flag"
}

func (r *CephMonitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_monitor"
}

func (r *CephPoolResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_pool"
}

func (r *CephOSDResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_ceph_osd"
}

func (r *CephFlagResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Toggles one Ceph cluster flag through `PUT /cluster/ceph/flags/{flag}`. Destroy disables the flag again.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, MarkdownDescription: "Flag name."},
			"flag":    schema.StringAttribute{Required: true, MarkdownDescription: "Flag: `nobackfill`, `nodeep-scrub`, `nodown`, `noin`, `noout`, `norebalance`, `norecover`, `noscrub`, `notieragent`, `noup` or `pause`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"enabled": schema.BoolAttribute{Required: true, MarkdownDescription: "Whether the flag is set."},
		},
	}
}

func (r *CephMonitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Creates and destroys a Ceph monitor through `/nodes/{node}/ceph/mon/{monid}`. Import as `<node>/<monid>`.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<node>/<monid>` form."},
			"node":         schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"monid":        schema.StringAttribute{Required: true, MarkdownDescription: "Monitor id, usually the node name.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"mon_address":  schema.StringAttribute{Optional: true, MarkdownDescription: "Bind address of the monitor.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"quorum":       schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the monitor is in quorum."},
			"state":        schema.StringAttribute{Computed: true, MarkdownDescription: "Monitor state."},
			"ceph_version": schema.StringAttribute{Computed: true, MarkdownDescription: "Ceph version of the monitor."},
		},
	}
}

func (r *CephPoolResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Ceph pool through `/nodes/{node}/ceph/pool`. Import as `<node>/<name>`.",
		Attributes: map[string]schema.Attribute{
			"id":                schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<node>/<name>` form."},
			"node":              schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node used for the API call.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"name":              schema.StringAttribute{Required: true, MarkdownDescription: "Pool name.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"size":              schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(2), MarkdownDescription: "Replica count."},
			"min_size":          schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(1), MarkdownDescription: "Minimum replicas for writes."},
			"pg_num":            schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(32), MarkdownDescription: "Placement group count."},
			"pg_num_min":        schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), MarkdownDescription: "Lower autoscaler bound."},
			"pg_autoscale_mode": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Autoscale mode: `on`, `off` or `warn`."},
			"crush_rule_name":   schema.StringAttribute{Computed: true, MarkdownDescription: "CRUSH rule of the pool."},
			"percent_used":      schema.Float64Attribute{Computed: true, MarkdownDescription: "Used capacity in percent."},
		},
	}
}

func (r *CephOSDResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Creates an OSD from a device through `POST /nodes/{node}/ceph/osd` and destroys it again. Import as `<node>/<osdid>`.",
		Attributes: map[string]schema.Attribute{
			"id":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<node>/<osdid>` form."},
			"node":               schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"dev":                schema.StringAttribute{Required: true, MarkdownDescription: "Block device such as `/dev/sdb`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"osds_per_device":    schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(1), MarkdownDescription: "OSDs per device.", PlanModifiers: []planmodifier.Int64{int64planmodifierRequiresReplace()}},
			"db_dev":             schema.StringAttribute{Optional: true, MarkdownDescription: "Dedicated DB device.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"wal_dev":            schema.StringAttribute{Optional: true, MarkdownDescription: "Dedicated WAL device.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"device_class":       schema.StringAttribute{Optional: true, MarkdownDescription: "CRUSH device class.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"encrypted":          schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Encrypt the OSD.", PlanModifiers: []planmodifier.Bool{boolPlanModifierRequiresReplace()}},
			"cleanup_on_destroy": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Wipe the device on destroy."},
			"osdid":              schema.Int64Attribute{Computed: true, MarkdownDescription: "Numeric OSD id assigned by Ceph."},
		},
	}
}

func (r *CephFlagResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *CephMonitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *CephPoolResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *CephOSDResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (r *CephFlagResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *CephFlagResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *CephFlagResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cephFlagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.SetCephFlag(ctx, state.Flag.ValueString(), false); err != nil {
		resp.Diagnostics.AddError("Unable to Disable Ceph Flag", err.Error())
	}
}
func (r *CephFlagResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cephFlagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.refresh(ctx, state.Flag.ValueString(), &resp.State, &resp.Diagnostics)
}
func (r *CephFlagResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
func (r *CephFlagResource) apply(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan cephFlagModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	if err := r.client.SetCephFlag(ctx, plan.Flag.ValueString(), plan.Enabled.ValueBool()); err != nil {
		diags.AddError("Unable to Set Ceph Flag", err.Error())
		return
	}
	r.refresh(ctx, plan.Flag.ValueString(), state, diags)
}
func (r *CephFlagResource) refresh(ctx context.Context, flag string, state *tfsdk.State, diags *diag.Diagnostics) {
	flags, err := r.client.CephFlags(ctx)
	if err != nil {
		diags.AddError("Unable to Read Ceph Flags", err.Error())
		return
	}
	for _, entry := range flags {
		if entry.Name == flag {
			diags.Append(state.Set(ctx, &cephFlagModel{ID: types.StringValue(flag), Flag: types.StringValue(flag), Enabled: nodesBoolOrNull(entry.Value)})...)
			return
		}
	}
	diags.AddAttributeError(path.Root("flag"), "Unknown Ceph flag", fmt.Sprintf("%q is not reported by Ceph.", flag))
}

func (r *CephMonitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cephMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.CreateCephMonitor(ctx, plan.Node.ValueString(), plan.MonID.ValueString(), stringValue(plan.MonAddress))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Ceph Monitor", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	next := plan
	next.ID = types.StringValue(plan.Node.ValueString() + "/" + plan.MonID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
}
func (r *CephMonitorResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *CephMonitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cephMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.DeleteCephMonitor(ctx, state.Node.ValueString(), state.MonID.ValueString())
	if err != nil {
		if !isNotFoundError(err) {
			resp.Diagnostics.AddError("Unable to Delete Ceph Monitor", err.Error())
		}
		return
	}
	nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics)
}
func (r *CephMonitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cephMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	monitors, err := r.client.CephMonitors(ctx, state.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Ceph Monitors", err.Error())
		return
	}
	for _, monitor := range monitors {
		if monitor.Name == state.MonID.ValueString() {
			next := state
			next.Quorum = nodesBoolOrNull(monitor.Quorum)
			next.State = stringOrNull(monitor.State)
			next.CephVersion = stringOrNull(monitor.CephVersion)
			resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}
func (r *CephMonitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	setCephImportID(ctx, req, resp, "monid")
}

func (r *CephPoolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics, true)
}
func (r *CephPoolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics, false)
}
func (r *CephPoolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cephPoolModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.DeleteCephPool(ctx, state.Node.ValueString(), state.Name.ValueString())
	if err != nil {
		if !isNotFoundError(err) {
			resp.Diagnostics.AddError("Unable to Delete Ceph Pool", err.Error())
		}
		return
	}
	nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics)
}
func (r *CephPoolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cephPoolModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.refresh(ctx, state.Node.ValueString(), state.Name.ValueString(), &resp.State, &resp.Diagnostics)
}
func (r *CephPoolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	setCephImportID(ctx, req, resp, "name")
}
func (r *CephPoolResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics, create bool) {
	var plan cephPoolModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	node, name := plan.Node.ValueString(), plan.Name.ValueString()
	pool := CephPool{
		Name: name, Size: plan.Size.ValueInt64(), MinSize: plan.MinSize.ValueInt64(),
		PGNum: plan.PGNum.ValueInt64(), PGNumMin: plan.PGNumMin.ValueInt64(), PGAutoscaleMode: stringValue(plan.PGAutoscaleMode),
	}
	if create {
		upid, err := r.client.CreateCephPool(ctx, node, pool)
		if err != nil {
			diags.AddError("Unable to Create Ceph Pool", err.Error())
			return
		}
		if !nodeDiskWait(ctx, r.client, upid, diags) {
			return
		}
	} else if err := r.client.UpdateCephPool(ctx, node, name, pool); err != nil {
		diags.AddError("Unable to Update Ceph Pool", err.Error())
		return
	}
	r.refresh(ctx, node, name, state, diags)
}
func (r *CephPoolResource) refresh(ctx context.Context, node, name string, state *tfsdk.State, diags *diag.Diagnostics) {
	pools, err := r.client.CephPools(ctx, node)
	if err != nil {
		diags.AddError("Unable to Read Ceph Pools", err.Error())
		return
	}
	for _, pool := range pools {
		if pool.Name == name {
			next := cephPoolModel{
				ID: types.StringValue(node + "/" + name), Node: types.StringValue(node), Name: types.StringValue(name),
				Size: types.Int64Value(pool.Size), MinSize: types.Int64Value(pool.MinSize), PGNum: types.Int64Value(pool.PGNum),
				PGNumMin: types.Int64Value(pool.PGNumMin), PGAutoscaleMode: stringOrNull(pool.PGAutoscaleMode),
				CrushRuleName: stringOrNull(pool.CrushRuleName), PercentUsed: types.Float64Value(pool.PercentUsed),
			}
			diags.Append(state.Set(ctx, &next)...)
			return
		}
	}
	state.RemoveResource(ctx)
}

func (r *CephOSDResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cephOSDModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node, dev := plan.Node.ValueString(), plan.Dev.ValueString()
	upid, err := r.client.CreateCephOSD(ctx, node, dev, stringValue(plan.DBDev), stringValue(plan.WALDev), stringValue(plan.DeviceClass), plan.OSDsPerDevice.ValueInt64(), plan.Encrypted.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Ceph OSD", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	osdid, err := r.client.FindCephOSDByDevice(ctx, node, dev)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Locate Created Ceph OSD", err.Error())
		return
	}
	next := plan
	next.ID = types.StringValue(node + "/" + strconv.FormatInt(osdid, 10))
	next.OSDID = types.Int64Value(osdid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
}
func (r *CephOSDResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}
func (r *CephOSDResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cephOSDModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.MarkCephOSDOut(ctx, state.Node.ValueString(), state.OSDID.ValueInt64()); err == nil {
		if err := r.client.WaitCephOSDOut(ctx, state.Node.ValueString(), state.OSDID.ValueInt64()); err != nil {
			resp.Diagnostics.AddError("Unable to Mark Ceph OSD Out", err.Error())
			return
		}
		if err := r.client.StopCephOSDService(ctx, state.Node.ValueString(), state.OSDID.ValueInt64()); err != nil {
			resp.Diagnostics.AddError("Unable to Stop Ceph OSD", err.Error())
			return
		}
	}
	upid, err := r.client.DeleteCephOSD(ctx, state.Node.ValueString(), state.OSDID.ValueInt64(), state.CleanupOnDestroy.ValueBool())
	if err != nil {
		if !isNotFoundError(err) {
			resp.Diagnostics.AddError("Unable to Delete Ceph OSD", err.Error())
		}
		return
	}
	nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics)
}
func (r *CephOSDResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cephOSDModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	dev := state.Dev.ValueString()
	var osdid int64
	var err error
	if dev != "" {
		osdid, err = r.client.FindCephOSDByDevice(ctx, state.Node.ValueString(), dev)
		if err != nil {
			if isNotFoundError(err) {
				resp.State.RemoveResource(ctx)
				return
			}
			resp.Diagnostics.AddError("Unable to Read Ceph OSD", err.Error())
			return
		}
	} else {
		osdid = state.OSDID.ValueInt64()
		devices, deviceErr := r.client.cephOSDDevices(ctx, state.Node.ValueString(), strconv.FormatInt(osdid, 10))
		if deviceErr != nil {
			resp.State.RemoveResource(ctx)
			return
		}
		if len(devices) > 0 && devices[0].PhysicalDevice != "" {
			state.Dev = types.StringValue("/dev/" + devices[0].PhysicalDevice)
		}
	}
	state.OSDID = types.Int64Value(osdid)
	if state.Encrypted.IsNull() {
		state.Encrypted = types.BoolValue(false)
	}
	if state.OSDsPerDevice.IsNull() {
		state.OSDsPerDevice = types.Int64Value(1)
	}
	if state.CleanupOnDestroy.IsNull() {
		state.CleanupOnDestroy = types.BoolValue(true)
	}
	if state.DeviceClass.IsUnknown() {
		state.DeviceClass = types.StringNull()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
func (r *CephOSDResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	setCephImportID(ctx, req, resp, "")
}

func setCephImportID(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, second string) {
	node, rest, ok := splitCephImport(req.ID)
	if !ok {
		resp.Diagnostics.AddAttributeError(path.Root("id"), "Invalid import id", "use <node>/<id>.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	if second != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(second), rest)...)
	} else {
		value, err := strconv.ParseInt(rest, 10, 64)
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("id"), "Invalid OSD id", "the OSD id must be numeric.")
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("osdid"), value)...)
	}
}

func splitCephImport(id string) (string, string, bool) {
	for i := 0; i < len(id); i++ {
		if id[i] == '/' {
			return id[:i], id[i+1:], true
		}
	}
	return "", "", false
}

func int64planmodifierRequiresReplace() planmodifier.Int64 {
	return int64planmodifier.RequiresReplace()
}

func boolPlanModifierRequiresReplace() planmodifier.Bool {
	return boolplanmodifier.RequiresReplace()
}

func isNotFoundError(err error) bool {
	return errors.Is(err, errNotFound)
}
