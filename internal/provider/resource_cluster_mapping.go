// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	clusterMappingKindPCI = "pci"
	clusterMappingKindUSB = "usb"
	clusterMappingKindDir = "dir"
)

type clusterMappingModel struct {
	ID            types.String `tfsdk:"id"`
	Description   types.String `tfsdk:"description"`
	LiveMigration types.Bool   `tfsdk:"live_migration_capable"`
	Map           types.Set    `tfsdk:"map"`
}

type clusterMappingEntry struct {
	Node        string
	Path        string
	ID          string
	SubsystemID string
	IOMMUGroup  int64
	Description string
}

var _ resource.Resource = &ClusterMappingPCIResource{}
var _ resource.ResourceWithImportState = &ClusterMappingPCIResource{}
var _ resource.ResourceWithValidateConfig = &ClusterMappingPCIResource{}
var _ resource.Resource = &ClusterMappingUSBResource{}
var _ resource.ResourceWithImportState = &ClusterMappingUSBResource{}
var _ resource.ResourceWithValidateConfig = &ClusterMappingUSBResource{}
var _ resource.Resource = &ClusterMappingDirResource{}
var _ resource.ResourceWithImportState = &ClusterMappingDirResource{}
var _ resource.ResourceWithValidateConfig = &ClusterMappingDirResource{}

type ClusterMappingPCIResource struct{ client *Client }
type ClusterMappingUSBResource struct{ client *Client }
type ClusterMappingDirResource struct{ client *Client }

func NewClusterMappingPCIResource() resource.Resource { return &ClusterMappingPCIResource{} }
func NewClusterMappingUSBResource() resource.Resource { return &ClusterMappingUSBResource{} }
func NewClusterMappingDirResource() resource.Resource { return &ClusterMappingDirResource{} }

func (r *ClusterMappingPCIResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_mapping_pci"
}

func (r *ClusterMappingUSBResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_mapping_usb"
}

func (r *ClusterMappingDirResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_mapping_dir"
}

func clusterMappingSchema(kind string) schema.Schema {
	pathAttribute := schema.StringAttribute{Required: true, MarkdownDescription: clusterMappingPathDescription(kind)}
	if kind == clusterMappingKindUSB {
		pathAttribute = schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: clusterMappingPathDescription(kind)}
	}
	entryAttributes := map[string]schema.Attribute{
		"node": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster node the map entry belongs to."},
		"path": pathAttribute,
	}
	if kind == clusterMappingKindPCI {
		entryAttributes["id"] = schema.StringAttribute{Required: true, MarkdownDescription: "Expected vendor:device id (`VVVV:DDDD`), used to detect hardware changes."}
		entryAttributes["subsystem_id"] = schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Expected subsystem vendor:device id (`VVVV:DDDD`)."}
		entryAttributes["iommugroup"] = schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Expected IOMMU group, used to detect hardware changes."}
		entryAttributes["description"] = schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Node specific device description."}
	}
	if kind == clusterMappingKindUSB {
		entryAttributes["id"] = schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Expected vendor:product id (`VVVV:PPPP`); optional when a port path is mapped."}
	}
	attributes := map[string]schema.Attribute{
		"id":          schema.StringAttribute{Required: true, MarkdownDescription: "Stable mapping identifier (`pve-configid`). Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"description": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Mapping description."},
		"map": schema.SetNestedAttribute{
			Required:            true,
			MarkdownDescription: clusterMappingMapDescription(kind),
			NestedObject:        schema.NestedAttributeObject{Attributes: entryAttributes},
		},
	}
	if kind == clusterMappingKindPCI {
		attributes["live_migration_capable"] = schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Marks the devices as live-migratable (experimental, needs hardware and driver support).", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}}
	} else {
		attributes["live_migration_capable"] = schema.BoolAttribute{Computed: true, MarkdownDescription: "Always null; only PCI mappings support live migration."}
	}
	return schema.Schema{MarkdownDescription: clusterMappingResourceDescription(kind), Attributes: attributes}
}

func clusterMappingPathDescription(kind string) string {
	switch kind {
	case clusterMappingKindPCI:
		return "PCI address (`bus:dev.func`), optionally a semicolon separated list of alternatives."
	case clusterMappingKindUSB:
		return "USB port path such as `1-2.3`."
	default:
		return "Absolute directory path shared with the guest."
	}
}

