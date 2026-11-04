// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource = &NodeTasksDataSource{}
	_ datasource.DataSource = &NodeCapabilitiesDataSource{}
	_ datasource.DataSource = &NodeNetstatDataSource{}
	_ datasource.DataSource = &NodeReportDataSource{}
	_ datasource.DataSource = &APLTemplatesDataSource{}
	_ datasource.DataSource = &NodeRRDImageDataSource{}
	_ datasource.DataSource = &NodeSDNDataSource{}
	_ datasource.DataSource = &StorageIdentityDataSource{}
	_ datasource.DataSource = &StorageImportMetadataDataSource{}
	_ datasource.DataSource = &StorageRRDImageDataSource{}
	_ datasource.DataSource = &StorageRRDDataDataSource{}
	_ datasource.DataSource = &FileRestoreListDataSource{}
	_ datasource.DataSource = &FileRestoreDownloadDataSource{}
	_ datasource.DataSource = &QueryURLMetadataDataSource{}
	_ datasource.DataSource = &QueryOCITagsDataSource{}
	_ datasource.DataSource = &LXCInterfacesDataSource{}
	_ datasource.DataSource = &AccessPermissionsDataSource{}
	_ datasource.DataSource = &NotificationTargetsDataSource{}
	_ datasource.DataSource = &CloudInitDumpDataSource{}
	_ datasource.DataSource = &AgentFileReadDataSource{}
	_ datasource.DataSource = &AgentFSFreezeStatusDataSource{}
)

type NodeTasksDataSource struct{ client *Client }
type NodeCapabilitiesDataSource struct{ client *Client }
type NodeNetstatDataSource struct{ client *Client }
type NodeReportDataSource struct{ client *Client }
type APLTemplatesDataSource struct{ client *Client }
type NodeRRDImageDataSource struct{ client *Client }
type NodeSDNDataSource struct{ client *Client }
type StorageIdentityDataSource struct{ client *Client }
type StorageImportMetadataDataSource struct{ client *Client }
type StorageRRDImageDataSource struct{ client *Client }
type StorageRRDDataDataSource struct{ client *Client }
type FileRestoreListDataSource struct{ client *Client }
type FileRestoreDownloadDataSource struct{ client *Client }
type QueryURLMetadataDataSource struct{ client *Client }
type QueryOCITagsDataSource struct{ client *Client }
type LXCInterfacesDataSource struct{ client *Client }
type AccessPermissionsDataSource struct{ client *Client }
type NotificationTargetsDataSource struct{ client *Client }
type CloudInitDumpDataSource struct{ client *Client }
type AgentFileReadDataSource struct{ client *Client }
type AgentFSFreezeStatusDataSource struct{ client *Client }

func NewNodeTasksDataSource() datasource.DataSource        { return &NodeTasksDataSource{} }
func NewNodeCapabilitiesDataSource() datasource.DataSource { return &NodeCapabilitiesDataSource{} }
func NewNodeNetstatDataSource() datasource.DataSource      { return &NodeNetstatDataSource{} }
func NewNodeReportDataSource() datasource.DataSource       { return &NodeReportDataSource{} }
func NewAPLTemplatesDataSource() datasource.DataSource     { return &APLTemplatesDataSource{} }
func NewNodeRRDImageDataSource() datasource.DataSource     { return &NodeRRDImageDataSource{} }
func NewNodeSDNDataSource() datasource.DataSource          { return &NodeSDNDataSource{} }
func NewStorageIdentityDataSource() datasource.DataSource  { return &StorageIdentityDataSource{} }
func NewStorageImportMetadataDataSource() datasource.DataSource {
	return &StorageImportMetadataDataSource{}
}
func NewStorageRRDImageDataSource() datasource.DataSource { return &StorageRRDImageDataSource{} }
func NewStorageRRDDataDataSource() datasource.DataSource  { return &StorageRRDDataDataSource{} }
func NewFileRestoreListDataSource() datasource.DataSource { return &FileRestoreListDataSource{} }
func NewFileRestoreDownloadDataSource() datasource.DataSource {
	return &FileRestoreDownloadDataSource{}
}
func NewQueryURLMetadataDataSource() datasource.DataSource  { return &QueryURLMetadataDataSource{} }
func NewQueryOCITagsDataSource() datasource.DataSource      { return &QueryOCITagsDataSource{} }
func NewLXCInterfacesDataSource() datasource.DataSource     { return &LXCInterfacesDataSource{} }
func NewAccessPermissionsDataSource() datasource.DataSource { return &AccessPermissionsDataSource{} }
func NewNotificationTargetsDataSource() datasource.DataSource {
	return &NotificationTargetsDataSource{}
}
func NewCloudInitDumpDataSource() datasource.DataSource { return &CloudInitDumpDataSource{} }
func NewAgentFileReadDataSource() datasource.DataSource { return &AgentFileReadDataSource{} }
func NewAgentFSFreezeStatusDataSource() datasource.DataSource {
	return &AgentFSFreezeStatusDataSource{}
}

