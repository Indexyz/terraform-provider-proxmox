// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccProxmoxE2ESmoke(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: `
data "proxmox_version" "current" {}

data "proxmox_nodes" "current" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.proxmox_version.current", "version"),
					resource.TestCheckResourceAttrWith("data.proxmox_nodes.current", "nodes.#", func(value string) error {
						count, err := strconv.Atoi(value)
						if err != nil {
							return fmt.Errorf("nodes.# is not an integer: %w", err)
						}
						if count < 1 {
							return fmt.Errorf("expected at least one Proxmox node, got %d", count)
						}
						return nil
					}),
				),
			},
		},
	})
}
