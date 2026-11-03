// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &StorageContentsDataSource{}
var _ datasource.DataSourceWithValidateConfig = &StorageContentsDataSource{}

type StorageContentsDataSource struct{ client *Client }

type storageContentsModel struct {
	ID      types.String            `tfsdk:"id"`
	Node    types.String            `tfsdk:"node"`
	Storage types.String            `tfsdk:"storage"`
	Content types.String            `tfsdk:"content"`
	Volumes []storageContentsRecord `tfsdk:"volumes"`
}

type storageContentsRecord struct {
	VolID        types.String `tfsdk:"volid"`
	Content      types.String `tfsdk:"content"`
	Format       types.String `tfsdk:"format"`
	Size         types.Int64  `tfsdk:"size"`
	VMID         types.Int64  `tfsdk:"vmid"`
	Notes        types.String `tfsdk:"notes"`
	Protected    types.Bool   `tfsdk:"protected"`
	CreationTime types.Int64  `tfsdk:"ctime"`
	Verification types.String `tfsdk:"verification"`
	Encrypted    types.String `tfsdk:"encrypted"`
}

type storageContentEntry struct {
	VolID        string               `json:"volid"`
	Format       string               `json:"format"`
	Size         int64                `json:"size"`
	VMID         proxmoxOptionalInt64 `json:"vmid"`
	Notes        string               `json:"notes"`
	Protected    proxmoxOptionalBool  `json:"protected"`
	CreationTime int64                `json:"ctime"`
	Verification string               `json:"verification"`
	Encrypted    string               `json:"encrypted"`
}

func NewStorageContentsDataSource() datasource.DataSource { return &StorageContentsDataSource{} }

func (d *StorageContentsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage_contents"
}

func (d *StorageContentsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists volume content of one storage on a node from `/nodes/{node}/storage/{storage}/content`, sorted by volume id. `content` filters by Proxmox content type (for example `iso` or `backup`).",
		Attributes: map[string]datasourceschema.Attribute{
			"id":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Identifier in `<node>/<storage>[/<content>]` form."},
			"node":    datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Node whose storage view is used."},
			"storage": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Storage to list."},
			"content": datasourceschema.StringAttribute{Optional: true, MarkdownDescription: "Restrict to one content type."},
			"volumes": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Volumes sorted by `volid`.",
				NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
					"volid":        datasourceschema.StringAttribute{Computed: true},
					"content":      datasourceschema.StringAttribute{Computed: true},
					"format":       datasourceschema.StringAttribute{Computed: true},
					"size":         datasourceschema.Int64Attribute{Computed: true},
					"vmid":         datasourceschema.Int64Attribute{Computed: true},
					"notes":        datasourceschema.StringAttribute{Computed: true},
					"protected":    datasourceschema.BoolAttribute{Computed: true},
					"ctime":        datasourceschema.Int64Attribute{Computed: true},
					"verification": datasourceschema.StringAttribute{Computed: true},
					"encrypted":    datasourceschema.StringAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *StorageContentsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *StorageContentsDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config storageContentsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if isConfiguredValue(config.Node) && !proxmoxNodeNamePattern.MatchString(config.Node.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("node"), "Invalid node name", "node must be an existing cluster node name.")
	}
}

func (d *StorageContentsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config storageContentsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node, storage := config.Node.ValueString(), config.Storage.ValueString()
	query := url.Values{}
	contentFilter := stringValue(config.Content)
	if contentFilter != "" {
		query.Set("content", contentFilter)
	}
	var entries []storageContentEntry
	apiPath := fmt.Sprintf("/nodes/%s/storage/%s/content", url.PathEscape(node), url.PathEscape(storage))
	if err := d.client.do(ctx, "GET", apiPath, query, nil, &entries); err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Storage Contents", fmt.Sprintf("Unable to call `%s`: %s", apiPath, err))
		return
	}
	records := make([]storageContentsRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, storageContentsRecord{
			VolID:        types.StringValue(entry.VolID),
			Content:      types.StringValue(contentTypeOfVolume(entry.VolID)),
			Format:       stringOrNull(entry.Format),
			Size:         int64OrNull(optionalPositive(entry.Size)),
			VMID:         int64OrNull(entry.VMID.Ptr()),
			Notes:        stringOrNull(entry.Notes),
			Protected:    nodesBoolOrNull(entry.Protected),
			CreationTime: int64OrNull(optionalPositive(entry.CreationTime)),
			Verification: stringOrNull(entry.Verification),
			Encrypted:    stringOrNull(entry.Encrypted),
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].VolID.ValueString() < records[j].VolID.ValueString() })
	id := node + "/" + storage
	if contentFilter != "" {
		id += "/" + contentFilter
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &storageContentsModel{ID: types.StringValue(id), Node: config.Node, Storage: config.Storage, Content: config.Content, Volumes: records})...)
}

// contentTypeOfVolume derives the Proxmox content type from the volume id.
func contentTypeOfVolume(volID string) string {
	_, rest, _ := strings.Cut(volID, ":")
	switch {
	case strings.HasPrefix(rest, "iso/"):
		return "iso"
	case strings.HasPrefix(rest, "vztmpl/"):
		return "vztmpl"
	case strings.HasPrefix(rest, "backup/"):
		return "backup"
	case strings.HasPrefix(rest, "import/"):
		return "import"
	case strings.Contains(rest, "cloudinit"):
		return "snippets"
	case strings.HasPrefix(rest, "snippets/"):
		return "snippets"
	default:
		return "images"
	}
}

func optionalPositive(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}

func notificationConfigureDS(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *Client {
	if req.ProviderData == nil {
		return nil
	}
	client, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", err.Error())
		return nil
	}
	return client
}
