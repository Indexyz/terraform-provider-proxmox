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

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const agentExecPID = 4242

func TestClientAgentExec(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/qemu/320/agent/exec" && r.Method == http.MethodPost:
			if !handler.form(w, r, url.Values{"command": {"/bin/sh", "-c", "echo hi"}, "input-data": {"seed"}}) {
				return
			}
			handler.envelope(w, map[string]any{"pid": agentExecPID})
		case r.URL.Path == "/api2/json/nodes/pve-1/qemu/320/agent/exec-status" && r.Method == http.MethodGet:
			if r.URL.Query().Get("pid") != "4242" {
				handler.fail(w, "unexpected pid query: %s", r.URL.RawQuery)
				return
			}
			handler.envelope(w, map[string]any{"exited": true, "exitcode": 0, "out-data": "hi\n"})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)
	pid, err := client.AgentExec(context.Background(), "pve-1", 320, []string{"/bin/sh", "-c", "echo hi"}, "seed")
	if err != nil || pid != agentExecPID {
		t.Fatalf("AgentExec() = %d, %v", pid, err)
	}
	status, err := client.AgentExecStatus(context.Background(), "pve-1", 320, pid)
	if err != nil || status.Exited.Ptr() == nil || !*status.Exited.Ptr() || status.OutData != "hi\n" {
		t.Fatalf("unexpected status: %#v (%v)", status, err)
	}
	handler.assert(t)
}

func TestQemuAgentDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/qemu/320/agent": []map[string]any{{"name": "exec"}, {"name": "get-osinfo"}},
		"/api2/json/nodes/pve-1/qemu/320/agent/get-osinfo": map[string]any{"result": map[string]any{
			"id": "debian", "name": "Debian GNU/Linux", "pretty-name": "Debian GNU/Linux 13", "version": "13",
			"version-id": "13", "machine": "x86_64", "kernel-release": "6.12", "kernel-version": "#1",
		}},
		"/api2/json/nodes/pve-1/qemu/320/agent/get-host-name":          map[string]any{"result": map[string]any{"host-name": "agent-test"}},
		"/api2/json/nodes/pve-1/qemu/320/agent/get-timezone":           map[string]any{"result": map[string]any{"zone": "UTC", "offset": 3600}},
		"/api2/json/nodes/pve-1/qemu/320/agent/get-fsinfo":             map[string]any{"result": []map[string]any{{"name": "sda1", "mountpoint": "/", "type": "ext4", "total-bytes": 100, "used-bytes": 40, "disk": []map[string]any{{"dev": "/dev/sda1", "bus-type": "scsi", "target": 0}}}}},
		"/api2/json/nodes/pve-1/qemu/320/agent/network-get-interfaces": map[string]any{"result": []map[string]any{{"name": "eth0", "hardware-address": "aa:bb", "ip-addresses": []map[string]any{{"ip-address": "10.0.0.5", "ip-address-type": "ipv4", "prefix": 24}}, "statistics": map[string]any{"rx-bytes": 10}}}},
	}
	readResp := runSDNDataSource(t, NewQemuAgentDataSource(), responses, map[string]any{"node": "pve-1", "vmid": int64(320)})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state qemuAgentModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if len(state.Commands.Elements()) != 2 || state.HostName.ValueString() != "agent-test" || state.TimezoneOffset.ValueInt64() != 3600 {
		t.Fatalf("unexpected state: %#v", state)
	}
	if len(state.Filesystems.Elements()) != 1 || len(state.NetworkInterfaces.Elements()) != 1 {
		t.Fatalf("unexpected collections: %#v", state)
	}
}

func TestQemuAgentExecLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/agent/exec") && r.Method == http.MethodPost:
			handler.envelope(w, map[string]any{"pid": agentExecPID})
		case strings.HasSuffix(r.URL.Path, "/agent/exec-status"):
			handler.envelope(w, map[string]any{"exited": true, "exitcode": 3, "out-data": "out", "err-data": "bad"})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	res := &QemuAgentExecResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := qemuAgentExecModel{
		Node: types.StringValue("pve-1"), VMID: types.Int64Value(320),
		Command:        types.ListValueMust(types.StringType, []attr.Value{types.StringValue("/bin/false")}),
		TimeoutSeconds: types.Int64Value(10), Triggers: testStringMap(t, map[string]string{"gen": "1"}),
	}
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("exec diagnostics: %v", resp.Diagnostics)
	}
	var state qemuAgentExecModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.ID.ValueString() != "pve-1/320/4242" || state.ExitCode.ValueInt64() != 3 || state.Stderr.ValueString() != "bad" {
		t.Fatalf("unexpected state: %#v", state)
	}
}
