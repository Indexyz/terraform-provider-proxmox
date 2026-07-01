// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package ci

import (
	"os"
	"strings"
	"testing"
)

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
