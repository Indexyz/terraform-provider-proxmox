// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestStorageVolumeClientRequests(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.envelope(w, map[string]any{"format": "raw", "notes": "n", "protected": 1, "size": 10, "used": 5})
		case http.MethodPut:
			if !handler.form(w, r, url.Values{"notes": {"n"}, "protected": {"1"}}) {
				return
			}
			handler.envelope(w, nil)
		case http.MethodDelete:
			handler.envelope(w, nil)
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)
	metadata, err := client.StorageVolume(context.Background(), "pve-1", "local", "local:backup/b.vma.zst")
	if err != nil || metadata.Protected.Ptr() == nil || !*metadata.Protected.Ptr() {
		t.Fatalf("unexpected metadata: %#v (%v)", metadata, err)
	}
	if err := client.SetStorageVolumeMetadata(context.Background(), "pve-1", "local", "local:backup/b.vma.zst", "n", true); err != nil {
		t.Fatalf("set metadata unexpected error: %v", err)
	}
	if err := client.DeleteStorageVolume(context.Background(), "pve-1", "local", "local:backup/b.vma.zst"); err != nil {
		t.Fatalf("delete unexpected error: %v", err)
	}
	handler.assert(t)
}

func TestStorageVolumeLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":null}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"format":"raw","notes":"n","protected":1,"size":10}}`))
		}
	}))
	defer server.Close()
	res := &StorageVolumeResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := storageVolumeModel{
		Node: types.StringValue("pve-1"), Storage: types.StringValue("local"), Volume: types.StringValue("local:backup/b.vma.zst"),
		Notes: types.StringValue("n"), Protected: types.BoolValue(true),
	}
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("volume diagnostics: %v", resp.Diagnostics)
	}
	var state storageVolumeModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.ID.ValueString() != "pve-1/local/local:backup/b.vma.zst" || !state.Protected.ValueBool() || state.Size.ValueInt64() != 10 {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestStorageVolumeImportState(t *testing.T) {
	schema := testResourceSchema(t, &StorageVolumeResource{})
	tfType := schema.Schema.Type().TerraformType(context.Background())
	resp := resource.ImportStateResponse{State: tfsdk.State{Schema: schema.Schema, Raw: tftypes.NewValue(tfType, nil)}}
	(&StorageVolumeResource{}).ImportState(context.Background(), resource.ImportStateRequest{ID: "pve-1/local/local:backup/b.vma.zst"}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}
}

func TestStorageVolumeTransferLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			if !handler.form(w, r, url.Values{"target": {"local:323/copy.raw"}, "target_node": {"pve-2"}}) {
				return
			}
			handler.envelope(w, nodeBackupTaskUPID)
		default:
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		}
	}))
	defer server.Close()
	res := &StorageVolumeTransferResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := storageVolumeTransferModel{
		Node: types.StringValue("pve-1"), Storage: types.StringValue("local"), Volume: types.StringValue("local:323/vm-323-disk-0.raw"),
		Target: types.StringValue("local:323/copy.raw"), TargetNode: types.StringValue("pve-2"),
		Triggers: testStringMap(t, map[string]string{"gen": "1"}),
	}
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("transfer diagnostics: %v", resp.Diagnostics)
	}
	var state storageVolumeTransferModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.ID.ValueString() != "local/local:323/vm-323-disk-0.raw" {
		t.Fatalf("unexpected state: %#v", state)
	}
}
