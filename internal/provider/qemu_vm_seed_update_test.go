// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestQemuSeedUpdateSafety(t *testing.T) {
	for _, tc := range []struct {
		name, current, status, digest, from string
		history                             bool
		failPUT                             bool
		applyBeforeFailure                  bool
		readbackMismatch                    bool
		wantError                           string
	}{
		{name: "empty_bay", current: "none,media=cdrom", status: "stopped", digest: "abc"},
		{name: "native_cloudinit", current: "local-lvm:vm-101-cloudinit,media=cdrom", status: "stopped", digest: "abc"},
		{name: "owned", current: "local:iso/old.iso,media=cdrom", status: "stopped", digest: "abc", history: true},
		{name: "explicit_migration", current: "local:iso/old.iso,media=cdrom", status: "stopped", digest: "abc", from: "local:iso/old.iso"},
		{name: "unproven", current: "local:iso/old.iso,media=cdrom", status: "stopped", digest: "abc", wantError: "unproven ISO"},
		{name: "foreign_after_refresh", current: "local:iso/foreign.iso,media=cdrom", status: "stopped", digest: "abc", history: true, wantError: "unproven ISO"},
		{name: "running", current: "local:iso/old.iso,media=cdrom", status: "running", digest: "abc", history: true, wantError: "Stop the workspace"},
		{name: "paused", current: "local:iso/old.iso,media=cdrom", status: "paused", digest: "abc", history: true, wantError: "Stop the workspace"},
		{name: "missing_digest", current: "local:iso/old.iso,media=cdrom", status: "stopped", history: true, wantError: "configuration digest"},
		{name: "hard_disk", current: "local:iso/old.iso,media=disk", status: "stopped", digest: "abc", history: true, from: "local:iso/old.iso", wantError: "would replace"},
		{name: "wrong_migration_volume", current: "local:iso/foreign.iso,media=cdrom", status: "stopped", digest: "abc", from: "local:iso/old.iso", wantError: "unproven ISO"},
		{name: "ambiguous_write_retry", current: "local:iso/old.iso,media=cdrom", status: "stopped", digest: "abc", history: true, failPUT: true, applyBeforeFailure: true, wantError: "checksum mismatch"},
		{name: "readback_mismatch", current: "local:iso/old.iso,media=cdrom", status: "stopped", digest: "abc", history: true, readbackMismatch: true, wantError: "Live media does not match"},
		{name: "CAS_rejection", current: "local:iso/old.iso,media=cdrom", status: "stopped", digest: "abc", history: true, failPUT: true, wantError: "checksum mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			h := &lifecycleHandler{}
			wire := tc.current
			puts, starts := 0, 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !h.auth(w, r) {
					return
				}
				switch {
				case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/config"):
					h.envelope(w, map[string]any{"ide2": wire, "scsi0": "local-lvm:vm-101-disk-0,size=32G", "digest": tc.digest})
				case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/storage"):
					h.envelope(w, noCloudStorageEntries("local"))
				case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/content"):
					h.envelope(w, []map[string]any{{"volid": "local:iso/new.iso", "content": "iso"}})
				case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/status/current"):
					h.envelope(w, map[string]any{"status": tc.status})
				case r.Method == "PUT" && strings.HasSuffix(r.URL.Path, "/config"):
					puts++
					if !h.form(w, r, url.Values{"ide2": {"local:iso/new.iso,media=cdrom"}, "digest": {"abc"}}) {
						return
					}
					if tc.failPUT && puts == 1 {
						if tc.applyBeforeFailure {
							wire = "local:iso/new.iso,media=cdrom"
						}
						http.Error(w, "checksum mismatch", http.StatusConflict)
						return
					}
					if !tc.readbackMismatch {
						wire = "local:iso/new.iso,media=cdrom"
					}
					h.envelope(w, nil)
				case strings.HasSuffix(r.URL.Path, "/status/start"):
					starts++
					h.fail(w, "seed update must not implicitly start/reboot")
				default:
					h.fail(w, "unexpected %s %s", r.Method, r.URL)
				}
			}))
			defer s.Close()
			res := &QemuVMResource{client: testLifecycleClient(t, s)}
			schema := testResourceSchema(t, res)
			model := minimalQemuVMModel("pve-1", 101)
			model.ID = types.StringValue("pve-1/101")
			model.NoCloudCDROMSlot = types.StringValue("ide2")
			// A refresh can observe foreign media; that must not grant ownership.
			model.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry(qemuVMWireDiskVolume(tc.current))})
			state := testResourceState(t, schema, model)
			plan := model
			plan.Disk = mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{"ide2": cdromDiskEntry("local:iso/new.iso")})
			plan.NoCloudUpdateFrom = types.StringValue(tc.from)
			resp := resource.UpdateResponse{State: state}
			initializeResourcePrivate(t, &resp)
			if tc.history {
				if d := writeQemuSeedAttachment(ctx, resp.Private, qemuSeedAttachment{Node: "pve-1", VMID: 101, Slot: "ide2", Volume: "local:iso/old.iso"}); d.HasError() {
					t.Fatal(d)
				}
			}
			res.Update(ctx, resource.UpdateRequest{State: state, Plan: testResourcePlan(t, schema, plan), Private: resp.Private}, &resp)
			if tc.wantError != "" {
				if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, tc.wantError) {
					t.Fatalf("got %v, want %s", resp.Diagnostics, tc.wantError)
				}
				if !tc.failPUT && !tc.readbackMismatch && puts != 0 {
					t.Fatalf("unsafe update issued %d PUTs", puts)
				}
			} else {
				if resp.Diagnostics.HasError() {
					t.Fatal(resp.Diagnostics)
				}
				if puts != 1 {
					t.Fatalf("want exactly one config PUT, got %d", puts)
				}
			}
			history, d := readQemuSeedAttachment(ctx, resp.Private)
			if d.HasError() {
				t.Fatal(d)
			}
			if tc.wantError == "" && (history.Volume != "local:iso/new.iso" || history.Pending != "") {
				t.Fatalf("bad confirmed history: %#v", history)
			}
			if (tc.failPUT || tc.readbackMismatch) && (history.Volume != "local:iso/old.iso" || history.Pending != "local:iso/new.iso") {
				t.Fatalf("missing retry intent: %#v", history)
			}
			if tc.applyBeforeFailure || tc.name == "CAS_rejection" {
				retry := resource.UpdateResponse{State: resp.State}
				initializeResourcePrivate(t, &retry)
				res.Update(ctx, resource.UpdateRequest{State: resp.State, Plan: testResourcePlan(t, schema, plan), Private: resp.Private}, &retry)
				if retry.Diagnostics.HasError() {
					t.Fatalf("authorized pending retry failed: %v", retry.Diagnostics)
				}
				confirmed, d := readQemuSeedAttachment(ctx, retry.Private)
				if d.HasError() || confirmed.Volume != "local:iso/new.iso" || confirmed.Pending != "" || puts != 2 {
					t.Fatalf("pending retry did not converge: %#v %v, puts=%d", confirmed, d, puts)
				}
			}
			if starts != 0 {
				t.Fatal("seed update started guest")
			}
			h.assert(t)
		})
	}
}

