// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestQemuDiskSizeBytes(t *testing.T) {
	for _, tc := range []struct {
		size  string
		bytes float64
	}{
		{"1024", 1024}, {"1K", 1024}, {"1024M", 1 << 30}, {"1.5G", 1.5 * (1 << 30)}, {"1T", 1 << 40},
	} {
		got, err := qemuDiskSizeBytes(tc.size)
		if err != nil || got != tc.bytes {
			t.Fatalf("size %s: got %v, %v", tc.size, got, err)
		}
	}
	for _, size := range []string{"", "0G", "-1G", "+1G", "NaN", "1P"} {
		if _, err := qemuDiskSizeBytes(size); err == nil {
			t.Fatalf("accepted %q", size)
		}
	}
}

func TestQemuResizeRejectsUnsafeDisks(t *testing.T) {
	for _, tc := range []struct{ name, wire, want string }{
		{"missing", "", "does not exist"},
		{"cdrom", "local:iso/seed.iso,media=cdrom,size=1G", "not a resizable"},
		{"cloudinit", "local:vm-101-cloudinit,size=1G", "not a resizable"},
		{"shrink", "local-lvm:vm-101-disk-0,size=64G", "cannot shrink"},
		{"unknown_size", "local-lvm:vm-101-disk-0", "invalid observed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &lifecycleHandler{}
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/config") {
					h.fail(w, "unsafe mutation: %s", r.URL)
					return
				}
				data := map[string]any{}
				if tc.wire != "" {
					data["scsi0"] = tc.wire
				}
				h.envelope(w, data)
			}))
			defer s.Close()
			model := minimalQemuVMModel("pve-1", 101)
			model.DiskResize = types.MapValueMust(types.Int64Type, map[string]attr.Value{"scsi0": types.Int64Value(32)})
			err := (&QemuVMResource{client: testLifecycleClient(t, s)}).resizeDisks(context.Background(), model)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v; want %s", err, tc.want)
			}
			h.assert(t)
		})
	}
}