func clusterMappingMapDescription(kind string) string {
	switch kind {
	case clusterMappingKindPCI:
		return "Node to PCI device mapping. Every entry needs `node`, `path` and the expected `id`; Proxmox compares them at guest start."
	case clusterMappingKindUSB:
		return "Node to USB device mapping. Every entry needs `node` and either `path` or `id`."
	default:
		return "Node to directory mapping. Every entry needs `node` and an absolute `path`."
	}
}

func clusterMappingResourceDescription(kind string) string {
	switch kind {
	case clusterMappingKindPCI:
		return "Manages a cluster-wide PCI hardware mapping through `/cluster/mapping/pci`, referenced by `hostpci.mapping`."
	case clusterMappingKindUSB:
		return "Manages a cluster-wide USB hardware mapping through `/cluster/mapping/usb`, referenced by `usb.mapping`."
	default:
		return "Manages a cluster-wide directory mapping through `/cluster/mapping/dir`, referenced by `virtiofs` directory ids."
	}
}

func (r *ClusterMappingPCIResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = clusterMappingSchema(clusterMappingKindPCI)
}

func (r *ClusterMappingUSBResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = clusterMappingSchema(clusterMappingKindUSB)
}

func (r *ClusterMappingDirResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = clusterMappingSchema(clusterMappingKindDir)
}

func (r *ClusterMappingPCIResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clusterMappingConfigure(req, resp)
}

func (r *ClusterMappingUSBResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clusterMappingConfigure(req, resp)
}

func (r *ClusterMappingDirResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clusterMappingConfigure(req, resp)
}

func clusterMappingConfigure(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *Client {
	if req.ProviderData == nil {
		return nil
	}
	client, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", err.Error())
		return nil
	}
	return client
}

func clusterMappingAttrTypes(kind string) map[string]attr.Type {
	typesMap := map[string]attr.Type{"node": types.StringType, "path": types.StringType}
	if kind == clusterMappingKindPCI {
		typesMap["id"] = types.StringType
		typesMap["subsystem_id"] = types.StringType
		typesMap["iommugroup"] = types.Int64Type
		typesMap["description"] = types.StringType
	}
	if kind == clusterMappingKindUSB {
		typesMap["id"] = types.StringType
	}
	return typesMap
}

func validateClusterMapping(kind string, config clusterMappingModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.ID) && !clusterMappingIDPattern.MatchString(config.ID.ValueString()) {
		diags.AddAttributeError(path.Root("id"), "Invalid mapping identifier", "id must start with a letter, use only letters, digits, hyphens and underscores, and be at least two characters long.")
	}
	if !config.Map.IsNull() && !config.Map.IsUnknown() {
		entries, entryDiags := expandClusterMappingEntries(kind, config.Map)
		diags.Append(entryDiags...)
		seen := map[string]bool{}
		for _, entry := range entries {
			if seen[entry.Node] {
				diags.AddAttributeError(path.Root("map"), "Duplicate mapping node", fmt.Sprintf("node %q is mapped twice.", entry.Node))
			}
			seen[entry.Node] = true
		}
		if len(entries) == 0 {
			diags.AddAttributeError(path.Root("map"), "Missing mapping entries", "map needs at least one node entry.")
		}
	}
	return diags
}

