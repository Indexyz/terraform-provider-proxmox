// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	nodeDiskKindZFS       = "zfs"
	nodeDiskKindLVM       = "lvm"
	nodeDiskKindLVMThin   = "lvmthin"
	nodeDiskKindDirectory = "directory"
)

var (
	nodeDiskNamePattern   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)
	nodeDiskDevicePattern = regexp.MustCompile(`^(/dev/[A-Za-z0-9_./-]+|/dev/disk/by-id/[A-Za-z0-9_.:-]+)$`)
	nodeZFSSpecials       = []string{"single", "mirror", "raid10", "raidz", "raidz2", "raidz3", "draid", "draid2", "draid3"}
	nodeZFSCompressions   = []string{"on", "off", "gzip", "lz4", "lzjb", "zle", "zstd"}
	nodeDirectoryFS       = []string{"ext4", "xfs"}
)

type nodeDiskZFSPoolModel struct {
	ID          types.String `tfsdk:"id"`
	Node        types.String `tfsdk:"node"`
	Name        types.String `tfsdk:"name"`
	Devices     types.Set    `tfsdk:"devices"`
	RaidLevel   types.String `tfsdk:"raid_level"`
	Ashift      types.Int64  `tfsdk:"ashift"`
	Compression types.String `tfsdk:"compression"`
	DraidConfig types.String `tfsdk:"draid_config"`
	AddStorage  types.Bool   `tfsdk:"add_storage"`
	Health      types.String `tfsdk:"health"`
}

type nodeDiskLVMModel struct {
	ID         types.String `tfsdk:"id"`
	Node       types.String `tfsdk:"node"`
	Name       types.String `tfsdk:"name"`
	Device     types.String `tfsdk:"device"`
	AddStorage types.Bool   `tfsdk:"add_storage"`
}

type nodeDiskLVMThinModel struct {
	ID         types.String `tfsdk:"id"`
	Node       types.String `tfsdk:"node"`
	Name       types.String `tfsdk:"name"`
	Device     types.String `tfsdk:"device"`
	AddStorage types.Bool   `tfsdk:"add_storage"`
}

type nodeDiskDirectoryModel struct {
	ID         types.String `tfsdk:"id"`
	Node       types.String `tfsdk:"node"`
	Name       types.String `tfsdk:"name"`
	Device     types.String `tfsdk:"device"`
	Filesystem types.String `tfsdk:"filesystem"`
	AddStorage types.Bool   `tfsdk:"add_storage"`
}

type nodeDiskCommandModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	Disk     types.String `tfsdk:"disk"`
	UUID     types.String `tfsdk:"uuid"`
	Triggers types.Map    `tfsdk:"triggers"`
	UPID     types.String `tfsdk:"upid"`
}

var _ resource.Resource = &NodeDiskZFSResource{}
var _ resource.ResourceWithImportState = &NodeDiskZFSResource{}
var _ resource.ResourceWithValidateConfig = &NodeDiskZFSResource{}
var _ resource.Resource = &NodeDiskLVMResource{}
var _ resource.ResourceWithImportState = &NodeDiskLVMResource{}
var _ resource.ResourceWithValidateConfig = &NodeDiskLVMResource{}
var _ resource.Resource = &NodeDiskLVMThinResource{}
var _ resource.ResourceWithImportState = &NodeDiskLVMThinResource{}
var _ resource.ResourceWithValidateConfig = &NodeDiskLVMThinResource{}
var _ resource.Resource = &NodeDiskDirectoryResource{}
var _ resource.ResourceWithImportState = &NodeDiskDirectoryResource{}
var _ resource.ResourceWithValidateConfig = &NodeDiskDirectoryResource{}
var _ resource.Resource = &NodeGPTInitResource{}
var _ resource.ResourceWithValidateConfig = &NodeGPTInitResource{}
var _ resource.Resource = &NodeDiskWipeResource{}
var _ resource.ResourceWithValidateConfig = &NodeDiskWipeResource{}

type NodeDiskZFSResource struct{ client *Client }
type NodeDiskLVMResource struct{ client *Client }
type NodeDiskLVMThinResource struct{ client *Client }
type NodeDiskDirectoryResource struct{ client *Client }
type NodeGPTInitResource struct{ client *Client }
type NodeDiskWipeResource struct{ client *Client }

