// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerraformCoreSeedInPlaceUpdate(t *testing.T) {
	terraformBin, err := exec.LookPath("terraform")
	if err != nil {
		t.Skip("terraform CLI unavailable")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go unavailable")
	}
	pve := newTerraformCorePVE()
	server := httptest.NewServer(pve)
	defer server.Close()
	base := t.TempDir()
	binDir := filepath.Join(base, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	modOut, err := exec.Command(goBin, "env", "GOMOD").Output()
	if err != nil {
		t.Fatal(err)
	}
	build := exec.Command(goBin, "build", "-o", filepath.Join(binDir, "terraform-provider-proxmox"), ".")
	build.Dir = filepath.Dir(strings.TrimSpace(string(modOut)))
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	config := `terraform {
  required_providers {
    proxmox = { source = "indexyz/proxmox" }
  }
}
variable "generation" { type = number }
provider "proxmox" {
  endpoint = "ENDPOINT"
  api_token_id = "terraform@pve!provider"
  api_token_secret = "token-secret"
}
resource "proxmox_nocloud_iso" "seed" {
  node = "pve-1"
  storage = "local"
  filename = "in-place-${var.generation}.iso"
  user_data = "#cloud-config\ngrowpart: {mode: auto, devices: ['/']}\n# revision ${var.generation}\n"
  meta_data = "instance-id: stable-instance\n"
  network_config = "version: 2\nethernets: {eth0: {dhcp4: true}}\n"
  lifecycle { create_before_destroy = true }
}
resource "proxmox_qemu_vm" "vm" {
  node = "pve-1"
  vm_id = 105
  name = "in-place-test"
  clone = { source_vmid = 9000, full = true }
  nocloud_cdrom_slot = "ide2"
  disk = { ide2 = { media = "cdrom", volume = proxmox_nocloud_iso.seed.volume_id } }
  stop_on_destroy = true
}
`
	files := map[string]string{
		"main.tf":      strings.ReplaceAll(config, "ENDPOINT", server.URL),
		"terraform.rc": fmt.Sprintf("provider_installation {\n dev_overrides {\n \"indexyz/proxmox\" = %q\n }\n}\n", binDir),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(base, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(gen string, wantError string, args ...string) {
		t.Helper()
		cmd := exec.Command(terraformBin, args...)
		cmd.Dir = base
		cmd.Env = append(os.Environ(), "TF_CLI_CONFIG_FILE="+filepath.Join(base, "terraform.rc"), "TF_VAR_generation="+gen)
		out, err := cmd.CombinedOutput()
		if wantError != "" {
			if err == nil || !strings.Contains(string(out), wantError) {
				t.Fatalf("expected %s: %v\n%s", wantError, err, out)
			}
		} else if err != nil {
			t.Fatalf("terraform %v: %v\n%s", args, err, out)
		}
	}
	run("1", "", "apply", "-auto-approve", "-input=false", "-no-color")
	requireTerraformCoreRootVolume(t, pve, 105)
	pve.resetEvents()
	run("2", "", "apply", "-auto-approve", "-input=false", "-no-color")
	events := pve.snapshot()
	for _, event := range events {
		if strings.HasPrefix(event, "clone:") || strings.HasPrefix(event, "vm-delete:") || strings.HasPrefix(event, "start:") {
			t.Fatalf("seed update replaced/started VM: %v", events)
		}
	}
	requireTerraformCoreOrder(t, events, "upload:in-place-2.iso", "attach:local:iso/in-place-2.iso,media=cdrom")
	requireTerraformCoreOrder(t, events, "attach:local:iso/in-place-2.iso,media=cdrom", "iso-delete:in-place-1.iso")
	requireTerraformCoreRootVolume(t, pve, 105)
	pve.resetEvents()
	run("2", "", "apply", "-auto-approve", "-input=false", "-no-color")
	if events := pve.snapshot(); len(events) != 0 {
		t.Fatalf("repeat apply mutated resources: %v", events)
	}
	// Terraform may skip Update after refresh discovers the new target already
	// attached. Read must confirm ONLY the durable pre-authorized intent, or
	// the next seed revision would be stuck with a pending target forever.
	pve.mu.Lock()
	pve.FailNextConfigAfterMutation = true
	pve.mu.Unlock()
	run("3", "simulated lost reply", "apply", "-auto-approve", "-input=false", "-no-color")
	run("3", "", "apply", "-auto-approve", "-input=false", "-no-color")
	run("4", "", "apply", "-auto-approve", "-input=false", "-no-color")
	requireTerraformCoreRootVolume(t, pve, 105)
	if err := pve.swapIde2(105, "local:iso/foreign.iso,media=cdrom"); err != nil {
		t.Fatal(err)
	}
	pve.resetEvents()
	run("5", "unproven ISO", "apply", "-auto-approve", "-input=false", "-no-color")
	for _, event := range pve.snapshot() {
		if strings.HasPrefix(event, "attach:") || strings.HasPrefix(event, "vm-delete:") {
			t.Fatalf("foreign media overwritten: %v", pve.snapshot())
		}
	}
	run("5", "", "destroy", "-auto-approve", "-input=false", "-no-color")
	pve.assertNoFailures(t)
}