type nodeTasksModel struct {
	Node  types.String     `tfsdk:"node"`
	Limit types.Int64      `tfsdk:"limit"`
	Tasks []nodeTaskRecord `tfsdk:"tasks"`
}
type nodeTaskRecord struct {
	UPID      types.String `tfsdk:"upid"`
	Type      types.String `tfsdk:"type"`
	ID        types.String `tfsdk:"id"`
	Status    types.String `tfsdk:"status"`
	User      types.String `tfsdk:"user"`
	StartTime types.Int64  `tfsdk:"starttime"`
	EndTime   types.Int64  `tfsdk:"endtime"`
}
type nodeCapabilitiesModel struct {
	Node         types.String `tfsdk:"node"`
	Capabilities types.List   `tfsdk:"capabilities"`
}
type nodeNetstatModel struct {
	Node    types.String    `tfsdk:"node"`
	Entries []netstatRecord `tfsdk:"entries"`
}
type netstatRecord struct {
	Dev  types.String `tfsdk:"dev"`
	In   types.String `tfsdk:"in"`
	Out  types.String `tfsdk:"out"`
	VMID types.Int64  `tfsdk:"vmid"`
}
type nodeReportModel struct {
	Node   types.String `tfsdk:"node"`
	Report types.String `tfsdk:"report"`
}
type aplTemplatesModel struct {
	Node      types.String        `tfsdk:"node"`
	Templates []aplTemplateRecord `tfsdk:"templates"`
}
type aplTemplateRecord struct {
	Package     types.String `tfsdk:"package"`
	Template    types.String `tfsdk:"template"`
	OS          types.String `tfsdk:"os"`
	Version     types.String `tfsdk:"version"`
	Arch        types.String `tfsdk:"architecture"`
	Section     types.String `tfsdk:"section"`
	Description types.String `tfsdk:"description"`
	Headline    types.String `tfsdk:"headline"`
	Location    types.String `tfsdk:"location"`
	InfoPage    types.String `tfsdk:"infopage"`
	Maintainer  types.String `tfsdk:"maintainer"`
}
type nodeRRDImageModel struct {
	Node      types.String `tfsdk:"node"`
	Timeframe types.String `tfsdk:"timeframe"`
	CF        types.String `tfsdk:"cf"`
	DS        types.String `tfsdk:"ds"`
	Image     types.String `tfsdk:"image"`
}
type nodeSDNModel struct {
	Node     types.String `tfsdk:"node"`
	Sections types.List   `tfsdk:"sections"`
}

func (d *NodeTasksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_tasks"
}
func (d *NodeCapabilitiesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_capabilities"
}
func (d *NodeNetstatDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_netstat"
}
func (d *NodeReportDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_report"
}
func (d *APLTemplatesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_apl_templates"
}
func (d *NodeRRDImageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_rrd_image"
}
func (d *NodeSDNDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_sdn"
}

func (d *NodeTasksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists tasks of a node from `/nodes/{node}/tasks`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":  datasourceschema.StringAttribute{Required: true},
			"limit": datasourceschema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Maximum entries, default 100."},
			"tasks": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"upid":      datasourceschema.StringAttribute{Computed: true},
				"type":      datasourceschema.StringAttribute{Computed: true},
				"id":        datasourceschema.StringAttribute{Computed: true},
				"status":    datasourceschema.StringAttribute{Computed: true},
				"user":      datasourceschema.StringAttribute{Computed: true},
				"starttime": datasourceschema.Int64Attribute{Computed: true},
				"endtime":   datasourceschema.Int64Attribute{Computed: true},
			}}},
		},
	}
}

func (d *NodeCapabilitiesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists capabilities of a node from `/nodes/{node}/capabilities`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":         datasourceschema.StringAttribute{Required: true},
			"capabilities": datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *NodeNetstatDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists tap interface counters of a node from `/nodes/{node}/netstat`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node": datasourceschema.StringAttribute{Required: true},
			"entries": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"dev":  datasourceschema.StringAttribute{Computed: true},
				"in":   datasourceschema.StringAttribute{Computed: true},
				"out":  datasourceschema.StringAttribute{Computed: true},
				"vmid": datasourceschema.Int64Attribute{Computed: true},
			}}},
		},
	}
}

func (d *NodeReportDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the node diagnostic report from `/nodes/{node}/report`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":   datasourceschema.StringAttribute{Required: true},
			"report": datasourceschema.StringAttribute{Computed: true},
		},
	}
}

func (d *APLTemplatesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists appliance templates from `/nodes/{node}/aplinfo`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node": datasourceschema.StringAttribute{Required: true},
			"templates": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"package":      datasourceschema.StringAttribute{Computed: true},
				"template":     datasourceschema.StringAttribute{Computed: true},
				"os":           datasourceschema.StringAttribute{Computed: true},
				"version":      datasourceschema.StringAttribute{Computed: true},
				"architecture": datasourceschema.StringAttribute{Computed: true},
				"section":      datasourceschema.StringAttribute{Computed: true},
				"description":  datasourceschema.StringAttribute{Computed: true},
				"headline":     datasourceschema.StringAttribute{Computed: true},
				"location":     datasourceschema.StringAttribute{Computed: true},
				"infopage":     datasourceschema.StringAttribute{Computed: true},
				"maintainer":   datasourceschema.StringAttribute{Computed: true},
			}}},
		},
	}
}

func (d *NodeRRDImageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads a node RRD graph PNG as base64 from `/nodes/{node}/rrd`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":      datasourceschema.StringAttribute{Required: true},
			"timeframe": datasourceschema.StringAttribute{Required: true},
			"cf":        datasourceschema.StringAttribute{Required: true},
			"ds":        datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Data source, e.g. `cpu`."},
			"image":     datasourceschema.StringAttribute{Computed: true},
		},
	}
}

