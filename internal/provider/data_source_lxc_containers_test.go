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

func TestLXCContainersDataSourceMetadata(t *testing.T) {
	t.Parallel()

	ds := NewLXCContainersDataSource()
	var resp datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "proxmox"}, &resp)

	if resp.TypeName != "proxmox_lxc_containers" {
		t.Fatalf("unexpected data source name: %q", resp.TypeName)
	}
}

func TestLXCContainersDataSourceSchemaAttributes(t *testing.T) {
	t.Parallel()

	var schemaResp datasource.SchemaResponse
	NewLXCContainersDataSource().Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)

	for _, key := range []string{"id", "template", "name", "node", "containers"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("expected data source attribute %q", key)
		}
	}

	recordAttrs := lxcContainersDataSourceAttribute().NestedObject.Attributes
	for _, key := range []string{"vm_id", "name", "node", "template", "status", "tags", "pool", "max_cpu", "max_memory", "max_disk"} {
		if _, ok := recordAttrs[key]; !ok {
			t.Fatalf("expected record attribute %q", key)
		}
	}
}

func TestLXCContainersDataSourceReadsAndFilters(t *testing.T) {
	t.Parallel()

	// Fixture order is deliberately unsorted and mixes guest types; the
	// lxc/105 entry has no RRD-backed name/template yet, like a freshly
	// created container on real PVE.
	responses := map[string]any{
		"/api2/json/cluster/resources?type=vm": []map[string]any{
			{"id": "lxc/105", "vmid": 105, "node": "pve1", "type": "lxc", "status": "running"},
			{"id": "qemu/100", "vmid": 100, "name": "web-1", "node": "pve2", "type": "qemu", "template": 0, "status": "running"},
			{"id": "lxc/101", "vmid": 101, "name": "ct-1", "node": "pve1", "type": "lxc", "template": 0, "status": "stopped", "tags": "prod;db", "pool": "apps", "maxcpu": 2, "maxmem": 1024, "maxdisk": 4294967296},
			{"id": "lxc/99", "vmid": 99, "name": "debian-13", "node": "pve1", "type": "lxc", "template": 1, "status": "stopped"},
		},
	}

	tests := []struct {
		name   string
		config map[string]any
		want   []string
	}{
		{
			name:   "no filters returns sorted lxc guests only",
			config: map[string]any{},
			want: []string{
				"99|debian-13|pve1|true",
				"101|ct-1|pve1|false",
				"105|<null>|pve1|<null>",
			},
		},
		{
			name:   "template filter true",
			config: map[string]any{"template": true},
			want: []string{
				"99|debian-13|pve1|true",
			},
		},
		{
			name:   "template filter false treats missing flag as non-template",
			config: map[string]any{"template": false},
			want: []string{
				"101|ct-1|pve1|false",
				"105|<null>|pve1|<null>",
			},
		},
		{
			name:   "name filter exact match",
			config: map[string]any{"name": "ct-1"},
			want: []string{
				"101|ct-1|pve1|false",
			},
		},
		{
			name:   "name filter miss yields empty list",
			config: map[string]any{"name": "missing"},
			want:   []string{},
		},
		{
			name:   "qemu name does not match lxc list",
			config: map[string]any{"name": "web-1"},
			want:   []string{},
		},
		{
			name:   "node filter",
			config: map[string]any{"node": "pve1"},
			want: []string{
				"99|debian-13|pve1|true",
				"101|ct-1|pve1|false",
				"105|<null>|pve1|<null>",
			},
		},
		{
			name:   "combined filters match",
			config: map[string]any{"template": true, "name": "debian-13", "node": "pve1"},
			want: []string{
				"99|debian-13|pve1|true",
			},
		},
		{
			name:   "combined filters exclude",
			config: map[string]any{"template": true, "name": "ct-1"},
			want:   []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runLXCContainersRead(t, test.config, responses, test.want)
		})
	}
}

