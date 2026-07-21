// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccProxmoxE2EReadOnly(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: `
data "proxmox_version" "current" {}

data "proxmox_nodes" "current" {}

locals {
  node = data.proxmox_nodes.current.nodes[0].name
}

data "proxmox_node" "current" {
  node = local.node
}

data "proxmox_node_dns" "current" {
  node = local.node
}

data "proxmox_node_time" "current" {
  node = local.node
}

data "proxmox_cluster_resources" "nodes" {
  type = "node"
}

data "proxmox_cluster_metrics_servers" "current" {}

data "proxmox_storage" "local" {
  storage = "local"
}

data "proxmox_storages" "current" {}

data "proxmox_pools" "current" {}

data "proxmox_groups" "current" {}

data "proxmox_role" "admin" {
  role_id = "PVEAdmin"
}

data "proxmox_roles" "current" {}

data "proxmox_user" "root" {
  user_id = "root@pam"
}

data "proxmox_users" "current" {}

data "proxmox_realm" "pam" {
  realm = "pam"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.proxmox_version.current", "version"),
					resource.TestCheckResourceAttrWith("data.proxmox_version.current", "release", func(value string) error {
						if !strings.HasPrefix(value, "9.") {
							return fmt.Errorf("expected Proxmox VE 9 release, got %q", value)
						}
						return nil
					}),
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
					resource.TestCheckResourceAttrSet("data.proxmox_nodes.current", "nodes.0.name"),
					resource.TestCheckResourceAttrSet("data.proxmox_node.current", "cpu_model"),
					resource.TestCheckResourceAttrSet("data.proxmox_node.current", "memory_total"),
					resource.TestCheckResourceAttrSet("data.proxmox_node.current", "pve_version"),
					resource.TestCheckResourceAttrSet("data.proxmox_node_dns.current", "id"),
					resource.TestCheckResourceAttrSet("data.proxmox_node_time.current", "timezone"),
					resource.TestCheckResourceAttr("data.proxmox_cluster_resources.nodes", "type", "node"),
					resource.TestCheckResourceAttrSet("data.proxmox_cluster_resources.nodes", "resources.0.node"),
					resource.TestCheckResourceAttr("data.proxmox_cluster_metrics_servers.current", "id", "cluster_metrics_servers"),
					resource.TestCheckResourceAttr("data.proxmox_storage.local", "id", "local"),
					resource.TestCheckResourceAttr("data.proxmox_storage.local", "type", "dir"),
					resource.TestCheckResourceAttrSet("data.proxmox_storages.current", "storages.0.storage"),
					resource.TestCheckResourceAttr("data.proxmox_pools.current", "id", "pools"),
					resource.TestCheckResourceAttr("data.proxmox_groups.current", "id", "groups"),
					resource.TestCheckResourceAttr("data.proxmox_role.admin", "id", "PVEAdmin"),
					resource.TestCheckResourceAttrSet("data.proxmox_roles.current", "roles.0.role_id"),
					resource.TestCheckResourceAttr("data.proxmox_user.root", "id", "root@pam"),
					resource.TestCheckResourceAttrSet("data.proxmox_users.current", "users.0.user_id"),
					resource.TestCheckResourceAttr("data.proxmox_realm.pam", "id", "pam"),
					resource.TestCheckResourceAttr("data.proxmox_realm.pam", "realm", "pam"),
					resource.TestCheckResourceAttr("data.proxmox_realm.pam", "type", "pam"),
				),
			},
		},
	})
}