func NewNodeDiskZFSResource() resource.Resource       { return &NodeDiskZFSResource{} }
func NewNodeDiskLVMResource() resource.Resource       { return &NodeDiskLVMResource{} }
func NewNodeDiskLVMThinResource() resource.Resource   { return &NodeDiskLVMThinResource{} }
func NewNodeDiskDirectoryResource() resource.Resource { return &NodeDiskDirectoryResource{} }
func NewNodeGPTInitResource() resource.Resource       { return &NodeGPTInitResource{} }
func NewNodeDiskWipeResource() resource.Resource      { return &NodeDiskWipeResource{} }

func (r *NodeDiskZFSResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_disk_zfs"
}
func (r *NodeDiskLVMResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_disk_lvm"
}
func (r *NodeDiskLVMThinResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_disk_lvmthin"
}
func (r *NodeDiskDirectoryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_disk_directory"
}
func (r *NodeGPTInitResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_gpt_init"
}
func (r *NodeDiskWipeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_disk_wipe"
}

func nodeDiskBaseSchema(identity string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id":   schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<node>/<name>` form."},
		"node": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node that owns the disk. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"name": schema.StringAttribute{Required: true, MarkdownDescription: identity, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
	}
}

func (r *NodeDiskZFSResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := nodeDiskBaseSchema("ZFS pool name; also the created storage id. Changes require replacement.")
	attributes["devices"] = schema.SetAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Block devices such as `/dev/disk/by-id/...`. Proxmox wipes them.", PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()}}
	attributes["raid_level"] = schema.StringAttribute{Required: true, MarkdownDescription: "Pool layout: `single`, `mirror`, `raid10`, `raidz`, `raidz2`, `raidz3`, `draid`, `draid2`, `draid3`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attributes["ashift"] = schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Sector shift, 9 to 16.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}}
	attributes["compression"] = schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "ZFS compression: `on`, `off`, `gzip`, `lz4`, `lzjb`, `zle`, `zstd`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attributes["draid_config"] = schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "dRAID configuration such as `k=2,m=1,f=0`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attributes["add_storage"] = schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Create a Proxmox storage entry for the pool. The Proxmox default is `true`.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}}
	attributes["health"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Pool health reported by Proxmox."}
	resp.Schema = schema.Schema{MarkdownDescription: "Creates a ZFS pool on one node through `/nodes/{node}/disks/zfs` and waits for the worker task. **Proxmox wipes the given devices.** Destroy removes the pool, its storage config and the disks data.", Attributes: attributes}
}

func (r *NodeDiskLVMResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := nodeDiskBaseSchema("Volume group name; also the created storage id. Changes require replacement.")
	attributes["device"] = schema.StringAttribute{Required: true, MarkdownDescription: "Block device such as `/dev/disk/by-id/...`. **Proxmox wipes it.**", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attributes["add_storage"] = schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Create a Proxmox LVM storage entry. The Proxmox default is `true`.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}}
	resp.Schema = schema.Schema{MarkdownDescription: "Creates an LVM volume group on one node through `/nodes/{node}/disks/lvm` and waits for the worker task. **Proxmox wipes the device.** Destroy removes the volume group, its storage config and the data.", Attributes: attributes}
}

func (r *NodeDiskLVMThinResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := nodeDiskBaseSchema("Thin pool name. Changes require replacement.")
	attributes["device"] = schema.StringAttribute{Required: true, MarkdownDescription: "Block device for the new volume group and thin pool. **Proxmox wipes it.**", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attributes["add_storage"] = schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Create a Proxmox LVM-thin storage entry. The Proxmox default is `true`.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}}
	resp.Schema = schema.Schema{MarkdownDescription: "Creates an LVM thin pool inside an existing volume group through `/nodes/{node}/disks/lvmthin` and waits for the worker task. Destroy removes the thin pool, its storage config and the data.", Attributes: attributes}
}

