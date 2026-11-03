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

var _ datasource.DataSource = &NodeNetworkInterfacesDataSource{}
var _ datasource.DataSourceWithValidateConfig = &NodeNetworkInterfacesDataSource{}
var _ datasource.DataSource = &ClusterFirewallOptionsDataSource{}
var _ datasource.DataSource = &NodeFirewallOptionsDataSource{}
var _ datasource.DataSourceWithValidateConfig = &NodeFirewallOptionsDataSource{}
var _ datasource.DataSource = &GuestFirewallOptionsDataSource{}
var _ datasource.DataSourceWithValidateConfig = &GuestFirewallOptionsDataSource{}

type NodeNetworkInterfacesDataSource struct{ client *Client }
type ClusterFirewallOptionsDataSource struct{ client *Client }
type NodeFirewallOptionsDataSource struct{ client *Client }
type GuestFirewallOptionsDataSource struct{ client *Client }

type nodeNetworkInterfacesModel struct {
	ID         types.String                `tfsdk:"id"`
	Node       types.String                `tfsdk:"node"`
	Interfaces []nodeNetworkInterfaceEntry `tfsdk:"interfaces"`
	Changes    types.String                `tfsdk:"changes"`
}

type nodeNetworkInterfaceEntry struct {
	Iface       types.String `tfsdk:"iface"`
	Type        types.String `tfsdk:"type"`
	Active      types.Bool   `tfsdk:"active"`
	Exists      types.Bool   `tfsdk:"exists"`
	Autostart   types.Bool   `tfsdk:"autostart"`
	MTU         types.Int64  `tfsdk:"mtu"`
	Address     types.String `tfsdk:"address"`
	Address6    types.String `tfsdk:"address6"`
	Gateway     types.String `tfsdk:"gateway"`
	Gateway6    types.String `tfsdk:"gateway6"`
	CIDR        types.String `tfsdk:"cidr"`
	CIDR6       types.String `tfsdk:"cidr6"`
	BridgePorts types.String `tfsdk:"bridge_ports"`
	OVSBridge   types.String `tfsdk:"ovs_bridge"`
	Comments    types.String `tfsdk:"comments"`
	Comments6   types.String `tfsdk:"comments6"`
}

type nodeFirewallOptionsDSModel struct {
	Node               types.String `tfsdk:"node"`
	Enable             types.Bool   `tfsdk:"enable"`
	NDP                types.Bool   `tfsdk:"ndp"`
	NFConntrack        types.Bool   `tfsdk:"nf_conntrack_allow_invalid"`
	NFTables           types.Bool   `tfsdk:"nftables"`
	Nosmurfs           types.Bool   `tfsdk:"nosmurfs"`
	SMURFLogLevel      types.String `tfsdk:"smurf_log_level"`
	TCPFlagsLogLevel   types.String `tfsdk:"tcp_flags_log_level"`
	TCPFlags           types.String `tfsdk:"tcpflags"`
	ProtectionSynflood types.Bool   `tfsdk:"protection_synflood"`
	LogLevelIn         types.String `tfsdk:"log_level_in"`
	LogLevelOut        types.String `tfsdk:"log_level_out"`
}

func NewNodeNetworkInterfacesDataSource() datasource.DataSource {
	return &NodeNetworkInterfacesDataSource{}
}
func NewClusterFirewallOptionsDataSource() datasource.DataSource {
	return &ClusterFirewallOptionsDataSource{}
}
func NewNodeFirewallOptionsDataSource() datasource.DataSource {
	return &NodeFirewallOptionsDataSource{}
}
func NewGuestFirewallOptionsDataSource() datasource.DataSource {
	return &GuestFirewallOptionsDataSource{}
}

func (d *NodeNetworkInterfacesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_network_interfaces"
}

func (d *ClusterFirewallOptionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_firewall_options"
}

func (d *NodeFirewallOptionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_firewall_options"
}

func (d *GuestFirewallOptionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_firewall_options"
}

func (d *NodeNetworkInterfacesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists the network interfaces of one node, including staged changes, from `/nodes/{node}/network`. `changes` carries the staged diff as reported by Proxmox.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":      datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Node name."},
			"node":    datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"changes": datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Staged diff between `interfaces` and `interfaces.new`."},
			"interfaces": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Interfaces sorted by name.",
				NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
					"iface":        datasourceschema.StringAttribute{Computed: true},
					"type":         datasourceschema.StringAttribute{Computed: true},
					"active":       datasourceschema.BoolAttribute{Computed: true},
					"exists":       datasourceschema.BoolAttribute{Computed: true},
					"autostart":    datasourceschema.BoolAttribute{Computed: true},
					"mtu":          datasourceschema.Int64Attribute{Computed: true},
					"address":      datasourceschema.StringAttribute{Computed: true},
					"address6":     datasourceschema.StringAttribute{Computed: true},
					"gateway":      datasourceschema.StringAttribute{Computed: true},
					"gateway6":     datasourceschema.StringAttribute{Computed: true},
					"cidr":         datasourceschema.StringAttribute{Computed: true},
					"cidr6":        datasourceschema.StringAttribute{Computed: true},
					"bridge_ports": datasourceschema.StringAttribute{Computed: true},
					"ovs_bridge":   datasourceschema.StringAttribute{Computed: true},
					"comments":     datasourceschema.StringAttribute{Computed: true},
					"comments6":    datasourceschema.StringAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *ClusterFirewallOptionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the cluster firewall options from `/cluster/firewall/options`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":             datasourceschema.StringAttribute{Computed: true},
			"enable":         datasourceschema.BoolAttribute{Computed: true},
			"ebtables":       datasourceschema.BoolAttribute{Computed: true},
			"policy_in":      datasourceschema.StringAttribute{Computed: true},
			"policy_out":     datasourceschema.StringAttribute{Computed: true},
			"policy_forward": datasourceschema.StringAttribute{Computed: true},
			"log_ratelimit":  datasourceschema.StringAttribute{Computed: true},
		},
	}
}