func expandClusterMappingEntries(kind string, value types.Set) ([]clusterMappingEntry, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return nil, diags
	}
	entries := make([]clusterMappingEntry, 0, len(value.Elements()))
	for _, element := range value.Elements() {
		object, ok := element.(types.Object)
		if !ok {
			diags.AddAttributeError(path.Root("map"), "Invalid mapping entry", "map entries must be objects.")
			continue
		}
		attributes := object.Attributes()
		entry := clusterMappingEntry{
			Node: clusterMappingStringValue(attributes["node"]),
			Path: clusterMappingStringValue(attributes["path"]),
		}
		if kind == clusterMappingKindPCI {
			entry.ID = clusterMappingStringValue(attributes["id"])
			entry.SubsystemID = clusterMappingStringValue(attributes["subsystem_id"])
			if group, ok := attributes["iommugroup"].(types.Int64); ok {
				entry.IOMMUGroup = group.ValueInt64()
			}
			entry.Description = clusterMappingStringValue(attributes["description"])
		}
		if kind == clusterMappingKindUSB {
			entry.ID = clusterMappingStringValue(attributes["id"])
		}
		if entry.Node == "" {
			diags.AddAttributeError(path.Root("map"), "Missing mapping field", "node must be set for every map entry.")
		}
		if entry.Path == "" && kind != clusterMappingKindUSB {
			diags.AddAttributeError(path.Root("map"), "Missing mapping field", "path must be set for every map entry.")
		}
		if entry.Node != "" && !proxmoxNodeNamePattern.MatchString(entry.Node) {
			diags.AddAttributeError(path.Root("map"), "Invalid mapping node", fmt.Sprintf("%q is not a valid node name.", entry.Node))
		}
		switch kind {
		case clusterMappingKindPCI:
			if entry.Path != "" && !clusterMappingPCIPathPattern.MatchString(entry.Path) {
				diags.AddAttributeError(path.Root("map"), "Invalid PCI path", fmt.Sprintf("%q is not a bus:dev.func address.", entry.Path))
			}
			if entry.ID != "" && !clusterMappingHIDPattern.MatchString(entry.ID) {
				diags.AddAttributeError(path.Root("map"), "Invalid PCI id", fmt.Sprintf("%q is not a VVVV:DDDD id.", entry.ID))
			}
		case clusterMappingKindUSB:
			if entry.Path != "" && !clusterMappingUSBPathPattern.MatchString(entry.Path) {
				diags.AddAttributeError(path.Root("map"), "Invalid USB path", fmt.Sprintf("%q is not a bus-port path.", entry.Path))
			}
			if entry.ID != "" && !clusterMappingHIDPattern.MatchString(entry.ID) {
				diags.AddAttributeError(path.Root("map"), "Invalid USB id", fmt.Sprintf("%q is not a VVVV:PPPP id.", entry.ID))
			}
			if entry.Path == "" && entry.ID == "" {
				diags.AddAttributeError(path.Root("map"), "Missing USB device", "map entries need a path or an id.")
			}
		case clusterMappingKindDir:
			if entry.Path != "" && !strings.HasPrefix(entry.Path, "/") {
				diags.AddAttributeError(path.Root("map"), "Invalid directory path", fmt.Sprintf("%q must be absolute.", entry.Path))
			}
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Node < entries[j].Node })
	return entries, diags
}

func clusterMappingStringValue(value attr.Value) string {
	if stringValue, ok := value.(types.String); ok && !stringValue.IsNull() && !stringValue.IsUnknown() {
		return stringValue.ValueString()
	}
	return ""
}

func clusterMappingFormEntries(entries []clusterMappingEntry) []string {
	encoded := make([]string, 0, len(entries))
	for _, entry := range entries {
		pairs := map[string]string{"node": entry.Node, "path": entry.Path}
		if entry.ID != "" {
			pairs["id"] = entry.ID
		}
		if entry.SubsystemID != "" {
			pairs["subsystem-id"] = entry.SubsystemID
		}
		if entry.IOMMUGroup > 0 {
			pairs["iommugroup"] = fmt.Sprintf("%d", entry.IOMMUGroup)
		}
		if entry.Description != "" {
			pairs["description"] = entry.Description
		}
		encoded = append(encoded, encodeClusterMappingEntry(pairs))
	}
	return encoded
}

