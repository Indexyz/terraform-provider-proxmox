// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource = &GuestFeatureDataSource{}
	_ datasource.DataSource = &GuestFirewallRefsDataSource{}
	_ datasource.DataSource = &GuestPendingDataSource{}
	_ datasource.DataSource = &GuestRRDImageDataSource{}
	_ datasource.DataSource = &GuestRRDDataDataSource{}
	_ datasource.DataSource = &GuestVNCWebsocketDataSource{}
	_ datasource.DataSource = &NodeVNCWebsocketDataSource{}
)

type GuestFeatureDataSource struct{ client *Client }
type GuestFirewallRefsDataSource struct{ client *Client }
type GuestPendingDataSource struct{ client *Client }
type GuestRRDImageDataSource struct{ client *Client }
type GuestRRDDataDataSource struct{ client *Client }
type GuestVNCWebsocketDataSource struct{ client *Client }
type NodeVNCWebsocketDataSource struct{ client *Client }

type guestFeatureDSModel struct {
	ID         types.String `tfsdk:"id"`
	Type       types.String `tfsdk:"type"`
	Node       types.String `tfsdk:"node"`
	VMID       types.Int64  `tfsdk:"vmid"`
	Feature    types.String `tfsdk:"feature"`
	Snapname   types.String `tfsdk:"snapname"`
	HasFeature types.Bool   `tfsdk:"has_feature"`
	Nodes      types.List   `tfsdk:"nodes"`
}
type guestFirewallRefsDSModel struct {
	ID   types.String          `tfsdk:"id"`
	Type types.String          `tfsdk:"type"`
	Node types.String          `tfsdk:"node"`
	VMID types.Int64           `tfsdk:"vmid"`
	Kind types.String          `tfsdk:"kind"`
	Refs []guestFirewallRecord `tfsdk:"refs"`
}
type guestFirewallRecord struct {
	Type    types.String `tfsdk:"type"`
	Ref     types.String `tfsdk:"ref"`
	Name    types.String `tfsdk:"name"`
	Comment types.String `tfsdk:"comment"`
	Scope   types.String `tfsdk:"scope"`
}
type guestPendingDSModel struct {
	ID      types.String `tfsdk:"id"`
	Type    types.String `tfsdk:"type"`
	Node    types.String `tfsdk:"node"`
	VMID    types.Int64  `tfsdk:"vmid"`
	Pending types.String `tfsdk:"pending"`
}
type guestRRDImageDSModel struct {
	ID        types.String `tfsdk:"id"`
	Type      types.String `tfsdk:"type"`
	Node      types.String `tfsdk:"node"`
	VMID      types.Int64  `tfsdk:"vmid"`
	CF        types.String `tfsdk:"cf"`
	DS        types.String `tfsdk:"ds"`
	Timeframe types.String `tfsdk:"timeframe"`
	Image     types.String `tfsdk:"image"`
}
type guestRRDDataDSModel struct {
	ID        types.String `tfsdk:"id"`
	Type      types.String `tfsdk:"type"`
	Node      types.String `tfsdk:"node"`
	VMID      types.Int64  `tfsdk:"vmid"`
	CF        types.String `tfsdk:"cf"`
	Timeframe types.String `tfsdk:"timeframe"`
	Data      types.String `tfsdk:"data"`
}
type guestVNCWebsocketDSModel struct {
	ID         types.String `tfsdk:"id"`
	Type       types.String `tfsdk:"type"`
	Node       types.String `tfsdk:"node"`
	VMID       types.Int64  `tfsdk:"vmid"`
	VNCTicket  types.String `tfsdk:"vnc_ticket"`
	Port       types.String `tfsdk:"port"`
	ResultPort types.String `tfsdk:"result_port"`
}
type nodeVNCWebsocketDSModel struct {
	ID         types.String `tfsdk:"id"`
	Node       types.String `tfsdk:"node"`
	VNCTicket  types.String `tfsdk:"vnc_ticket"`
	Port       types.String `tfsdk:"port"`
	ResultPort types.String `tfsdk:"result_port"`
}

func NewGuestFeatureDataSource() datasource.DataSource      { return &GuestFeatureDataSource{} }
func NewGuestFirewallRefsDataSource() datasource.DataSource { return &GuestFirewallRefsDataSource{} }
func NewGuestPendingDataSource() datasource.DataSource      { return &GuestPendingDataSource{} }
func NewGuestRRDImageDataSource() datasource.DataSource     { return &GuestRRDImageDataSource{} }
func NewGuestRRDDataDataSource() datasource.DataSource      { return &GuestRRDDataDataSource{} }
func NewGuestVNCWebsocketDataSource() datasource.DataSource { return &GuestVNCWebsocketDataSource{} }
func NewNodeVNCWebsocketDataSource() datasource.DataSource  { return &NodeVNCWebsocketDataSource{} }

