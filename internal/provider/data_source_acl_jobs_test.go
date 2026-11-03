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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

func TestBackupJobsDataSourceFiltersByNode(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/backup": []map[string]any{
			{"id": "job-b", "node": "node2", "all": 1, "enabled": 1, "storage": "backup", "mode": "snapshot", "next-run": 1700000000},
			{"id": "job-a", "node": "node1", "vmid": "100,101", "schedule": "02:00", "mode": "suspend", "compress": "zstd", "comment": "nightly", "exclude": "102", "pool": "prod"},
		},
	}
	readResp := runSDNDataSource(t, NewBackupJobsDataSource(), responses, map[string]any{})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state backupJobsDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	got := make([]string, 0, len(state.Jobs))
	for _, job := range state.Jobs {
		got = append(got, fmt.Sprintf("%s|%s|%v|%s|%s|%s|%d", job.ID.ValueString(), job.Node.ValueString(), job.All.ValueBool(), job.VMIDs.ValueString(), job.ExcludeVMID.ValueString(), job.Mode.ValueString(), job.NextRun.ValueInt64()))
	}
	want := []string{"job-a|node1|false|100,101|102|suspend|0", "job-b|node2|true|||snapshot|1700000000"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected projection:\n got %v\nwant %v", got, want)
	}

	filtered := runSDNDataSource(t, NewBackupJobsDataSource(), responses, map[string]any{"node": "node1"})
	var filteredState backupJobsDataSourceModel
	if diags := filtered.State.Get(context.Background(), &filteredState); diags.HasError() || len(filteredState.Jobs) != 1 || filteredState.Jobs[0].ID.ValueString() != "job-a" {
		t.Fatalf("node filter failed: %#v (%v)", filteredState.Jobs, diags)
	}
}

func TestReplicationJobsDataSourceFiltersByGuest(t *testing.T) {
	responses := map[string]any{
		"/api2/json/cluster/replication": []map[string]any{
			{"id": "101-0", "guest": 101, "jobnum": 0, "type": "local", "source": "node1", "target": "node2", "rate": 10.5, "disable": 1},
			{"id": "102-0", "guest": 102, "jobnum": 0, "type": "local", "source": "node1", "target": "node3", "schedule": "*/15", "comment": "db"},
		},
	}
	readResp := runSDNDataSource(t, NewReplicationJobsDataSource(), responses, map[string]any{})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state replicationJobsDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if len(state.Jobs) != 2 || state.Jobs[0].ID.ValueString() != "101-0" || state.Jobs[0].Rate.ValueFloat64() != 10.5 || !state.Jobs[0].Disable.ValueBool() {
		t.Fatalf("unexpected projection: %#v", state.Jobs)
	}
	if state.Jobs[1].Target.ValueString() != "node3" || state.Jobs[1].Comment.ValueString() != "db" {
		t.Fatalf("unexpected second job: %#v", state.Jobs[1])
	}

	filtered := runSDNDataSource(t, NewReplicationJobsDataSource(), responses, map[string]any{"guest": int64(102)})
	var filteredState replicationJobsDataSourceModel
	if diags := filtered.State.Get(context.Background(), &filteredState); diags.HasError() || len(filteredState.Jobs) != 1 || filteredState.Jobs[0].ID.ValueString() != "102-0" {
		t.Fatalf("guest filter failed: %#v (%v)", filteredState.Jobs, diags)
	}
}

func TestACLDataSourceFiltersAndSorts(t *testing.T) {
	responses := map[string]any{
		"/api2/json/access/acl": []map[string]any{
			{"path": "/vms/102", "type": "user", "ugid": "bob@pve", "roleid": "PVEVMUser", "propagate": 1},
			{"path": "/vms/101", "type": "group", "ugid": "ops", "roleid": "PVEVMAdmin"},
			{"path": "/vms/101", "type": "user", "ugid": "alice@pve", "roleid": "PVEAuditor", "propagate": 0},
		},
	}
	readResp := runSDNDataSource(t, NewACLDataSource(), responses, map[string]any{})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state aclDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	got := make([]string, 0, len(state.Entries))
	for _, entry := range state.Entries {
		got = append(got, fmt.Sprintf("%s|%s|%s|%s|%v", entry.Path.ValueString(), entry.Type.ValueString(), entry.UGID.ValueString(), entry.RoleID.ValueString(), entry.Propagate.ValueBool()))
	}
	want := []string{"/vms/101|group|ops|PVEVMAdmin|false", "/vms/101|user|alice@pve|PVEAuditor|false", "/vms/102|user|bob@pve|PVEVMUser|true"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected projection:\n got %v\nwant %v", got, want)
	}

	filtered := runSDNDataSource(t, NewACLDataSource(), responses, map[string]any{"path": "/vms/101", "type": "user"})
	var filteredState aclDataSourceModel
	if diags := filtered.State.Get(context.Background(), &filteredState); diags.HasError() || len(filteredState.Entries) != 1 || filteredState.Entries[0].UGID.ValueString() != "alice@pve" {
		t.Fatalf("filters failed: %#v (%v)", filteredState.Entries, diags)
	}
}

func TestJobAndACLDataSourcesEmptyAndError(t *testing.T) {
	for name, testCase := range map[string]struct {
		dataSource datasource.DataSource
		path       string
		config     map[string]any
	}{
		"backup_jobs":      {NewBackupJobsDataSource(), "/api2/json/cluster/backup", map[string]any{}},
		"replication_jobs": {NewReplicationJobsDataSource(), "/api2/json/cluster/replication", map[string]any{}},
		"acl":              {NewACLDataSource(), "/api2/json/access/acl", map[string]any{}},
	} {
		t.Run(name, func(t *testing.T) {
			empty := runSDNDataSource(t, testCase.dataSource, map[string]any{testCase.path: []any{}}, testCase.config)
			if empty.Diagnostics.HasError() {
				t.Fatalf("empty read failed: %v", empty.Diagnostics)
			}

			handler := &lifecycleHandler{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handler.auth(w, r)
				writeAPIError(t, w, http.StatusForbidden, "missing permission")
			}))
			defer server.Close()
			var schemaResp datasource.SchemaResponse
			testCase.dataSource.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
			configurable, ok := testCase.dataSource.(datasource.DataSourceWithConfigure)
			if !ok {
				t.Fatalf("%T does not implement DataSourceWithConfigure", testCase.dataSource)
			}
			configurable.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: testLifecycleClient(t, server)}, &datasource.ConfigureResponse{})
			readResp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
			testCase.dataSource.Read(context.Background(), datasource.ReadRequest{Config: testDataSourceConfig(t, schemaResp, testCase.config)}, &readResp)
			if len(readResp.Diagnostics) == 0 {
				t.Fatal("expected API error diagnostics")
			}
		})
	}
}