func clusterMappingStateMap(kind string, raw []string) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	objectType := types.ObjectType{AttrTypes: clusterMappingAttrTypes(kind)}
	items := make([]attr.Value, 0, len(raw))
	for _, item := range raw {
		pairs := map[string]string{}
		for _, segment := range strings.Split(item, ",") {
			key, value, ok := strings.Cut(segment, "=")
			if ok {
				pairs[key] = value
			}
		}
		values := map[string]attr.Value{"node": types.StringValue(pairs["node"]), "path": types.StringValue(pairs["path"])}
		if kind == clusterMappingKindPCI {
			values["id"] = types.StringValue(pairs["id"])
			values["subsystem_id"] = stringOrNull(pairs["subsystem-id"])
			group := int64(0)
			if pairs["iommugroup"] != "" {
				_, _ = fmt.Sscanf(pairs["iommugroup"], "%d", &group)
			}
			values["iommugroup"] = int64OrNull(&group)
			values["description"] = stringOrNull(pairs["description"])
		}
		if kind == clusterMappingKindUSB {
			values["id"] = stringOrNull(pairs["id"])
		}
		items = append(items, types.ObjectValueMust(objectType.AttrTypes, values))
	}
	value, valueDiags := types.SetValueFrom(context.Background(), objectType, items)
	diags.Append(valueDiags...)
	return value, diags
}

func clusterMappingManagedFields(config clusterMappingModel) []string {
	fields := []string{"map"}
	if isConfiguredValue(config.Description) {
		fields = append(fields, "description")
	}
	return fields
}

func (r *ClusterMappingPCIResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	clusterMappingCreate(ctx, kindOf(r), r.client, req, resp)
}

func (r *ClusterMappingUSBResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	clusterMappingCreate(ctx, clusterMappingKindUSB, r.client, req, resp)
}

func (r *ClusterMappingDirResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	clusterMappingCreate(ctx, clusterMappingKindDir, r.client, req, resp)
}

func (r *ClusterMappingPCIResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	clusterMappingRead(ctx, clusterMappingKindPCI, r.client, req, resp)
}

func (r *ClusterMappingUSBResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	clusterMappingRead(ctx, clusterMappingKindUSB, r.client, req, resp)
}

func (r *ClusterMappingDirResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	clusterMappingRead(ctx, clusterMappingKindDir, r.client, req, resp)
}

func (r *ClusterMappingPCIResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	clusterMappingUpdate(ctx, clusterMappingKindPCI, r.client, req, resp)
}

func (r *ClusterMappingUSBResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	clusterMappingUpdate(ctx, clusterMappingKindUSB, r.client, req, resp)
}

func (r *ClusterMappingDirResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	clusterMappingUpdate(ctx, clusterMappingKindDir, r.client, req, resp)
}

func (r *ClusterMappingPCIResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	clusterMappingDelete(ctx, clusterMappingKindPCI, r.client, req, resp)
}

func (r *ClusterMappingUSBResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	clusterMappingDelete(ctx, clusterMappingKindUSB, r.client, req, resp)
}

func (r *ClusterMappingDirResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	clusterMappingDelete(ctx, clusterMappingKindDir, r.client, req, resp)
}

func (r *ClusterMappingPCIResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	clusterMappingValidate(ctx, clusterMappingKindPCI, req, resp)
}

func (r *ClusterMappingUSBResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	clusterMappingValidate(ctx, clusterMappingKindUSB, req, resp)
}

func (r *ClusterMappingDirResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	clusterMappingValidate(ctx, clusterMappingKindDir, req, resp)
}

func (r *ClusterMappingPCIResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	clusterMappingImport(ctx, req, resp)
}

func (r *ClusterMappingUSBResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	clusterMappingImport(ctx, req, resp)
}

func (r *ClusterMappingDirResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	clusterMappingImport(ctx, req, resp)
}

func kindOf(r any) string {
	switch r.(type) {
	case *ClusterMappingPCIResource:
		return clusterMappingKindPCI
	case *ClusterMappingUSBResource:
		return clusterMappingKindUSB
	default:
		return clusterMappingKindDir
	}
}

func clusterMappingCreate(ctx context.Context, kind string, client *Client, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterMappingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entries, diags := expandClusterMappingEntries(kind, plan.Map)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	entry := ClusterMapping{Description: stringValue(plan.Description), Map: clusterMappingFormEntries(entries)}
	if isConfiguredValue(plan.LiveMigration) {
		entry.LiveMigration = proxmoxOptionalBool{value: boolPointerValue(plan.LiveMigration)}
	}
	id := plan.ID.ValueString()
	if err := client.CreateClusterMapping(ctx, kind, id, entry); err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxmox Cluster Mapping", err.Error())
		return
	}
	state, stateDiags := clusterMappingReadState(ctx, kind, client, id)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	storeClusterFirewallManagedFields(ctx, resp.Private, clusterMappingManagedKey(kind), clusterMappingManagedFields(state), &resp.Diagnostics)
}