func (d *NodeSDNDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists SDN sections available on a node from `/nodes/{node}/sdn`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":     datasourceschema.StringAttribute{Required: true},
			"sections": datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

type storageIdentityModel struct {
	Node     types.String `tfsdk:"node"`
	Storage  types.String `tfsdk:"storage"`
	Identity types.String `tfsdk:"identity"`
}
type storageImportMetadataModel struct {
	Node     types.String `tfsdk:"node"`
	Storage  types.String `tfsdk:"storage"`
	Volume   types.String `tfsdk:"volume"`
	Metadata types.String `tfsdk:"metadata"`
}
type storageRRDImageModel struct {
	Node      types.String `tfsdk:"node"`
	Storage   types.String `tfsdk:"storage"`
	Timeframe types.String `tfsdk:"timeframe"`
	CF        types.String `tfsdk:"cf"`
	DS        types.String `tfsdk:"ds"`
	Image     types.String `tfsdk:"image"`
}
type storageRRDDataModel struct {
	Node      types.String `tfsdk:"node"`
	Storage   types.String `tfsdk:"storage"`
	Timeframe types.String `tfsdk:"timeframe"`
	CF        types.String `tfsdk:"cf"`
	Data      types.String `tfsdk:"data"`
}
type fileRestoreListModel struct {
	Node     types.String        `tfsdk:"node"`
	Storage  types.String        `tfsdk:"storage"`
	Volume   types.String        `tfsdk:"volume"`
	Filepath types.String        `tfsdk:"filepath"`
	Entries  []fileRestoreRecord `tfsdk:"entries"`
}
type fileRestoreRecord struct {
	Filepath types.String `tfsdk:"filepath"`
	Type     types.String `tfsdk:"type"`
	Size     types.Int64  `tfsdk:"size"`
	Mtime    types.Int64  `tfsdk:"mtime"`
	Leaf     types.Bool   `tfsdk:"leaf"`
	Text     types.String `tfsdk:"text"`
}
type fileRestoreDownloadModel struct {
	Node     types.String `tfsdk:"node"`
	Storage  types.String `tfsdk:"storage"`
	Volume   types.String `tfsdk:"volume"`
	Filepath types.String `tfsdk:"filepath"`
	Tar      types.Bool   `tfsdk:"tar"`
	Content  types.String `tfsdk:"content"`
}
type queryURLMetadataModel struct {
	Node     types.String `tfsdk:"node"`
	URL      types.String `tfsdk:"url"`
	Verify   types.Bool   `tfsdk:"verify_certificates"`
	Filename types.String `tfsdk:"filename"`
	Size     types.Int64  `tfsdk:"size"`
	Mimetype types.String `tfsdk:"mimetype"`
}
type queryOCITagsModel struct {
	Node      types.String `tfsdk:"node"`
	Reference types.String `tfsdk:"reference"`
	Tags      types.List   `tfsdk:"tags"`
}
type lxcInterfacesModel struct {
	Node       types.String         `tfsdk:"node"`
	VMID       types.Int64          `tfsdk:"vmid"`
	Interfaces []lxcInterfaceRecord `tfsdk:"interfaces"`
}
type lxcInterfaceRecord struct {
	Name  types.String `tfsdk:"name"`
	MAC   types.String `tfsdk:"mac"`
	Inet  types.String `tfsdk:"inet"`
	Inet6 types.String `tfsdk:"inet6"`
}
type accessPermissionsModel struct {
	Path        types.String       `tfsdk:"path"`
	UserID      types.String       `tfsdk:"userid"`
	Permissions []permissionRecord `tfsdk:"permissions"`
}
type permissionRecord struct {
	Path        types.String `tfsdk:"path"`
	Permissions types.Map    `tfsdk:"permissions"`
}
type notificationTargetsModel struct {
	Targets []notificationTargetRecord `tfsdk:"targets"`
}
type notificationTargetRecord struct {
	Name    types.String `tfsdk:"name"`
	Type    types.String `tfsdk:"type"`
	Comment types.String `tfsdk:"comment"`
	Origin  types.String `tfsdk:"origin"`
	Disable types.Bool   `tfsdk:"disable"`
}
type cloudInitDumpModel struct {
	Node    types.String `tfsdk:"node"`
	VMID    types.Int64  `tfsdk:"vmid"`
	Type    types.String `tfsdk:"type"`
	Content types.String `tfsdk:"content"`
}
type agentFileReadModel struct {
	Node      types.String `tfsdk:"node"`
	VMID      types.Int64  `tfsdk:"vmid"`
	File      types.String `tfsdk:"file"`
	Count     types.Int64  `tfsdk:"read_count"`
	Offset    types.Int64  `tfsdk:"offset"`
	Decode    types.Bool   `tfsdk:"decode"`
	Content   types.String `tfsdk:"content"`
	Truncated types.Bool   `tfsdk:"truncated"`
}
type agentFSFreezeStatusModel struct {
	Node   types.String `tfsdk:"node"`
	VMID   types.Int64  `tfsdk:"vmid"`
	Frozen types.Bool   `tfsdk:"frozen"`
}

func (d *StorageIdentityDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage_identity"
}
func (d *StorageImportMetadataDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage_import_metadata"
}
func (d *StorageRRDImageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage_rrd_image"
}
func (d *StorageRRDDataDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage_rrddata"
}
func (d *FileRestoreListDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage_file_restore_list"
}
func (d *FileRestoreDownloadDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage_file_restore_download"
}
func (d *QueryURLMetadataDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_query_url_metadata"
}
func (d *QueryOCITagsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_query_oci_repo_tags"
}
func (d *LXCInterfacesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lxc_interfaces"
}
func (d *AccessPermissionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_permissions"
}
func (d *NotificationTargetsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_targets"
}
func (d *CloudInitDumpDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_cloudinit_dump"
}
func (d *AgentFileReadDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_agent_file"
}
func (d *AgentFSFreezeStatusDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_agent_fsfreeze_status"
}

func (d *StorageIdentityDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the storage identity JSON from `/nodes/{node}/storage/{storage}/identity`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":     datasourceschema.StringAttribute{Required: true},
			"storage":  datasourceschema.StringAttribute{Required: true},
			"identity": datasourceschema.StringAttribute{Computed: true},
		},
	}
}
func (d *StorageImportMetadataDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads import metadata of a volume from `/nodes/{node}/storage/{storage}/import-metadata`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":     datasourceschema.StringAttribute{Required: true},
			"storage":  datasourceschema.StringAttribute{Required: true},
			"volume":   datasourceschema.StringAttribute{Required: true},
			"metadata": datasourceschema.StringAttribute{Computed: true},
		},
	}
}
func (d *StorageRRDImageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads a storage RRD graph PNG as base64 from `/nodes/{node}/storage/{storage}/rrd`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":      datasourceschema.StringAttribute{Required: true},
			"storage":   datasourceschema.StringAttribute{Required: true},
			"timeframe": datasourceschema.StringAttribute{Required: true},
			"cf":        datasourceschema.StringAttribute{Required: true},
			"ds":        datasourceschema.StringAttribute{Required: true},
			"image":     datasourceschema.StringAttribute{Computed: true},
		},
	}
}
func (d *StorageRRDDataDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads raw storage RRD points from `/nodes/{node}/storage/{storage}/rrddata`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":      datasourceschema.StringAttribute{Required: true},
			"storage":   datasourceschema.StringAttribute{Required: true},
			"timeframe": datasourceschema.StringAttribute{Required: true},
			"cf":        datasourceschema.StringAttribute{Optional: true},
			"data":      datasourceschema.StringAttribute{Computed: true},
		},
	}
}
func (d *FileRestoreListDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists files in a file-restore snapshot from `/nodes/{node}/storage/{storage}/file-restore/list`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":     datasourceschema.StringAttribute{Required: true},
			"storage":  datasourceschema.StringAttribute{Required: true},
			"volume":   datasourceschema.StringAttribute{Required: true},
			"filepath": datasourceschema.StringAttribute{Optional: true},
			"entries": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"filepath": datasourceschema.StringAttribute{Computed: true},
				"type":     datasourceschema.StringAttribute{Computed: true},
				"size":     datasourceschema.Int64Attribute{Computed: true},
				"mtime":    datasourceschema.Int64Attribute{Computed: true},
				"leaf":     datasourceschema.BoolAttribute{Computed: true},
				"text":     datasourceschema.StringAttribute{Computed: true},
			}}},
		},
	}
}
func (d *FileRestoreDownloadDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Downloads a file from a file-restore snapshot through `/nodes/{node}/storage/{storage}/file-restore/download`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":     datasourceschema.StringAttribute{Required: true},
			"storage":  datasourceschema.StringAttribute{Required: true},
			"volume":   datasourceschema.StringAttribute{Required: true},
			"filepath": datasourceschema.StringAttribute{Required: true},
			"tar":      datasourceschema.BoolAttribute{Optional: true},
			"content":  datasourceschema.StringAttribute{Computed: true},
		},
	}
}
func (d *QueryURLMetadataDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Queries metadata of a download URL through `/nodes/{node}/query-url-metadata`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":                datasourceschema.StringAttribute{Required: true},
			"url":                 datasourceschema.StringAttribute{Required: true},
			"verify_certificates": datasourceschema.BoolAttribute{Optional: true},
			"filename":            datasourceschema.StringAttribute{Computed: true},
			"size":                datasourceschema.Int64Attribute{Computed: true},
			"mimetype":            datasourceschema.StringAttribute{Computed: true},
		},
	}
}
func (d *QueryOCITagsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists tags of an OCI repository through `/nodes/{node}/query-oci-repo-tags`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":      datasourceschema.StringAttribute{Required: true},
			"reference": datasourceschema.StringAttribute{Required: true},
			"tags":      datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}
