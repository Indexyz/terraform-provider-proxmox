// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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

func TestAccProxmoxE2EQemuVMTaskWaiting(t *testing.T) {
	suffix := strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	sourceName := "e2e-qemu-source-" + suffix
	cloneName := "e2e-qemu-clone-" + suffix
	sourceVMID := int64(acctest.RandIntRange(900000, 999999))
	cloneVMID := sourceVMID + 1
	var client *Client
	var node string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheck(t)

			cfg, diags := providerConfigFromModel(ProxmoxProviderModel{}, "test")
			if diags.HasError() {
				t.Fatalf("build Proxmox client configuration from acceptance environment: %v", diags)
			}

			var err error
			client, err = NewClient(context.Background(), cfg)
			if err != nil {
				t.Fatalf("build Proxmox client from acceptance environment: %v", err)
			}

			nodes, err := client.Nodes(context.Background())
			if err != nil {
				t.Fatalf("discover Proxmox node for QEMU acceptance test: %v", err)
			}
			if len(nodes) == 0 {
				t.Fatal("discover Proxmox node for QEMU acceptance test: no nodes returned")
			}
			node = nodes[0].Name

			for _, vmID := range []int64{cloneVMID, sourceVMID} {
				if _, err := client.GetQemuVMConfig(context.Background(), node, vmID); err == nil {
					t.Fatalf("QEMU VM %s/%d already exists; refusing to reuse its VMID", node, vmID)
				} else if !errors.Is(err, errNotFound) {
					t.Fatalf("verify QEMU VM %s/%d is absent before acceptance test: %v", node, vmID, err)
				}
			}

			t.Cleanup(func() {
				testAccCleanupOwnedQemuVM(t, client, node, cloneVMID, cloneName)
				testAccCleanupOwnedQemuVM(t, client, node, sourceVMID, sourceName)
			})
		},
		CheckDestroy: func(_ *terraform.State) error {
			for _, vmID := range []int64{cloneVMID, sourceVMID} {
				if _, err := client.GetQemuVMConfig(context.Background(), node, vmID); err == nil {
					return fmt.Errorf("QEMU VM %s/%d still exists after Terraform destroy", node, vmID)
				} else if !errors.Is(err, errNotFound) {
					return fmt.Errorf("verify QEMU VM %s/%d was destroyed: %w", node, vmID, err)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: testAccProxmoxE2EQemuVMTaskWaitingConfig(sourceVMID, cloneVMID, sourceName, cloneName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckResourceNode("proxmox_qemu_vm.source", &node),
					resource.TestCheckResourceAttr("proxmox_qemu_vm.source", "vm_id", strconv.FormatInt(sourceVMID, 10)),
					resource.TestCheckResourceAttrWith("proxmox_qemu_vm.source", "id", func(value string) error {
						if value != fmt.Sprintf("%s/%d", node, sourceVMID) {
							return fmt.Errorf("expected source id %q, got %q", fmt.Sprintf("%s/%d", node, sourceVMID), value)
						}
						return nil
					}),
					resource.TestCheckResourceAttr("proxmox_qemu_vm.source", "name", sourceName),
					resource.TestCheckResourceAttr("proxmox_qemu_vm.source", "status", "stopped"),
					testAccCheckResourceNode("proxmox_qemu_vm.clone", &node),
					resource.TestCheckResourceAttr("proxmox_qemu_vm.clone", "vm_id", strconv.FormatInt(cloneVMID, 10)),
					resource.TestCheckResourceAttrWith("proxmox_qemu_vm.clone", "id", func(value string) error {
						if value != fmt.Sprintf("%s/%d", node, cloneVMID) {
							return fmt.Errorf("expected clone id %q, got %q", fmt.Sprintf("%s/%d", node, cloneVMID), value)
						}
						return nil
					}),
					resource.TestCheckResourceAttr("proxmox_qemu_vm.clone", "name", cloneName),
					resource.TestCheckResourceAttr("proxmox_qemu_vm.clone", "status", "stopped"),
					resource.TestCheckResourceAttrWith("proxmox_qemu_vm.clone", "clone.source_node", func(value string) error {
						if value != node {
							return fmt.Errorf("expected clone source node %q, got %q", node, value)
						}
						return nil
					}),
					resource.TestCheckResourceAttr("proxmox_qemu_vm.clone", "clone.source_vmid", strconv.FormatInt(sourceVMID, 10)),
					resource.TestCheckResourceAttr("proxmox_qemu_vm.clone", "clone.full", "true"),
				),
			},
		},
	})
}

func testAccCheckResourceNode(resourceName string, node *string) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(resourceName, "node", func(value string) error {
		if value != *node {
			return fmt.Errorf("expected node %q, got %q", *node, value)
		}
		return nil
	})
}

func testAccCleanupOwnedQemuVM(t *testing.T, client *Client, node string, vmID int64, ownedName string) {
	t.Helper()

	config, err := client.GetQemuVMConfig(context.Background(), node, vmID)
	if errors.Is(err, errNotFound) {
		return
	}
	if err != nil {
		t.Errorf("read QEMU VM %s/%d during acceptance cleanup: %v", node, vmID, err)
		return
	}
	if config.Name != ownedName {
		t.Errorf("refusing to delete QEMU VM %s/%d during acceptance cleanup: expected owned name %q, got %q", node, vmID, ownedName, config.Name)
		return
	}

	if err := client.DeleteQemuVM(context.Background(), node, vmID); err != nil && !errors.Is(err, errNotFound) {
		t.Errorf("delete owned QEMU VM %s/%d during acceptance cleanup: %v", node, vmID, err)
		return
	}
	if _, err := client.GetQemuVMConfig(context.Background(), node, vmID); err == nil {
		t.Errorf("verify owned QEMU VM %s/%d deletion during acceptance cleanup: VM still exists", node, vmID)
	} else if !errors.Is(err, errNotFound) {
		t.Errorf("verify owned QEMU VM %s/%d deletion during acceptance cleanup: %v", node, vmID, err)
	}
}

func testAccProxmoxE2EQemuVMTaskWaitingConfig(sourceVMID, cloneVMID int64, sourceName, cloneName string) string {
	return fmt.Sprintf(`
data "proxmox_nodes" "current" {}

locals {
  node = data.proxmox_nodes.current.nodes[0].name
}

resource "proxmox_qemu_vm" "source" {
  node  = local.node
  vm_id = %d
  name  = %q
}

resource "proxmox_qemu_vm" "clone" {
  node  = local.node
  vm_id = %d
  name  = %q

  clone = {
    source_node = local.node
    source_vmid = proxmox_qemu_vm.source.vm_id
    full        = true
  }
}
`, sourceVMID, sourceName, cloneVMID, cloneName)
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
