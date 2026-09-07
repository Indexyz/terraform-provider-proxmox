// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestQemuVMsDataSourceMetadata(t *testing.T) {
	t.Parallel()

	ds := NewQemuVMsDataSource()
	var resp datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "proxmox"}, &resp)

	if resp.TypeName != "proxmox_qemu_vms" {
		t.Fatalf("unexpected data source name: %q", resp.TypeName)
	}
}

func TestQemuVMsDataSourceSchemaAttributes(t *testing.T) {
	t.Parallel()

	var schemaResp datasource.SchemaResponse
	NewQemuVMsDataSource().Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)

	for _, key := range []string{"id", "template", "name", "node", "vms"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("expected data source attribute %q", key)
		}
	}

	recordAttrs := qemuVMsDataSourceAttribute().NestedObject.Attributes
	for _, key := range []string{"vm_id", "name", "node", "template", "status", "tags", "pool", "max_cpu", "max_memory", "max_disk"} {
		if _, ok := recordAttrs[key]; !ok {
			t.Fatalf("expected record attribute %q", key)
		}
	}
}

func TestQemuVMsDataSourceReadsAndFilters(t *testing.T) {
	t.Parallel()

	// Fixture order is deliberately unsorted and mixes guest types; the
	// qemu/105 entry has no RRD-backed name/template yet, like a freshly
	// created guest on real PVE.
	responses := map[string]any{
		"/api2/json/cluster/resources?type=vm": []map[string]any{
			{"id": "qemu/105", "vmid": 105, "node": "pve1", "type": "qemu", "status": "stopped", "maxcpu": 1, "maxmem": 512, "maxdisk": 1073741824},
			{"id": "qemu/100", "vmid": 100, "name": "web-1", "node": "pve2", "type": "qemu", "template": 0, "status": "running", "tags": "prod;web", "pool": "apps", "maxcpu": 4, "maxmem": 8192, "maxdisk": 107374182400},
			{"id": "lxc/101", "vmid": 101, "name": "ct-1", "node": "pve1", "type": "lxc", "template": 0, "status": "stopped"},
			{"id": "qemu/99", "vmid": 99, "name": "ubuntu-24.04-cloudinit", "node": "pve1", "type": "qemu", "template": 1, "status": "stopped", "maxcpu": 2, "maxmem": 2048, "maxdisk": 8589934592},
		},
	}

	tests := []struct {
		name   string
		config map[string]any
		want   []string
	}{
		{
			name:   "no filters returns sorted qemu guests only",
			config: map[string]any{},
			want: []string{
				"99|ubuntu-24.04-cloudinit|pve1|true",
				"100|web-1|pve2|false",
				"105|<null>|pve1|<null>",
			},
		},
		{
			name:   "template filter true",
			config: map[string]any{"template": true},
			want: []string{
				"99|ubuntu-24.04-cloudinit|pve1|true",
			},
		},
		{
			name:   "template filter false treats missing flag as non-template",
			config: map[string]any{"template": false},
			want: []string{
				"100|web-1|pve2|false",
				"105|<null>|pve1|<null>",
			},
		},
		{
			name:   "name filter exact match",
			config: map[string]any{"name": "web-1"},
			want: []string{
				"100|web-1|pve2|false",
			},
		},
		{
			name:   "name filter miss yields empty list",
			config: map[string]any{"name": "missing"},
			want:   []string{},
		},
		{
			name:   "node filter",
			config: map[string]any{"node": "pve2"},
			want: []string{
				"100|web-1|pve2|false",
			},
		},
		{
			name:   "combined filters match",
			config: map[string]any{"template": true, "name": "ubuntu-24.04-cloudinit", "node": "pve1"},
			want: []string{
				"99|ubuntu-24.04-cloudinit|pve1|true",
			},
		},
		{
			name:   "combined filters exclude",
			config: map[string]any{"template": true, "name": "web-1"},
			want:   []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runQemuVMsRead(t, test.config, responses, test.want)
		})
	}
}

func TestQemuVMsDataSourceToleratesBooleanTemplateFlag(t *testing.T) {
	t.Parallel()

	responses := map[string]any{
		"/api2/json/cluster/resources?type=vm": []map[string]any{
			{"id": "qemu/99", "vmid": 99, "name": "tmpl", "node": "pve1", "type": "qemu", "template": true, "status": "stopped"},
		},
	}
	runQemuVMsRead(t, map[string]any{"template": true}, responses, []string{"99|tmpl|pve1|true"})
}

func TestClusterResourcesDataSourceDecodesNumericFlags(t *testing.T) {
	t.Parallel()

	seen := map[string]int{}
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		key := r.URL.EscapedPath()
		if r.URL.RawQuery != "" {
			key += "?" + r.URL.RawQuery
		}
		data, ok := map[string]any{
			// An unfiltered /cluster/resources query legitimately returns guests
			// and storage entries together, exercising both numeric flags.
			"/api2/json/cluster/resources": []map[string]any{
				{"id": "qemu/100", "vmid": 100, "name": "tmpl", "node": "pve1", "type": "qemu", "template": 1, "status": "stopped"},
				{"id": "storage/pve1/local", "storage": "local", "node": "pve1", "type": "storage", "plugintype": "dir", "shared": 0, "status": "available", "content": "iso"},
			},
		}[key]
		if !ok || r.Method != http.MethodGet {
			handler.fail(w, "unexpected request: %s %s", r.Method, key)
			return
		}
		seen[key]++
		handler.envelope(w, data)
	}))
	defer server.Close()

	ds := NewClusterResourcesDataSource()
	var schemaResp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
	configurable := ds.(datasource.DataSourceWithConfigure)
	var configureResp datasource.ConfigureResponse
	configurable.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: testLifecycleClient(t, server)}, &configureResp)
	if configureResp.Diagnostics.HasError() {
		t.Fatalf("configure diagnostics: %v", configureResp.Diagnostics)
	}

	readResp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	ds.Read(context.Background(), datasource.ReadRequest{Config: testDataSourceConfig(t, schemaResp, map[string]any{})}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}

	var state ClusterResourcesDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("get state: %v", diags)
	}
	if len(state.Resources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(state.Resources))
	}
	if state.Resources[0].Template.IsNull() || !state.Resources[0].Template.ValueBool() {
		t.Fatalf("expected numeric template=1 to decode as true, got %v", state.Resources[0].Template)
	}
	if state.Resources[1].Shared.IsNull() || state.Resources[1].Shared.ValueBool() {
		t.Fatalf("expected numeric shared=0 to decode as false, got %v", state.Resources[1].Shared)
	}
}