func (d *LXCInterfacesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists interfaces of a running container from `/nodes/{node}/lxc/{vmid}/interfaces`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node": datasourceschema.StringAttribute{Required: true},
			"vmid": datasourceschema.Int64Attribute{Required: true},
			"interfaces": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"name":  datasourceschema.StringAttribute{Computed: true},
				"mac":   datasourceschema.StringAttribute{Computed: true},
				"inet":  datasourceschema.StringAttribute{Computed: true},
				"inet6": datasourceschema.StringAttribute{Computed: true},
			}}},
		},
	}
}
func (d *AccessPermissionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads effective permissions from `/access/permissions`.",
		Attributes: map[string]datasourceschema.Attribute{
			"path":   datasourceschema.StringAttribute{Required: true},
			"userid": datasourceschema.StringAttribute{Optional: true},
			"permissions": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"path":        datasourceschema.StringAttribute{Computed: true},
				"permissions": datasourceschema.MapAttribute{Computed: true, ElementType: types.BoolType},
			}}},
		},
	}
}
func (d *NotificationTargetsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists notification targets from `/cluster/notifications/targets`.",
		Attributes: map[string]datasourceschema.Attribute{
			"targets": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"name":    datasourceschema.StringAttribute{Computed: true},
				"type":    datasourceschema.StringAttribute{Computed: true},
				"comment": datasourceschema.StringAttribute{Computed: true},
				"origin":  datasourceschema.StringAttribute{Computed: true},
				"disable": datasourceschema.BoolAttribute{Computed: true},
			}}},
		},
	}
}
func (d *CloudInitDumpDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Dumps generated cloud-init data through `/nodes/{node}/qemu/{vmid}/cloudinit/dump`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":    datasourceschema.StringAttribute{Required: true},
			"vmid":    datasourceschema.Int64Attribute{Required: true},
			"type":    datasourceschema.StringAttribute{Required: true, MarkdownDescription: "`user`, `network` or `meta`."},
			"content": datasourceschema.StringAttribute{Computed: true},
		},
	}
}
func (d *AgentFileReadDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads a guest file through the agent at `/nodes/{node}/qemu/{vmid}/agent/file-read`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":       datasourceschema.StringAttribute{Required: true},
			"vmid":       datasourceschema.Int64Attribute{Required: true},
			"file":       datasourceschema.StringAttribute{Required: true},
			"read_count": datasourceschema.Int64Attribute{Optional: true},
			"offset":     datasourceschema.Int64Attribute{Optional: true},
			"decode":     datasourceschema.BoolAttribute{Optional: true, MarkdownDescription: "Base64-decode the content."},
			"content":    datasourceschema.StringAttribute{Computed: true},
			"truncated":  datasourceschema.BoolAttribute{Computed: true},
		},
	}
}
func (d *AgentFSFreezeStatusDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the guest filesystem freeze state through `/nodes/{node}/qemu/{vmid}/agent/fsfreeze-status`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":   datasourceschema.StringAttribute{Required: true},
			"vmid":   datasourceschema.Int64Attribute{Required: true},
			"frozen": datasourceschema.BoolAttribute{Computed: true},
		},
	}
}