func (d *NodeFirewallOptionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the firewall options of one node from `/nodes/{node}/firewall/options`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":                       datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"enable":                     datasourceschema.BoolAttribute{Computed: true},
			"ndp":                        datasourceschema.BoolAttribute{Computed: true},
			"nf_conntrack_allow_invalid": datasourceschema.BoolAttribute{Computed: true},
			"nftables":                   datasourceschema.BoolAttribute{Computed: true},
			"nosmurfs":                   datasourceschema.BoolAttribute{Computed: true},
			"smurf_log_level":            datasourceschema.StringAttribute{Computed: true},
			"tcp_flags_log_level":        datasourceschema.StringAttribute{Computed: true},
			"tcpflags":                   datasourceschema.StringAttribute{Computed: true},
			"protection_synflood":        datasourceschema.BoolAttribute{Computed: true},
			"log_level_in":               datasourceschema.StringAttribute{Computed: true},
			"log_level_out":              datasourceschema.StringAttribute{Computed: true},
		},
	}
}

func (d *GuestFirewallOptionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the firewall options of one QEMU/LXC guest from `/nodes/{node}/{guest}/{vmid}/firewall/options`.",
		Attributes: map[string]datasourceschema.Attribute{
			"id":            datasourceschema.StringAttribute{Computed: true},
			"node":          datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"guest_type":    datasourceschema.StringAttribute{Required: true, MarkdownDescription: "`qemu` or `lxc`."},
			"vm_id":         datasourceschema.Int64Attribute{Required: true, MarkdownDescription: "Guest VMID."},
			"enable":        datasourceschema.BoolAttribute{Computed: true},
			"dhcp":          datasourceschema.BoolAttribute{Computed: true},
			"ipfilter":      datasourceschema.BoolAttribute{Computed: true},
			"macfilter":     datasourceschema.BoolAttribute{Computed: true},
			"ndp":           datasourceschema.BoolAttribute{Computed: true},
			"radv":          datasourceschema.BoolAttribute{Computed: true},
			"policy_in":     datasourceschema.StringAttribute{Computed: true},
			"policy_out":    datasourceschema.StringAttribute{Computed: true},
			"log_level_in":  datasourceschema.StringAttribute{Computed: true},
			"log_level_out": datasourceschema.StringAttribute{Computed: true},
		},
	}
}

func (d *NodeNetworkInterfacesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *ClusterFirewallOptionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *NodeFirewallOptionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *GuestFirewallOptionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *NodeNetworkInterfacesDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config nodeNetworkInterfacesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if !resp.Diagnostics.HasError() {
		validateNodeName(config.Node, resp)
	}
}

func (d *NodeFirewallOptionsDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config nodeFirewallOptionsDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if !resp.Diagnostics.HasError() {
		validateNodeName(config.Node, resp)
	}
}

func (d *GuestFirewallOptionsDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config guestFirewallOptionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateGuestFirewallScope(config.Node, config.GuestType, config.VMID)...)
}

func validateNodeName(node types.String, resp *datasource.ValidateConfigResponse) {
	if isConfiguredValue(node) && !proxmoxNodeNamePattern.MatchString(node.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("node"), "Invalid node name", "node must be an existing cluster node name.")
	}
}