func TestAccProxmoxE2ECRUD(t *testing.T) {
	suffix := strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	poolID := "e2epool" + suffix
	groupID := "e2egroup" + suffix
	roleID := "E2ERole" + suffix
	userID := "e2euser" + suffix + "@pve"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: testAccProxmoxE2ECRUDConfig(poolID, groupID, roleID, userID, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("proxmox_pool.managed", "id", poolID),
					resource.TestCheckResourceAttr("proxmox_pool.managed", "comment", "Created by the Proxmox e2e test"),
					resource.TestCheckResourceAttr("proxmox_group.managed", "id", groupID),
					resource.TestCheckResourceAttr("proxmox_group.managed", "comment", "Created by the Proxmox e2e test"),
					resource.TestCheckResourceAttr("proxmox_role.managed", "id", roleID),
					resource.TestCheckResourceAttr("proxmox_role.managed", "privs", "Sys.Audit"),
					resource.TestCheckResourceAttr("proxmox_user.managed", "id", userID),
					resource.TestCheckResourceAttr("proxmox_user.managed", "email", "created@example.invalid"),
					resource.TestCheckResourceAttr("proxmox_user.managed", "groups", groupID),
					resource.TestCheckResourceAttr("proxmox_user_token.managed", "id", userID+"/e2e"),
					resource.TestCheckResourceAttr("proxmox_user_token.managed", "full_token_id", userID+"!e2e"),
					resource.TestCheckResourceAttrSet("proxmox_user_token.managed", "value"),
					resource.TestCheckResourceAttr("proxmox_user_token.managed", "privsep", "true"),
					resource.TestCheckResourceAttr("proxmox_acl.managed", "id", "/pool/"+poolID),
					resource.TestCheckResourceAttr("proxmox_acl.managed", "propagate", "true"),
					resource.TestCheckResourceAttr("proxmox_acl.managed", "groups.#", "1"),
					resource.TestCheckResourceAttr("data.proxmox_pool.managed", "id", poolID),
					resource.TestCheckResourceAttr("data.proxmox_group.managed", "id", groupID),
					resource.TestCheckResourceAttr("data.proxmox_group.managed", "members.0", userID),
					resource.TestCheckResourceAttr("data.proxmox_role.managed", "id", roleID),
					resource.TestCheckResourceAttr("data.proxmox_user.managed", "id", userID),
				),
			},
			{
				Config: testAccProxmoxE2ECRUDConfig(poolID, groupID, roleID, userID, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("proxmox_pool.managed", "comment", "Updated by the Proxmox e2e test"),
					resource.TestCheckResourceAttr("proxmox_group.managed", "comment", "Updated by the Proxmox e2e test"),
					resource.TestCheckResourceAttr("proxmox_role.managed", "privs", "Pool.Audit"),
					resource.TestCheckResourceAttr("proxmox_user.managed", "comment", "Updated by the Proxmox e2e test"),
					resource.TestCheckResourceAttr("proxmox_user.managed", "email", "updated@example.invalid"),
					resource.TestCheckResourceAttr("proxmox_user_token.managed", "comment", "Updated by the Proxmox e2e test"),
					resource.TestCheckResourceAttr("proxmox_user_token.managed", "privsep", "false"),
					resource.TestCheckResourceAttrSet("proxmox_user_token.managed", "value"),
					resource.TestCheckResourceAttr("proxmox_acl.managed", "propagate", "false"),
					resource.TestCheckResourceAttr("proxmox_acl.managed", "groups.#", "0"),
					resource.TestCheckResourceAttr("proxmox_acl.managed", "users.#", "1"),
					resource.TestCheckResourceAttr("data.proxmox_pool.managed", "comment", "Updated by the Proxmox e2e test"),
					resource.TestCheckResourceAttr("data.proxmox_group.managed", "comment", "Updated by the Proxmox e2e test"),
					resource.TestCheckResourceAttr("data.proxmox_role.managed", "privs", "Pool.Audit"),
					resource.TestCheckResourceAttr("data.proxmox_user.managed", "email", "updated@example.invalid"),
				),
			},
		},
	})
}

func testAccProxmoxE2ECRUDConfig(poolID, groupID, roleID, userID string, updated bool) string {
	comment := "Created by the Proxmox e2e test"
	email := "created@example.invalid"
	privs := "Sys.Audit"
	tokenPrivsep := true
	aclPropagate := true
	aclGroups := "[proxmox_group.managed.group_id]"
	if updated {
		comment = "Updated by the Proxmox e2e test"
		email = "updated@example.invalid"
		privs = "Pool.Audit"
		tokenPrivsep = false
		aclPropagate = false
		aclGroups = "[]"
	}

	return fmt.Sprintf(`
resource "proxmox_pool" "managed" {
  pool_id = %q
  comment = %q
}

resource "proxmox_group" "managed" {
  group_id = %q
  comment  = %q
}

resource "proxmox_role" "managed" {
  role_id = %q
  privs   = %q
}

resource "proxmox_user" "managed" {
  user_id   = %q
  comment   = %q
  email     = %q
  firstname = "Proxmox"
  lastname  = "E2E"
  groups    = proxmox_group.managed.group_id
}

resource "proxmox_user_token" "managed" {
  user_id  = proxmox_user.managed.user_id
  token_id = "e2e"
  comment  = %q
  privsep  = %t
}

resource "proxmox_acl" "managed" {
  path      = "/pool/${proxmox_pool.managed.pool_id}"
  propagate = %t
  roles     = [proxmox_role.managed.role_id]
  users     = [proxmox_user.managed.user_id]
  groups    = %s
}

data "proxmox_pool" "managed" {
  pool_id = proxmox_pool.managed.pool_id
}

data "proxmox_group" "managed" {
  group_id   = proxmox_group.managed.group_id
  depends_on = [proxmox_user.managed]
}

data "proxmox_role" "managed" {
  role_id = proxmox_role.managed.role_id
}

data "proxmox_user" "managed" {
  user_id = proxmox_user.managed.user_id
}
`, poolID, comment, groupID, comment, roleID, privs, userID, comment, email, comment, tokenPrivsep, aclPropagate, aclGroups)
}
