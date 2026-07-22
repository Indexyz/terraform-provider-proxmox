// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package ci

import (
	"os"
	"strings"
	"testing"
)

func TestProxmoxE2EWorkflowPinsPVE9Environment(t *testing.T) {
	workflow, err := os.ReadFile("../../.github/workflows/test.yml")
	if err != nil {
		t.Fatalf("read test workflow: %v", err)
	}

	content := string(workflow)
	for _, required := range []string{
		"runs-on: ubuntu-24.04",
		"PROXMOX_E2E_ISO_VERSION: '9.2-1'",
		"PROXMOX_E2E_ISO_SHA256: 4e88fe416df9b527624a175f24c9aa07c714d3332afb1ee3dbf3879573ef2c6c",
		"PROXMOX_E2E_ASSISTANT_VERSION: '9.2.7'",
		"PROXMOX_E2E_ASSISTANT_SHA256: 92d34cd218bcabea83b72dd45f6e92ab571a221a8cbe1b548076685bc9234f15",
		"/dists/trixie/pve-no-subscription/",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("Proxmox e2e workflow must include %q to test against PVE 9", required)
		}
	}
}

func TestProxmoxE2EWorkflowRunsOnlyOwnedE2ETests(t *testing.T) {
	workflow, err := os.ReadFile("../../.github/workflows/test.yml")
	if err != nil {
		t.Fatalf("read test workflow: %v", err)
	}

	const command = "go test -v -cover -timeout 120m -run '^TestAccProxmoxE2E(ReadOnly|CRUD|QemuVMTaskWaiting)$' ./internal/provider/"
	if !strings.Contains(string(workflow), command) {
		t.Fatalf("Proxmox e2e workflow must run the explicit read-only, CRUD, and QEMU task-waiting tests with coverage")
	}
}

func TestProxmoxE2EWorkflowGrantsRunnerKVMAccess(t *testing.T) {
	workflow, err := os.ReadFile("../../.github/workflows/test.yml")
	if err != nil {
		t.Fatalf("read test workflow: %v", err)
	}

	content := string(workflow)
	for _, required := range []string{
		"Enable KVM access",
		"sudo chmod a+rw /dev/kvm",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("Proxmox e2e workflow must include %q so QEMU can use KVM instead of slow TCG", required)
		}
	}
}
