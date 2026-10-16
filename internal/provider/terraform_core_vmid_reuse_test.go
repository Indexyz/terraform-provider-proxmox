// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Proxmox hands a freed VMID to the next guest that asks for one. A guest
// tracked in state that disappears out of band can therefore be replaced by
// an unrelated guest at the same node/VMID, which a VMID-only refresh would
// adopt and a destroy would then delete (observed in production, 2026-10-16).
// The recorded name is the guest's identity: a different guest at the tracked
// VMID means the tracked guest is gone.
const terraformCoreReuseMainTFTemplate = `terraform {
  required_providers {
    proxmox = {
      source = "indexyz/proxmox"
    }
  }
}

variable "api_token_secret" {
  type      = string
  sensitive = true
}

variable "name" {
  type = string
}

provider "proxmox" {
  endpoint         = "{{ENDPOINT}}"
  api_token_id     = "terraform@pve!provider"
  api_token_secret = var.api_token_secret
}

resource "proxmox_qemu_vm" "runner" {
  node        = "pve-1"
  vm_id_start = 100
  name        = var.name

  clone = {
    source_node = "pve-1"
    source_vmid = 9000
    full        = true
  }

  start_on_create = true
  stop_on_destroy = true
}
`

// reuseVMID models the VMID reuse: the tracked guest is gone and an unrelated
// running guest now holds its VMID.
func (p *terraformCorePVE) reuseVMID(vmID int64, name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.vms[vmID] = &terraformCoreVM{Name: name, Running: true}
}

func (p *terraformCorePVE) vmName(vmID int64) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	vm, ok := p.vms[vmID]
	if !ok {
		return "", false
	}
	return vm.Name, true
}

// TestTerraformCoreVMIDReuseIsNeverAdopted proves, through the real Terraform
// CLI, that a guest at the tracked VMID with a different name is treated as
// the tracked guest's deletion by refresh and by a refresh-less destroy, and
// that renaming a guest through Terraform keeps its identity even when the
// rename's reply is lost.
func TestTerraformCoreVMIDReuseIsNeverAdopted(t *testing.T) {
	terraformBin, err := exec.LookPath("terraform")
	if err != nil {
		t.Skip("terraform CLI not available in PATH; harness cannot run")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not available in PATH; harness cannot build the provider")
	}
	modOut, err := exec.Command(goBin, "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	moduleRoot := filepath.Dir(strings.TrimSpace(string(modOut)))

	pve := newTerraformCorePVE()
	server := httptest.NewServer(pve)
	defer server.Close()

	base := t.TempDir()
	binDir := filepath.Join(base, "bin")
	build := exec.Command(goBin, "build", "-o", filepath.Join(binDir, "terraform-provider-proxmox"), ".")
	build.Dir = moduleRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build provider binary: %v\n%s", err, out)
	}
	workdir := filepath.Join(base, "config")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("create terraform workdir: %v", err)
	}
	mainTF := strings.ReplaceAll(terraformCoreReuseMainTFTemplate, "{{ENDPOINT}}", server.URL)
	files := map[string]string{
		"main.tf":      mainTF,
		"terraform.rc": fmt.Sprintf("provider_installation {\n  dev_overrides {\n    \"registry.terraform.io/indexyz/proxmox\" = %q\n  }\n}\n", binDir),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(workdir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	terraform := func(name string, wantExit int, args ...string) string {
		t.Helper()
		t.Logf("terraform %v (name=%s)", args, name)
		cmd := exec.Command(terraformBin, args...)
		cmd.Dir = workdir
		cmd.Env = append(os.Environ(),
			"TF_CLI_CONFIG_FILE="+filepath.Join(workdir, "terraform.rc"),
			"TF_VAR_api_token_secret=token-secret",
			"TF_VAR_name="+name,
		)
		out, err := cmd.CombinedOutput()
		exitCode := 0
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if err != nil {
			t.Fatalf("terraform %v: %v\n%s", args, err, out)
		}
		if exitCode != wantExit {
			t.Fatalf("terraform %v exit code = %d, want %d\n%s", args, exitCode, wantExit, out)
		}
		return string(out)
	}
	requireForeignUntouched := func(vmID int64, name string) {
		t.Helper()
		events := pve.snapshot()
		requireTerraformCoreAbsent(t, events, fmt.Sprintf("stop:%d", vmID))
		requireTerraformCoreAbsent(t, events, fmt.Sprintf("vm-delete:%d", vmID))
		if got, ok := pve.vmName(vmID); !ok || got != name {
			t.Fatalf("foreign guest %d = (%q, %v), want %q intact", vmID, got, ok, name)
		}
	}

	// A rename is an in-place update of the same guest. The PUT lands but
	// its reply is lost: the apply fails, yet the next refresh must still
	// recognize the renamed guest instead of treating it as gone.
	terraform("runner-a", 0, "apply", "-auto-approve", "-input=false")
	requireTerraformCoreVMID(t, workdir, "runner", 100)
	pve.mu.Lock()
	pve.FailNextConfigAfterMutation = true
	pve.mu.Unlock()
	terraform("runner-b", 1, "apply", "-auto-approve", "-input=false")
	if got, _ := pve.vmName(100); got != "runner-b" {
		t.Fatalf("rename did not reach the guest: %q", got)
	}
	terraform("runner-b", 0, "plan", "-detailed-exitcode", "-input=false")
	terraform("runner-b", 0, "apply", "-auto-approve", "-input=false")
	requireTerraformCoreVMID(t, workdir, "runner", 100)

	// The tracked guest vanished and another guest took VMID 100: refresh
	// drops the tracked guest, so destroy has nothing to stop or delete.
	pve.reuseVMID(100, "someone-else")
	pve.resetEvents()
	terraform("runner-b", 0, "destroy", "-auto-approve", "-input=false")
	requireForeignUntouched(100, "someone-else")

	// Without refresh, Delete itself must check identity before acting.
	terraform("runner-c", 0, "apply", "-auto-approve", "-input=false")
	requireTerraformCoreVMID(t, workdir, "runner", 101)
	pve.reuseVMID(101, "someone-else-too")
	pve.resetEvents()
	terraform("runner-c", 0, "destroy", "-refresh=false", "-auto-approve", "-input=false")
	requireForeignUntouched(101, "someone-else-too")
	requireForeignUntouched(100, "someone-else")
	if state := terraform("runner-c", 0, "state", "list"); strings.TrimSpace(state) != "" {
		t.Fatalf("state still tracks resources: %q", state)
	}

	pve.assertNoFailures(t)
}
