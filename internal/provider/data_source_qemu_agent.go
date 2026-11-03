// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &QemuAgentDataSource{}
var _ datasource.DataSourceWithValidateConfig = &QemuAgentDataSource{}
var _ resource.Resource = &QemuAgentExecResource{}
var _ resource.ResourceWithValidateConfig = &QemuAgentExecResource{}

type QemuAgentDataSource struct{ client *Client }
type QemuAgentExecResource struct{ client *Client }

type qemuAgentModel struct {
	Node              types.String `tfsdk:"node"`
	VMID              types.Int64  `tfsdk:"vmid"`
	Commands          types.List   `tfsdk:"commands"`
	HostName          types.String `tfsdk:"host_name"`
	Timezone          types.String `tfsdk:"timezone"`
	TimezoneOffset    types.Int64  `tfsdk:"timezone_offset"`
	OSInfo            types.Object `tfsdk:"osinfo"`
	Filesystems       types.List   `tfsdk:"filesystems"`
	NetworkInterfaces types.List   `tfsdk:"network_interfaces"`
}

type qemuAgentOSInfoModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	PrettyName    types.String `tfsdk:"pretty_name"`
	Version       types.String `tfsdk:"version"`
	VersionID     types.String `tfsdk:"version_id"`
	Machine       types.String `tfsdk:"machine"`
	KernelRelease types.String `tfsdk:"kernel_release"`
	KernelVersion types.String `tfsdk:"kernel_version"`
}

type qemuAgentFilesystemModel struct {
	Name           types.String `tfsdk:"name"`
	Mountpoint     types.String `tfsdk:"mountpoint"`
	Type           types.String `tfsdk:"type"`
	TotalBytes     types.Int64  `tfsdk:"total_bytes"`
	UsedBytes      types.Int64  `tfsdk:"used_bytes"`
	TotalBytesPriv types.Int64  `tfsdk:"total_bytes_privileged"`
	UsedBytesPriv  types.Int64  `tfsdk:"used_bytes_privileged"`
	Disks          types.List   `tfsdk:"disks"`
}

type qemuAgentDiskModel struct {
	Dev     types.String `tfsdk:"dev"`
	Serial  types.String `tfsdk:"serial"`
	BusType types.String `tfsdk:"bus_type"`
	Target  types.Int64  `tfsdk:"target"`
	Unit    types.Int64  `tfsdk:"unit"`
	Bus     types.Int64  `tfsdk:"bus"`
}

type qemuAgentInterfaceModel struct {
	Name       types.String `tfsdk:"name"`
	MAC        types.String `tfsdk:"mac"`
	Addresses  types.List   `tfsdk:"addresses"`
	Statistics types.Map    `tfsdk:"statistics"`
}

type qemuAgentAddressModel struct {
	Address types.String `tfsdk:"address"`
	Type    types.String `tfsdk:"type"`
	Prefix  types.Int64  `tfsdk:"prefix"`
}

type qemuAgentExecModel struct {
	ID             types.String `tfsdk:"id"`
	Node           types.String `tfsdk:"node"`
	VMID           types.Int64  `tfsdk:"vmid"`
	Command        types.List   `tfsdk:"command"`
	InputData      types.String `tfsdk:"input_data"`
	TimeoutSeconds types.Int64  `tfsdk:"timeout_seconds"`
	Triggers       types.Map    `tfsdk:"triggers"`
	PID            types.Int64  `tfsdk:"pid"`
	ExitCode       types.Int64  `tfsdk:"exit_code"`
	Stdout         types.String `tfsdk:"stdout"`
	Stderr         types.String `tfsdk:"stderr"`
}

func NewQemuAgentDataSource() datasource.DataSource { return &QemuAgentDataSource{} }
func NewQemuAgentExecResource() resource.Resource   { return &QemuAgentExecResource{} }

func (d *QemuAgentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_agent"
}

func (r *QemuAgentExecResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_agent_exec"
}

func qemuAgentOSInfoType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"id": types.StringType, "name": types.StringType, "pretty_name": types.StringType,
		"version": types.StringType, "version_id": types.StringType, "machine": types.StringType,
		"kernel_release": types.StringType, "kernel_version": types.StringType,
	}}
}