func (d *GuestFeatureDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_feature"
}
func (d *GuestFirewallRefsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_firewall_refs"
}
func (d *GuestPendingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_pending"
}
func (d *GuestRRDImageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_rrd_image"
}
func (d *GuestRRDDataDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_rrddata"
}
func (d *GuestVNCWebsocketDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guest_vnc_websocket"
}
func (d *NodeVNCWebsocketDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_vnc_websocket"
}

func guestTypeDS() datasourceschema.StringAttribute {
	return datasourceschema.StringAttribute{Required: true}
}
func guestExtraAttrs() map[string]datasourceschema.Attribute {
	return map[string]datasourceschema.Attribute{
		"id": datasourceschema.StringAttribute{Computed: true}, "type": guestTypeDS(),
		"node": datasourceschema.StringAttribute{Required: true}, "vmid": datasourceschema.Int64Attribute{Required: true},
	}
}

func (d *GuestFeatureDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := guestExtraAttrs()
	attrs["feature"] = datasourceschema.StringAttribute{Required: true}
	attrs["snapname"] = datasourceschema.StringAttribute{Optional: true}
	attrs["has_feature"] = datasourceschema.BoolAttribute{Computed: true}
	attrs["nodes"] = datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType}
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Checks a guest feature through `GET /nodes/{node}/{type}/{vmid}/feature`.", Attributes: attrs}
}
func (d *GuestFirewallRefsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := guestExtraAttrs()
	attrs["kind"] = datasourceschema.StringAttribute{Optional: true}
	attrs["refs"] = datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
		"type": datasourceschema.StringAttribute{Computed: true}, "ref": datasourceschema.StringAttribute{Computed: true},
		"name": datasourceschema.StringAttribute{Computed: true}, "comment": datasourceschema.StringAttribute{Computed: true},
		"scope": datasourceschema.StringAttribute{Computed: true},
	}}}
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Lists guest firewall references from `GET /nodes/{node}/{type}/{vmid}/firewall/refs`.", Attributes: attrs}
}
func (d *GuestPendingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := guestExtraAttrs()
	attrs["pending"] = datasourceschema.StringAttribute{Computed: true}
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Reads pending guest configuration changes as JSON from `GET /nodes/{node}/{type}/{vmid}/pending`.", Attributes: attrs}
}
func rrdAttrs() map[string]datasourceschema.Attribute {
	attrs := guestExtraAttrs()
	attrs["cf"] = datasourceschema.StringAttribute{Optional: true, Computed: true}
	attrs["timeframe"] = datasourceschema.StringAttribute{Optional: true, Computed: true}
	return attrs
}
func (d *GuestRRDImageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := rrdAttrs()
	attrs["ds"] = datasourceschema.StringAttribute{Required: true}
	attrs["image"] = datasourceschema.StringAttribute{Computed: true}
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Reads a guest RRD graph as PNG data from `GET /nodes/{node}/{type}/{vmid}/rrd`.", Attributes: attrs}
}
func (d *GuestRRDDataDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := rrdAttrs()
	attrs["data"] = datasourceschema.StringAttribute{Computed: true}
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Reads guest RRD data points as JSON from `GET /nodes/{node}/{type}/{vmid}/rrddata`.", Attributes: attrs}
}
func (d *GuestVNCWebsocketDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := guestExtraAttrs()
	attrs["vnc_ticket"] = datasourceschema.StringAttribute{Required: true, Sensitive: true}
	attrs["port"] = datasourceschema.StringAttribute{Required: true}
	attrs["result_port"] = datasourceschema.StringAttribute{Computed: true}
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Resolves the VNC websocket port through `GET /nodes/{node}/{type}/{vmid}/vncwebsocket`.", Attributes: attrs}
}
func (d *NodeVNCWebsocketDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{MarkdownDescription: "Resolves the node shell VNC websocket port through `GET /nodes/{node}/vncwebsocket`.", Attributes: map[string]datasourceschema.Attribute{
		"id": datasourceschema.StringAttribute{Computed: true}, "node": datasourceschema.StringAttribute{Required: true},
		"vnc_ticket": datasourceschema.StringAttribute{Required: true, Sensitive: true}, "port": datasourceschema.StringAttribute{Required: true}, "result_port": datasourceschema.StringAttribute{Computed: true},
	}}
}

