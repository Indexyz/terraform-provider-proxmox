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

func TestStoragesDataSourceMetadata(t *testing.T) {
	t.Parallel()

	ds := NewStoragesDataSource()
	var resp datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "proxmox"}, &resp)

	if resp.TypeName != "proxmox_storages" {
		t.Fatalf("unexpected data source name: %q", resp.TypeName)
	}
}

func TestStoragesDataSourceSchemaAttributes(t *testing.T) {
	t.Parallel()

	var schemaResp datasource.SchemaResponse
	NewStoragesDataSource().Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)

	for _, key := range []string{"id", "type", "largest", "storages"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("expected data source attribute %q", key)
		}
	}

	recordAttrs := storagesDataSourceAttribute().NestedObject.Attributes
	for _, key := range []string{"storage", "type", "content", "nodes", "disable", "shared", "capacity_bytes"} {
		if _, ok := recordAttrs[key]; !ok {
			t.Fatalf("expected record attribute %q", key)
		}
	}
}

// baseStoragesResponses mocks a cluster with four configured storages:
// local (node-local dir, reported on two nodes with different sizes),
// bigzfs (largest overall), nfsbak (present in /cluster/resources without
// maxdisk, like a storage not yet indexed), and olddir (absent from
// /cluster/resources entirely).
func baseStoragesResponses() map[string]any {
	return map[string]any{
		"/api2/json/storage": []map[string]any{
			{"storage": "local", "type": "dir", "content": "iso,vztmpl", "nodes": "pve1,pve2", "disable": 0, "shared": 0},
			{"storage": "bigzfs", "type": "zfs", "content": "images", "shared": 0},
			{"storage": "nfsbak", "type": "nfs", "content": "backup", "shared": 1},
			{"storage": "olddir", "type": "dir", "content": "iso"},
		},
		"/api2/json/cluster/resources?type=storage": []map[string]any{
			// Descending sizes prove max aggregation rather than last-entry-wins.
			{"id": "storage/pve2/local", "storage": "local", "node": "pve2", "type": "storage", "plugintype": "dir", "status": "available", "maxdisk": 150},
			{"id": "storage/pve1/local", "storage": "local", "node": "pve1", "type": "storage", "plugintype": "dir", "shared": 0, "status": "available", "maxdisk": 100},
			{"id": "storage/pve1/bigzfs", "storage": "bigzfs", "node": "pve1", "type": "storage", "plugintype": "zfs", "status": "available", "maxdisk": 500},
			{"id": "storage/pve1/nfsbak", "storage": "nfsbak", "node": "pve1", "type": "storage", "plugintype": "nfs", "shared": 1, "status": "unknown"},
		},
	}
}

func TestStoragesDataSourceReadsAndFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		config    map[string]any
		responses map[string]any
		want      []string
	}{
		{
			name:      "no filters aggregates the max capacity across nodes",
			config:    map[string]any{},
			responses: baseStoragesResponses(),
			want: []string{
				"local|dir|150",
				"bigzfs|zfs|500",
				"nfsbak|nfs|<null>",
				"olddir|dir|<null>",
			},
		},
		{
			name:      "type filter",
			config:    map[string]any{"type": "zfs"},
			responses: baseStoragesResponses(),
			want: []string{
				"bigzfs|zfs|500",
			},
		},
		{
			name:      "type filter miss yields empty list",
			config:    map[string]any{"type": "rbd"},
			responses: baseStoragesResponses(),
			want:      []string{},
		},
		{
			name:      "empty type disables the filter",
			config:    map[string]any{"type": ""},
			responses: baseStoragesResponses(),
			want: []string{
				"local|dir|150",
				"bigzfs|zfs|500",
				"nfsbak|nfs|<null>",
				"olddir|dir|<null>",
			},
		},
		{
			name:      "largest keeps the single winner",
			config:    map[string]any{"largest": true},
			responses: baseStoragesResponses(),
			want: []string{
				"bigzfs|zfs|500",
			},
		},
		{
			name:      "largest applies after the type filter",
			config:    map[string]any{"largest": true, "type": "dir"},
			responses: baseStoragesResponses(),
			want: []string{
				"local|dir|150",
			},
		},
		{
			name:      "largest excludes storages without reported capacity",
			config:    map[string]any{"largest": true, "type": "nfs"},
			responses: baseStoragesResponses(),
			want:      []string{},
		},
		{
			name:   "largest with all capacities missing yields empty list",
			config: map[string]any{"largest": true},
			responses: map[string]any{
				"/api2/json/storage":                        []map[string]any{{"storage": "local", "type": "dir", "content": "iso"}},
				"/api2/json/cluster/resources?type=storage": []map[string]any{},
			},
			want: []string{},
		},
		{
			name:   "largest returns all tied storages",
			config: map[string]any{"largest": true},
			responses: map[string]any{
				"/api2/json/storage": []map[string]any{
					{"storage": "a", "type": "dir", "content": "iso"},
					{"storage": "b", "type": "zfs", "content": "images"},
					{"storage": "c", "type": "nfs", "content": "backup"},
				},
				"/api2/json/cluster/resources?type=storage": []map[string]any{
					{"storage": "a", "node": "pve1", "type": "storage", "plugintype": "dir", "maxdisk": 300},
					{"storage": "b", "node": "pve1", "type": "storage", "plugintype": "zfs", "maxdisk": 300},
					{"storage": "c", "node": "pve1", "type": "storage", "plugintype": "nfs", "maxdisk": 100},
				},
			},
			want: []string{
				"a|dir|300",
				"b|zfs|300",
			},
		},
		{
			name:   "reported zero capacity is preserved",
			config: map[string]any{},
			responses: map[string]any{
				"/api2/json/storage":                        []map[string]any{{"storage": "empty", "type": "dir", "content": "iso"}},
				"/api2/json/cluster/resources?type=storage": []map[string]any{{"storage": "empty", "node": "pve1", "type": "storage", "plugintype": "dir", "maxdisk": 0}},
			},
			want: []string{
				"empty|dir|0",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runStoragesRead(t, test.config, test.responses, test.want)
		})
	}
}

// runStoragesRead executes one storages data source read against a mock PVE
// API and asserts the resulting storages list via a compact projection:
// `<storage>|<type>|<capacity_bytes>` with `<null>` for absent values.
func runStoragesRead(t *testing.T, config map[string]any, responses map[string]any, want []string) {
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

	ds := NewStoragesDataSource()
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

	var state StoragesDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("get state: %v", diags)
	}

	var storagesList types.List
	if diags := readResp.State.GetAttribute(context.Background(), path.Root("storages"), &storagesList); diags.HasError() {
		t.Fatalf("get storages list: %v", diags)
	}
	if storagesList.IsNull() {
		t.Fatalf("expected storages to be a non-null list")
	}

	if _, configured := config["largest"]; configured {
		want, ok := config["largest"].(bool)
		if !ok {
			t.Fatalf("unexpected largest filter value %#v", config["largest"])
		}
		if state.Largest.IsNull() || state.Largest.ValueBool() != want {
			t.Fatalf("expected largest filter %v to echo into state, got %v", want, state.Largest)
		}
	} else if !state.Largest.IsNull() {
		t.Fatalf("expected unset largest filter to stay null, got %v", state.Largest)
	}
	if value, configured := config["type"]; configured {
		want, ok := value.(string)
		if !ok {
			t.Fatalf("unexpected type filter value %#v", value)
		}
		if state.Type.IsNull() || state.Type.ValueString() != want {
			t.Fatalf("expected type filter %q to echo into state, got %v", value, state.Type)
		}
	} else if !state.Type.IsNull() {
		t.Fatalf("expected unset type filter to stay null, got %v", state.Type)
	}

	got := make([]string, 0, len(state.Storages))
	for _, record := range state.Storages {
		recordType := "<null>"
		if !record.Type.IsNull() {
			recordType = record.Type.ValueString()
		}
		capacity := "<null>"
		if !record.CapacityBytes.IsNull() {
			capacity = fmt.Sprintf("%d", record.CapacityBytes.ValueInt64())
		}
		got = append(got, fmt.Sprintf("%s|%s|%s", record.Storage.ValueString(), recordType, capacity))
	}

	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected storages projection:\n got %v\nwant %v", got, want)
	}
	if len(state.Storages) != len(want) {
		t.Fatalf("unexpected storages count: got %d want %d", len(state.Storages), len(want))
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