func TestLXCContainersDataSourceToleratesBooleanTemplateFlag(t *testing.T) {
	t.Parallel()

	responses := map[string]any{
		"/api2/json/cluster/resources?type=vm": []map[string]any{
			{"id": "lxc/99", "vmid": 99, "name": "tmpl", "node": "pve1", "type": "lxc", "template": true, "status": "stopped"},
		},
	}
	runLXCContainersRead(t, map[string]any{"template": true}, responses, []string{"99|tmpl|pve1|true"})
}

func TestLXCContainersDataSourceSortsNumerically(t *testing.T) {
	t.Parallel()

	responses := map[string]any{
		"/api2/json/cluster/resources?type=vm": []map[string]any{
			{"id": "lxc/1000", "vmid": 1000, "name": "high", "node": "pve1", "type": "lxc", "template": 0, "status": "stopped"},
			{"id": "lxc/99", "vmid": 99, "name": "low", "node": "pve1", "type": "lxc", "template": 0, "status": "stopped"},
			{"id": "lxc/100", "vmid": 100, "name": "middle", "node": "pve1", "type": "lxc", "template": 0, "status": "stopped"},
		},
	}
	runLXCContainersRead(t, map[string]any{}, responses, []string{
		"99|low|pve1|false",
		"100|middle|pve1|false",
		"1000|high|pve1|false",
	})
}

// runLXCContainersRead executes one lxc_containers read against a mock PVE API
// and asserts the resulting containers list via a compact projection:
// `<vm_id>|<name>|<node>|<template>` with `<null>` for absent values.
func runLXCContainersRead(t *testing.T, config map[string]any, responses map[string]any, want []string) {
	t.Helper()

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
		handler.envelope(w, data)
	}))
	defer server.Close()

	ds := NewLXCContainersDataSource()
	var schemaResp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
	configurable, ok := ds.(datasource.DataSourceWithConfigure)
	if !ok {
		t.Fatalf("%T does not implement DataSourceWithConfigure", ds)
	}
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

	var state LXCContainersDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("get state: %v", diags)
	}
	if state.ID.ValueString() != "lxc_containers" {
		t.Fatalf("unexpected id marker: %q", state.ID.ValueString())
	}

	var containersList types.List
	if diags := readResp.State.GetAttribute(context.Background(), path.Root("containers"), &containersList); diags.HasError() {
		t.Fatalf("get containers list: %v", diags)
	}
	if containersList.IsNull() {
		t.Fatalf("expected containers to be a non-null list")
	}

	if _, configured := config["template"]; configured {
		want, ok := config["template"].(bool)
		if !ok {
			t.Fatalf("unexpected template filter value %#v", config["template"])
		}
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
			want, ok := value.(string)
			if !ok {
				t.Fatalf("unexpected %s filter value %#v", filter.key, value)
			}
			if filter.got.IsNull() || filter.got.ValueString() != want {
				t.Fatalf("expected %s filter %q to echo into state, got %v", filter.key, value, filter.got)
			}
		} else if !filter.got.IsNull() {
			t.Fatalf("expected unset %s filter to stay null, got %v", filter.key, filter.got)
		}
	}

	got := make([]string, 0, len(state.Containers))
	for _, record := range state.Containers {
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
		t.Fatalf("unexpected containers projection:\n got %v\nwant %v", got, want)
	}
	if len(state.Containers) != len(want) {
		t.Fatalf("unexpected containers count: got %d want %d", len(state.Containers), len(want))
	}

	for _, record := range state.Containers {
		if record.VMID.ValueInt64() != 101 {
			continue
		}
		if !record.Tags.Equal(types.StringValue("prod;db")) || !record.Pool.Equal(types.StringValue("apps")) {
			t.Fatalf("expected ct-1 tags/pool to be preserved, got tags=%v pool=%v", record.Tags, record.Pool)
		}
		if !record.Status.Equal(types.StringValue("stopped")) || record.MaxCPU.IsNull() || record.MaxCPU.ValueFloat64() != 2 {
			t.Fatalf("unexpected ct-1 record: %#v", record)
		}
	}
}
