// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &QemuVMsDataSource{}

type QemuVMsDataSource struct {
	client *Client
}

type QemuVMsDataSourceModel struct {
	ID       types.String              `tfsdk:"id"`
	Template types.Bool                `tfsdk:"template"`
	Name     types.String              `tfsdk:"name"`
	Node     types.String              `tfsdk:"node"`
	VMs      []QemuVMsDataSourceRecord `tfsdk:"vms"`
}

type QemuVMsDataSourceRecord struct {
	VMID      types.Int64   `tfsdk:"vm_id"`
	Name      types.String  `tfsdk:"name"`
	Node      types.String  `tfsdk:"node"`
	Template  types.Bool    `tfsdk:"template"`
	Status    types.String  `tfsdk:"status"`
	Tags      types.String  `tfsdk:"tags"`
	Pool      types.String  `tfsdk:"pool"`
	MaxCPU    types.Float64 `tfsdk:"max_cpu"`
	MaxMemory types.Int64   `tfsdk:"max_memory"`
	MaxDisk   types.Int64   `tfsdk:"max_disk"`
}

func NewQemuVMsDataSource() datasource.DataSource {
	return &QemuVMsDataSource{}
}

func (d *QemuVMsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_vms"
}

func (d *QemuVMsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Lists QEMU virtual machines and templates from `/cluster/resources`. The optional `template`, `name`, and `node` filters are applied by the provider. Guests the authenticated identity cannot audit (VM.Audit) are omitted by Proxmox, and `name`/`template` come from RRD stats, so a guest created in the last minute may be missing from or incomplete in the results.",
		Attributes: map[string]datasourceschema.Attribute{
			"id": datasourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique identifier for this data source call.",
			},
			"template": datasourceschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Restrict the result to templates (`true`) or non-template guests (`false`). A guest whose `template` flag is not reported yet counts as a non-template, matching the Proxmox API default of `0`.",
			},
			"name": datasourceschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Exact, case-sensitive guest name match. An empty value disables this filter.",
			},
			"node": datasourceschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Exact match against the hosting node. An empty value disables this filter.",
			},
			"vms": qemuVMsDataSourceAttribute(),
		},
	}
}

func qemuVMsDataSourceAttribute() datasourceschema.ListNestedAttribute {
	return datasourceschema.ListNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Matching QEMU guests, sorted by `vm_id` ascending.",
		NestedObject: datasourceschema.NestedAttributeObject{
			Attributes: map[string]datasourceschema.Attribute{
				"vm_id":      datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Numeric VMID, usable as `clone.source_vmid` or `proxmox_qemu_vm.vm_id`."},
				"name":       datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Guest name, when reported."},
				"node":       datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Hosting node."},
				"template":   datasourceschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the guest is a template; null while Proxmox has not reported the flag yet."},
				"status":     datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Runtime status."},
				"tags":       datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Guest tags as a semicolon-separated string, when set."},
				"pool":       datasourceschema.StringAttribute{Computed: true, MarkdownDescription: "Resource pool, when set and auditable."},
				"max_cpu":    datasourceschema.Float64Attribute{Computed: true, MarkdownDescription: "Allocated vCPUs."},
				"max_memory": datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Allocated memory in bytes."},
				"max_disk":   datasourceschema.Int64Attribute{Computed: true, MarkdownDescription: "Allocated root disk size in bytes."},
			},
		},
	}
}

func (d *QemuVMsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *QemuVMsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config QemuVMsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resources, err := d.client.ClusterResources(ctx, "vm")
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox QEMU VMs", fmt.Sprintf("Unable to call `/cluster/resources`: %s", err))
		return
	}

	filterName := stringValue(config.Name)
	filterNode := stringValue(config.Node)
	filterTemplates := !config.Template.IsNull() && config.Template.ValueBool()
	filterNonTemplates := !config.Template.IsNull() && !config.Template.ValueBool()

	records := make([]QemuVMsDataSourceRecord, 0, len(resources))
	for _, resource := range resources {
		if resource.Type != "qemu" {
			continue
		}
		if filterNode != "" && resource.Node != filterNode {
			continue
		}
		if filterName != "" && resource.Name != filterName {
			continue
		}
		isTemplate := resource.Template.Ptr() != nil && *resource.Template.Ptr()
		if filterTemplates && !isTemplate {
			continue
		}
		if filterNonTemplates && isTemplate {
			continue
		}

		records = append(records, QemuVMsDataSourceRecord{
			VMID:      int64OrNull(resource.VMID),
			Name:      stringOrNull(resource.Name),
			Node:      stringOrNull(resource.Node),
			Template:  boolOrNull(resource.Template.Ptr()),
			Status:    stringOrNull(resource.Status),
			Tags:      stringOrNull(resource.Tags),
			Pool:      stringOrNull(resource.Pool),
			MaxCPU:    float64OrNull(resource.MaxCPU),
			MaxMemory: int64OrNull(resource.MaxMemory),
			MaxDisk:   int64OrNull(resource.MaxDisk),
		})
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].VMID.ValueInt64() < records[j].VMID.ValueInt64()
	})

	state := QemuVMsDataSourceModel{
		ID:       types.StringValue("qemu_vms"),
		Template: config.Template,
		Name:     config.Name,
		Node:     config.Node,
		VMs:      records,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
