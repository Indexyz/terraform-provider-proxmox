// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// This file proves the documented NoCloud generation chain with the real
// Terraform CLI (provider dev_overrides against a local mock PVE API):
// generation 1 apply -> generation 2 replacement -> destroy. Unlike the
// framework-level chain tests, Terraform core itself computes the execution
// order from the configuration's dependency graph, so the assertions below
// verify the ordering the guide promises instead of hand-invoked resource
// callbacks: the seed is uploaded before the VM clones, a generation
// replacement destroys the old VM before creating the replacement and deletes
// the old seed only after the replacement VM references the new one, and the
// final destroy stops and deletes the VM before its seed.

const terraformCoreTemplateName = "ubuntu-nocloud-template"

type terraformCoreVM struct {
	Name    string
	Scsi0   string
	Net0    string
	Ide2    string
	Running bool
}

type terraformCorePVE struct {
	mu       sync.Mutex
	seq      int
	uploaded map[string]bool
	vms      map[int64]*terraformCoreVM
	events   []string
	failures []string
}

func newTerraformCorePVE() *terraformCorePVE {
	return &terraformCorePVE{
		uploaded: map[string]bool{},
		vms:      map[int64]*terraformCoreVM{},
	}
}

func (p *terraformCorePVE) fail(w http.ResponseWriter, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	p.mu.Lock()
	p.failures = append(p.failures, message)
	p.mu.Unlock()
	http.Error(w, message, http.StatusInternalServerError)
}

func (p *terraformCorePVE) envelope(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"data": data}); err != nil {
		p.fail(w, "encode response: %v", err)
	}
}

func (p *terraformCorePVE) event(format string, args ...any) {
	p.events = append(p.events, fmt.Sprintf(format, args...))
}

func (p *terraformCorePVE) nextUPID(dtype string) string {
	p.seq++
	return fmt.Sprintf("UPID:pve-1:%s-%d", dtype, p.seq)
}

func (p *terraformCorePVE) volumeID(filename string) string {
	return "local:iso/" + filename
}

func (p *terraformCorePVE) contentItems() []map[string]any {
	items := []map[string]any{}
	for filename := range p.uploaded {
		items = append(items, map[string]any{
			"volid":   p.volumeID(filename),
			"format":  "iso",
			"size":    393216,
			"content": "iso",
		})
	}
	return items
}