func qemuAgentDiskType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"dev": types.StringType, "serial": types.StringType, "bus_type": types.StringType,
		"target": types.Int64Type, "unit": types.Int64Type, "bus": types.Int64Type,
	}}
}

func qemuAgentAddressType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"address": types.StringType, "type": types.StringType, "prefix": types.Int64Type,
	}}
}

func (d *QemuAgentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads guest agent facts of a running QEMU VM: supported commands, OS info, host name, timezone, filesystems and network interfaces. Requires the guest agent inside the VM.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":            datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"vmid":            datasourceschema.Int64Attribute{Required: true, MarkdownDescription: "QEMU VM ID."},
			"commands":        datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Commands the agent supports."},
			"host_name":       datasourceschema.StringAttribute{Computed: true},
			"timezone":        datasourceschema.StringAttribute{Computed: true},
			"timezone_offset": datasourceschema.Int64Attribute{Computed: true},
			"osinfo": datasourceschema.SingleNestedAttribute{Computed: true, Attributes: map[string]datasourceschema.Attribute{
				"id":             datasourceschema.StringAttribute{Computed: true},
				"name":           datasourceschema.StringAttribute{Computed: true},
				"pretty_name":    datasourceschema.StringAttribute{Computed: true},
				"version":        datasourceschema.StringAttribute{Computed: true},
				"version_id":     datasourceschema.StringAttribute{Computed: true},
				"machine":        datasourceschema.StringAttribute{Computed: true},
				"kernel_release": datasourceschema.StringAttribute{Computed: true},
				"kernel_version": datasourceschema.StringAttribute{Computed: true},
			}},
			"filesystems": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"name":                   datasourceschema.StringAttribute{Computed: true},
				"mountpoint":             datasourceschema.StringAttribute{Computed: true},
				"type":                   datasourceschema.StringAttribute{Computed: true},
				"total_bytes":            datasourceschema.Int64Attribute{Computed: true},
				"used_bytes":             datasourceschema.Int64Attribute{Computed: true},
				"total_bytes_privileged": datasourceschema.Int64Attribute{Computed: true},
				"used_bytes_privileged":  datasourceschema.Int64Attribute{Computed: true},
				"disks": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
					"dev":      datasourceschema.StringAttribute{Computed: true},
					"serial":   datasourceschema.StringAttribute{Computed: true},
					"bus_type": datasourceschema.StringAttribute{Computed: true},
					"target":   datasourceschema.Int64Attribute{Computed: true},
					"unit":     datasourceschema.Int64Attribute{Computed: true},
					"bus":      datasourceschema.Int64Attribute{Computed: true},
				}}},
			}}},
			"network_interfaces": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"name": datasourceschema.StringAttribute{Computed: true},
				"mac":  datasourceschema.StringAttribute{Computed: true},
				"addresses": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
					"address": datasourceschema.StringAttribute{Computed: true},
					"type":    datasourceschema.StringAttribute{Computed: true},
					"prefix":  datasourceschema.Int64Attribute{Computed: true},
				}}},
				"statistics": datasourceschema.MapAttribute{Computed: true, ElementType: types.Int64Type},
			}}},
		},
	}
}