func TestQemuSeedUpdateOwnershipIdentityAndRetry(t *testing.T) {
	model := minimalQemuVMModel("pve-1", 101)
	model.NoCloudCDROMSlot = types.StringValue("ide2")
	planned := map[string]string{"ide2": "local:iso/new.iso"}
	current := QemuVMConfig{Disk: map[string]string{"ide2": "local:iso/old.iso,media=cdrom"}}
	valid := qemuSeedAttachment{Node: "pve-1", VMID: 101, Slot: "ide2", Volume: "local:iso/old.iso"}
	for _, history := range []qemuSeedAttachment{
		{Node: "other", VMID: 101, Slot: "ide2", Volume: valid.Volume},
		{Node: "pve-1", VMID: 102, Slot: "ide2", Volume: valid.Volume},
		{Node: "pve-1", VMID: 101, Slot: "ide3", Volume: valid.Volume},
	} {
		if _, _, err := authorizeQemuSeedUpdate(model, planned, current, history); err == nil {
			t.Fatalf("accepted wrong identity: %#v", history)
		}
	}
	valid.Pending = "local:iso/new.iso"
	current.Disk["ide2"] = "local:iso/new.iso,media=cdrom"
	if _, swap, err := authorizeQemuSeedUpdate(model, planned, current, valid); err != nil || swap {
		t.Fatalf("retry converged target: %v %v", swap, err)
	}
	planned["ide2"] = "local:iso/third.iso"
	if _, _, err := authorizeQemuSeedUpdate(model, planned, current, valid); err == nil {
		t.Fatal("abandoned unresolved pending attachment")
	}
}
