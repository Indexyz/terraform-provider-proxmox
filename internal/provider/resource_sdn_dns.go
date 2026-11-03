// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SDNDNSResource{}
var _ resource.ResourceWithImportState = &SDNDNSResource{}
var _ resource.ResourceWithValidateConfig = &SDNDNSResource{}

const sdnDNSManagedFieldsKey = "sdn-dns-managed-fields"

var (
	sdnDNSIDPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]*[a-zA-Z0-9]$`)
	sdnDNSTypes     = []string{"powerdns"}
)

type SDNDNSResource struct {
	client *Client
}

type sdnDNSModel struct {
	ID            types.String `tfsdk:"id"`
	DNS           types.String `tfsdk:"dns"`
	Type          types.String `tfsdk:"type"`
	URL           types.String `tfsdk:"url"`
	Key           types.String `tfsdk:"key"`
	TTL           types.Int64  `tfsdk:"ttl"`
	ReverseMaskV6 types.Int64  `tfsdk:"reversemaskv6"`
	Fingerprint   types.String `tfsdk:"fingerprint"`
	State         types.String `tfsdk:"state"`
	Pending       types.Map    `tfsdk:"pending"`
}

func NewSDNDNSResource() resource.Resource {
	return &SDNDNSResource{}
}

func (r *SDNDNSResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_dns"
}

func (r *SDNDNSResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox VE SDN DNS provider through `/cluster/sdn/dns`. Only the `powerdns` type exists. DNS providers stage like the other SDN objects and only take effect when `proxmox_sdn_apply` commits; a zone references them by id.",
		Attributes: map[string]schema.Attribute{
			"id":            schema.StringAttribute{Computed: true, MarkdownDescription: "DNS provider identifier (same as `dns`).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"dns":           schema.StringAttribute{Required: true, MarkdownDescription: "DNS provider identifier: starts with a letter, only letters and digits, at least 2 characters. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"type":          schema.StringAttribute{Required: true, MarkdownDescription: "DNS provider type; only `powerdns`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"url":           schema.StringAttribute{Required: true, MarkdownDescription: "PowerDNS API URL."},
			"key":           schema.StringAttribute{Required: true, MarkdownDescription: "PowerDNS API key. Proxmox stores it in `/etc/pve/sdn/dns.cfg`, not in a private file."},
			"ttl":           schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Default DNS record TTL."},
			"reversemaskv6": schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Netmask used for the IPv6 reverse zone name."},
			"fingerprint":   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "SHA-256 fingerprint of the PowerDNS TLS certificate."},
			"state":         schema.StringAttribute{Computed: true, MarkdownDescription: "Proxmox staging state: `new` or `changed` while the DNS provider is not committed yet."},
			"pending":       schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Properties Proxmox currently reports as staged but not committed, with `deleted` for removals."},
		},
	}
}

func (r *SDNDNSResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", err.Error())
		return
	}
	r.client = client
}

func (r *SDNDNSResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config sdnDNSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateSDNDNSConfig(config)...)
}

func validateSDNDNSConfig(config sdnDNSModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.DNS) {
		dns := config.DNS.ValueString()
		if len(dns) < 2 || !sdnDNSIDPattern.MatchString(dns) {
			diags.AddAttributeError(path.Root("dns"), "Invalid SDN DNS identifier", "dns must start with a letter, contain only letters and digits, and be at least 2 characters long.")
		}
	}
	if isConfiguredValue(config.Type) && !slices.Contains(sdnDNSTypes, config.Type.ValueString()) {
		diags.AddAttributeError(path.Root("type"), "Invalid SDN DNS type", "type must be powerdns.")
	}
	if isConfiguredValue(config.TTL) && config.TTL.ValueInt64() < 0 {
		diags.AddAttributeError(path.Root("ttl"), "Invalid DNS TTL", "ttl must not be negative.")
	}
	if isConfiguredValue(config.ReverseMaskV6) && (config.ReverseMaskV6.ValueInt64() < 0 || config.ReverseMaskV6.ValueInt64() > 128) {
		diags.AddAttributeError(path.Root("reversemaskv6"), "Invalid IPv6 reverse mask", "reversemaskv6 must be between 0 and 128.")
	}
	return diags
}

func sdnDNSRequestFromModels(config, plan sdnDNSModel) SDNDNSRequest {
	request := SDNDNSRequest{Type: plan.Type.ValueString()}
	if isConfiguredValue(config.URL) {
		request.URL = stringPointer(plan.URL)
	}
	if isConfiguredValue(config.Key) {
		request.Key = stringPointer(plan.Key)
	}
	if isConfiguredValue(config.TTL) {
		request.TTL = int64PointerValue(plan.TTL)
	}
	if isConfiguredValue(config.ReverseMaskV6) {
		request.ReverseMaskV6 = int64PointerValue(plan.ReverseMaskV6)
	}
	if isConfiguredValue(config.Fingerprint) {
		request.Fingerprint = stringPointer(plan.Fingerprint)
	}
	return request
}

func sdnDNSManagedFields(config sdnDNSModel) []string {
	var fields []string
	for _, field := range []struct {
		name       string
		configured bool
	}{
		{"url", isConfiguredValue(config.URL)},
		{"key", isConfiguredValue(config.Key)},
		{"ttl", isConfiguredValue(config.TTL)},
		{"reversemaskv6", isConfiguredValue(config.ReverseMaskV6)},
		{"fingerprint", isConfiguredValue(config.Fingerprint)},
	} {
		if field.configured {
			fields = append(fields, field.name)
		}
	}
	slices.Sort(fields)
	return fields
}

func (r *SDNDNSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config sdnDNSModel
	var plan sdnDNSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	createReq := sdnDNSRequestFromModels(config, plan)
	dns := plan.DNS.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		createReq.LockToken = token
		return r.client.CreateSDNDNS(ctx, dns, createReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, dns)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNDNSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sdnDNSModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.DNS.ValueString())
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

func (r *SDNDNSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config sdnDNSModel
	var plan sdnDNSModel
	var prior sdnDNSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManaged, diags := readSDNDNSManagedFields(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetSDNDNS(ctx, prior.DNS.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN DNS", err.Error())
		return
	}
	updateReq := sdnDNSRequestFromModels(config, plan)
	updateReq.Delete = sdnDeleteKeys(sdnDNSManagedFields(config), previouslyManaged)
	dns := prior.DNS.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		updateReq.LockToken = token
		if current.Digest != "" {
			updateReq.Digest = &current.Digest
		}
		return r.client.UpdateSDNDNS(ctx, dns, updateReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, dns)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNDNSResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sdnDNSModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.GetSDNDNS(ctx, state.DNS.ValueString()); err != nil {
		if errors.Is(err, errNotFound) {
			return
		}
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN DNS", err.Error())
		return
	}
	dns := state.DNS.ValueString()
	withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		return r.client.DeleteSDNDNS(ctx, dns, token)
	})
}

func (r *SDNDNSResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if len(req.ID) < 2 || !sdnDNSIDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected an SDN DNS identifier: a letter, letters and digits only, at least 2 characters.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("dns"), req.ID)...)
}

func (r *SDNDNSResource) readState(ctx context.Context, dns string) (sdnDNSModel, diag.Diagnostics) {
	entry, err := r.client.GetSDNDNS(ctx, dns)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return sdnDNSModel{ID: types.StringNull()}, nil
		}
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox SDN DNS", fmt.Sprintf("Unable to read SDN DNS provider %q: %s", dns, err))
		return sdnDNSModel{}, diags
	}
	if entry.State == "deleted" {
		return sdnDNSModel{ID: types.StringNull()}, nil
	}
	return sdnDNSStateFromAPI(ctx, entry)
}

func sdnDNSStateFromAPI(ctx context.Context, entry SDNDNS) (sdnDNSModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	staged := sdnPendingMap(entry.Pending)
	state := sdnDNSModel{
		ID:            types.StringValue(entry.DNS),
		DNS:           types.StringValue(entry.DNS),
		Type:          types.StringValue(entry.Type),
		URL:           sdnStagedString(entry.URL, staged["url"]),
		Key:           sdnStagedString(entry.Key, staged["key"]),
		TTL:           sdnStagedInt64(entry.TTL.Ptr(), staged["ttl"]),
		ReverseMaskV6: sdnStagedInt64(entry.ReverseMaskV6.Ptr(), staged["reversemaskv6"]),
		Fingerprint:   sdnStagedString(entry.Fingerprint, staged["fingerprint"]),
		State:         stringOrNull(entry.State),
		Pending:       types.MapNull(types.StringType),
	}
	if len(staged) > 0 {
		pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		diags.Append(pendingDiags...)
		state.Pending = pending
	}
	return state, diags
}

func (r *SDNDNSResource) storeManagedFields(ctx context.Context, config sdnDNSModel, private nodeNetworkPrivateData, diags *diag.Diagnostics) {
	managedFields, err := json.Marshal(sdnDNSManagedFields(config))
	if err != nil {
		diags.AddError("Unable to Store SDN DNS State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, sdnDNSManagedFieldsKey, managedFields)...)
}

func readSDNDNSManagedFields(ctx context.Context, private nodeNetworkPrivateData) ([]string, diag.Diagnostics) {
	value, diags := private.GetKey(ctx, sdnDNSManagedFieldsKey)
	if diags.HasError() || len(value) == 0 {
		return nil, diags
	}
	var fields []string
	if err := json.Unmarshal(value, &fields); err != nil {
		diags.AddError("Unable to Read SDN DNS State", fmt.Sprintf("unable to decode managed fields: %v", err))
	}
	return fields, diags
}