func (r *NodeDiskDirectoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := nodeDiskBaseSchema("Directory storage id. Changes require replacement.")
	attributes["device"] = schema.StringAttribute{Required: true, MarkdownDescription: "Block device that is formatted for the directory. **Proxmox wipes it.**", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attributes["filesystem"] = schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("ext4"), MarkdownDescription: "Filesystem to create: `ext4` or `xfs`. The Proxmox default is `ext4`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attributes["add_storage"] = schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Create a Proxmox directory storage entry. The Proxmox default is `true`.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}}
	resp.Schema = schema.Schema{MarkdownDescription: "Formats a block device and mounts it as directory storage through `/nodes/{node}/disks/directory`. **Proxmox wipes the device.** Destroy unmounts, deletes the storage config and removes the filesystem.", Attributes: attributes}
}

func (r *NodeGPTInitResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Initializes a GPT partition table on a node disk through `POST /nodes/{node}/disks/initgpt`. The action is imperative: every change replaces the instance and re-initializes, destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<node>/<disk>` form."},
			"node":     schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"disk":     schema.StringAttribute{Required: true, MarkdownDescription: "Block device such as `/dev/disk/by-id/...`. **Proxmox overwrites the partition table.**", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"uuid":     schema.StringAttribute{Optional: true, MarkdownDescription: "Explicit GPT disk UUID.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to initialize again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"upid":     schema.StringAttribute{Computed: true, MarkdownDescription: "Worker task UPID."},
		},
	}
}

func (r *NodeDiskWipeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Wipes all signatures and partition tables from a node disk through `PUT /nodes/{node}/disks/wipedisk`. The action is imperative: every change replaces the instance and wipes again, destroy is a no-op.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<node>/<disk>` form."},
			"node":     schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"disk":     schema.StringAttribute{Required: true, MarkdownDescription: "Block device such as `/dev/disk/by-id/...`.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"triggers": schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Values that decide when to wipe again.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}},
			"uuid":     schema.StringAttribute{Computed: true, MarkdownDescription: "Always null; wiping does not set a GPT UUID."},
			"upid":     schema.StringAttribute{Computed: true, MarkdownDescription: "Worker task UPID."},
		},
	}
}

func (r *NodeDiskZFSResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *NodeDiskLVMResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *NodeDiskLVMThinResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *NodeDiskDirectoryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *NodeGPTInitResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}
func (r *NodeDiskWipeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationConfigure(req, resp)
}

func validateNodeDiskIdentity(node, name types.String) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(node) && !proxmoxNodeNamePattern.MatchString(node.ValueString()) {
		diags.AddAttributeError(path.Root("node"), "Invalid node name", "node must be an existing cluster node name.")
	}
	if isConfiguredValue(name) && !nodeDiskNamePattern.MatchString(name.ValueString()) {
		diags.AddAttributeError(path.Root("name"), "Invalid name", "name must start with a letter and use only letters, digits, dots, hyphens and underscores.")
	}
	return diags
}

func validateNodeDiskDevice(device types.String) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(device) && !nodeDiskDevicePattern.MatchString(device.ValueString()) {
		diags.AddAttributeError(path.Root("device"), "Invalid device path", "device must be an absolute `/dev/...` path.")
	}
	return diags
}

func (r *NodeDiskZFSResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeDiskZFSPoolModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateNodeDiskIdentity(config.Node, config.Name)...)
	if isConfiguredValue(config.RaidLevel) && !slices.Contains(nodeZFSSpecials, config.RaidLevel.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("raid_level"), "Invalid raid level", "raid_level must be one of "+strings.Join(nodeZFSSpecials, ", ")+".")
	}
	if isConfiguredValue(config.Compression) && !slices.Contains(nodeZFSCompressions, config.Compression.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("compression"), "Invalid compression", "compression must be one of "+strings.Join(nodeZFSCompressions, ", ")+".")
	}
	if isConfiguredValue(config.Ashift) && (config.Ashift.ValueInt64() < 9 || config.Ashift.ValueInt64() > 16) {
		resp.Diagnostics.AddAttributeError(path.Root("ashift"), "Invalid ashift", "ashift must be between 9 and 16.")
	}
	if isConfiguredValue(config.DraidConfig) && isConfiguredValue(config.RaidLevel) && !strings.HasPrefix(config.RaidLevel.ValueString(), "draid") {
		resp.Diagnostics.AddAttributeError(path.Root("draid_config"), "Unexpected dRAID config", "draid_config is only valid for draid raid levels.")
	}
	if isConfiguredValue(config.Devices) {
		devices := stringSetValues(config.Devices)
		if len(devices) == 0 {
			resp.Diagnostics.AddAttributeError(path.Root("devices"), "Missing devices", "devices needs at least one block device.")
		}
		if isConfiguredValue(config.RaidLevel) && slices.Contains([]string{"mirror", "raid10", "raidz", "raidz2", "raidz3", "draid", "draid2", "draid3"}, config.RaidLevel.ValueString()) && len(devices) < 2 {
			resp.Diagnostics.AddAttributeError(path.Root("devices"), "Too few devices", "the selected raid level needs at least two devices.")
		}
		if isConfiguredValue(config.RaidLevel) && config.RaidLevel.ValueString() == "single" && len(devices) > 1 {
			resp.Diagnostics.AddAttributeError(path.Root("devices"), "Too many singles", "raid_level single accepts one device.")
		}
	}
}

