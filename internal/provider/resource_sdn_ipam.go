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

var _ resource.Resource = &SDNIPAMResource{}
var _ resource.ResourceWithImportState = &SDNIPAMResource{}
var _ resource.ResourceWithValidateConfig = &SDNIPAMResource{}

const sdnIPAMManagedFieldsKey = "sdn-ipam-managed-fields"

const (
	sdnIPAMTypePVE     = "pve"
	sdnIPAMTypeNetbox  = "netbox"
	sdnIPAMTypePHPIPAM = "phpipam"
)

var (
	sdnIPAMIDPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]*[a-zA-Z0-9]$`)
	sdnIPAMTypes     = []string{sdnIPAMTypePVE, sdnIPAMTypeNetbox, sdnIPAMTypePHPIPAM}
)

// sdnIPAMTypeFields lists the optional fields each IPAM type accepts.
var sdnIPAMTypeFields = map[string][]string{
	sdnIPAMTypeNetbox:  {"url", "token", "fingerprint"},
	sdnIPAMTypePHPIPAM: {"url", "token", "section", "fingerprint"},
	sdnIPAMTypePVE:     {},
}

var sdnIPAMRequiredFields = map[string][]string{
	sdnIPAMTypeNetbox:  {"url", "token"},
	sdnIPAMTypePHPIPAM: {"url", "token", "section"},
}

type SDNIPAMResource struct {
	client *Client
}

type sdnIPAMModel struct {
	ID          types.String `tfsdk:"id"`
	IPAM        types.String `tfsdk:"ipam"`
	Type        types.String `tfsdk:"type"`
	URL         types.String `tfsdk:"url"`
	Token       types.String `tfsdk:"token"`
	Section     types.Int64  `tfsdk:"section"`
	Fingerprint types.String `tfsdk:"fingerprint"`
	State       types.String `tfsdk:"state"`
	Pending     types.Map    `tfsdk:"pending"`
}

func NewSDNIPAMResource() resource.Resource {
	return &SDNIPAMResource{}
}

func (r *SDNIPAMResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_ipam"
}

func (r *SDNIPAMResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox VE SDN IPAM through `/cluster/sdn/ipams`. Supported types are `pve` (built-in), `netbox` and `phpipam`. IPAMs stage like the other SDN objects and only take effect when `proxmox_sdn_apply` commits. A zone cannot change its IPAM once subnets exist.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, MarkdownDescription: "IPAM identifier (same as `ipam`).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"ipam":        schema.StringAttribute{Required: true, MarkdownDescription: "IPAM identifier: starts with a letter, only letters and digits, at least 2 characters. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"type":        schema.StringAttribute{Required: true, MarkdownDescription: "IPAM type: `pve`, `netbox`, or `phpipam`. Changes require replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"url":         schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "API URL of the external IPAM. Required for `netbox` and `phpipam`."},
			"token":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "API token of the external IPAM. Required for `netbox` and `phpipam`. Proxmox stores it in `/etc/pve/sdn/ipams.cfg`, not in a private file."},
			"section":     schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "phpIPAM section id. Required for `phpipam`."},
			"fingerprint": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "SHA-256 fingerprint of the external IPAM TLS certificate."},
			"state":       schema.StringAttribute{Computed: true, MarkdownDescription: "Proxmox staging state: `new` or `changed` while the IPAM is not committed yet."},
			"pending":     schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Properties Proxmox currently reports as staged but not committed, with `deleted` for removals."},
		},
	}
}

func (r *SDNIPAMResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SDNIPAMResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config sdnIPAMModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateSDNIPAMConfig(config)...)
}

func validateSDNIPAMConfig(config sdnIPAMModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if isConfiguredValue(config.IPAM) {
		ipam := config.IPAM.ValueString()
		if len(ipam) < 2 || !sdnIPAMIDPattern.MatchString(ipam) {
			diags.AddAttributeError(path.Root("ipam"), "Invalid SDN IPAM identifier", "ipam must start with a letter, contain only letters and digits, and be at least 2 characters long.")
		}
	}
	if isConfiguredValue(config.Section) && config.Section.ValueInt64() < 0 {
		diags.AddAttributeError(path.Root("section"), "Invalid IPAM section", "section must not be negative.")
	}
	if !isConfiguredValue(config.Type) {
		return diags
	}
	ipamType := config.Type.ValueString()
	if !slices.Contains(sdnIPAMTypes, ipamType) {
		diags.AddAttributeError(path.Root("type"), "Invalid SDN IPAM type", "type must be pve, netbox, or phpipam.")
		return diags
	}
	allowed := sdnIPAMTypeFields[ipamType]
	for _, field := range []struct {
		name       string
		configured bool
	}{
		{"url", isConfiguredValue(config.URL)},
		{"token", isConfiguredValue(config.Token)},
		{"section", isConfiguredValue(config.Section)},
		{"fingerprint", isConfiguredValue(config.Fingerprint)},
	} {
		if field.configured && !slices.Contains(allowed, field.name) {
			diags.AddAttributeError(path.Root(field.name), "Unsupported field for IPAM type", fmt.Sprintf("%s is not valid for %s IPAMs.", field.name, ipamType))
		}
	}
	for _, name := range sdnIPAMRequiredFields[ipamType] {
		configured := map[string]bool{
			"url":     isConfiguredValue(config.URL),
			"token":   isConfiguredValue(config.Token),
			"section": isConfiguredValue(config.Section),
		}[name]
		if !configured {
			diags.AddAttributeError(path.Root(name), "Missing IPAM field", fmt.Sprintf("%s is required for %s IPAMs.", name, ipamType))
		}
	}
	return diags
}

func sdnIPAMRequestFromModels(config, plan sdnIPAMModel) SDNIPAMRequest {
	request := SDNIPAMRequest{Type: plan.Type.ValueString()}
	if isConfiguredValue(config.URL) {
		request.URL = stringPointer(plan.URL)
	}
	if isConfiguredValue(config.Token) {
		request.Token = stringPointer(plan.Token)
	}
	if isConfiguredValue(config.Section) {
		request.Section = int64PointerValue(plan.Section)
	}
	if isConfiguredValue(config.Fingerprint) {
		request.Fingerprint = stringPointer(plan.Fingerprint)
	}
	return request
}

func sdnIPAMManagedFields(config sdnIPAMModel) []string {
	var fields []string
	for _, field := range []struct {
		name       string
		configured bool
	}{
		{"url", isConfiguredValue(config.URL)},
		{"token", isConfiguredValue(config.Token)},
		{"section", isConfiguredValue(config.Section)},
		{"fingerprint", isConfiguredValue(config.Fingerprint)},
	} {
		if field.configured {
			fields = append(fields, field.name)
		}
	}
	slices.Sort(fields)
	return fields
}

func (r *SDNIPAMResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config sdnIPAMModel
	var plan sdnIPAMModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	createReq := sdnIPAMRequestFromModels(config, plan)
	ipam := plan.IPAM.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		createReq.LockToken = token
		return r.client.CreateSDNIPAM(ctx, ipam, createReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, ipam)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNIPAMResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sdnIPAMModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshed, diags := r.readState(ctx, state.IPAM.ValueString())
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

func (r *SDNIPAMResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config sdnIPAMModel
	var plan sdnIPAMModel
	var prior sdnIPAMModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	previouslyManaged, diags := readSDNIPAMManagedFields(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetSDNIPAM(ctx, prior.IPAM.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN IPAM", err.Error())
		return
	}
	updateReq := sdnIPAMRequestFromModels(config, plan)
	updateReq.Delete = sdnDeleteKeys(sdnIPAMManagedFields(config), previouslyManaged)
	ipam := prior.IPAM.ValueString()
	if !withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		updateReq.LockToken = token
		if current.Digest != "" {
			updateReq.Digest = &current.Digest
		}
		return r.client.UpdateSDNIPAM(ctx, ipam, updateReq)
	}) {
		return
	}
	state, stateDiags := r.readState(ctx, ipam)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	r.storeManagedFields(ctx, config, resp.Private, &resp.Diagnostics)
}

func (r *SDNIPAMResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sdnIPAMModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.GetSDNIPAM(ctx, state.IPAM.ValueString()); err != nil {
		if errors.Is(err, errNotFound) {
			return
		}
		resp.Diagnostics.AddError("Unable to Read Current Proxmox SDN IPAM", err.Error())
		return
	}
	ipam := state.IPAM.ValueString()
	withSDNLock(ctx, r.client, &resp.Diagnostics, func(token string) error {
		return r.client.DeleteSDNIPAM(ctx, ipam, token)
	})
}

func (r *SDNIPAMResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if len(req.ID) < 2 || !sdnIPAMIDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Unexpected Import Identifier", "Expected an SDN IPAM identifier: a letter, letters and digits only, at least 2 characters.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ipam"), req.ID)...)
}

func (r *SDNIPAMResource) readState(ctx context.Context, ipam string) (sdnIPAMModel, diag.Diagnostics) {
	entry, err := r.client.GetSDNIPAM(ctx, ipam)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return sdnIPAMModel{ID: types.StringNull()}, nil
		}
		var diags diag.Diagnostics
		diags.AddError("Unable to Read Proxmox SDN IPAM", fmt.Sprintf("Unable to read SDN IPAM %q: %s", ipam, err))
		return sdnIPAMModel{}, diags
	}
	if entry.State == "deleted" {
		return sdnIPAMModel{ID: types.StringNull()}, nil
	}
	return sdnIPAMStateFromAPI(ctx, entry)
}

func sdnIPAMStateFromAPI(ctx context.Context, entry SDNIPAM) (sdnIPAMModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	staged := sdnPendingMap(entry.Pending)
	state := sdnIPAMModel{
		ID:          types.StringValue(entry.IPAM),
		IPAM:        types.StringValue(entry.IPAM),
		Type:        types.StringValue(entry.Type),
		URL:         sdnStagedString(entry.URL, staged["url"]),
		Token:       sdnStagedString(entry.Token, staged["token"]),
		Section:     sdnStagedInt64(entry.Section.Ptr(), staged["section"]),
		Fingerprint: sdnStagedString(entry.Fingerprint, staged["fingerprint"]),
		State:       stringOrNull(entry.State),
		Pending:     types.MapNull(types.StringType),
	}
	if len(staged) > 0 {
		pending, pendingDiags := types.MapValueFrom(ctx, types.StringType, staged)
		diags.Append(pendingDiags...)
		state.Pending = pending
	}
	return state, diags
}

func (r *SDNIPAMResource) storeManagedFields(ctx context.Context, config sdnIPAMModel, private nodeNetworkPrivateData, diags *diag.Diagnostics) {
	managedFields, err := json.Marshal(sdnIPAMManagedFields(config))
	if err != nil {
		diags.AddError("Unable to Store SDN IPAM State", fmt.Sprintf("unable to encode managed fields: %v", err))
		return
	}
	diags.Append(private.SetKey(ctx, sdnIPAMManagedFieldsKey, managedFields)...)
}

func readSDNIPAMManagedFields(ctx context.Context, private nodeNetworkPrivateData) ([]string, diag.Diagnostics) {
	value, diags := private.GetKey(ctx, sdnIPAMManagedFieldsKey)
	if diags.HasError() || len(value) == 0 {
		return nil, diags
	}
	var fields []string
	if err := json.Unmarshal(value, &fields); err != nil {
		diags.AddError("Unable to Read SDN IPAM State", fmt.Sprintf("unable to decode managed fields: %v", err))
	}
	return fields, diags
}

// sdnDeleteKeys returns the previously managed keys that are no longer
// configured, for the SDN `delete` field.
func sdnDeleteKeys(current, previouslyManaged []string) []string {
	var deleted []string
	for _, key := range previouslyManaged {
		if !slices.Contains(current, key) {
			deleted = append(deleted, key)
		}
	}
	slices.Sort(deleted)
	return deleted
}
