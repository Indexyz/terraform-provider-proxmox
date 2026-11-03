// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/url"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &GuestSnapshotsDataSource{}
var _ datasource.DataSourceWithValidateConfig = &GuestSnapshotsDataSource{}

type GuestSnapshotsDataSource struct {
	client *Client
	kind   string
}

type guestSnapshotsModel struct {
	ID        types.String           `tfsdk:"id"`
	Node      types.String           `tfsdk:"node"`
	VMID      types.Int64            `tfsdk:"vmid"`
	Snapshots []guestSnapshotsRecord `tfsdk:"snapshots"`
}

type guestSnapshotsRecord struct {
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Parent      types.String `tfsdk:"parent"`
	SnapTime    types.Int64  `tfsdk:"snaptime"`
	VMState     types.Bool   `tfsdk:"vmstate"`
}

type guestSnapshotEntry struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Parent      string              `json:"parent"`
	SnapTime    int64               `json:"snaptime"`
	VMState     proxmoxOptionalBool `json:"vmstate"`
}

func NewQemuSnapshotsDataSource() datasource.DataSource {
	return &GuestSnapshotsDataSource{kind: "qemu"}
}

func NewLXCSnapshotsDataSource() datasource.DataSource {
	return &GuestSnapshotsDataSource{kind: "lxc"}
}

func (d *GuestSnapshotsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.kind + "_snapshots"
}

func (d *GuestSnapshotsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	snapshotAttributes := map[string]datasourceschema.Attribute{
		"name":        datasourceschema.StringAttribute{Computed: true},
		"description": datasourceschema.StringAttribute{Computed: true},
		"parent":      datasourceschema.StringAttribute{Computed: true},
		"snaptime":    datasourceschema.Int64Attribute{Computed: true},
	}
	snapshotAttributes["vmstate"] = datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the snapshot includes RAM state (QEMU only)."}
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: fmt.Sprintf("Lists the snapshots of one %s guest from `/nodes/{node}/%s/{vmid}/snapshot`, sorted by name. The synthetic `current` entry is included.", d.kind, d.kind),
		Attributes: map[string]datasourceschema.Attribute{
			"id":   datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Guest identifier."},
			"node": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Node that runs the guest."},
			"vmid": datasourceschema.Int64Attribute{Required: true, MarkdownDescription: "Guest VMID."},
			"snapshots": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Snapshots sorted by name.",
				NestedObject:        datasourceschema.NestedAttributeObject{Attributes: snapshotAttributes},
			},
		},
	}
}

func (d *GuestSnapshotsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *GuestSnapshotsDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config guestSnapshotsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if isConfiguredValue(config.Node) && !proxmoxNodeNamePattern.MatchString(config.Node.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("node"), "Invalid node name", "node must be an existing cluster node name.")
	}
	if isConfiguredValue(config.VMID) && config.VMID.ValueInt64() < 100 {
		resp.Diagnostics.AddAttributeError(path.Root("vmid"), "Invalid guest VMID", "vmid must be at least 100.")
	}
}

func (d *GuestSnapshotsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config guestSnapshotsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node, vmid := config.Node.ValueString(), config.VMID.ValueInt64()
	apiPath := fmt.Sprintf("/nodes/%s/%s/%d/snapshot", url.PathEscape(node), d.kind, vmid)
	var entries []guestSnapshotEntry
	if err := d.client.do(ctx, "GET", apiPath, nil, nil, &entries); err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Snapshots", fmt.Sprintf("Unable to call `%s`: %s", apiPath, err))
		return
	}
	records := make([]guestSnapshotsRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, guestSnapshotsRecord{
			Name:        types.StringValue(entry.Name),
			Description: stringOrNull(entry.Description),
			Parent:      stringOrNull(entry.Parent),
			SnapTime:    int64OrNull(optionalPositive(entry.SnapTime)),
			VMState:     nodesBoolOrNull(entry.VMState),
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Name.ValueString() < records[j].Name.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &guestSnapshotsModel{ID: types.StringValue(fmt.Sprintf("%s/%d", node, vmid)), Node: config.Node, VMID: config.VMID, Snapshots: records})...)
}