// runQemuVMsRead executes one qemu_vms data source read against a mock PVE API
// and asserts the resulting vms list via a compact projection:
// `<vm_id>|<name>|<node>|<template>` with `<null>` for absent values.
func runQemuVMsRead(t *testing.T, config map[string]any, responses map[string]any, want []string) {
	t.Helper()

	seen := map[string]int{}
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		key := r.URL.EscapedPath()
		if r.URL.RawQuery != "" {
			key += "?" + r.URL.RawQuery
		}
		data, ok := responses[key]
		if !ok || r.Method != http.MethodGet {
			handler.fail(w, "unexpected request: %s %s", r.Method, key)
			return
		}
		seen[key]++
		handler.envelope(w, data)
	}))
	defer server.Close()

	ds := NewQemuVMsDataSource()
	var schemaResp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
	configurable := ds.(datasource.DataSourceWithConfigure)
	var configureResp datasource.ConfigureResponse
	configurable.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: testLifecycleClient(t, server)}, &configureResp)
	if configureResp.Diagnostics.HasError() {
		t.Fatalf("configure diagnostics: %v", configureResp.Diagnostics)
	}

	readResp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	ds.Read(context.Background(), datasource.ReadRequest{Config: testDataSourceConfig(t, schemaResp, config)}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}

	var state QemuVMsDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("get state: %v", diags)
	}
	if state.ID.ValueString() != "qemu_vms" {
		t.Fatalf("unexpected id marker: %q", state.ID.ValueString())
	}

	var vmsList types.List
	if diags := readResp.State.GetAttribute(context.Background(), path.Root("vms"), &vmsList); diags.HasError() {
		t.Fatalf("get vms list: %v", diags)
	}
	if vmsList.IsNull() {
		t.Fatalf("expected vms to be a non-null list")
	}

	if _, configured := config["template"]; configured {
		want := config["template"].(bool)
		if state.Template.IsNull() || state.Template.ValueBool() != want {
			t.Fatalf("expected template filter %v to echo into state, got %v", want, state.Template)
		}
	} else if !state.Template.IsNull() {
		t.Fatalf("expected unset template filter to stay null, got %v", state.Template)
	}
	for _, filter := range []struct {
		key string
		got types.String
	}{
		{"name", state.Name},
		{"node", state.Node},
	} {
		if value, configured := config[filter.key]; configured {
			if filter.got.IsNull() || filter.got.ValueString() != value.(string) {
				t.Fatalf("expected %s filter %q to echo into state, got %v", filter.key, value, filter.got)
			}
		} else if !filter.got.IsNull() {
			t.Fatalf("expected unset %s filter to stay null, got %v", filter.key, filter.got)
		}
	}

	got := make([]string, 0, len(state.VMs))
	for _, record := range state.VMs {
		name := "<null>"
		if !record.Name.IsNull() {
			name = record.Name.ValueString()
		}
		template := "<null>"
		if !record.Template.IsNull() && record.Template.ValueBool() {
			template = "true"
		} else if !record.Template.IsNull() {
			template = "false"
		}
		got = append(got, fmt.Sprintf("%d|%s|%s|%s", record.VMID.ValueInt64(), name, record.Node.ValueString(), template))
	}

	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected vms projection:\n got %v\nwant %v", got, want)
	}
	if len(state.VMs) != len(want) {
		t.Fatalf("unexpected vms count: got %d want %d", len(state.VMs), len(want))
	}

	for _, record := range state.VMs {
		if record.VMID.ValueInt64() == 100 {
			if !record.Tags.Equal(types.StringValue("prod;web")) || !record.Pool.Equal(types.StringValue("apps")) {
				t.Fatalf("expected web-1 tags/pool to be preserved, got tags=%v pool=%v", record.Tags, record.Pool)
			}
			if !record.Status.Equal(types.StringValue("running")) ||
				!record.MaxCPU.Equal(types.Float64Value(4)) ||
				!record.MaxMemory.Equal(types.Int64Value(8192)) ||
				!record.MaxDisk.Equal(types.Int64Value(107374182400)) {
				t.Fatalf("unexpected web-1 status/capacity fields: %#v", record)
			}
		}
		if record.VMID.ValueInt64() == 105 && (!record.Pool.IsNull() || !record.Tags.IsNull()) {
			t.Fatalf("expected unset pool/tags to map to null, got pool=%v tags=%v", record.Pool, record.Tags)
		}
	}

	if len(seen) != len(responses) {
		t.Fatalf("unexpected endpoint call count: %v", seen)
	}
	for endpoint := range responses {
		if seen[endpoint] != 1 {
			t.Errorf("endpoint %s called %d times", endpoint, seen[endpoint])
		}
	}
}