func clusterMappingReadState(ctx context.Context, kind string, client *Client, id string) (clusterMappingModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	current, err := client.GetClusterMapping(ctx, kind, id)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return clusterMappingModel{ID: types.StringNull()}, diags
		}
		diags.AddError("Unable to Read Proxmox Cluster Mapping", err.Error())
		return clusterMappingModel{}, diags
	}
	mapping, mapDiags := clusterMappingStateMap(kind, current.Map)
	diags.Append(mapDiags...)
	state := clusterMappingModel{
		ID:          types.StringValue(current.ID),
		Description: stringOrNull(current.Description),
		Map:         mapping,
	}
	if kind == clusterMappingKindPCI {
		state.LiveMigration = boolOrNull(current.LiveMigration.Ptr())
	}
	return state, diags
}

func clusterMappingRead(ctx context.Context, kind string, client *Client, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterMappingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := clusterMappingReadState(ctx, kind, client, state.ID.ValueString())
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

func clusterMappingUpdate(ctx context.Context, kind string, client *Client, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config clusterMappingModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	managed, privateDiags := readClusterFirewallManagedFields(ctx, req.Private, clusterMappingManagedKey(kind))
	resp.Diagnostics.Append(privateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := config.ID.ValueString()
	current, err := client.GetClusterMapping(ctx, kind, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Cluster Mapping", err.Error())
		return
	}
	entries, diags := expandClusterMappingEntries(kind, config.Map)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current.Map = clusterMappingFormEntries(entries)
	current.Description = clusterFirewallManagedString(current.Description, config.Description, clusterFirewallFieldManaged(managed, "description"))
	var deleted []string
	if (isConfiguredValue(config.Description) && config.Description.ValueString() == "") || (!isConfiguredValue(config.Description) && clusterFirewallFieldManaged(managed, "description")) {
		deleted = append(deleted, "description")
	}
	if kind == clusterMappingKindPCI {
		current.LiveMigration = clusterFirewallManagedBool(current.LiveMigration, config.LiveMigration, false)
	}
	if err := client.UpdateClusterMapping(ctx, kind, id, current, deleted); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxmox Cluster Mapping", err.Error())
		return
	}
	state, stateDiags := clusterMappingReadState(ctx, kind, client, id)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	storeClusterFirewallManagedFields(ctx, resp.Private, clusterMappingManagedKey(kind), clusterMappingManagedFields(state), &resp.Diagnostics)
}

func clusterMappingDelete(ctx context.Context, kind string, client *Client, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterMappingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := client.GetClusterMapping(ctx, kind, state.ID.ValueString())
	if errors.Is(err, errNotFound) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Cluster Mapping", err.Error())
		return
	}
	if err := client.DeleteClusterMapping(ctx, kind, current.ID); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Proxmox Cluster Mapping", err.Error())
	}
}

func clusterMappingValidate(ctx context.Context, kind string, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config clusterMappingModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateClusterMapping(kind, config)...)
}

func clusterMappingImport(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := strings.TrimSpace(req.ID)
	if !clusterMappingIDPattern.MatchString(id) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected a mapping identifier (pve-configid).")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func clusterMappingManagedKey(kind string) string {
	return "cluster-mapping-" + kind + "-managed-fields"
}

var (
	clusterMappingIDPattern      = regexpMustCompile(`^[A-Za-z][A-Za-z0-9_-]+$`)
	clusterMappingPCIPathPattern = regexpMustCompile(`^[0-9a-fA-F]{4,}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}(\.[0-9a-fA-F])?(;[0-9a-fA-F]{4,}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}(\.[0-9a-fA-F])?)*$`)
	clusterMappingUSBPathPattern = regexpMustCompile(`^[0-9]+-[0-9]+(\.[0-9]+)*$`)
	clusterMappingHIDPattern     = regexpMustCompile(`^[0-9A-Fa-f]{4}:[0-9A-Fa-f]{4}$`)
)

func regexpMustCompile(pattern string) *regexp.Regexp { return regexp.MustCompile(pattern) }