func (d *NodeNetworkInterfacesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeNetworkInterfacesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := config.Node.ValueString()
	ifaces, changes, err := d.client.NodeNetworkInterfaces(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Node Network", fmt.Sprintf("Unable to call `/nodes/%s/network`: %s", node, err))
		return
	}
	entries := make([]nodeNetworkInterfaceEntry, 0, len(ifaces))
	for _, iface := range ifaces {
		entries = append(entries, nodeNetworkInterfaceEntry{
			Iface:       types.StringValue(iface.Iface),
			Type:        stringOrNull(iface.Type),
			Active:      nodesBoolOrNull(iface.Active),
			Exists:      nodesBoolOrNull(iface.Exists),
			Autostart:   nodesBoolOrNull(iface.Autostart),
			MTU:         int64OrNull(iface.MTU.Ptr()),
			Address:     stringOrNull(iface.Address),
			Address6:    stringOrNull(iface.Address6),
			Gateway:     stringOrNull(iface.Gateway),
			Gateway6:    stringOrNull(iface.Gateway6),
			CIDR:        stringOrNull(iface.CIDR),
			CIDR6:       stringOrNull(iface.CIDR6),
			BridgePorts: stringOrNull(iface.BridgePorts),
			OVSBridge:   stringOrNull(iface.OVSBridge),
			Comments:    stringOrNull(iface.Comments),
			Comments6:   stringOrNull(iface.Comments6),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Iface.ValueString() < entries[j].Iface.ValueString() })
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeNetworkInterfacesModel{ID: types.StringValue(node), Node: config.Node, Interfaces: entries, Changes: stringOrNull(joinChanges(changes))})...)
}

func joinChanges(changes []string) string {
	if len(changes) == 0 {
		return ""
	}
	joined := changes[0]
	for _, change := range changes[1:] {
		joined += "\n" + change
	}
	return joined
}

func (d *ClusterFirewallOptionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config clusterFirewallOptionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var options struct {
		Enable        proxmoxOptionalBool `json:"enable"`
		EBTables      proxmoxOptionalBool `json:"ebtables"`
		PolicyIn      string              `json:"policy_in"`
		PolicyOut     string              `json:"policy_out"`
		PolicyForward string              `json:"policy_forward"`
		LogRatelimit  string              `json:"log_ratelimit"`
	}
	if err := d.client.do(ctx, "GET", "/cluster/firewall/options", nil, nil, &options); err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Cluster Firewall Options", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &clusterFirewallOptionsModel{
		Enable: nodesBoolOrNull(options.Enable), Ebtables: nodesBoolOrNull(options.EBTables),
		PolicyIn: stringOrNull(options.PolicyIn), PolicyOut: stringOrNull(options.PolicyOut),
		PolicyForward: stringOrNull(options.PolicyForward), LogRateLimit: stringOrNull(options.LogRatelimit),
	})...)
}

func (d *NodeFirewallOptionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeFirewallOptionsDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := config.Node.ValueString()
	var options struct {
		Enable             proxmoxOptionalBool `json:"enable"`
		NDP                proxmoxOptionalBool `json:"ndp"`
		NFConntrackInvalid proxmoxOptionalBool `json:"nf_conntrack_allow_invalid"`
		NFTables           proxmoxOptionalBool `json:"nftables"`
		Nosmurfs           proxmoxOptionalBool `json:"nosmurfs"`
		SMURFLogLevel      string              `json:"smurf_log_level"`
		TCPFlagsLogLevel   string              `json:"tcp_flags_log_level"`
		TCPFlags           string              `json:"tcpflags"`
		ProtectionSynflood proxmoxOptionalBool `json:"protection_synflood"`
		LogLevelIn         string              `json:"log_level_in"`
		LogLevelOut        string              `json:"log_level_out"`
	}
	if err := d.client.do(ctx, "GET", "/nodes/"+url.PathEscape(node)+"/firewall/options", nil, nil, &options); err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Node Firewall Options", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeFirewallOptionsDSModel{
		Node: config.Node, Enable: nodesBoolOrNull(options.Enable), NDP: nodesBoolOrNull(options.NDP),
		NFConntrack: nodesBoolOrNull(options.NFConntrackInvalid), NFTables: nodesBoolOrNull(options.NFTables),
		Nosmurfs: nodesBoolOrNull(options.Nosmurfs), SMURFLogLevel: stringOrNull(options.SMURFLogLevel),
		TCPFlagsLogLevel: stringOrNull(options.TCPFlagsLogLevel), TCPFlags: stringOrNull(options.TCPFlags),
		ProtectionSynflood: nodesBoolOrNull(options.ProtectionSynflood),
		LogLevelIn:         stringOrNull(options.LogLevelIn), LogLevelOut: stringOrNull(options.LogLevelOut),
	})...)
}

func (d *GuestFirewallOptionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config guestFirewallOptionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	scope := guestFirewallScopeFromModel(config.Node, config.GuestType, config.VMID)
	options, err := d.client.GetGuestFirewallOptions(ctx, scope.GuestType, scope.Node, scope.VMID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Guest Firewall Options", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &guestFirewallOptionsModel{
		Node: config.Node, GuestType: config.GuestType, VMID: config.VMID,
		Enable: nodesBoolOrNull(options.Enable), DHCP: nodesBoolOrNull(options.DHCP),
		IPFilter: nodesBoolOrNull(options.IPFilter), MACFilter: nodesBoolOrNull(options.MACFilter),
		NDP: nodesBoolOrNull(options.NDP), RADV: nodesBoolOrNull(options.RADV),
		PolicyIn: stringOrNull(options.PolicyIn), PolicyOut: stringOrNull(options.PolicyOut),
		LogLevelIn: stringOrNull(options.LogLevelIn), LogLevelOut: stringOrNull(options.LogLevelOut),
	})...)
}