func (d *NodeTasksDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *NodeCapabilitiesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *NodeNetstatDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *NodeReportDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *APLTemplatesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *NodeRRDImageDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *NodeSDNDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *StorageIdentityDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *StorageImportMetadataDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *StorageRRDImageDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *StorageRRDDataDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *FileRestoreListDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *FileRestoreDownloadDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *QueryURLMetadataDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *QueryOCITagsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *LXCInterfacesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *AccessPermissionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *NotificationTargetsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *CloudInitDumpDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *AgentFileReadDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *AgentFSFreezeStatusDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *NodeTasksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeTasksModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	limit := int64(100)
	if !config.Limit.IsNull() && !config.Limit.IsUnknown() {
		limit = config.Limit.ValueInt64()
	}
	tasks, err := d.client.NodeTasks(ctx, config.Node.ValueString(), limit)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Node Tasks", err.Error())
		return
	}
	records := make([]nodeTaskRecord, 0, len(tasks))
	for _, task := range tasks {
		records = append(records, nodeTaskRecord{UPID: types.StringValue(task.UPID), Type: stringOrNull(task.Type), ID: stringOrNull(task.ID), Status: stringOrNull(task.Status), User: stringOrNull(task.User), StartTime: types.Int64Value(task.StartTime), EndTime: types.Int64Value(task.EndTime)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeTasksModel{Node: config.Node, Limit: types.Int64Value(limit), Tasks: records})...)
}

func (d *NodeCapabilitiesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeCapabilitiesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	capabilities, err := d.client.NodeCapabilities(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Node Capabilities", err.Error())
		return
	}
	value, diags := types.ListValueFrom(ctx, types.StringType, capabilities)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeCapabilitiesModel{Node: config.Node, Capabilities: value})...)
}