func (r *NodeDiskLVMResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeDiskLVMModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateNodeDiskIdentity(config.Node, config.Name)...)
	resp.Diagnostics.Append(validateNodeDiskDevice(config.Device)...)
}

func (r *NodeDiskLVMThinResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeDiskLVMThinModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateNodeDiskIdentity(config.Node, config.Name)...)
	resp.Diagnostics.Append(validateNodeDiskDevice(config.Device)...)
}

func (r *NodeDiskDirectoryResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeDiskDirectoryModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateNodeDiskIdentity(config.Node, config.Name)...)
	resp.Diagnostics.Append(validateNodeDiskDevice(config.Device)...)
	if isConfiguredValue(config.Filesystem) && !slices.Contains(nodeDirectoryFS, config.Filesystem.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("filesystem"), "Invalid filesystem", "filesystem must be ext4 or xfs.")
	}
}

func validateNodeDiskCommand(node, disk types.String) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(node) && !proxmoxNodeNamePattern.MatchString(node.ValueString()) {
		diags.AddAttributeError(path.Root("node"), "Invalid node name", "node must be an existing cluster node name.")
	}
	resp := validateNodeDiskDevice(disk)
	for _, diagnostic := range resp {
		diags.AddAttributeError(path.Root("disk"), "Invalid device path", diagnostic.Detail())
	}
	return diags
}

func (r *NodeGPTInitResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeDiskCommandModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateNodeDiskCommand(config.Node, config.Disk)...)
}

func (r *NodeDiskWipeResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config nodeDiskCommandModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateNodeDiskCommand(config.Node, config.Disk)...)
}

func nodeDiskWait(ctx context.Context, client *Client, upid string, diags *diag.Diagnostics) bool {
	if upid == "" {
		return true
	}
	owner, err := taskOwnerNode(upid)
	if err != nil {
		diags.AddError("Unexpected Proxmox Task UPID", err.Error())
		return false
	}
	if err := client.waitForNodeTask(ctx, owner, upid); err != nil {
		diags.AddError("Proxmox Disk Task Failed", fmt.Sprintf("task %q did not finish: %s", upid, err))
		return false
	}
	return true
}