func (r *QemuAgentExecResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Runs a command through the guest agent (`POST .../agent/exec`), polls the exit status and records the output. Every change replaces the instance, destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<node>/<vmid>/<pid>` form."},
			"node":            schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"vmid":            schema.Int64Attribute{Required: true, MarkdownDescription: "QEMU VM ID.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"command":         schema.ListAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Command and arguments.", PlanModifiers: []planmodifier.List{listRequiresReplace()}},
			"input_data":      schema.StringAttribute{Optional: true, MarkdownDescription: "Data passed to the command's stdin.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"timeout_seconds": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(60), MarkdownDescription: "How long to wait for exit.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"triggers":        schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to run the command again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"pid":             schema.Int64Attribute{Computed: true, MarkdownDescription: "Guest process id."},
			"exit_code":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Exit code of the command."},
			"stdout":          schema.StringAttribute{Computed: true, MarkdownDescription: "Captured standard output."},
			"stderr":          schema.StringAttribute{Computed: true, MarkdownDescription: "Captured standard error."},
		},
	}
}

func (d *QemuAgentDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (r *QemuAgentExecResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func (d *QemuAgentDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config qemuAgentModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateNodeName(config.Node, resp)
	if isConfiguredValue(config.VMID) && config.VMID.ValueInt64() < 100 {
		resp.Diagnostics.AddAttributeError(agentPathAttr("vmid"), "Invalid VMID", "vmid must be at least 100.")
	}
}

func (r *QemuAgentExecResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config qemuAgentExecModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateNodeNameResource(config.Node, resp)
	if isConfiguredValue(config.VMID) && config.VMID.ValueInt64() < 100 {
		resp.Diagnostics.AddAttributeError(agentPathAttr("vmid"), "Invalid VMID", "vmid must be at least 100.")
	}
	if isConfiguredValue(config.Command) && len(config.Command.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(agentPathAttr("command"), "Empty command", "command must contain at least the executable.")
	}
	if isConfiguredValue(config.TimeoutSeconds) && config.TimeoutSeconds.ValueInt64() < 1 {
		resp.Diagnostics.AddAttributeError(agentPathAttr("timeout_seconds"), "Invalid timeout", "timeout_seconds must be positive.")
	}
}

func (d *QemuAgentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config qemuAgentModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node, vmid := config.Node.ValueString(), config.VMID.ValueInt64()
	commands, err := d.client.AgentCommands(ctx, node, vmid)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Reach Proxmox Guest Agent", err.Error())
		return
	}
	osinfo, err := d.client.AgentOSInfo(ctx, node, vmid)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Guest OS Info", err.Error())
		return
	}
	hostName, err := d.client.AgentHostName(ctx, node, vmid)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Guest Host Name", err.Error())
		return
	}
	timezone, err := d.client.AgentTimezone(ctx, node, vmid)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Guest Timezone", err.Error())
		return
	}
	filesystems, err := d.client.AgentFilesystems(ctx, node, vmid)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Guest Filesystems", err.Error())
		return
	}
	interfaces, err := d.client.AgentInterfaces(ctx, node, vmid)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Guest Network Interfaces", err.Error())
		return
	}
	commandValues, commandDiags := types.ListValueFrom(ctx, types.StringType, commands)
	resp.Diagnostics.Append(commandDiags...)
	osinfoValue, osinfoDiags := types.ObjectValueFrom(ctx, qemuAgentOSInfoType().AttrTypes, qemuAgentOSInfoModel{
		ID: stringOrNull(osinfo.ID), Name: stringOrNull(osinfo.Name), PrettyName: stringOrNull(osinfo.PrettyName),
		Version: stringOrNull(osinfo.Version), VersionID: stringOrNull(osinfo.VersionID), Machine: stringOrNull(osinfo.Machine),
		KernelRelease: stringOrNull(osinfo.KernelRelease), KernelVersion: stringOrNull(osinfo.KernelVersion),
	})
	resp.Diagnostics.Append(osinfoDiags...)
	filesystemValues, filesystemDiags := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: qemuAgentFilesystemType().AttrTypes}, flattenAgentFilesystems(filesystems))
	resp.Diagnostics.Append(filesystemDiags...)
	interfaceValues, interfaceDiags := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: qemuAgentInterfaceType().AttrTypes}, flattenAgentInterfaces(interfaces))
	resp.Diagnostics.Append(interfaceDiags...)
	state := qemuAgentModel{
		Node: config.Node, VMID: config.VMID, Commands: commandValues, HostName: stringOrNull(hostName),
		Timezone: stringOrNull(timezone.Zone), TimezoneOffset: types.Int64Value(timezone.Offset), OSInfo: osinfoValue,
		Filesystems: filesystemValues, NetworkInterfaces: interfaceValues,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *QemuAgentExecResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *QemuAgentExecResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.run(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *QemuAgentExecResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *QemuAgentExecResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *QemuAgentExecResource) run(ctx context.Context, planGetter interface {
	Get(context.Context, any) diag.Diagnostics
}, state *tfsdk.State, diags *diag.Diagnostics) {
	var plan qemuAgentExecModel
	diags.Append(planGetter.Get(ctx, &plan)...)
	if diags.HasError() {
		return
	}
	command := make([]string, 0, len(plan.Command.Elements()))
	for _, element := range plan.Command.Elements() {
		value, ok := element.(types.String)
		if !ok {
			diags.AddAttributeError(agentPathAttr("command"), "Invalid command", "command must contain only strings.")
			return
		}
		command = append(command, value.ValueString())
	}
	node, vmid := plan.Node.ValueString(), plan.VMID.ValueInt64()
	pid, err := r.client.AgentExec(ctx, node, vmid, command, stringValue(plan.InputData))
	if err != nil {
		diags.AddError("Unable to Run Proxmox Guest Agent Command", err.Error())
		return
	}
	status, err := r.client.WaitAgentExec(ctx, node, vmid, pid, time.Duration(plan.TimeoutSeconds.ValueInt64())*time.Second)
	if err != nil {
		diags.AddError("Unable to Wait for Proxmox Guest Agent Command", err.Error())
		return
	}
	next := plan
	next.ID = types.StringValue(fmt.Sprintf("%s/%d/%d", node, vmid, pid))
	next.PID = types.Int64Value(pid)
	next.ExitCode = types.Int64Value(status.ExitCode)
	next.Stdout = stringOrNull(status.OutData)
	next.Stderr = stringOrNull(status.ErrData)
	diags.Append(state.Set(ctx, &next)...)
}

func qemuAgentFilesystemType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"name": types.StringType, "mountpoint": types.StringType, "type": types.StringType,
		"total_bytes": types.Int64Type, "used_bytes": types.Int64Type,
		"total_bytes_privileged": types.Int64Type, "used_bytes_privileged": types.Int64Type,
		"disks": types.ListType{ElemType: qemuAgentDiskType()},
	}}
}

func flattenAgentFilesystems(filesystems []AgentFilesystem) []qemuAgentFilesystemModel {
	records := make([]qemuAgentFilesystemModel, 0, len(filesystems))
	for _, filesystem := range filesystems {
		disks := make([]qemuAgentDiskModel, 0, len(filesystem.Disks))
		for _, disk := range filesystem.Disks {
			disks = append(disks, qemuAgentDiskModel{
				Dev: stringOrNull(disk.Dev), Serial: stringOrNull(disk.Serial), BusType: stringOrNull(disk.BusType),
				Target: types.Int64Value(disk.Target), Unit: types.Int64Value(disk.Unit), Bus: types.Int64Value(disk.Bus),
			})
		}
		diskValue, _ := types.ListValueFrom(context.Background(), qemuAgentDiskType(), disks)
		records = append(records, qemuAgentFilesystemModel{
			Name: stringOrNull(filesystem.Name), Mountpoint: stringOrNull(filesystem.Mountpoint), Type: stringOrNull(filesystem.Type),
			TotalBytes: types.Int64Value(filesystem.TotalBytes), UsedBytes: types.Int64Value(filesystem.UsedBytes),
			TotalBytesPriv: types.Int64Value(filesystem.TotalBytesPriv), UsedBytesPriv: types.Int64Value(filesystem.UsedBytesPriv),
			Disks: diskValue,
		})
	}
	return records
}

func qemuAgentInterfaceType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"name":       types.StringType,
		"mac":        types.StringType,
		"addresses":  types.ListType{ElemType: qemuAgentAddressType()},
		"statistics": types.MapType{ElemType: types.Int64Type},
	}}
}

func flattenAgentInterfaces(interfaces []AgentInterface) []qemuAgentInterfaceModel {
	records := make([]qemuAgentInterfaceModel, 0, len(interfaces))
	for _, iface := range interfaces {
		addresses := make([]qemuAgentAddressModel, 0, len(iface.Addresses))
		for _, address := range iface.Addresses {
			addresses = append(addresses, qemuAgentAddressModel{
				Address: stringOrNull(address.Address), Type: stringOrNull(address.Type), Prefix: types.Int64Value(address.Prefix),
			})
		}
		addressValue, _ := types.ListValueFrom(context.Background(), qemuAgentAddressType(), addresses)
		statistics, _ := types.MapValueFrom(context.Background(), types.Int64Type, iface.Statistics)
		records = append(records, qemuAgentInterfaceModel{
			Name: stringOrNull(iface.Name), MAC: stringOrNull(iface.MAC), Addresses: addressValue, Statistics: statistics,
		})
	}
	return records
}

func agentPathAttr(name string) path.Path {
	return path.Root(name)
}

func listRequiresReplace() planmodifier.List {
	return listplanmodifier.RequiresReplace()
}