func (d *NodeNetstatDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeNetstatModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entries, err := d.client.NodeNetstat(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Node Netstat", err.Error())
		return
	}
	records := make([]netstatRecord, 0, len(entries))
	for _, entry := range entries {
		vmid, _ := strconv.ParseInt(string(entry.VMID), 10, 64)
		records = append(records, netstatRecord{Dev: stringOrNull(entry.Dev), In: stringOrNull(entry.In), Out: stringOrNull(entry.Out), VMID: types.Int64Value(vmid)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeNetstatModel{Node: config.Node, Entries: records})...)
}

func (d *NodeReportDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeReportModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	report, err := d.client.NodeReport(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Node Report", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeReportModel{Node: config.Node, Report: types.StringValue(report)})...)
}

func (d *APLTemplatesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config aplTemplatesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	templates, err := d.client.APLTemplates(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read APL Templates", err.Error())
		return
	}
	records := make([]aplTemplateRecord, 0, len(templates))
	for _, template := range templates {
		records = append(records, aplTemplateRecord{Package: stringOrNull(template.Package), Template: stringOrNull(template.Template), OS: stringOrNull(template.OS), Version: stringOrNull(template.Version), Arch: stringOrNull(template.Arch), Section: stringOrNull(template.Section), Description: stringOrNull(template.Description), Headline: stringOrNull(template.Headline), Location: stringOrNull(template.Location), InfoPage: stringOrNull(template.InfoPage), Maintainer: stringOrNull(template.Maintainer)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &aplTemplatesModel{Node: config.Node, Templates: records})...)
}

func (d *NodeRRDImageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeRRDImageModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	image, err := d.client.NodeRRDImage(ctx, config.Node.ValueString(), config.Timeframe.ValueString(), config.CF.ValueString(), config.DS.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Node RRD Image", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeRRDImageModel{Node: config.Node, Timeframe: config.Timeframe, CF: config.CF, DS: config.DS, Image: types.StringValue(image)})...)
}

func (d *NodeSDNDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeSDNModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sections, err := d.client.NodeSDN(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Node SDN", err.Error())
		return
	}
	value, diags := types.ListValueFrom(ctx, types.StringType, sections)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeSDNModel{Node: config.Node, Sections: value})...)
}

func (d *StorageIdentityDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config storageIdentityModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	identity, err := d.client.StorageIdentity(ctx, config.Node.ValueString(), config.Storage.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Storage Identity", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &storageIdentityModel{Node: config.Node, Storage: config.Storage, Identity: types.StringValue(identity)})...)
}

func (d *StorageImportMetadataDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config storageImportMetadataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	metadata, err := d.client.StorageImportMetadata(ctx, config.Node.ValueString(), config.Storage.ValueString(), config.Volume.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Storage Import Metadata", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &storageImportMetadataModel{Node: config.Node, Storage: config.Storage, Volume: config.Volume, Metadata: types.StringValue(metadata)})...)
}

func (d *StorageRRDImageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config storageRRDImageModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	image, err := d.client.StorageRRDImage(ctx, config.Node.ValueString(), config.Storage.ValueString(), config.Timeframe.ValueString(), config.CF.ValueString(), config.DS.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Storage RRD Image", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &storageRRDImageModel{Node: config.Node, Storage: config.Storage, Timeframe: config.Timeframe, CF: config.CF, DS: config.DS, Image: types.StringValue(image)})...)
}

func (d *StorageRRDDataDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config storageRRDDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data, err := d.client.StorageRRDData(ctx, config.Node.ValueString(), config.Storage.ValueString(), config.Timeframe.ValueString(), stringValue(config.CF))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Storage RRD Data", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &storageRRDDataModel{Node: config.Node, Storage: config.Storage, Timeframe: config.Timeframe, CF: config.CF, Data: types.StringValue(data)})...)
}

func (d *FileRestoreListDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config fileRestoreListModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entries, err := d.client.FileRestoreList(ctx, config.Node.ValueString(), config.Storage.ValueString(), config.Volume.ValueString(), stringValue(config.Filepath))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read File Restore List", err.Error())
		return
	}
	records := make([]fileRestoreRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, fileRestoreRecord{Filepath: stringOrNull(entry.Filepath), Type: stringOrNull(entry.Type), Size: types.Int64Value(entry.Size), Mtime: types.Int64Value(entry.Mtime), Leaf: nodesBoolOrNull(entry.Leaf), Text: stringOrNull(entry.Text)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &fileRestoreListModel{Node: config.Node, Storage: config.Storage, Volume: config.Volume, Filepath: config.Filepath, Entries: records})...)
}

func (d *FileRestoreDownloadDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config fileRestoreDownloadModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	content, err := d.client.FileRestoreDownload(ctx, config.Node.ValueString(), config.Storage.ValueString(), config.Volume.ValueString(), config.Filepath.ValueString(), config.Tar.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Download File Restore File", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &fileRestoreDownloadModel{Node: config.Node, Storage: config.Storage, Volume: config.Volume, Filepath: config.Filepath, Tar: config.Tar, Content: types.StringValue(content)})...)
}

func (d *QueryURLMetadataDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config queryURLMetadataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	verify := true
	if !config.Verify.IsNull() && !config.Verify.IsUnknown() {
		verify = config.Verify.ValueBool()
	}
	metadata, err := d.client.QueryURLMetadata(ctx, config.Node.ValueString(), config.URL.ValueString(), verify)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Query URL Metadata", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &queryURLMetadataModel{Node: config.Node, URL: config.URL, Verify: types.BoolValue(verify), Filename: stringOrNull(metadata.Filename), Size: types.Int64Value(metadata.Size), Mimetype: stringOrNull(metadata.Mimetype)})...)
}

func (d *QueryOCITagsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config queryOCITagsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tags, err := d.client.QueryOCIRepoTags(ctx, config.Node.ValueString(), config.Reference.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Query OCI Tags", err.Error())
		return
	}
	value, diags := types.ListValueFrom(ctx, types.StringType, tags)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &queryOCITagsModel{Node: config.Node, Reference: config.Reference, Tags: value})...)
}

func (d *LXCInterfacesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config lxcInterfacesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	interfaces, err := d.client.LXCInterfaces(ctx, config.Node.ValueString(), config.VMID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read LXC Interfaces", err.Error())
		return
	}
	records := make([]lxcInterfaceRecord, 0, len(interfaces))
	for _, iface := range interfaces {
		records = append(records, lxcInterfaceRecord{Name: stringOrNull(iface.Name), MAC: stringOrNull(iface.MAC), Inet: stringOrNull(iface.Inet), Inet6: stringOrNull(iface.Inet6)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &lxcInterfacesModel{Node: config.Node, VMID: config.VMID, Interfaces: records})...)
}

func (d *AccessPermissionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config accessPermissionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entries, err := d.client.AccessPermissions(ctx, config.Path.ValueString(), stringValue(config.UserID))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Access Permissions", err.Error())
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	records := make([]permissionRecord, 0, len(entries))
	for _, entry := range entries {
		value, diags := types.MapValueFrom(ctx, types.BoolType, entry.Permissions)
		resp.Diagnostics.Append(diags...)
		records = append(records, permissionRecord{Path: types.StringValue(entry.Path), Permissions: value})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &accessPermissionsModel{Path: config.Path, UserID: config.UserID, Permissions: records})...)
}

func (d *NotificationTargetsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config notificationTargetsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	targets, err := d.client.NotificationTargets(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Notification Targets", err.Error())
		return
	}
	records := make([]notificationTargetRecord, 0, len(targets))
	for _, target := range targets {
		records = append(records, notificationTargetRecord{Name: stringOrNull(target.Name), Type: stringOrNull(target.Type), Comment: stringOrNull(target.Comment), Origin: stringOrNull(target.Origin), Disable: nodesBoolOrNull(target.Disable)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &notificationTargetsModel{Targets: records})...)
}

func (d *CloudInitDumpDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config cloudInitDumpModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	content, err := d.client.CloudInitDump(ctx, config.Node.ValueString(), config.VMID.ValueInt64(), config.Type.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Dump Cloud-Init Data", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cloudInitDumpModel{Node: config.Node, VMID: config.VMID, Type: config.Type, Content: types.StringValue(content)})...)
}

func (d *AgentFileReadDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config agentFileReadModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := d.client.AgentFileRead(ctx, config.Node.ValueString(), config.VMID.ValueInt64(), config.File.ValueString(), config.Count.ValueInt64(), config.Offset.ValueInt64(), config.Decode.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Guest File", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &agentFileReadModel{Node: config.Node, VMID: config.VMID, File: config.File, Count: config.Count, Offset: config.Offset, Decode: config.Decode, Content: types.StringValue(result.Content), Truncated: nodesBoolOrNull(result.Truncated)})...)
}

func (d *AgentFSFreezeStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config agentFSFreezeStatusModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	raw, err := d.client.AgentCommand(ctx, config.Node.ValueString(), config.VMID.ValueInt64(), "fsfreeze-status")
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Guest Freeze Status", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &agentFSFreezeStatusModel{Node: config.Node, VMID: config.VMID, Frozen: types.BoolValue(raw == "frozen" || raw == "true" || raw == "1")})...)
}

type proxyModel struct {
	Node        types.String `tfsdk:"node"`
	Command     types.String `tfsdk:"command"`
	CommandOpts types.String `tfsdk:"command_opts"`
	Proxy       types.String `tfsdk:"proxy"`
	Port        types.String `tfsdk:"port"`
	Ticket      types.String `tfsdk:"ticket"`
	User        types.String `tfsdk:"user"`
	UPID        types.String `tfsdk:"upid"`
	Result      types.String `tfsdk:"result"`
}
type qemuVNCProxyModel struct {
	Node             types.String `tfsdk:"node"`
	VMID             types.Int64  `tfsdk:"vmid"`
	GeneratePassword types.Bool   `tfsdk:"generate_password"`
	Websocket        types.Bool   `tfsdk:"websocket"`
	Result           types.String `tfsdk:"result"`
}
type vncTicketModel struct {
	UserID    types.String `tfsdk:"userid"`
	Path      types.String `tfsdk:"path"`
	VNCTicket types.String `tfsdk:"vncticket"`
	Privs     types.String `tfsdk:"privs"`
	Port      types.String `tfsdk:"port"`
	Ticket    types.String `tfsdk:"ticket"`
	User      types.String `tfsdk:"user"`
	UPID      types.String `tfsdk:"upid"`
}

type NodeTermProxyDataSource struct{ client *Client }
type NodeVNCShellDataSource struct{ client *Client }
type NodeSpiceShellDataSource struct{ client *Client }
type QemuVNCProxyDataSource struct{ client *Client }
type AccessVNCTicketDataSource struct{ client *Client }

var (
	_ datasource.DataSource = &NodeTermProxyDataSource{}
	_ datasource.DataSource = &NodeVNCShellDataSource{}
	_ datasource.DataSource = &NodeSpiceShellDataSource{}
	_ datasource.DataSource = &QemuVNCProxyDataSource{}
	_ datasource.DataSource = &AccessVNCTicketDataSource{}
)

func NewNodeTermProxyDataSource() datasource.DataSource   { return &NodeTermProxyDataSource{} }
func NewNodeVNCShellDataSource() datasource.DataSource    { return &NodeVNCShellDataSource{} }
func NewNodeSpiceShellDataSource() datasource.DataSource  { return &NodeSpiceShellDataSource{} }
func NewQemuVNCProxyDataSource() datasource.DataSource    { return &QemuVNCProxyDataSource{} }
func NewAccessVNCTicketDataSource() datasource.DataSource { return &AccessVNCTicketDataSource{} }

func (d *NodeTermProxyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_termproxy"
}
func (d *NodeVNCShellDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_vncshell"
}
func (d *NodeSpiceShellDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_spiceshell"
}
func (d *QemuVNCProxyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_vncproxy"
}
func (d *AccessVNCTicketDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_vncticket"
}

func (d *NodeTermProxyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Requests a node terminal proxy ticket from `/nodes/{node}/termproxy`.", Attributes: map[string]datasourceschema.Attribute{
		"node": datasourceschema.StringAttribute{Required: true}, "command": datasourceschema.StringAttribute{Optional: true},
		"command_opts": datasourceschema.StringAttribute{Optional: true}, "result": datasourceschema.StringAttribute{Computed: true}, "proxy": datasourceschema.StringAttribute{Computed: true}, "port": datasourceschema.StringAttribute{Computed: true}, "ticket": datasourceschema.StringAttribute{Computed: true, Sensitive: true}, "user": datasourceschema.StringAttribute{Computed: true}, "upid": datasourceschema.StringAttribute{Computed: true},
	}}
}
func (d *NodeVNCShellDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Requests a node VNC shell proxy from `/nodes/{node}/vncshell`.", Attributes: map[string]datasourceschema.Attribute{
		"node": datasourceschema.StringAttribute{Required: true}, "command": datasourceschema.StringAttribute{Optional: true},
		"command_opts": datasourceschema.StringAttribute{Optional: true}, "result": datasourceschema.StringAttribute{Computed: true}, "proxy": datasourceschema.StringAttribute{Computed: true}, "port": datasourceschema.StringAttribute{Computed: true}, "ticket": datasourceschema.StringAttribute{Computed: true, Sensitive: true}, "user": datasourceschema.StringAttribute{Computed: true}, "upid": datasourceschema.StringAttribute{Computed: true},
	}}
}
func (d *NodeSpiceShellDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Requests a node SPICE shell proxy from `/nodes/{node}/spiceshell`.", Attributes: map[string]datasourceschema.Attribute{
		"node": datasourceschema.StringAttribute{Required: true}, "command": datasourceschema.StringAttribute{Optional: true},
		"command_opts": datasourceschema.StringAttribute{Optional: true}, "proxy": datasourceschema.StringAttribute{Required: true}, "result": datasourceschema.StringAttribute{Computed: true}, "port": datasourceschema.StringAttribute{Computed: true}, "ticket": datasourceschema.StringAttribute{Computed: true, Sensitive: true}, "user": datasourceschema.StringAttribute{Computed: true}, "upid": datasourceschema.StringAttribute{Computed: true},
	}}
}
func (d *QemuVNCProxyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Requests a VNC proxy for a VM from `/nodes/{node}/qemu/{vmid}/vncproxy`.", Attributes: map[string]datasourceschema.Attribute{
		"node": datasourceschema.StringAttribute{Required: true}, "vmid": datasourceschema.Int64Attribute{Required: true},
		"generate_password": datasourceschema.BoolAttribute{Optional: true}, "websocket": datasourceschema.BoolAttribute{Optional: true}, "result": datasourceschema.StringAttribute{Computed: true},
	}}
}
func (d *AccessVNCTicketDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Exchanges a VNC ticket through `/access/vncticket`.", Attributes: map[string]datasourceschema.Attribute{
		"userid": datasourceschema.StringAttribute{Required: true}, "path": datasourceschema.StringAttribute{Required: true},
		"vncticket": datasourceschema.StringAttribute{Required: true, Sensitive: true}, "privs": datasourceschema.StringAttribute{Optional: true},
		"port": datasourceschema.StringAttribute{Computed: true}, "ticket": datasourceschema.StringAttribute{Computed: true, Sensitive: true},
		"user": datasourceschema.StringAttribute{Computed: true}, "upid": datasourceschema.StringAttribute{Computed: true},
	}}
}