func (d *GuestFeatureDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *GuestFirewallRefsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *GuestPendingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *GuestRRDImageDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *GuestRRDDataDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *GuestVNCWebsocketDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}
func (d *NodeVNCWebsocketDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *GuestFeatureDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config guestFeatureDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := d.client.GuestFeature(ctx, config.Node.ValueString(), config.Type.ValueString(), formatInt64(config.VMID.ValueInt64()), config.Feature.ValueString(), stringValue(config.Snapname))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Check Guest Feature", err.Error())
		return
	}
	config.ID = types.StringValue(config.Type.ValueString() + "/" + formatInt64(config.VMID.ValueInt64()) + "/" + config.Feature.ValueString())
	config.HasFeature = boolOrNull(result.HasFeature.Ptr())
	config.Nodes, _ = stringListValue(ctx, result.Nodes)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *GuestFirewallRefsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config guestFirewallRefsDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refs, err := d.client.GuestFirewallRefs(ctx, config.Node.ValueString(), config.Type.ValueString(), formatInt64(config.VMID.ValueInt64()), stringValue(config.Kind))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Firewall Refs", err.Error())
		return
	}
	records := make([]guestFirewallRecord, 0, len(refs))
	for _, ref := range refs {
		records = append(records, guestFirewallRecord{Type: stringOrNull(ref.Type), Ref: stringOrNull(ref.Ref), Name: stringOrNull(ref.Name), Comment: stringOrNull(ref.Comment), Scope: stringOrNull(ref.Scope)})
	}
	config.ID = types.StringValue(config.Type.ValueString() + "/" + formatInt64(config.VMID.ValueInt64()))
	config.Refs = records
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *GuestPendingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config guestPendingDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	pending, err := d.client.GuestPending(ctx, config.Node.ValueString(), config.Type.ValueString(), formatInt64(config.VMID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Pending Guest Config", err.Error())
		return
	}
	config.ID = types.StringValue(config.Type.ValueString() + "/" + formatInt64(config.VMID.ValueInt64()))
	config.Pending = types.StringValue(pending)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *GuestRRDImageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config guestRRDImageDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cf := firstNonEmpty(stringValue(config.CF), "AVERAGE")
	timeframe := firstNonEmpty(stringValue(config.Timeframe), "hour")
	image, err := d.client.GuestRRDImage(ctx, config.Node.ValueString(), config.Type.ValueString(), formatInt64(config.VMID.ValueInt64()), cf, config.DS.ValueString(), timeframe)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Guest RRD Image", err.Error())
		return
	}
	config.ID = types.StringValue(config.Type.ValueString() + "/" + formatInt64(config.VMID.ValueInt64()))
	config.CF = types.StringValue(cf)
	config.Timeframe = types.StringValue(timeframe)
	config.Image = types.StringValue(image)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *GuestRRDDataDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config guestRRDDataDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cf := firstNonEmpty(stringValue(config.CF), "AVERAGE")
	timeframe := firstNonEmpty(stringValue(config.Timeframe), "hour")
	data, err := d.client.GuestRRDData(ctx, config.Node.ValueString(), config.Type.ValueString(), formatInt64(config.VMID.ValueInt64()), cf, timeframe)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Guest RRD Data", err.Error())
		return
	}
	config.ID = types.StringValue(config.Type.ValueString() + "/" + formatInt64(config.VMID.ValueInt64()))
	config.CF = types.StringValue(cf)
	config.Timeframe = types.StringValue(timeframe)
	config.Data = types.StringValue(data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *GuestVNCWebsocketDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config guestVNCWebsocketDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	port, err := d.client.GuestVNCWebsocketPort(ctx, config.Node.ValueString(), config.Type.ValueString(), formatInt64(config.VMID.ValueInt64()), config.VNCTicket.ValueString(), config.Port.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Resolve VNC Websocket", err.Error())
		return
	}
	config.ID = types.StringValue(config.Type.ValueString() + "/" + formatInt64(config.VMID.ValueInt64()))
	config.ResultPort = stringOrNull(port)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *NodeVNCWebsocketDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeVNCWebsocketDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	port, err := d.client.NodeVNCWebsocketPort(ctx, config.Node.ValueString(), config.VNCTicket.ValueString(), config.Port.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Resolve Node VNC Websocket", err.Error())
		return
	}
	config.ID = types.StringValue(config.Node.ValueString())
	config.ResultPort = stringOrNull(port)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
