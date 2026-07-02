// Copyright IBM Corp. 2021, 2026
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

var _ datasource.DataSource = &LXCContainerDataSource{}

type LXCContainerDataSource struct {
	client *Client
}

func NewLXCContainerDataSource() datasource.DataSource {
	return &LXCContainerDataSource{}
}

func (d *LXCContainerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lxc_container"
}

func (d *LXCContainerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads a single Proxmox VE LXC container from `/nodes/{node}/lxc/{vmid}/config` and `/status/current`.",
		Attributes:          lxcContainerDataSourceAttributes(),
	}
}

func (d *LXCContainerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *LXCContainerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config lxcContainerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vmID := config.VMID.ValueInt64()
	node := config.Node.ValueString()

	lxcConfig, err := d.client.GetLXCContainerConfig(ctx, node, vmID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			resp.Diagnostics.AddError(
				"Proxmox LXC Container Not Found",
				fmt.Sprintf("LXC container %q was not found on node %q.", strconv.FormatInt(vmID, 10), node),
			)
			return
		}
		resp.Diagnostics.AddError("Unable to Read Proxmox LXC Container Config", err.Error())
		return
	}

	status, err := d.client.GetLXCContainerStatus(ctx, node, vmID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			resp.Diagnostics.AddError(
				"Proxmox LXC Container Not Found",
				fmt.Sprintf("LXC container %q was not found on node %q.", strconv.FormatInt(vmID, 10), node),
			)
			return
		}
		resp.Diagnostics.AddError("Unable to Read Proxmox LXC Container Status", err.Error())
		return
	}

	state, diags := lxcContainerStateFromAPI(ctx, node, vmID, lxcConfig, status, nil)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
