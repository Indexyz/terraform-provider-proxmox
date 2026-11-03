// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &NodeCertificatesInfoDataSource{}
var _ datasource.DataSource = &NodeSubscriptionDataSource{}

type NodeCertificatesInfoDataSource struct{ client *Client }
type NodeSubscriptionDataSource struct{ client *Client }

type nodeCertificatesInfoModel struct {
	Node         types.String               `tfsdk:"node"`
	Certificates []nodeCertificateInfoModel `tfsdk:"certificates"`
}

type nodeCertificateInfoModel struct {
	Filename      types.String `tfsdk:"filename"`
	Fingerprint   types.String `tfsdk:"fingerprint"`
	Issuer        types.String `tfsdk:"issuer"`
	NotAfter      types.Int64  `tfsdk:"notafter"`
	NotBefore     types.Int64  `tfsdk:"notbefore"`
	PEM           types.String `tfsdk:"pem"`
	PublicKeyBits types.Int64  `tfsdk:"public_key_bits"`
	PublicKeyType types.String `tfsdk:"public_key_type"`
	SAN           types.List   `tfsdk:"san"`
	Subject       types.String `tfsdk:"subject"`
}

type nodeSubscriptionModel struct {
	Node        types.String `tfsdk:"node"`
	CheckTime   types.Int64  `tfsdk:"checktime"`
	Key         types.String `tfsdk:"key"`
	Level       types.String `tfsdk:"level"`
	Message     types.String `tfsdk:"message"`
	NextDueDate types.String `tfsdk:"nextduedate"`
	ProductName types.String `tfsdk:"productname"`
	RegDate     types.String `tfsdk:"regdate"`
	ServerID    types.String `tfsdk:"serverid"`
	Signature   types.String `tfsdk:"signature"`
	Sockets     types.Int64  `tfsdk:"sockets"`
	Status      types.String `tfsdk:"status"`
	URL         types.String `tfsdk:"url"`
}

func NewNodeCertificatesInfoDataSource() datasource.DataSource {
	return &NodeCertificatesInfoDataSource{}
}

func NewNodeSubscriptionDataSource() datasource.DataSource {
	return &NodeSubscriptionDataSource{}
}

func (d *NodeCertificatesInfoDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_certificates_info"
}

func (d *NodeSubscriptionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_subscription"
}

func (d *NodeCertificatesInfoDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the certificates of a node from `GET /nodes/{node}/certificates/info`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node": datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"certificates": datasourceschema.ListNestedAttribute{Computed: true, NestedObject: datasourceschema.NestedAttributeObject{Attributes: map[string]datasourceschema.Attribute{
				"filename":        datasourceschema.StringAttribute{Computed: true},
				"fingerprint":     datasourceschema.StringAttribute{Computed: true},
				"issuer":          datasourceschema.StringAttribute{Computed: true},
				"notafter":        datasourceschema.Int64Attribute{Computed: true},
				"notbefore":       datasourceschema.Int64Attribute{Computed: true},
				"pem":             datasourceschema.StringAttribute{Computed: true},
				"public_key_bits": datasourceschema.Int64Attribute{Computed: true},
				"public_key_type": datasourceschema.StringAttribute{Computed: true},
				"san":             datasourceschema.ListAttribute{Computed: true, ElementType: types.StringType},
				"subject":         datasourceschema.StringAttribute{Computed: true},
			}}},
		},
	}
}

func (d *NodeSubscriptionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the subscription state of a node from `GET /nodes/{node}/subscription`.",
		Attributes: map[string]datasourceschema.Attribute{
			"node":        datasourceschema.StringAttribute{Required: true, MarkdownDescription: "Cluster node."},
			"checktime":   datasourceschema.Int64Attribute{Computed: true},
			"key":         datasourceschema.StringAttribute{Computed: true},
			"level":       datasourceschema.StringAttribute{Computed: true},
			"message":     datasourceschema.StringAttribute{Computed: true},
			"nextduedate": datasourceschema.StringAttribute{Computed: true},
			"productname": datasourceschema.StringAttribute{Computed: true},
			"regdate":     datasourceschema.StringAttribute{Computed: true},
			"serverid":    datasourceschema.StringAttribute{Computed: true},
			"signature":   datasourceschema.StringAttribute{Computed: true},
			"sockets":     datasourceschema.Int64Attribute{Computed: true},
			"status":      datasourceschema.StringAttribute{Computed: true},
			"url":         datasourceschema.StringAttribute{Computed: true},
		},
	}
}

func (d *NodeCertificatesInfoDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *NodeSubscriptionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationConfigureDS(req, resp)
}

func (d *NodeCertificatesInfoDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeCertificatesInfoModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	infos, err := d.client.NodeCertificateInfos(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Node Certificates", err.Error())
		return
	}
	records := make([]nodeCertificateInfoModel, 0, len(infos))
	for _, info := range infos {
		san, sanDiags := types.ListValueFrom(ctx, types.StringType, info.SAN)
		resp.Diagnostics.Append(sanDiags...)
		records = append(records, nodeCertificateInfoModel{
			Filename: stringOrNull(info.Filename), Fingerprint: stringOrNull(info.Fingerprint), Issuer: stringOrNull(info.Issuer),
			NotAfter: types.Int64Value(info.NotAfter), NotBefore: types.Int64Value(info.NotBefore), PEM: stringOrNull(info.PEM),
			PublicKeyBits: types.Int64Value(info.PublicKeyBits), PublicKeyType: stringOrNull(info.PublicKeyType),
			SAN: san, Subject: stringOrNull(info.Subject),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &nodeCertificatesInfoModel{Node: config.Node, Certificates: records})...)
}

func (d *NodeSubscriptionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nodeSubscriptionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	subscription, err := d.client.NodeSubscription(ctx, config.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox Node Subscription", err.Error())
		return
	}
	state := nodeSubscriptionModel{
		Node: config.Node, CheckTime: types.Int64Value(subscription.CheckTime), Key: stringOrNull(subscription.Key),
		Level: stringOrNull(subscription.Level), Message: stringOrNull(subscription.Message), NextDueDate: stringOrNull(subscription.NextDueDate),
		ProductName: stringOrNull(subscription.ProductName), RegDate: stringOrNull(subscription.RegDate), ServerID: stringOrNull(subscription.ServerID),
		Signature: stringOrNull(subscription.Signature), Sockets: types.Int64Value(subscription.Sockets), Status: stringOrNull(subscription.Status), URL: stringOrNull(subscription.URL),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