func (p *terraformCorePVE) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if got, want := r.Header.Get("Authorization"), "PVEAPIToken=terraform@pve!provider=token-secret"; got != want {
		p.fail(w, "unexpected authorization header: got %q want %q", got, want)
		return
	}

	// r.URL.Path is the decoded path; the volume DELETE segment embeds a
	// %2F-escaped volume id that decodes to local:iso/<filename>.
	path := r.URL.Path
	switch {
	case r.Method == http.MethodGet && path == "/api2/json/cluster/resources":
		if got := r.URL.Query().Get("type"); got != "vm" {
			p.fail(w, "unexpected resources type filter: %q", got)
			return
		}
		p.envelope(w, []map[string]any{{
			"vmid": 9000, "name": terraformCoreTemplateName, "node": "pve-1",
			"template": 1, "status": "stopped", "type": "qemu",
		}})
	case r.Method == http.MethodGet && path == "/api2/json/cluster/nextid":
		candidate, err := strconv.ParseInt(r.URL.Query().Get("vmid"), 10, 64)
		if err != nil {
			p.fail(w, "nextid without vmid candidate: %q", r.URL.RawQuery)
			return
		}
		p.mu.Lock()
		_, taken := p.vms[candidate]
		p.mu.Unlock()
		if taken {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"data":null,"errors":{"vmid":"VM %d already exists"}}`, candidate)
			return
		}
		p.envelope(w, candidate)
	case r.Method == http.MethodGet && path == "/api2/json/nodes/pve-1/storage":
		p.envelope(w, []map[string]any{
			{"storage": "local", "type": "dir", "content": "iso,vztmpl", "active": 1, "enabled": 1, "shared": 1},
		})
	case r.Method == http.MethodGet && path == "/api2/json/nodes/pve-1/storage/local/content":
		if got := r.URL.Query().Get("content"); got != "iso" {
			p.fail(w, "unexpected content query: %q", got)
			return
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		p.envelope(w, p.contentItems())
	case r.Method == http.MethodPost && path == "/api2/json/nodes/pve-1/storage/local/upload":
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			p.fail(w, "parse upload form: %v", err)
			return
		}
		if got := r.FormValue("content"); got != "iso" {
			p.fail(w, "unexpected upload content: %q", got)
			return
		}
		files := r.MultipartForm.File["filename"]
		if len(files) != 1 {
			p.fail(w, "expected exactly one filename file part, got %d", len(files))
			return
		}
		filename := files[0].Filename
		p.mu.Lock()
		p.uploaded[filename] = true
		p.event("upload:%s", filename)
		upid := p.nextUPID("imgcopy")
		p.mu.Unlock()
		p.envelope(w, upid)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/api2/json/nodes/pve-1/tasks/") && strings.HasSuffix(path, "/status"):
		p.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
	case r.Method == http.MethodPost && path == "/api2/json/nodes/pve-1/qemu/9000/clone":
		if err := r.ParseForm(); err != nil {
			p.fail(w, "parse clone form: %v", err)
			return
		}
		newID, err := strconv.ParseInt(r.FormValue("newid"), 10, 64)
		if err != nil {
			p.fail(w, "clone without newid: %q", r.Form.Encode())
			return
		}
		p.mu.Lock()
		p.vms[newID] = &terraformCoreVM{
			Name:  r.FormValue("name"),
			Scsi0: fmt.Sprintf("local-lvm:vm-%d-disk-0,size=8G", newID),
			Net0:  fmt.Sprintf("virtio=BC:24:11:%02X:%02X:%02X,bridge=vmbr0", (newID>>16)&0xFF, (newID>>8)&0xFF, newID&0xFF),
			Ide2:  fmt.Sprintf("local:vm-%d-cloudinit,media=cdrom", newID),
		}
		p.event("clone:%d", newID)
		upid := p.nextUPID("qemuclone")
		p.mu.Unlock()
		p.envelope(w, upid)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/api2/json/nodes/pve-1/qemu/") && strings.HasSuffix(path, "/config"):
		p.mu.Lock()
		defer p.mu.Unlock()
		vm, ok := p.vmFromPath(path)
		if !ok {
			http.Error(w, `{"data":null}`, http.StatusNotFound)
			return
		}
		// The mock models a plausible bootable template clone: the guest
		// inherits the template's system disk (scsi0), NIC (net0), and the ide2
		// cloud-init medium the marked workflow explicitly replaces. The root disk
		// and NIC stay inherited on every config read; they are not managed disk
		// slots of the plan, so no update request may ever carry them.
		p.envelope(w, map[string]any{
			"name":  vm.Name,
			"scsi0": vm.Scsi0,
			"net0":  vm.Net0,
			"ide2":  vm.Ide2,
		})
	case r.Method == http.MethodPut && strings.HasPrefix(path, "/api2/json/nodes/pve-1/qemu/") && strings.HasSuffix(path, "/config"):
		if err := r.ParseForm(); err != nil {
			p.fail(w, "parse update form: %v", err)
			return
		}
		p.mu.Lock()
		vm, ok := p.vmFromPath(path)
		if !ok {
			p.mu.Unlock()
			http.Error(w, `{"data":null}`, http.StatusNotFound)
			return
		}
		// The inherited system disk and NIC are not managed by Terraform: a
		// PUT carrying their slots would overwrite or detach the template's
		// bootable root volume. The offending form is captured before the
		// lock is dropped because p.fail locks p.mu itself.
		for _, key := range []string{"scsi0", "net0"} {
			if _, sent := r.Form[key]; sent {
				encoded := r.Form.Encode()
				p.mu.Unlock()
				p.fail(w, "update request must not manage inherited slot %q: %q", key, encoded)
				return
			}
		}
		if name := r.FormValue("name"); name != "" {
			vm.Name = name
		}
		if ide2 := r.FormValue("ide2"); ide2 != "" {
			vm.Ide2 = ide2
			p.event("attach:%s", ide2)
		}
		p.mu.Unlock()
		p.envelope(w, nil)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/api2/json/nodes/pve-1/qemu/") && strings.HasSuffix(path, "/status/current"):
		p.mu.Lock()
		defer p.mu.Unlock()
		vm, ok := p.vmFromPath(path)
		if !ok {
			http.Error(w, `{"data":null}`, http.StatusNotFound)
			return
		}
		status := "stopped"
		if vm.Running {
			status = "running"
		}
		p.envelope(w, map[string]any{"status": status, "uptime": 5})
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/api2/json/nodes/pve-1/qemu/") && strings.HasSuffix(path, "/status/start"):
		p.mu.Lock()
		vmID, _ := p.vmIDFromPath(path)
		p.event("start:%d", vmID)
		upid := p.nextUPID("qmstart")
		if vm, ok := p.vmFromPath(path); ok {
			vm.Running = true
		}
		p.mu.Unlock()
		p.envelope(w, upid)
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/api2/json/nodes/pve-1/qemu/") && strings.HasSuffix(path, "/status/stop"):
		p.mu.Lock()
		vmID, _ := p.vmIDFromPath(path)
		p.event("stop:%d", vmID)
		upid := p.nextUPID("qmstop")
		if vm, ok := p.vmFromPath(path); ok {
			vm.Running = false
		}
		p.mu.Unlock()
		p.envelope(w, upid)
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/api2/json/nodes/pve-1/qemu/") && strings.HasSuffix(path, "/status/shutdown"):
		if err := r.ParseForm(); err != nil {
			p.fail(w, "parse shutdown form: %v", err)
			return
		}
		p.mu.Lock()
		vmID, _ := p.vmIDFromPath(path)
		// The declarative power-off always pins the wire contract of the
		// single shutdown task: server-side graceful wait, then forced stop.
		if got, want := r.Form.Encode(), "forceStop=1&timeout=90"; got != want {
			p.mu.Unlock()
			p.fail(w, "unexpected shutdown form of guest %d: %q", vmID, got)
			return
		}
		p.event("shutdown:%d", vmID)
		upid := p.nextUPID("qmshutdown")
		if vm, ok := p.vmFromPath(path); ok {
			vm.Running = false
		}
		p.mu.Unlock()
		p.envelope(w, upid)
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/api2/json/nodes/pve-1/qemu/"):
		p.mu.Lock()
		vmID, _ := p.vmIDFromPath(path)
		p.event("vm-delete:%d", vmID)
		upid := p.nextUPID("qmdestroy")
		delete(p.vms, vmID)
		p.mu.Unlock()
		p.envelope(w, upid)
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/api2/json/nodes/pve-1/storage/local/content/local:iso/"):
		filename := strings.TrimPrefix(path, "/api2/json/nodes/pve-1/storage/local/content/local:iso/")
		p.mu.Lock()
		if !p.uploaded[filename] {
			p.mu.Unlock()
			p.fail(w, "delete of unmanaged file %q", filename)
			return
		}
		delete(p.uploaded, filename)
		p.event("iso-delete:%s", filename)
		upid := p.nextUPID("imgdel")
		p.mu.Unlock()
		p.envelope(w, upid)
	default:
		p.fail(w, "unexpected harness request: %s %s", r.Method, path)
	}
}

func (p *terraformCorePVE) vmIDFromPath(path string) (int64, bool) {
	rest := strings.TrimPrefix(path, "/api2/json/nodes/pve-1/qemu/")
	id, err := strconv.ParseInt(strings.SplitN(rest, "/", 2)[0], 10, 64)
	return id, err == nil
}

func (p *terraformCorePVE) vmFromPath(path string) (*terraformCoreVM, bool) {
	vmID, ok := p.vmIDFromPath(path)
	if !ok {
		return nil, false
	}
	vm, ok := p.vms[vmID]
	return vm, ok
}

// snapshot returns a copy of the event log so assertions cannot race the
// handler.
func (p *terraformCorePVE) snapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.events...)
}

func (p *terraformCorePVE) resetEvents() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = nil
}

// swapIde2 replaces the managed ide2 medium of a guest out of band, the way
// an operator would swap the ISO directly on the PVE host.
func (p *terraformCorePVE) swapIde2(vmID int64, volume string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	vm, ok := p.vms[vmID]
	if !ok {
		return fmt.Errorf("guest %d missing from mock PVE", vmID)
	}
	vm.Ide2 = volume
	return nil
}

// setRunning flips a guest's power state out of band, the way an operator
// stopping or starting a guest directly on the PVE host would.
func (p *terraformCorePVE) setRunning(vmID int64, running bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	vm, ok := p.vms[vmID]
	if !ok {
		return fmt.Errorf("guest %d missing from mock PVE", vmID)
	}
	vm.Running = running
	return nil
}

func (p *terraformCorePVE) assertNoFailures(t *testing.T) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.failures) > 0 {
		t.Fatalf("mock PVE handler failures: %v", p.failures)
	}
}

func (p *terraformCorePVE) quiesced() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.uploaded) == 0 && len(p.vms) == 0
}

func indexOfTerraformCoreEvent(events []string, prefix string) int {
	for i, event := range events {
		if strings.HasPrefix(event, prefix) {
			return i
		}
	}
	return -1
}

// requireTerraformCoreOrder asserts both events happened and the first before
// the second.
func requireTerraformCoreOrder(t *testing.T, events []string, first, then string) {
	t.Helper()
	firstIndex := indexOfTerraformCoreEvent(events, first)
	if firstIndex < 0 {
		t.Fatalf("expected event %q is missing from: %v", first, events)
	}
	thenIndex := indexOfTerraformCoreEvent(events, then)
	if thenIndex < 0 {
		t.Fatalf("expected event %q is missing from: %v", then, events)
	}
	if firstIndex >= thenIndex {
		t.Fatalf("event %q (index %d) must happen before %q (index %d): %v", first, firstIndex, then, thenIndex, events)
	}
}

func requireTerraformCoreAbsent(t *testing.T, events []string, prefix string) {
	t.Helper()
	if index := indexOfTerraformCoreEvent(events, prefix); index >= 0 {
		t.Fatalf("unexpected event %q at index %d: %v", prefix, index, events)
	}
}

type terraformCoreState struct {
	Resources []struct {
		Name      string `json:"name"`
		Type      string `json:"type"`
		Instances []struct {
			Attributes struct {
				VMID float64        `json:"vm_id"`
				Disk map[string]any `json:"disk"`
			} `json:"attributes"`
		} `json:"instances"`
	} `json:"resources"`
}

func requireTerraformCoreVMID(t *testing.T, workdir, resourceName string, want float64) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(workdir, "terraform.tfstate"))
	if err != nil {
		t.Fatalf("read terraform state: %v", err)
	}
	var state terraformCoreState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("decode terraform state: %v", err)
	}
	for _, resource := range state.Resources {
		if resource.Type != "proxmox_qemu_vm" || resource.Name != resourceName || len(resource.Instances) == 0 {
			continue
		}
		if got := resource.Instances[0].Attributes.VMID; got != want {
			t.Fatalf("allocated vm_id = %v, want %v", got, want)
		}
		return
	}
	t.Fatalf("proxmox_qemu_vm.%s not found in state: %s", resourceName, raw)
}

// requireTerraformCoreBareDiskEmpty asserts the bootable-template clone that
// configures the constrained empty key set keeps a known empty managed disk
// map in state: a JSON empty object, never null (an unconstrained import)
// and never carrying the inherited template slots.
func requireTerraformCoreBareDiskEmpty(t *testing.T, workdir string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(workdir, "terraform.tfstate"))
	if err != nil {
		t.Fatalf("read terraform state: %v", err)
	}
	var state terraformCoreState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("decode terraform state: %v", err)
	}
	for _, resource := range state.Resources {
		if resource.Type != "proxmox_qemu_vm" || resource.Name != "bare" || len(resource.Instances) == 0 {
			continue
		}
		disk := resource.Instances[0].Attributes.Disk
		if disk == nil {
			t.Fatalf("bare clone disk must stay the known empty map, got null in state: %s", raw)
		}
		if len(disk) != 0 {
			t.Fatalf("bare clone disk must stay the known empty map, got %v in state: %s", disk, raw)
		}
		return
	}
	t.Fatalf("proxmox_qemu_vm.bare not found in state: %s", raw)
}

// requireTerraformCoreRootVolume asserts the cloned guest still carries the
// template's system disk and NIC, with the root volume identity following
// the allocated VMID exactly.
func requireTerraformCoreRootVolume(t *testing.T, p *terraformCorePVE, vmID int64) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	vm, ok := p.vms[vmID]
	if !ok {
		t.Fatalf("guest %d missing from mock PVE", vmID)
	}
	wantRoot := fmt.Sprintf("local-lvm:vm-%d-disk-0,size=8G", vmID)
	if vm.Scsi0 != wantRoot {
		t.Fatalf("root disk of guest %d = %q, want %q", vmID, vm.Scsi0, wantRoot)
	}
	if !strings.HasPrefix(vm.Net0, "virtio=") || !strings.Contains(vm.Net0, "bridge=vmbr0") {
		t.Fatalf("NIC of guest %d missing: %q", vmID, vm.Net0)
	}
}

const terraformCoreMainTFTemplate = `terraform {
  required_providers {
    proxmox = {
      source = "indexyz/proxmox"
    }
  }
}

variable "generation" {
  type = number
}

variable "api_token_secret" {
  type      = string
  sensitive = true
}

locals {
  # Zero-padded so it sorts and reads well in filenames and instance IDs.
  generation = format("%010d", var.generation)
}

provider "proxmox" {
  endpoint         = "{{ENDPOINT}}"
  api_token_id     = "terraform@pve!provider"
  api_token_secret = var.api_token_secret
}

# Token-only template lookup; one() errors when more than one template
# matches, the precondition turns a missing template into a clear plan error.
data "proxmox_qemu_vms" "template" {
  name     = "ubuntu-nocloud-template"
  template = true
}

locals {
  template = one(data.proxmox_qemu_vms.template.vms)
}

# One dedicated seed ISO per generation, with its own DHCP network-config.
resource "proxmox_nocloud_iso" "runner_seed" {
  node     = "pve-1"
  storage  = "local"
  filename = "runner-seed-${local.generation}.iso"

  meta_data = yamlencode({
    instance-id    = "iid-runner-${local.generation}"
    local-hostname = "runner"
  })

  user_data = templatefile("${path.module}/user-data.tftpl", {
    registration_token = "registration-secret"
  })

  network_config = yamlencode({
    version = 2
    ethernets = {
      nic0 = {
        match = {
          name = "en*"
        }
        dhcp4 = true
      }
    }
  })

  lifecycle {
    create_before_destroy = true
  }
}

resource "proxmox_qemu_vm" "runner" {
  node        = "pve-1"
  vm_id_start = 100
  name        = "tf-runner-${local.generation}"

  clone = {
    source_node = try(local.template.node, null)
    source_vmid = try(local.template.vm_id, null)
    full        = true
  }

  nocloud_cdrom_slot = "ide2"

  disk = {
    ide2 = {
      media  = "cdrom"
      volume = proxmox_nocloud_iso.runner_seed.volume_id
    }
  }

  start_on_create = true
  stop_on_destroy = true

  lifecycle {
    replace_triggered_by = [
      proxmox_nocloud_iso.runner_seed
    ]

    precondition {
      condition     = local.template != null
      error_message = "The template lookup must match exactly one template VM."
    }
  }
}
# A bootable-template clone that manages no disk slots at all: the empty
# disk map is the constrained empty key set. The guest physically inherits
# the template's system disk and NIC, but neither may become a managed
# state entry.
resource "proxmox_qemu_vm" "bare" {
  node  = "pve-1"
  vm_id = 101
  name  = "tf-bare-${local.generation}"

  clone = {
    source_node = try(local.template.node, null)
    source_vmid = try(local.template.vm_id, null)
    full        = true
  }

{{BARE_DISK}}
}
`

// The bare clone's managed disk map starts as the constrained empty key set
// and is later rewritten to a managed slot and back, proving the update
// transition to empty stays a known empty map.
const terraformCoreBareDiskEmpty = "  disk = {}"

const terraformCoreBareDiskIde2 = `  disk = {
    ide2 = {
      media  = "cdrom"
      volume = "local:iso/installer-live.iso"
    }
  }`

const terraformCoreUserDataTFTPL = `#cloud-config
runcmd:
  - echo register runner ${registration_token}
`

// TestTerraformCoreGenerationReplacementOrdering drives the guide's exact
// provisioning chain through the Terraform CLI against the mock PVE API and
// proves: generation 1 uploads the seed before cloning and starting the VM;
// generation 2 replaces the VM before the old seed is deleted; and the final
// destroy stops, deletes the VM, and only then deletes its seed.
func TestTerraformCoreGenerationReplacementOrdering(t *testing.T) {
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
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create provider bin dir: %v", err)
	}
	binPath := filepath.Join(binDir, "terraform-provider-proxmox")
	build := exec.Command(goBin, "build", "-o", binPath, ".")
	build.Dir = moduleRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build provider binary: %v\n%s", err, out)
	}

	workdir := filepath.Join(base, "config")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("create terraform workdir: %v", err)
	}
	// writeMainTF rewrites main.tf so the bare clone's managed disk map can
	// transition between the empty key set and a managed slot mid-run.
	writeMainTF := func(bareDisk string) {
		t.Helper()
		mainTF := strings.NewReplacer(
			"{{ENDPOINT}}", server.URL,
			"{{BARE_DISK}}", bareDisk,
		).Replace(terraformCoreMainTFTemplate)
		if err := os.WriteFile(filepath.Join(workdir, "main.tf"), []byte(mainTF), 0o600); err != nil {
			t.Fatalf("write main.tf: %v", err)
		}
	}
	writeMainTF(terraformCoreBareDiskEmpty)
	files := map[string]string{
		"user-data.tftpl": terraformCoreUserDataTFTPL,
		"terraform.rc":    fmt.Sprintf("provider_installation {\n  dev_overrides {\n    \"registry.terraform.io/indexyz/proxmox\" = %q\n  }\n}\n", binDir),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(workdir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	harnessEnv := func(generation string) []string {
		return append(os.Environ(),
			"TF_CLI_CONFIG_FILE="+filepath.Join(workdir, "terraform.rc"),
			"TF_VAR_generation="+generation,
			"TF_VAR_api_token_secret=token-secret",
		)
	}
	runTerraform := func(generation string, args ...string) {
		t.Helper()
		t.Logf("terraform %v", args)
		cmd := exec.Command(terraformBin, args...)
		cmd.Dir = workdir
		cmd.Env = harnessEnv(generation)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("terraform %v: %v\n%s", args, err, out)
		}
	}
	// runTerraformExpectExit tolerates the documented exit codes of
	// `terraform plan -detailed-exitcode` (0 no changes, 2 changes pending).
	runTerraformExpectExit := func(generation string, wantExit int, args ...string) {
		t.Helper()
		cmd := exec.Command(terraformBin, args...)
		cmd.Dir = workdir
		cmd.Env = harnessEnv(generation)
		out, err := cmd.CombinedOutput()
		exitCode := 0
		var exitErr *exec.ExitError
		if err != nil {
			if !errors.As(err, &exitErr) {
				t.Fatalf("terraform %v: %v\n%s", args, err, out)
			}
			exitCode = exitErr.ExitCode()
		}
		if exitCode != wantExit {
			t.Fatalf("terraform %v exit code = %d, want %d\n%s", args, exitCode, wantExit, out)
		}
	}

	// Generation 1: the seed upload precedes the clone, and the clone
	// precedes attaching the seed and starting the guest. The constant
	// vm_id_start = 100 allocates VMID 100. The bootable template clone
	// keeps its inherited root disk and NIC: the exact root volume identity
	// follows the allocated VMID, and no update request ever carries the
	// inherited slots.
	runTerraform("1", "apply", "-auto-approve", "-input=false")
	gen1 := pve.snapshot()
	requireTerraformCoreOrder(t, gen1, "upload:runner-seed-0000000001.iso", "clone:100")
	requireTerraformCoreOrder(t, gen1, "clone:100", "attach:local:iso/runner-seed-0000000001")
	requireTerraformCoreOrder(t, gen1, "attach:local:iso/runner-seed-0000000001", "start:100")
	requireTerraformCoreAbsent(t, gen1, "stop:")
	requireTerraformCoreAbsent(t, gen1, "vm-delete:")
	requireTerraformCoreAbsent(t, gen1, "iso-delete:")
	requireTerraformCoreVMID(t, workdir, "runner", 100)
	requireTerraformCoreVMID(t, workdir, "bare", 101)
	// The bare clone configures the constrained empty key set: the known
	// empty managed disk map must survive the create, even though the mock
	// clone physically inherited the template's scsi0 system disk.
	requireTerraformCoreBareDiskEmpty(t, workdir)
	requireTerraformCoreRootVolume(t, pve, 100)

	// A repeated apply is a no-op: the resource disk state honors the plan's
	// managed key set (ide2), so the inherited system disk and NIC never
	// reappear as new managed entries and Terraform Core accepts the state.
	pve.resetEvents()
	runTerraform("1", "apply", "-auto-approve", "-input=false")
	for _, event := range pve.snapshot() {
		t.Fatalf("no-op apply must not issue write requests, got %q", event)
	}

	// Managed-slot drift stays observable: after the seed medium is swapped
	// out of band, the plan reports the change; once the wire is restored the
	// plan is clean again, and the computed uptime never counts as a config
	// change.
	if err := pve.swapIde2(100, "local:iso/out-of-band-seed.iso,media=cdrom"); err != nil {
		t.Fatalf("swap managed seed out of band: %v", err)
	}
	runTerraformExpectExit("1", 2, "plan", "-detailed-exitcode", "-input=false")
	if err := pve.swapIde2(100, "local:iso/runner-seed-0000000001.iso,media=cdrom"); err != nil {
		t.Fatalf("restore managed seed: %v", err)
	}
	runTerraformExpectExit("1", 0, "plan", "-detailed-exitcode", "-input=false")

	// Update transitions of the bare clone's managed disk map: adding a slot
	// and returning to the constrained empty key set are both in-place
	// updates, and the transition to empty must keep the known empty map in
	// state (never null, never the inherited inventory) while the update
	// request never carries the inherited slots.
	writeMainTF(terraformCoreBareDiskIde2)
	runTerraform("1", "apply", "-auto-approve", "-input=false")
	writeMainTF(terraformCoreBareDiskEmpty)
	runTerraform("1", "apply", "-auto-approve", "-input=false")
	requireTerraformCoreBareDiskEmpty(t, workdir)
	runTerraformExpectExit("1", 0, "plan", "-detailed-exitcode", "-input=false")

	// Generation 2: the replacement seed is uploaded first, the old VM is
	// destroyed before the replacement is created, and the old seed is
	// deleted only after the replacement VM re-pointed to the new seed and
	// started. Terraform core derives this order from create_before_destroy
	// on the seed plus replace_triggered_by on the VM.
	pve.resetEvents()
	runTerraform("2", "apply", "-auto-approve", "-input=false")
	gen2 := pve.snapshot()
	requireTerraformCoreOrder(t, gen2, "upload:runner-seed-0000000002.iso", "clone:100")
	requireTerraformCoreOrder(t, gen2, "clone:100", "attach:local:iso/runner-seed-0000000002")
	requireTerraformCoreOrder(t, gen2, "attach:local:iso/runner-seed-0000000002", "start:100")
	requireTerraformCoreOrder(t, gen2, "vm-delete:100", "clone:100")
	requireTerraformCoreOrder(t, gen2, "vm-delete:100", "iso-delete:runner-seed-0000000001.iso")
	requireTerraformCoreOrder(t, gen2, "attach:local:iso/runner-seed-0000000002", "iso-delete:runner-seed-0000000001.iso")
	requireTerraformCoreVMID(t, workdir, "runner", 100)
	requireTerraformCoreRootVolume(t, pve, 100)

	// Final destroy: the guest is stopped and deleted before its seed file.
	pve.resetEvents()
	runTerraform("2", "destroy", "-auto-approve", "-input=false")
	destroyed := pve.snapshot()
	requireTerraformCoreOrder(t, destroyed, "stop:100", "vm-delete:100")
	requireTerraformCoreOrder(t, destroyed, "vm-delete:100", "iso-delete:runner-seed-0000000002.iso")
	if !pve.quiesced() {
		t.Fatalf("mock PVE still holds managed objects after destroy")
	}

	pve.assertNoFailures(t)
}

// The bounded declarative-power scenario: a clone managed with `power`
// instead of the lifecycle hooks, exercised through the real Terraform CLI
// against the same mock PVE API.
const terraformCorePowerMainTFTemplate = `terraform {
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

provider "proxmox" {
  endpoint         = "{{ENDPOINT}}"
  api_token_id     = "terraform@pve!provider"
  api_token_secret = var.api_token_secret
}

data "proxmox_qemu_vms" "template" {
  name     = "ubuntu-nocloud-template"
  template = true
}

locals {
  template = one(data.proxmox_qemu_vms.template.vms)
}

resource "proxmox_qemu_vm" "powered" {
  node  = "pve-1"
  vm_id = 102
  name  = "tf-powered"

  clone = {
    source_node = try(local.template.node, null)
    source_vmid = try(local.template.vm_id, null)
    full        = true
  }

{{POWER_BLOCK}}
}
`

const terraformCorePowerOn = "  power = true"

const terraformCorePowerOff = `  power                  = false
  power_shutdown_timeout = 90`

// TestTerraformCorePowerReconcile drives the declarative `power` semantics
// through the real Terraform CLI: the first apply starts the guest, an
// out-of-band stop is reconciled back to running by the next apply, and
// switching to `power = false` shuts the running guest down (graceful with
// timeout, then forced stop, server-side) before the config PUT.
func TestTerraformCorePowerReconcile(t *testing.T) {
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
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create provider bin dir: %v", err)
	}
	binPath := filepath.Join(binDir, "terraform-provider-proxmox")
	build := exec.Command(goBin, "build", "-o", binPath, ".")
	build.Dir = moduleRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build provider binary: %v\n%s", err, out)
	}

	workdir := filepath.Join(base, "config")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("create terraform workdir: %v", err)
	}
	writePowerMainTF := func(powerBlock string) {
		t.Helper()
		mainTF := strings.NewReplacer(
			"{{ENDPOINT}}", server.URL,
			"{{POWER_BLOCK}}", powerBlock,
		).Replace(terraformCorePowerMainTFTemplate)
		if err := os.WriteFile(filepath.Join(workdir, "main.tf"), []byte(mainTF), 0o600); err != nil {
			t.Fatalf("write main.tf: %v", err)
		}
	}
	writePowerMainTF(terraformCorePowerOn)
	if err := os.WriteFile(filepath.Join(workdir, "terraform.rc"), []byte(fmt.Sprintf("provider_installation {\n  dev_overrides {\n    \"registry.terraform.io/indexyz/proxmox\" = %q\n  }\n}\n", binDir)), 0o600); err != nil {
		t.Fatalf("write terraform.rc: %v", err)
	}

	harnessEnv := append(os.Environ(),
		"TF_CLI_CONFIG_FILE="+filepath.Join(workdir, "terraform.rc"),
		"TF_VAR_api_token_secret=token-secret",
	)
	runTerraform := func(args ...string) {
		t.Helper()
		t.Logf("terraform %v", args)
		cmd := exec.Command(terraformBin, args...)
		cmd.Dir = workdir
		cmd.Env = harnessEnv
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("terraform %v: %v\n%s", args, err, out)
		}
	}
	countEvents := func(events []string, prefix string) int {
		count := 0
		for _, event := range events {
			if strings.HasPrefix(event, prefix) {
				count++
			}
		}
		return count
	}

	// Generation 1 with power = true: the guest is started after the clone
	// and its config update.
	runTerraform("apply", "-auto-approve", "-input=false")
	gen1 := pve.snapshot()
	requireTerraformCoreOrder(t, gen1, "clone:102", "start:102")
	requireTerraformCoreAbsent(t, gen1, "stop:")
	requireTerraformCoreAbsent(t, gen1, "shutdown:")
	requireTerraformCoreVMID(t, workdir, "powered", 102)

	// An operator stops the guest out of band; the next apply reconciles it
	// back to running. Refresh alone only mirrors the drift into state.
	if err := pve.setRunning(102, false); err != nil {
		t.Fatalf("stop guest out of band: %v", err)
	}
	runTerraform("apply", "-auto-approve", "-input=false")
	reconciled := pve.snapshot()
	if got := countEvents(reconciled, "start:102"); got != 2 {
		t.Fatalf("expected the out-of-band stop to be reconciled by a second start, got %d start events: %v", got, reconciled)
	}
	requireTerraformCoreAbsent(t, reconciled, "shutdown:")

	// Switching to power = false with a shutdown timeout shuts the running
	// guest down through the single server-side escalation task; no start
	// may follow.
	writePowerMainTF(terraformCorePowerOff)
	runTerraform("apply", "-auto-approve", "-input=false")
	poweredOff := pve.snapshot()
	if got := countEvents(poweredOff, "shutdown:102"); got != 1 {
		t.Fatalf("expected exactly one shutdown event, got %d: %v", got, poweredOff)
	}
	if got := countEvents(poweredOff, "start:102"); got != 2 {
		t.Fatalf("power = false must not start the guest, got %d start events: %v", got, poweredOff)
	}

	// Destroy: the guest is already off, so only the delete remains.
	pve.resetEvents()
	runTerraform("destroy", "-auto-approve", "-input=false")
	destroyed := pve.snapshot()
	if got := countEvents(destroyed, "vm-delete:102"); got != 1 {
		t.Fatalf("expected exactly one vm-delete event, got %d: %v", got, destroyed)
	}
	if !pve.quiesced() {
		t.Fatalf("mock PVE still holds managed objects after destroy")
	}

	pve.assertNoFailures(t)
}
