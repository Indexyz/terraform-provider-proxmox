// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &SDNFabricNodeDataSource{}

type SDNFabricNodeDataSource struct {
	client *Client
}

type sdnFabricNodeDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	FabricID   types.String `tfsdk:"fabric_id"`
	NodeID     types.String `tfsdk:"node_id"`
	Protocol   types.String `tfsdk:"protocol"`
	IP         types.String `tfsdk:"ip"`
	IP6        types.String `tfsdk:"ip6"`
	Interfaces types.List   `tfsdk:"interfaces"`
	PublicKey  types.String `tfsdk:"public_key"`
	Role       types.String `tfsdk:"role"`
	Endpoint   types.String `tfsdk:"endpoint"`
	AllowedIPs types.List   `tfsdk:"allowed_ips"`
	Peers      types.List   `tfsdk:"peers"`
	State      types.String `tfsdk:"state"`
	Pending    types.Map    `tfsdk:"pending"`
}

func NewSDNFabricNodeDataSource() datasource.DataSource { return &SDNFabricNodeDataSource{} }

func (d *SDNFabricNodeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_fabric_node"
}

func (d *SDNFabricNodeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads one staged SDN fabric node from `/cluster/sdn/fabrics/node/{fabric_id}/{node_id}`. Fields without a typed counterpart stay in `pending`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":          datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Fabric node identifier (`<fabric_id>/<node_id>`)."},
			"fabric_id":   datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Fabric that owns the node."},
			"node_id":     datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Node entry to read."},
			"protocol":    datasourceschema.StringAttribute{Computed: true},
			"ip":          datasourceschema.StringAttribute{Computed: true},
			"ip6":         datasourceschema.StringAttribute{Computed: true},
			"interfaces":  datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
			"public_key":  datasourceschema.StringAttribute{Computed: true},
			"role":        datasourceschema.StringAttribute{Computed: true},
			"endpoint":    datasourceschema.StringAttribute{Computed: true},
			"allowed_ips": datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
			"peers":       datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
			"state":       datasourceschema.StringAttribute{Computed: true},
			"pending":     datasourceschema.MapAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *SDNFabricNodeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", err.Error())
		return
	}
	d.client = client
}

func (d *SDNFabricNodeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sdnFabricNodeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fabricID := config.FabricID.ValueString()
	nodeID := config.NodeID.ValueString()
	entry, err := d.client.GetSDNFabricNode(ctx, fabricID, nodeID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox SDN Fabric Node", fmt.Sprintf("Unable to call `/cluster/sdn/fabrics/node/%s/%s`: %s", fabricID, nodeID, err))
		return
	}
	staged := sdnPendingMap(entry.Pending)
	listValue := func(running []string, key string) types.List {
		value, listDiags := types.ListValueFrom(ctx, types.StringType, sdnStagedRawList(running, staged[key]))
		resp.Diagnostics.Append(listDiags...)
		return value
	}
	pending := types.MapNull(types.StringType)
	if len(staged) > 0 {
		value, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		resp.Diagnostics.Append(pendingDiags...)
		pending = value
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &sdnFabricNodeDataSourceModel{
		ID:         types.StringValue(fabricID + "/" + nodeID),
		FabricID:   types.StringValue(fabricID),
		NodeID:     types.StringValue(entry.NodeID),
		Protocol:   sdnStagedString(entry.Protocol, staged["protocol"]),
		IP:         sdnStagedString(entry.IP, staged["ip"]),
		IP6:        sdnStagedString(entry.IP6, staged["ip6"]),
		Interfaces: listValue(entry.Interfaces, "interfaces"),
		PublicKey:  sdnStagedString(entry.PublicKey, staged["public_key"]),
		Role:       sdnStagedString(entry.Role, staged["role"]),
		Endpoint:   sdnStagedString(entry.Endpoint, staged["endpoint"]),
		AllowedIPs: listValue(entry.AllowedIPs, "allowed_ips"),
		Peers:      listValue(entry.Peers, "peers"),
		State:      stringOrNull(entry.State),
		Pending:    pending,
	})...)
}