func (d *NodeTermProxyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *NodeVNCShellDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *NodeSpiceShellDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *QemuVNCProxyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *AccessVNCTicketDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func proxyJSON(value map[string]any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func (d *NodeTermProxyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config proxyModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := d.client.NodeTermProxy(ctx, config.Node.ValueString(), stringValue(config.Command), stringValue(config.CommandOpts))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Request Terminal Proxy", err.Error())
		return
	}
	setProxyResult(&config, result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
func (d *NodeVNCShellDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config proxyModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := d.client.NodeVNCShell(ctx, config.Node.ValueString(), stringValue(config.Command), stringValue(config.CommandOpts), 0, 0)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Request VNC Shell", err.Error())
		return
	}
	setProxyResult(&config, result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
func (d *NodeSpiceShellDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config proxyModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := d.client.NodeSpiceShell(ctx, config.Node.ValueString(), stringValue(config.Command), stringValue(config.CommandOpts), config.Proxy.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Request SPICE Shell", err.Error())
		return
	}
	config.Result = types.StringValue(proxyJSON(result))
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
func (d *QemuVNCProxyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config qemuVNCProxyModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := d.client.QemuVNCProxy(ctx, config.Node.ValueString(), config.VMID.ValueInt64(), config.GeneratePassword.ValueBool(), config.Websocket.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Request VNC Proxy", err.Error())
		return
	}
	config.Result = types.StringValue(proxyJSON(result))
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
func (d *AccessVNCTicketDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config vncTicketModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ticket, err := d.client.AccessVNCTicket(ctx, config.UserID.ValueString(), config.Path.ValueString(), config.VNCTicket.ValueString(), stringValue(config.Privs))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Exchange VNC Ticket", err.Error())
		return
	}
	config.Port = types.StringValue(ticket.Port)
	config.Ticket = types.StringValue(ticket.Ticket)
	config.User = stringOrNull(ticket.User)
	config.UPID = stringOrNull(ticket.UPID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func setProxyResult(config *proxyModel, result map[string]any) {
	config.Result = types.StringValue(proxyJSON(result))
	config.Port = types.StringValue(fmt.Sprint(result["port"]))
	config.Ticket = types.StringValue(fmt.Sprint(result["ticket"]))
	config.User = types.StringValue(fmt.Sprint(result["user"]))
	config.UPID = types.StringValue(fmt.Sprint(result["upid"]))
	if proxy, ok := result["proxy"]; ok {
		config.Proxy = types.StringValue(fmt.Sprint(proxy))
	}
}