func (r *NodeDiskZFSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nodeDiskZFSPoolModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	params := map[string]string{}
	if isConfiguredValue(plan.Ashift) {
		params["ashift"] = fmt.Sprintf("%d", plan.Ashift.ValueInt64())
	}
	if isConfiguredValue(plan.Compression) {
		params["compression"] = plan.Compression.ValueString()
	}
	if isConfiguredValue(plan.DraidConfig) {
		params["draid-config"] = plan.DraidConfig.ValueString()
	}
	if isConfiguredValue(plan.AddStorage) {
		params["add_storage"] = boolFlag(plan.AddStorage.ValueBool())
	}
	upid, err := r.client.CreateNodeZFS(ctx, plan.Node.ValueString(), plan.Name.ValueString(), stringSetValues(plan.Devices), plan.RaidLevel.ValueString(), params)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox ZFS Pool", err.Error())
		return
	}
	state, diags := nodeDiskZFSState(ctx, r.client, plan.Node.ValueString(), plan.Name.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if plan.AddStorage.ValueBool() {
		// add_storage makes the pool visible as storage; keep it in state even
		// before the async task settles.
		state.Health = plan.Health
	}
	if !nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	refreshed, refreshDiags := nodeDiskZFSState(ctx, r.client, plan.Node.ValueString(), plan.Name.ValueString())
	resp.Diagnostics.Append(refreshDiags...)
	refreshed.Node = plan.Node
	refreshed.Name = plan.Name
	for name, value := range map[string]*types.String{"ashift": nil} {
		_ = name
		_ = value
	}
	if refreshed.ID.IsNull() {
		refreshed.ID = types.StringValue(plan.Node.ValueString() + "/" + plan.Name.ValueString())
	}
	refreshed.Devices = plan.Devices
	refreshed.RaidLevel = plan.RaidLevel
	refreshed.Ashift = plan.Ashift
	if refreshed.Ashift.IsUnknown() {
		refreshed.Ashift = types.Int64Null()
	}
	refreshed.Compression = plan.Compression
	if refreshed.Compression.IsUnknown() {
		refreshed.Compression = types.StringNull()
	}
	refreshed.DraidConfig = plan.DraidConfig
	if refreshed.DraidConfig.IsUnknown() {
		refreshed.DraidConfig = types.StringNull()
	}
	refreshed.AddStorage = plan.AddStorage
	if refreshed.AddStorage.IsUnknown() {
		refreshed.AddStorage = types.BoolValue(true)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func nodeDiskZFSState(ctx context.Context, client *Client, node, name string) (nodeDiskZFSPoolModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	pools, err := client.NodeZFSPools(ctx, node)
	if err != nil {
		diags.AddError("Unable to Read Proxmox ZFS Pools", err.Error())
		return nodeDiskZFSPoolModel{}, diags
	}
	for _, pool := range pools {
		if pool.Name == name {
			return nodeDiskZFSPoolModel{ID: types.StringValue(node + "/" + name), Node: types.StringValue(node), Name: types.StringValue(name), Devices: types.SetNull(types.StringType), RaidLevel: types.StringNull(), Ashift: types.Int64Null(), Compression: types.StringNull(), DraidConfig: types.StringNull(), AddStorage: types.BoolValue(true), Health: stringOrNull(pool.Health)}, diags
		}
	}
	return nodeDiskZFSPoolModel{ID: types.StringNull()}, diags
}

func (r *NodeDiskZFSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nodeDiskZFSPoolModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := nodeDiskZFSState(ctx, r.client, state.Node.ValueString(), state.Name.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if refreshed.ID.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}
	refreshed.Devices = state.Devices
	refreshed.RaidLevel = state.RaidLevel
	refreshed.Ashift = state.Ashift
	refreshed.Compression = state.Compression
	refreshed.DraidConfig = state.DraidConfig
	refreshed.AddStorage = state.AddStorage
	resp.Diagnostics.Append(resp.State.Set(ctx, &refreshed)...)
}

func (r *NodeDiskZFSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Every attribute replaces, so an update only refreshes state.
	var plan nodeDiskZFSPoolModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NodeDiskZFSResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nodeDiskZFSPoolModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.DeleteNodeDisk(ctx, state.Node.ValueString(), nodeDiskKindZFS, state.Name.ValueString(), "", state.AddStorage.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox ZFS Pool", err.Error())
		return
	}
	nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics)
}

func (r *NodeDiskZFSResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	nodeDiskImport(ctx, req, resp)
}

func (r *NodeDiskLVMResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nodeDiskLVMModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.CreateNodeLVM(ctx, plan.Node.ValueString(), plan.Name.ValueString(), plan.Device.ValueString(), plan.AddStorage.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox LVM Volume Group", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	state := nodeDiskLVMModel{ID: types.StringValue(plan.Node.ValueString() + "/" + plan.Name.ValueString()), Node: plan.Node, Name: plan.Name, Device: plan.Device, AddStorage: plan.AddStorage}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NodeDiskLVMResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nodeDiskLVMModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	pools, err := r.client.NodeLVMPools(ctx, state.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox LVM Volume Groups", err.Error())
		return
	}
	for _, pool := range pools {
		if pool.Name == state.Name.ValueString() {
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *NodeDiskLVMResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan nodeDiskLVMModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NodeDiskLVMResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nodeDiskLVMModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.DeleteNodeDisk(ctx, state.Node.ValueString(), nodeDiskKindLVM, state.Name.ValueString(), "", state.AddStorage.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox LVM Volume Group", err.Error())
		return
	}
	nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics)
}

func (r *NodeDiskLVMResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	nodeDiskImport(ctx, req, resp)
}

func (r *NodeDiskLVMThinResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nodeDiskLVMThinModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.CreateNodeLVMThin(ctx, plan.Node.ValueString(), plan.Name.ValueString(), plan.Device.ValueString(), plan.AddStorage.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox LVM Thin Pool", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	state := nodeDiskLVMThinModel{ID: types.StringValue(plan.Node.ValueString() + "/" + plan.Name.ValueString()), Node: plan.Node, Name: plan.Name, Device: plan.Device, AddStorage: plan.AddStorage}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NodeDiskLVMThinResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nodeDiskLVMThinModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	pools, err := r.client.NodeLVMThinPools(ctx, state.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox LVM Thin Pools", err.Error())
		return
	}
	for _, pool := range pools {
		if pool.Name == state.Name.ValueString() {
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *NodeDiskLVMThinResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan nodeDiskLVMThinModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NodeDiskLVMThinResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nodeDiskLVMThinModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.DeleteNodeDisk(ctx, state.Node.ValueString(), nodeDiskKindLVMThin, state.Name.ValueString(), state.Name.ValueString(), state.AddStorage.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox LVM Thin Pool", err.Error())
		return
	}
	nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics)
}

func (r *NodeDiskLVMThinResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	nodeDiskImport(ctx, req, resp)
}

func (r *NodeDiskDirectoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nodeDiskDirectoryModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.CreateNodeDirectory(ctx, plan.Node.ValueString(), plan.Name.ValueString(), plan.Device.ValueString(), plan.Filesystem.ValueString(), plan.AddStorage.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Directory Storage", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	state := nodeDiskDirectoryModel{ID: types.StringValue(plan.Node.ValueString() + "/" + plan.Name.ValueString()), Node: plan.Node, Name: plan.Name, Device: plan.Device, Filesystem: plan.Filesystem, AddStorage: plan.AddStorage}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NodeDiskDirectoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nodeDiskDirectoryModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entries, err := r.client.NodeDirectories(ctx, state.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Directory Storages", err.Error())
		return
	}
	for _, entry := range entries {
		if entry.Path != "" && strings.HasSuffix(entry.Path, "/"+state.Name.ValueString()) {
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *NodeDiskDirectoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan nodeDiskDirectoryModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NodeDiskDirectoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nodeDiskDirectoryModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.DeleteNodeDisk(ctx, state.Node.ValueString(), nodeDiskKindDirectory, state.Name.ValueString(), "", state.AddStorage.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Directory Storage", err.Error())
		return
	}
	nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics)
}

func (r *NodeDiskDirectoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	nodeDiskImport(ctx, req, resp)
}

func nodeDiskImport(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, name, ok := strings.Cut(req.ID, "/")
	if !ok || node == "" || name == "" {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected `<node>/<name>`.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}

func (r *NodeGPTInitResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nodeDiskCommandModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.InitNodeGPT(ctx, plan.Node.ValueString(), plan.Disk.ValueString(), stringValue(plan.UUID))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Initialize GPT", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	next := plan
	next.ID = types.StringValue(plan.Node.ValueString() + "/" + plan.Disk.ValueString())
	next.UPID = types.StringValue(upid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
}

func (r *NodeGPTInitResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
	// Imperative action; state is kept.
}

func (r *NodeGPTInitResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan nodeDiskCommandModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.InitNodeGPT(ctx, plan.Node.ValueString(), plan.Disk.ValueString(), stringValue(plan.UUID))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Initialize GPT", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	next := plan
	next.ID = types.StringValue(plan.Node.ValueString() + "/" + plan.Disk.ValueString())
	next.UPID = types.StringValue(upid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
}

func (r *NodeGPTInitResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *NodeDiskWipeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nodeDiskCommandModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.WipeNodeDisk(ctx, plan.Node.ValueString(), plan.Disk.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Wipe Disk", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	next := plan
	next.UUID = types.StringNull()
	next.ID = types.StringValue(plan.Node.ValueString() + "/" + plan.Disk.ValueString())
	next.UPID = types.StringValue(upid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
}

func (r *NodeDiskWipeResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *NodeDiskWipeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan nodeDiskCommandModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.WipeNodeDisk(ctx, plan.Node.ValueString(), plan.Disk.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Wipe Disk", err.Error())
		return
	}
	if !nodeDiskWait(ctx, r.client, upid, &resp.Diagnostics) {
		return
	}
	next := plan
	next.UUID = types.StringNull()
	next.ID = types.StringValue(plan.Node.ValueString() + "/" + plan.Disk.ValueString())
	next.UPID = types.StringValue(upid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
}

func (r *NodeDiskWipeResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func boolFlag(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

var _ = errors.Is