func TestQemuCloneResizeBeforeStartAndIdempotentUpdate(t *testing.T) {
	h := &lifecycleHandler{}
	size := "8G"
	desired := "32G"
	var events []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.auth(w, r) {
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/clone"):
			events = append(events, "clone")
			h.envelope(w, "UPID:pve-1:qmclone:101")
		case strings.Contains(r.URL.Path, "/tasks/"):
			h.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/config"):
			h.envelope(w, map[string]any{"scsi0": "local-lvm:vm-101-disk-0,size=" + size, "digest": "abc"})
		case r.Method == "PUT" && strings.HasSuffix(r.URL.Path, "/config"):
			if err := r.ParseForm(); err != nil {
				h.fail(w, "parse form: %v", err)
				return
			}
			if r.Form.Has("scsi0") {
				h.fail(w, "resize must not replace inherited root disk")
				return
			}
			events = append(events, "config")
			h.envelope(w, nil)
		case r.Method == "PUT" && strings.HasSuffix(r.URL.Path, "/resize"):
			if !h.form(w, r, url.Values{"disk": {"scsi0"}, "size": {desired}, "digest": {"abc"}}) {
				return
			}
			events = append(events, "resize")
			size = desired
			h.envelope(w, "UPID:pve-1:resize:101")
		case strings.HasSuffix(r.URL.Path, "/status/start"):
			events = append(events, "start")
			h.envelope(w, "UPID:pve-1:qmstart:101")
		case strings.HasSuffix(r.URL.Path, "/status/current"):
			h.envelope(w, map[string]any{"status": "running"})
		default:
			h.fail(w, "unexpected request %s %s", r.Method, r.URL)
		}
	}))
	defer s.Close()
	res := &QemuVMResource{client: testLifecycleClient(t, s)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("pve-1", 101)
	model.Clone = mustQemuVMCloneValue(t, qemuVMCloneModel{SourceVMID: types.Int64Value(9000), Full: types.BoolValue(true)})
	model.Name = types.StringValue("resize-clone")
	model.Power = types.BoolValue(true)
	model.DiskResize = types.MapValueMust(types.Int64Type, map[string]attr.Value{"scsi0": types.Int64Value(32)})
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create: %v", resp.Diagnostics)
	}
	if !reflect.DeepEqual(events, []string{"clone", "config", "resize", "start"}) {
		t.Fatalf("order: %v", events)
	}
	var state qemuVMModel
	if d := resp.State.Get(context.Background(), &state); d.HasError() {
		t.Fatal(d)
	}
	if !state.DiskResize.Equal(model.DiskResize) {
		t.Fatal("lost resize targets")
	}
	events = nil
	update := resource.UpdateResponse{State: resp.State}
	res.Update(context.Background(), resource.UpdateRequest{State: resp.State, Plan: testResourcePlan(t, schema, state)}, &update)
	if update.Diagnostics.HasError() {
		t.Fatalf("update: %v", update.Diagnostics)
	}
	if !reflect.DeepEqual(events, []string{"config"}) {
		t.Fatalf("equal size must not resize/start: %v", events)
	}
	events = nil
	desired = "64G"
	state.DiskResize = types.MapValueMust(types.Int64Type, map[string]attr.Value{"scsi0": types.Int64Value(64)})
	growth := resource.UpdateResponse{State: update.State}
	res.Update(context.Background(), resource.UpdateRequest{State: update.State, Plan: testResourcePlan(t, schema, state)}, &growth)
	if growth.Diagnostics.HasError() {
		t.Fatalf("grow: %v", growth.Diagnostics)
	}
	if !reflect.DeepEqual(events, []string{"config", "resize"}) {
		t.Fatalf("growth order: %v", events)
	}
	var grown qemuVMModel
	if d := growth.State.Get(context.Background(), &grown); d.HasError() {
		t.Fatal(d)
	}
	if !grown.DiskResize.Equal(state.DiskResize) {
		t.Fatal("updated target not preserved")
	}
	events = nil
	read := resource.ReadResponse{State: growth.State}
	res.Read(context.Background(), resource.ReadRequest{State: growth.State}, &read)
	if read.Diagnostics.HasError() || len(events) != 0 {
		t.Fatalf("refresh must not resize: %v %v", read.Diagnostics, events)
	}
	events = nil
	failed := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &failed)
	if !failed.Diagnostics.HasError() || !containsDiagnostic(failed.Diagnostics, "cannot shrink") {
		t.Fatalf("expected shrink rejection: %v", failed.Diagnostics)
	}
	var partial qemuVMModel
	if d := failed.State.Get(context.Background(), &partial); d.HasError() {
		t.Fatal(d)
	}
	if partial.ID.IsNull() || !reflect.DeepEqual(events, []string{"clone", "config"}) {
		t.Fatalf("failed resize must keep clone tracked without starting: %v %v", partial.ID, events)
	}
	h.assert(t)
}

func TestQemuResizeTaskErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ack    any
		status int
		exit   string
	}{
		{"null", nil, 200, "OK"}, {"bad_ack", "garbage", 200, "OK"}, {"poll_404", "UPID:pve-1:resize:101", 404, "OK"}, {"task_failure", "UPID:pve-1:resize:101", 200, "resize failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &lifecycleHandler{}
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PUT" {
					h.envelope(w, tc.ack)
					return
				}
				if tc.status == 404 {
					http.NotFound(w, r)
					return
				}
				h.envelope(w, map[string]any{"status": "stopped", "exitstatus": tc.exit})
			}))
			defer s.Close()
			if err := testLifecycleClient(t, s).ResizeQemuDisk(context.Background(), "pve-1", 101, "scsi0", 32, ""); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestQemuResizeValidation(t *testing.T) {
	for _, tc := range []struct {
		slot string
		size int64
	}{{"scsi31", 32}, {"net0", 32}, {"scsi0", 0}, {"scsi0", -1}, {"scsi0", 1048577}} {
		model := minimalQemuVMModel("pve-1", 101)
		model.DiskResize = types.MapValueMust(types.Int64Type, map[string]attr.Value{tc.slot: types.Int64Value(tc.size)})
		if !validateQemuVMDiskResize(context.Background(), model).HasError() {
			t.Fatal(fmt.Sprintf("accepted %v", tc))
		}
	}
}
