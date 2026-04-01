package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &NodeDNSDataSource{}

type NodeDNSDataSource struct {
	client *Client
}

type NodeDNSDataSourceModel struct {
	ID     types.String `tfsdk:"id"`
	Node   types.String `tfsdk:"node"`
	DNS1   types.String `tfsdk:"dns1"`
	DNS2   types.String `tfsdk:"dns2"`
	DNS3   types.String `tfsdk:"dns3"`
	Search types.String `tfsdk:"search"`
}

func NewNodeDNSDataSource() datasource.DataSource {
	return &NodeDNSDataSource{}
}

func (d *NodeDNSDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_dns"
}

func (d *NodeDNSDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads DNS settings for a node from `/nodes/{node}/dns`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster node name.",
			},
			"dns1":   schema.StringAttribute{Computed: true},
			"dns2":   schema.StringAttribute{Computed: true},
			"dns3":   schema.StringAttribute{Computed: true},
			"search": schema.StringAttribute{Computed: true},
		},
	}
}

func (d *NodeDNSDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *NodeDNSDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NodeDNSDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	dns, err := d.client.NodeDNS(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Node DNS", fmt.Sprintf("Unable to call `/nodes/%s/dns`: %s", config.Node.ValueString(), err))
		return
	}

	state := NodeDNSDataSourceModel{
		ID:     config.Node,
		Node:   config.Node,
		DNS1:   stringOrNull(dns.DNS1),
		DNS2:   stringOrNull(dns.DNS2),
		DNS3:   stringOrNull(dns.DNS3),
		Search: stringOrNull(dns.Search),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
