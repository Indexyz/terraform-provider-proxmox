// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestDataSourceSuccessfulReads(t *testing.T) {
	vmID := int64(100)
	tests := []struct {
		name       string
		dataSource datasource.DataSource
		config     map[string]any
		responses  map[string]any
		attribute  path.Path
		want       string
	}{
		{
			name: "version", dataSource: NewVersionDataSource(), config: map[string]any{},
			responses: map[string]any{"/api2/json/version": map[string]any{"console": "xtermjs", "release": "9.2", "repoid": "abc", "version": "9.2.1"}},
			attribute: path.Root("version"), want: "9.2.1",
		},
		{
			name: "nodes", dataSource: NewNodesDataSource(), config: map[string]any{},
			responses: map[string]any{"/api2/json/nodes": []map[string]any{{"node": "pve", "status": "online", "maxcpu": 4, "maxmem": 8192, "mem": 1024, "cpu": 0.25, "uptime": 60}}},
			attribute: path.Root("nodes").AtListIndex(0).AtName("name"), want: "pve",
		},
		{
			name: "node", dataSource: NewNodeDataSource(), config: map[string]any{"node": "pve"},
			responses: map[string]any{"/api2/json/nodes/pve/status": map[string]any{
				"boot-info": map[string]any{"mode": "efi", "secureboot": true}, "cpu": 0.25,
				"cpuinfo":        map[string]any{"cores": 2, "cpus": 4, "model": "Unit CPU", "sockets": 2},
				"current-kernel": map[string]any{"machine": "x86_64", "release": "6.14", "sysname": "Linux", "version": "unit"},
				"loadavg":        []string{"0.1", "0.2", "0.3"}, "memory": map[string]any{"available": 7000, "free": 6000, "total": 8192, "used": 1192},
				"pveversion": "pve-manager/9.2.1", "rootfs": map[string]any{"avail": 100, "free": 90, "total": 120, "used": 30},
			}},
			attribute: path.Root("cpu_model"), want: "Unit CPU",
		},
		{
			name: "node dns", dataSource: NewNodeDNSDataSource(), config: map[string]any{"node": "pve"},
			responses: map[string]any{"/api2/json/nodes/pve/dns": map[string]any{"dns1": "1.1.1.1", "search": "example.test"}},
			attribute: path.Root("dns1"), want: "1.1.1.1",
		},
		{
			name: "node time", dataSource: NewNodeTimeDataSource(), config: map[string]any{"node": "pve"},
			responses: map[string]any{"/api2/json/nodes/pve/time": map[string]any{"localtime": 100, "time": 99, "timezone": "UTC"}},
			attribute: path.Root("timezone"), want: "UTC",
		},
		{
			name: "cluster resources", dataSource: NewClusterResourcesDataSource(), config: map[string]any{"type": "node"},
			responses: map[string]any{"/api2/json/cluster/resources?type=node": []map[string]any{{"id": "node/pve", "node": "pve", "type": "node", "status": "online", "cpu": 0.25, "maxcpu": 4}}},
			attribute: path.Root("resources").AtListIndex(0).AtName("id"), want: "node/pve",
		},
		{
			name: "metrics", dataSource: NewClusterMetricsServersDataSource(), config: map[string]any{},
			responses: map[string]any{"/api2/json/cluster/metrics/server": []map[string]any{{"id": "graphite", "type": "graphite", "server": "metrics.test", "port": 2003, "disable": 0}}},
			attribute: path.Root("servers").AtListIndex(0).AtName("id"), want: "graphite",
		},
		{
			name: "storage", dataSource: NewStorageDataSource(), config: map[string]any{"storage": "local"},
			responses: map[string]any{"/api2/json/storage/local": map[string]any{"storage": "local", "type": "dir", "content": "iso", "path": "/var/lib/vz", "disable": 0, "shared": 0}},
			attribute: path.Root("type"), want: "dir",
		},
		{
			name: "storages", dataSource: NewStoragesDataSource(), config: map[string]any{},
			responses: map[string]any{"/api2/json/storage": []map[string]any{{"storage": "local", "type": "dir", "content": "iso", "disable": 0, "shared": 0}}},
			attribute: path.Root("storages").AtListIndex(0).AtName("storage"), want: "local",
		},
		{
			name: "pool", dataSource: NewPoolDataSource(), config: map[string]any{"pool_id": "pool-a"},
			responses: map[string]any{"/api2/json/pools?poolid=pool-a": []map[string]any{{"poolid": "pool-a", "comment": "Unit", "members": []map[string]any{{"id": "qemu/100", "node": "pve", "type": "qemu", "vmid": vmID}}}}},
			attribute: path.Root("comment"), want: "Unit",
		},
		{
			name: "pools", dataSource: NewPoolsDataSource(), config: map[string]any{},
			responses: map[string]any{"/api2/json/pools": []map[string]any{{"poolid": "pool-a", "comment": "Unit", "members": []any{}}}},
			attribute: path.Root("pools").AtListIndex(0).AtName("pool_id"), want: "pool-a",
		},
		{
			name: "group", dataSource: NewGroupDataSource(), config: map[string]any{"group_id": "admins"},
			responses: map[string]any{"/api2/json/access/groups/admins": map[string]any{"comment": "Admins", "members": []string{"root@pam"}}},
			attribute: path.Root("comment"), want: "Admins",
		},
		{
			name: "groups", dataSource: NewGroupsDataSource(), config: map[string]any{},
			responses: map[string]any{"/api2/json/access/groups": []map[string]any{{"groupid": "admins", "comment": "Admins", "users": "root@pam"}}},
			attribute: path.Root("groups").AtListIndex(0).AtName("group_id"), want: "admins",
		},
		{
			name: "role", dataSource: NewRoleDataSource(), config: map[string]any{"role_id": "PVEAdmin"},
			responses: map[string]any{"/api2/json/access/roles/PVEAdmin": map[string]any{"Sys.Audit": 1}},
			attribute: path.Root("privs"), want: "Sys.Audit",
		},
		{
			name: "roles", dataSource: NewRolesDataSource(), config: map[string]any{},
			responses: map[string]any{"/api2/json/access/roles": []map[string]any{{"roleid": "PVEAdmin", "privs": "Sys.Audit"}}},
			attribute: path.Root("roles").AtListIndex(0).AtName("role_id"), want: "PVEAdmin",
		},
		{
			name: "user", dataSource: NewUserDataSource(), config: map[string]any{"user_id": "root@pam"},
			responses: map[string]any{"/api2/json/access/users/root@pam": map[string]any{"comment": "Root", "email": "root@example.test", "enable": 1, "expire": 0, "groups": []string{"admins"}}},
			attribute: path.Root("email"), want: "root@example.test",
		},
		{
			name: "users", dataSource: NewUsersDataSource(), config: map[string]any{},
			responses: map[string]any{"/api2/json/access/users": []map[string]any{{"userid": "root@pam", "comment": "Root", "email": "root@example.test", "enable": 1, "expire": 0, "groups": "admins"}}},
			attribute: path.Root("users").AtListIndex(0).AtName("user_id"), want: "root@pam",
		},
		{
			name: "realm", dataSource: NewRealmDataSource(), config: map[string]any{"realm": "pam"},
			responses: map[string]any{"/api2/json/access/domains/pam": map[string]any{"type": "pam", "comment": "Linux PAM", "default": 1}},
			attribute: path.Root("type"), want: "pam",
		},
		{
			name: "qemu", dataSource: NewQemuVMDataSource(), config: map[string]any{"node": "pve", "vm_id": 100},
			responses: map[string]any{
				"/api2/json/nodes/pve/qemu/100/config":         map[string]any{"name": "unit-vm", "memory": 512, "cores": 1},
				"/api2/json/nodes/pve/qemu/100/status/current": map[string]any{"status": "running", "uptime": 15},
			}, attribute: path.Root("name"), want: "unit-vm",
		},
		{
			name: "lxc", dataSource: NewLXCContainerDataSource(), config: map[string]any{"node": "pve", "vm_id": 101},
			responses: map[string]any{
				"/api2/json/nodes/pve/lxc/101/config":         map[string]any{"hostname": "unit-ct", "rootfs": "local:subvol-101-disk-0,size=8G", "memory": 512, "cores": 1},
				"/api2/json/nodes/pve/lxc/101/status/current": map[string]any{"status": "running", "uptime": 10},
			}, attribute: path.Root("hostname"), want: "unit-ct",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
				data, ok := test.responses[key]
				if !ok || r.Method != http.MethodGet {
					handler.fail(w, "unexpected request: %s %s", r.Method, key)
					return
				}
				seen[key]++
				handler.envelope(w, data)
			}))
			defer server.Close()

			var schemaResp datasource.SchemaResponse
			test.dataSource.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
			configurable, ok := test.dataSource.(datasource.DataSourceWithConfigure)
			if !ok {
				t.Fatalf("%T does not implement DataSourceWithConfigure", test.dataSource)
			}
			var configureResp datasource.ConfigureResponse
			configurable.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: testLifecycleClient(t, server)}, &configureResp)
			if configureResp.Diagnostics.HasError() {
				t.Fatalf("configure diagnostics: %v", configureResp.Diagnostics)
			}
			readResp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
			test.dataSource.Read(context.Background(), datasource.ReadRequest{Config: testDataSourceConfig(t, schemaResp, test.config)}, &readResp)
			if readResp.Diagnostics.HasError() {
				t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
			}
			var got types.String
			if diags := readResp.State.GetAttribute(context.Background(), test.attribute, &got); diags.HasError() {
				t.Fatalf("read %s from state: %v", test.attribute, diags)
			}
			if got.ValueString() != test.want {
				t.Fatalf("unexpected %s: got %q want %q", test.attribute, got.ValueString(), test.want)
			}
			handler.assert(t)
			for endpoint := range test.responses {
				if seen[endpoint] != 1 {
					t.Errorf("endpoint %s called %d times", endpoint, seen[endpoint])
				}
			}
		})
	}
}

func TestVersionDataSourcePreservesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"message":"missing Sys.Audit"}`)
	}))
	defer server.Close()
	ds := &VersionDataSource{client: testLifecycleClient(t, server)}
	var resp datasource.ReadResponse
	ds.Read(context.Background(), datasource.ReadRequest{}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "status 403") || !containsDiagnostic(resp.Diagnostics, "missing Sys.Audit") {
		t.Fatalf("expected preserved API error diagnostics, got %v", resp.Diagnostics)
	}
}

func containsDiagnostic(diags diag.Diagnostics, want string) bool {
	for _, diagnostic := range diags {
		if strings.Contains(diagnostic.Detail(), want) {
			return true
		}
	}
	return false
}
