// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

var _ datasource.DataSource = &QemuVMDataSource{}

type QemuVMDataSource struct {
	client *Client
}

func NewQemuVMDataSource() datasource.DataSource {
	return &QemuVMDataSource{}
}

func (d *QemuVMDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qemu_vm"
}

func (d *QemuVMDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads a single Proxmox VE QEMU virtual machine from `/nodes/{node}/qemu/{vmid}/config` and `/status/current`, including typed advanced config when supported.",
		Attributes:          qemuVMDataSourceAttributes(),
	}
}

func (d *QemuVMDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *QemuVMDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config qemuVMModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vmID := config.VMID.ValueInt64()
	node := config.Node.ValueString()

	qemuConfig, err := d.client.GetQemuVMConfig(ctx, node, vmID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			resp.Diagnostics.AddError(
				"Proxmox QEMU VM Not Found",
				fmt.Sprintf("QEMU virtual machine %q was not found on node %q.", strconv.FormatInt(vmID, 10), node),
			)
			return
		}
		resp.Diagnostics.AddError("Unable to Read Proxmox QEMU VM Config", err.Error())
		return
	}

	status, err := d.client.GetQemuVMStatus(ctx, node, vmID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxmox QEMU VM Status", err.Error())
		return
	}

	state, diags := qemuVMStateFromAPI(ctx, node, vmID, qemuConfig, status, nil)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
