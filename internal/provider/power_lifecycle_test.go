// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestClientShutdownQemuVM(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	ctx := context.Background()
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/qemu/440/status/shutdown":
			// The graceful-then-forced escalation is server-side: one call
			// must carry the timeout and forceStop=1.
			if !handler.form(w, r, url.Values{"forceStop": {"1"}, "timeout": {"120"}}) {
				return
			}
			handler.envelope(w, "UPID:pve one:qmshutdown:440")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qmshutdown:440/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected QEMU shutdown request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := testLifecycleClient(t, server)
	if err := client.ShutdownQemuVM(ctx, "pve one", 440, 120); err != nil {
		t.Fatalf("ShutdownQemuVM() unexpected error: %v", err)
	}
	wantCalls := []string{
		"POST /api2/json/nodes/pve%20one/qemu/440/status/shutdown",
		"GET /api2/json/nodes/pve%20one/tasks/UPID:pve%20one:qmshutdown:440/status",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected QEMU shutdown call order: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestClientShutdownQemuVMRejectsInvalidTaskAck(t *testing.T) {
	for _, tc := range []struct {
		name string
		ack  any
		want []string
	}{
		{name: "empty", ack: nil, want: []string{"shutdown task", "no UPID"}},
		{name: "malformed", ack: "not-a-upid", want: []string{"shutdown task", "invalid UPID"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			handler := &lifecycleHandler{}
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !handler.auth(w, r) {
					return
				}
				calls = append(calls, r.Method+" "+r.URL.EscapedPath())
				switch {
				case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/101/status/shutdown":
					handler.envelope(w, tc.ack)
				default:
					handler.fail(w, "unexpected QEMU shutdown ack request: %s %s", r.Method, r.URL.String())
				}
			}))
			defer server.Close()

			err := testLifecycleClient(t, server).ShutdownQemuVM(ctx, "pve-1", 101, 60)
			if err == nil {
				t.Fatalf("expected shutdown ack error, got nil")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("expected error containing %q, got %v", want, err)
				}
			}
			wantCalls := []string{"POST /api2/json/nodes/pve-1/qemu/101/status/shutdown"}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("invalid acknowledgement must not poll task status: got %v want %v", calls, wantCalls)
			}
			handler.assert(t)
		})
	}
}

func TestClientShutdownQemuVMTaskStatus404RemainsError(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	ctx := context.Background()
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/101/status/shutdown":
			handler.envelope(w, "UPID:pve-1:qmshutdown:101")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:qmshutdown:101/status":
			http.NotFound(w, r)
		default:
			handler.fail(w, "unexpected QEMU shutdown poll request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	err := testLifecycleClient(t, server).ShutdownQemuVM(ctx, "pve-1", 101, 60)
	if err == nil || !strings.Contains(err.Error(), "unable to poll task") {
		t.Fatalf("expected task status 404 to remain an error, got %v", err)
	}
	handler.assert(t)
}

func TestClientLXCContainerPowerMethods(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	ctx := context.Background()
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/301/status/start":
			if !handler.form(w, r, url.Values{}) {
				return
			}
			handler.envelope(w, "UPID:pve-1:vzstart:301")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:vzstart:301/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/301/status/stop":
			if !handler.form(w, r, url.Values{}) {
				return
			}
			handler.envelope(w, "UPID:pve-1:vzstop:301")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:vzstop:301/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/301/status/shutdown":
			if !handler.form(w, r, url.Values{"forceStop": {"1"}, "timeout": {"90"}}) {
				return
			}
			handler.envelope(w, "UPID:pve-1:vzshutdown:301")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:vzshutdown:301/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected LXC power request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := testLifecycleClient(t, server)
	if err := client.StartLXCContainer(ctx, "pve-1", 301); err != nil {
		t.Fatalf("StartLXCContainer() unexpected error: %v", err)
	}
	if err := client.StopLXCContainer(ctx, "pve-1", 301); err != nil {
		t.Fatalf("StopLXCContainer() unexpected error: %v", err)
	}
	if err := client.ShutdownLXCContainer(ctx, "pve-1", 301, 90); err != nil {
		t.Fatalf("ShutdownLXCContainer() unexpected error: %v", err)
	}
	wantCalls := []string{
		"POST /api2/json/nodes/pve-1/lxc/301/status/start",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:vzstart:301/status",
		"POST /api2/json/nodes/pve-1/lxc/301/status/stop",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:vzstop:301/status",
		"POST /api2/json/nodes/pve-1/lxc/301/status/shutdown",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:vzshutdown:301/status",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected LXC power call order: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestClientLXCContainerPowerMethodsRejectMissingTaskAck(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch r.URL.EscapedPath() {
		case "/api2/json/nodes/pve-1/lxc/301/status/start", "/api2/json/nodes/pve-1/lxc/301/status/stop", "/api2/json/nodes/pve-1/lxc/301/status/shutdown":
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected LXC power ack request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := testLifecycleClient(t, server)
	for name, callErr := range map[string]error{
		"start":    client.StartLXCContainer(ctx, "pve-1", 301),
		"stop":     client.StopLXCContainer(ctx, "pve-1", 301),
		"shutdown": client.ShutdownLXCContainer(ctx, "pve-1", 301, 60),
	} {
		if callErr == nil || !strings.Contains(callErr.Error(), "no UPID") {
			t.Fatalf("expected missing %s UPID error, got %v", name, callErr)
		}
	}
	wantCalls := []string{
		"POST /api2/json/nodes/pve-1/lxc/301/status/start",
		"POST /api2/json/nodes/pve-1/lxc/301/status/stop",
		"POST /api2/json/nodes/pve-1/lxc/301/status/shutdown",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("empty acknowledgements must not poll task status: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceValidateConfigPower(t *testing.T) {
	res := &QemuVMResource{}
	schema := testResourceSchema(t, res)

	both := minimalQemuVMModel("pve one", 420)
	both.Power = types.BoolValue(true)
	both.StartOnCreate = types.BoolValue(true)
	bothResp := resource.ValidateConfigResponse{}
	res.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, schema, both)}, &bothResp)
	if !bothResp.Diagnostics.HasError() || !containsDiagnostic(bothResp.Diagnostics, "power") {
		t.Fatalf("expected power/start_on_create conflict diagnostic: %v", bothResp.Diagnostics)
	}

	// power coexists with stop_on_destroy: destroy behavior is unchanged.
	coexist := minimalQemuVMModel("pve one", 420)
	coexist.Power = types.BoolValue(false)
	coexist.StopOnDestroy = types.BoolValue(true)
	coexistResp := resource.ValidateConfigResponse{}
	res.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, schema, coexist)}, &coexistResp)
	if coexistResp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics for power with stop_on_destroy: %v", coexistResp.Diagnostics)
	}

	for _, tc := range []struct {
		value    int64
		hasError bool
	}{
		{value: 0, hasError: true},
		{value: 601, hasError: true},
		{value: 1, hasError: false},
		{value: 600, hasError: false},
	} {
		model := minimalQemuVMModel("pve one", 420)
		model.Power = types.BoolValue(true)
		model.PowerShutdownTimeout = types.Int64Value(tc.value)
		resp := resource.ValidateConfigResponse{}
		res.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, schema, model)}, &resp)
		if resp.Diagnostics.HasError() != tc.hasError {
			t.Fatalf("power_shutdown_timeout %d: unexpected diagnostics %v", tc.value, resp.Diagnostics)
		}
		if tc.hasError && !containsDiagnostic(resp.Diagnostics, "must be between") {
			t.Fatalf("expected range diagnostic for %d: %v", tc.value, resp.Diagnostics)
		}
	}
}

func TestQemuVMResourceCreatePowerTrueStartsGuest(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu":
			if !handler.form(w, r, url.Values{"vmid": {"430"}, "name": {"powered-vm"}}) {
				return
			}
			handler.envelope(w, "UPID:pve-1:qemu-create-power")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:qemu-create-power/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/430/status/start":
			handler.envelope(w, "UPID:pve-1:qmstart:430")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:qmstart:430/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/430/config":
			handler.envelope(w, map[string]any{"name": "powered-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/430/status/current":
			handler.envelope(w, map[string]any{"status": "running", "uptime": 5})
		default:
			handler.fail(w, "unexpected power create request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("pve-1", 430)
	model.Name = types.StringValue("powered-vm")
	model.Power = types.BoolValue(true)
	model.PowerShutdownTimeout = types.Int64Value(120)
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("power create diagnostics: %v", resp.Diagnostics)
	}
	var created qemuVMModel
	if diags := resp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode power create state: %v", diags)
	}
	if !created.Power.ValueBool() || created.PowerShutdownTimeout.ValueInt64() != 120 || created.Status.ValueString() != "running" {
		t.Fatalf("unexpected power create state: %#v", created)
	}
	wantCalls := []string{
		"POST /api2/json/nodes/pve-1/qemu",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:qemu-create-power/status",
		"POST /api2/json/nodes/pve-1/qemu/430/status/start",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:qmstart:430/status",
		"GET /api2/json/nodes/pve-1/qemu/430/config",
		"GET /api2/json/nodes/pve-1/qemu/430/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected power create call order: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceClonePowerTrueStartsAfterConfigUpdate(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/qemu/9000/clone":
			if !handler.form(w, r, url.Values{"newid": {"431"}, "target": {"target node"}, "name": {"powered-clone"}}) {
				return
			}
			handler.envelope(w, "UPID:source node:qemu-clone-power")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-power/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/431/config":
			handler.envelope(w, nil)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/431/status/start":
			handler.envelope(w, "UPID:target node:qmstart:431")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/tasks/UPID:target%20node:qmstart:431/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/431/config":
			handler.envelope(w, map[string]any{"name": "powered-clone"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/target%20node/qemu/431/status/current":
			handler.envelope(w, map[string]any{"status": "running", "uptime": 3})
		default:
			handler.fail(w, "unexpected power clone request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("target node", 431)
	model.Name = types.StringValue("powered-clone")
	model.Power = types.BoolValue(true)
	model.Clone = mustQemuVMCloneValue(t, qemuVMCloneModel{SourceNode: types.StringValue("source node"), SourceVMID: types.Int64Value(9000)})
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("power clone diagnostics: %v", resp.Diagnostics)
	}
	wantCalls := []string{
		"POST /api2/json/nodes/source%20node/qemu/9000/clone",
		"GET /api2/json/nodes/source%20node/tasks/UPID:source%20node:qemu-clone-power/status",
		"PUT /api2/json/nodes/target%20node/qemu/431/config",
		"POST /api2/json/nodes/target%20node/qemu/431/status/start",
		"GET /api2/json/nodes/target%20node/tasks/UPID:target%20node:qmstart:431/status",
		"GET /api2/json/nodes/target%20node/qemu/431/config",
		"GET /api2/json/nodes/target%20node/qemu/431/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected power clone call order: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceCreatePowerFalseNeverStarts(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu":
			if !handler.form(w, r, url.Values{"vmid": {"432"}}) {
				return
			}
			handler.envelope(w, "UPID:pve-1:qemu-create-off")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:qemu-create-off/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/432/config":
			handler.envelope(w, map[string]any{"name": "stopped-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/432/status/current":
			handler.envelope(w, map[string]any{"status": "stopped", "uptime": 0})
		default:
			// No start or shutdown endpoint exists: power=false must never
			// touch the guest power state during create.
			handler.fail(w, "unexpected power=false create request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("pve-1", 432)
	model.Power = types.BoolValue(false)
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("power=false create diagnostics: %v", resp.Diagnostics)
	}
	var created qemuVMModel
	if diags := resp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode power=false create state: %v", diags)
	}
	if created.Power.ValueBool() || created.Status.ValueString() != "stopped" {
		t.Fatalf("unexpected power=false create state: %#v", created)
	}
	wantCalls := []string{
		"POST /api2/json/nodes/pve-1/qemu",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:qemu-create-off/status",
		"GET /api2/json/nodes/pve-1/qemu/432/config",
		"GET /api2/json/nodes/pve-1/qemu/432/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected power=false create call order: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceCreatePowerRetainedWhenStartFails(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu":
			handler.envelope(w, "UPID:pve-1:qemu-create-startfail")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:qemu-create-startfail/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/433/status/start":
			handler.envelope(w, "UPID:pve-1:qmstart:433")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:qmstart:433/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "start failed: KVM unavailable"})
		default:
			handler.fail(w, "unexpected power start-failure request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("pve-1", 433)
	model.Power = types.BoolValue(true)
	model.PowerShutdownTimeout = types.Int64Value(120)
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "start failed: KVM unavailable") {
		t.Fatalf("expected start failure diagnostics: %v", resp.Diagnostics)
	}
	var partial qemuVMModel
	if diags := resp.State.Get(context.Background(), &partial); diags.HasError() {
		t.Fatalf("decode partial power start-failure state: %v", diags)
	}
	if partial.ID.ValueString() != "pve-1/433" || partial.VMID.ValueInt64() != 433 || !partial.Power.ValueBool() || partial.PowerShutdownTimeout.ValueInt64() != 120 {
		t.Fatalf("expected tracked identity and desired power after failed start, got: %#v", partial)
	}
	wantCalls := []string{
		"POST /api2/json/nodes/pve-1/qemu",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:qemu-create-startfail/status",
		"POST /api2/json/nodes/pve-1/qemu/433/status/start",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:qmstart:433/status",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("failed power start must not trigger further actions: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceUpdateStartsObservedStoppedGuest(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	running := false
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/434/config":
			handler.envelope(w, map[string]any{"name": "reconciled-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/434/status/current":
			status := "stopped"
			if running {
				status = "running"
			}
			handler.envelope(w, map[string]any{"status": status, "uptime": 0})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/434/config":
			handler.envelope(w, nil)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/434/status/start":
			running = true
			handler.envelope(w, "UPID:pve-1:qmstart:434")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:qmstart:434/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected power reconcile request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	planModel := minimalQemuVMModel("pve-1", 434)
	planModel.Power = types.BoolValue(true)
	stateModel := minimalQemuVMModel("pve-1", 434)
	// The prior refresh mirrored the out-of-band stop into state.
	stateModel.Power = types.BoolValue(false)
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  testResourcePlan(t, schema, planModel),
		State: testResourceState(t, schema, stateModel),
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("power reconcile update diagnostics: %v", updateResp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/pve-1/qemu/434/config",
		"GET /api2/json/nodes/pve-1/qemu/434/status/current",
		"PUT /api2/json/nodes/pve-1/qemu/434/config",
		"POST /api2/json/nodes/pve-1/qemu/434/status/start",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:qmstart:434/status",
		"GET /api2/json/nodes/pve-1/qemu/434/config",
		"GET /api2/json/nodes/pve-1/qemu/434/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected power reconcile call order: got %v want %v", calls, wantCalls)
	}
	var updated qemuVMModel
	if diags := updateResp.State.Get(context.Background(), &updated); diags.HasError() {
		t.Fatalf("decode power reconcile state: %v", diags)
	}
	if !updated.Power.ValueBool() || updated.Status.ValueString() != "running" {
		t.Fatalf("expected reconciled running state, got: %#v", updated)
	}
	handler.assert(t)
}

func TestQemuVMResourceUpdateShutsObservedRunningGuestDownBeforePut(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	running := true
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/435/config":
			handler.envelope(w, map[string]any{"name": "maintenance-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/435/status/current":
			status := "stopped"
			if running {
				status = "running"
			}
			handler.envelope(w, map[string]any{"status": status, "uptime": 9})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/435/status/shutdown":
			// Single-call server-side escalation: timeout plus forceStop.
			if !handler.form(w, r, url.Values{"forceStop": {"1"}, "timeout": {"90"}}) {
				return
			}
			running = false
			handler.envelope(w, "UPID:pve-1:qmshutdown:435")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:qmshutdown:435/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/435/config":
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected shutdown-before-put request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	planModel := minimalQemuVMModel("pve-1", 435)
	planModel.Power = types.BoolValue(false)
	planModel.PowerShutdownTimeout = types.Int64Value(90)
	stateModel := minimalQemuVMModel("pve-1", 435)
	stateModel.Power = types.BoolValue(true)
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  testResourcePlan(t, schema, planModel),
		State: testResourceState(t, schema, stateModel),
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("shutdown-before-put update diagnostics: %v", updateResp.Diagnostics)
	}
	shutdown := -1
	put := -1
	for i, call := range calls {
		switch {
		case strings.HasSuffix(call, "/status/shutdown"):
			shutdown = i
		case strings.HasSuffix(call, "/435/config") && strings.HasPrefix(call, "PUT"):
			put = i
		}
	}
	if shutdown < 0 || put < 0 {
		t.Fatalf("expected shutdown and PUT calls, got %v", calls)
	}
	if shutdown > put {
		t.Fatalf("shutdown must happen before the config PUT: %v", calls)
	}
	if len(calls) != 7 {
		t.Fatalf("unexpected call count for shutdown-before-put: %v", calls)
	}
	var updated qemuVMModel
	if diags := updateResp.State.Get(context.Background(), &updated); diags.HasError() {
		t.Fatalf("decode shutdown-before-put state: %v", diags)
	}
	if updated.Power.ValueBool() || updated.PowerShutdownTimeout.ValueInt64() != 90 || updated.Status.ValueString() != "stopped" {
		t.Fatalf("expected reconciled stopped state with echoed timeout, got: %#v", updated)
	}
	handler.assert(t)
}

func TestQemuVMResourceUpdateMatchingPowerTakesNoAction(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/436/config":
			handler.envelope(w, map[string]any{"name": "steady-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/436/status/current":
			handler.envelope(w, map[string]any{"status": "running", "uptime": 300})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/436/config":
			handler.envelope(w, nil)
		default:
			// No start/shutdown endpoints: an already matching desired power
			// must not touch the guest.
			handler.fail(w, "unexpected matching-power request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	planModel := minimalQemuVMModel("pve-1", 436)
	planModel.Power = types.BoolValue(true)
	stateModel := minimalQemuVMModel("pve-1", 436)
	stateModel.Power = types.BoolValue(true)
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  testResourcePlan(t, schema, planModel),
		State: testResourceState(t, schema, stateModel),
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("matching-power update diagnostics: %v", updateResp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/pve-1/qemu/436/config",
		"GET /api2/json/nodes/pve-1/qemu/436/status/current",
		"PUT /api2/json/nodes/pve-1/qemu/436/config",
		"GET /api2/json/nodes/pve-1/qemu/436/config",
		"GET /api2/json/nodes/pve-1/qemu/436/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("matching power must not start or stop the guest: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceUpdateWithoutPowerSkipsStatusRead(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/437/config":
			handler.envelope(w, map[string]any{"name": "unmanaged-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/437/status/current":
			handler.envelope(w, map[string]any{"status": "stopped", "uptime": 0})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/437/config":
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected unmanaged-power request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	planModel := minimalQemuVMModel("pve-1", 437)
	stateModel := minimalQemuVMModel("pve-1", 437)
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  testResourcePlan(t, schema, planModel),
		State: testResourceState(t, schema, stateModel),
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("unmanaged-power update diagnostics: %v", updateResp.Diagnostics)
	}
	// Without `power` the update must not add a status read: the only status
	// GETs are the ordinary post-update state read.
	wantCalls := []string{
		"GET /api2/json/nodes/pve-1/qemu/437/config",
		"PUT /api2/json/nodes/pve-1/qemu/437/config",
		"GET /api2/json/nodes/pve-1/qemu/437/config",
		"GET /api2/json/nodes/pve-1/qemu/437/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("power-less update must not read status before the PUT: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceUpdatePowerStatusErrorAbortsBeforePut(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/438/config":
			handler.envelope(w, map[string]any{"name": "statusless-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/438/status/current":
			http.Error(w, "status unavailable", http.StatusInternalServerError)
		default:
			handler.fail(w, "unexpected status-error request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	planModel := minimalQemuVMModel("pve-1", 438)
	planModel.Power = types.BoolValue(true)
	stateModel := minimalQemuVMModel("pve-1", 438)
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  testResourcePlan(t, schema, planModel),
		State: testResourceState(t, schema, stateModel),
	}, &updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatalf("expected status read error diagnostics")
	}
	wantCalls := []string{
		"GET /api2/json/nodes/pve-1/qemu/438/config",
		"GET /api2/json/nodes/pve-1/qemu/438/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("status read failure must abort before the PUT: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceRefreshMirrorsPower(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/439/config":
			handler.envelope(w, map[string]any{"name": "drifted-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/439/status/current":
			handler.envelope(w, map[string]any{"status": "stopped", "uptime": 0})
		default:
			// Refresh is observed-only: no start or shutdown endpoints exist.
			handler.fail(w, "unexpected refresh request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("pve-1", 439)
	stateModel.Power = types.BoolValue(true)
	stateModel.PowerShutdownTimeout = types.Int64Value(120)
	state := testResourceState(t, schema, stateModel)
	readResp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("refresh mirror diagnostics: %v", readResp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/pve-1/qemu/439/config",
		"GET /api2/json/nodes/pve-1/qemu/439/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("refresh must stay observed-only: got %v want %v", calls, wantCalls)
	}
	var refreshed qemuVMModel
	if diags := readResp.State.Get(context.Background(), &refreshed); diags.HasError() {
		t.Fatalf("decode refreshed state: %v", diags)
	}
	if refreshed.Power.ValueBool() || refreshed.PowerShutdownTimeout.ValueInt64() != 120 {
		t.Fatalf("expected observed power mirror with echoed timeout, got: %#v", refreshed)
	}
	handler.assert(t)
}

func TestQemuVMStateFromAPIPowerMirror(t *testing.T) {
	t.Parallel()

	// Data source and import reads (no prior state) never infer power.
	dataSourceState, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{Name: "vm"}, QemuVMStatus{Status: "running"}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if !dataSourceState.Power.IsNull() || !dataSourceState.PowerShutdownTimeout.IsNull() {
		t.Fatalf("expected null power without prior state, got %#v", dataSourceState)
	}

	prior := minimalQemuVMModel("pve-1", 101)
	prior.Power = types.BoolValue(true)
	prior.PowerShutdownTimeout = types.Int64Value(90)
	// QEMU `running` and `paused` are both powered on; anything else is off.
	for status, wantOn := range map[string]bool{"running": true, "paused": true, "stopped": false, "stopping": false} {
		state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{Name: "vm"}, QemuVMStatus{Status: status}, &prior)
		if diags.HasError() {
			t.Fatalf("qemuVMStateFromAPI(%q) unexpected diagnostics: %v", status, diags)
		}
		if state.Power.IsNull() || state.Power.ValueBool() != wantOn {
			t.Fatalf("status %q: expected power mirror %v, got %#v", status, wantOn, state.Power)
		}
		if state.PowerShutdownTimeout.ValueInt64() != 90 {
			t.Fatalf("status %q: expected timeout echo, got %#v", status, state.PowerShutdownTimeout)
		}
	}
}

func TestLXCContainerResourceValidateConfigPowerTimeout(t *testing.T) {
	res := &LXCContainerResource{}
	schema := testResourceSchema(t, res)

	for _, tc := range []struct {
		value    int64
		hasError bool
	}{
		{value: 0, hasError: true},
		{value: 601, hasError: true},
		{value: 1, hasError: false},
		{value: 600, hasError: false},
	} {
		model := minimalLXCContainerModel("pve one", 501)
		model.PowerShutdownTimeout = types.Int64Value(tc.value)
		resp := resource.ValidateConfigResponse{}
		res.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, schema, model)}, &resp)
		if resp.Diagnostics.HasError() != tc.hasError {
			t.Fatalf("power_shutdown_timeout %d: unexpected diagnostics %v", tc.value, resp.Diagnostics)
		}
		if tc.hasError && !containsDiagnostic(resp.Diagnostics, "must be between") {
			t.Fatalf("expected range diagnostic for %d: %v", tc.value, resp.Diagnostics)
		}
	}
}

func TestLXCContainerResourceCreatePowerTrueStarts(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc":
			if !handler.form(w, r, url.Values{"vmid": {"510"}, "ostemplate": {"local:vztmpl/debian.tar.zst"}, "rootfs": {"local-lvm:8"}}) {
				return
			}
			handler.envelope(w, "UPID:pve-1:lxc-create-power")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:lxc-create-power/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/510/status/start":
			handler.envelope(w, "UPID:pve-1:vzstart:510")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:vzstart:510/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/510/config":
			handler.envelope(w, map[string]any{"rootfs": "local-lvm:vm-510-disk-0,size=8G"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/510/status/current":
			handler.envelope(w, map[string]any{"status": "running", "uptime": 5})
		default:
			handler.fail(w, "unexpected LXC power create request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &LXCContainerResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalLXCContainerModel("pve-1", 510)
	model.OSTemplate = types.StringValue("local:vztmpl/debian.tar.zst")
	model.RootFS = types.StringValue("local-lvm:8")
	model.Power = types.BoolValue(true)
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("LXC power create diagnostics: %v", resp.Diagnostics)
	}
	var created lxcContainerModel
	if diags := resp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode LXC power create state: %v", diags)
	}
	if !created.Power.ValueBool() || created.Status.ValueString() != "running" {
		t.Fatalf("unexpected LXC power create state: %#v", created)
	}
	wantCalls := []string{
		"POST /api2/json/nodes/pve-1/lxc",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:lxc-create-power/status",
		"POST /api2/json/nodes/pve-1/lxc/510/status/start",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:vzstart:510/status",
		"GET /api2/json/nodes/pve-1/lxc/510/config",
		"GET /api2/json/nodes/pve-1/lxc/510/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected LXC power create call order: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestLXCContainerResourceUpdateStartsStoppedContainerAfterPut(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	running := false
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/511/config":
			handler.envelope(w, map[string]any{"rootfs": "local-lvm:vm-511-disk-0"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/511/status/current":
			status := "stopped"
			if running {
				status = "running"
			}
			handler.envelope(w, map[string]any{"status": status, "uptime": 0})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/511/status/start":
			running = true
			handler.envelope(w, "UPID:pve-1:vzstart:511")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:vzstart:511/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected LXC start reconcile request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &LXCContainerResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	planModel := minimalLXCContainerModel("pve-1", 511)
	planModel.Power = types.BoolValue(true)
	stateModel := minimalLXCContainerModel("pve-1", 511)
	stateModel.Power = types.BoolValue(false)
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  testResourcePlan(t, schema, planModel),
		State: testResourceState(t, schema, stateModel),
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("LXC start reconcile update diagnostics: %v", updateResp.Diagnostics)
	}
	// No config changes: the update request is empty, so only the power
	// reconcile runs before the post-update state read.
	wantCalls := []string{
		"GET /api2/json/nodes/pve-1/lxc/511/config",
		"GET /api2/json/nodes/pve-1/lxc/511/status/current",
		"POST /api2/json/nodes/pve-1/lxc/511/status/start",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:vzstart:511/status",
		"GET /api2/json/nodes/pve-1/lxc/511/config",
		"GET /api2/json/nodes/pve-1/lxc/511/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected LXC start reconcile call order: got %v want %v", calls, wantCalls)
	}
	var updated lxcContainerModel
	if diags := updateResp.State.Get(context.Background(), &updated); diags.HasError() {
		t.Fatalf("decode LXC start reconcile state: %v", diags)
	}
	if !updated.Power.ValueBool() || updated.Status.ValueString() != "running" {
		t.Fatalf("expected reconciled running container, got: %#v", updated)
	}
	handler.assert(t)
}

func TestLXCContainerResourceUpdateShutsRunningContainerDownBeforePut(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	running := true
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/512/config":
			handler.envelope(w, map[string]any{"rootfs": "local-lvm:vm-512-disk-0"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/512/status/current":
			status := "stopped"
			if running {
				status = "running"
			}
			handler.envelope(w, map[string]any{"status": status, "uptime": 7})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/512/status/shutdown":
			if !handler.form(w, r, url.Values{"forceStop": {"1"}, "timeout": {"120"}}) {
				return
			}
			running = false
			handler.envelope(w, "UPID:pve-1:vzshutdown:512")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:vzshutdown:512/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/512/config":
			handler.envelope(w, "UPID:pve-1:lxc-update-power")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:lxc-update-power/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected LXC shutdown reconcile request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &LXCContainerResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	planModel := minimalLXCContainerModel("pve-1", 512)
	planModel.Hostname = types.StringValue("maintenance-ct")
	planModel.Power = types.BoolValue(false)
	planModel.PowerShutdownTimeout = types.Int64Value(120)
	stateModel := minimalLXCContainerModel("pve-1", 512)
	stateModel.Power = types.BoolValue(true)
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  testResourcePlan(t, schema, planModel),
		State: testResourceState(t, schema, stateModel),
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("LXC shutdown reconcile update diagnostics: %v", updateResp.Diagnostics)
	}
	shutdown := -1
	put := -1
	for i, call := range calls {
		switch {
		case strings.HasSuffix(call, "/status/shutdown"):
			shutdown = i
		case strings.HasPrefix(call, "PUT"):
			put = i
		}
	}
	if shutdown < 0 || put < 0 {
		t.Fatalf("expected shutdown and PUT calls, got %v", calls)
	}
	if shutdown > put {
		t.Fatalf("shutdown must happen before the config PUT: %v", calls)
	}
	var updated lxcContainerModel
	if diags := updateResp.State.Get(context.Background(), &updated); diags.HasError() {
		t.Fatalf("decode LXC shutdown reconcile state: %v", diags)
	}
	if updated.Power.ValueBool() || updated.PowerShutdownTimeout.ValueInt64() != 120 || updated.Status.ValueString() != "stopped" {
		t.Fatalf("expected reconciled stopped container with echoed timeout, got: %#v", updated)
	}
	handler.assert(t)
}

func TestLXCContainerResourceUpdateWithoutPowerSkipsStatusRead(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/513/config":
			handler.envelope(w, map[string]any{"rootfs": "local-lvm:vm-513-disk-0"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/513/status/current":
			handler.envelope(w, map[string]any{"status": "stopped", "uptime": 0})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/513/config":
			handler.envelope(w, "UPID:pve-1:lxc-update-plain")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:lxc-update-plain/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected LXC plain update request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &LXCContainerResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	planModel := minimalLXCContainerModel("pve-1", 513)
	planModel.Hostname = types.StringValue("renamed-ct")
	stateModel := minimalLXCContainerModel("pve-1", 513)
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  testResourcePlan(t, schema, planModel),
		State: testResourceState(t, schema, stateModel),
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("LXC plain update diagnostics: %v", updateResp.Diagnostics)
	}
	// Without `power` no status read may appear before the PUT: the only
	// status GETs belong to the ordinary post-update state read.
	wantCalls := []string{
		"GET /api2/json/nodes/pve-1/lxc/513/config",
		"PUT /api2/json/nodes/pve-1/lxc/513/config",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:lxc-update-plain/status",
		"GET /api2/json/nodes/pve-1/lxc/513/config",
		"GET /api2/json/nodes/pve-1/lxc/513/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("power-less LXC update must not read status before the PUT: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestLXCContainerResourceRefreshMirrorsPower(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/514/config":
			handler.envelope(w, map[string]any{"rootfs": "local-lvm:vm-514-disk-0"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/514/status/current":
			handler.envelope(w, map[string]any{"status": "stopped", "uptime": 0})
		default:
			// Refresh is observed-only: no start or shutdown endpoints exist.
			handler.fail(w, "unexpected LXC refresh request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &LXCContainerResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalLXCContainerModel("pve-1", 514)
	stateModel.Power = types.BoolValue(true)
	stateModel.PowerShutdownTimeout = types.Int64Value(45)
	state := testResourceState(t, schema, stateModel)
	readResp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("LXC refresh mirror diagnostics: %v", readResp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/pve-1/lxc/514/config",
		"GET /api2/json/nodes/pve-1/lxc/514/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("LXC refresh must stay observed-only: got %v want %v", calls, wantCalls)
	}
	var refreshed lxcContainerModel
	if diags := readResp.State.Get(context.Background(), &refreshed); diags.HasError() {
		t.Fatalf("decode refreshed LXC state: %v", diags)
	}
	if refreshed.Power.ValueBool() || refreshed.PowerShutdownTimeout.ValueInt64() != 45 {
		t.Fatalf("expected observed power mirror with echoed timeout, got: %#v", refreshed)
	}
	handler.assert(t)
}

func TestLXCContainerStateFromAPIPowerMirror(t *testing.T) {
	t.Parallel()

	// Data source and import reads (no prior state) never infer power.
	dataSourceState, diags := lxcContainerStateFromAPI(context.Background(), "pve-1", 101, LXCContainerConfig{Hostname: "ct"}, LXCContainerStatus{Status: "running"}, nil)
	if diags.HasError() {
		t.Fatalf("lxcContainerStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if !dataSourceState.Power.IsNull() || !dataSourceState.PowerShutdownTimeout.IsNull() {
		t.Fatalf("expected null power without prior state, got %#v", dataSourceState)
	}

	prior := minimalLXCContainerModel("pve-1", 101)
	prior.Power = types.BoolValue(false)
	prior.PowerShutdownTimeout = types.Int64Value(45)
	for status, wantOn := range map[string]bool{"running": true, "stopped": false} {
		state, diags := lxcContainerStateFromAPI(context.Background(), "pve-1", 101, LXCContainerConfig{Hostname: "ct"}, LXCContainerStatus{Status: status}, &prior)
		if diags.HasError() {
			t.Fatalf("lxcContainerStateFromAPI(%q) unexpected diagnostics: %v", status, diags)
		}
		if state.Power.IsNull() || state.Power.ValueBool() != wantOn {
			t.Fatalf("status %q: expected power mirror %v, got %#v", status, wantOn, state.Power)
		}
		if state.PowerShutdownTimeout.ValueInt64() != 45 {
			t.Fatalf("status %q: expected timeout echo, got %#v", status, state.PowerShutdownTimeout)
		}
	}
}

func TestLXCContainerResourceCreatePowerRetainedWhenStartFails(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc":
			if !handler.form(w, r, url.Values{"vmid": {"515"}, "ostemplate": {"local:vztmpl/debian.tar.zst"}, "rootfs": {"local-lvm:8"}}) {
				return
			}
			handler.envelope(w, "UPID:pve-1:lxc-create-startfail")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:lxc-create-startfail/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/515/status/start":
			handler.envelope(w, "UPID:pve-1:vzstart:515")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:vzstart:515/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "start failed: apparmor denied"})
		default:
			// No delete or re-create endpoints: a failed start must keep the
			// created container tracked, never orphaned or recreated in-call.
			handler.fail(w, "unexpected LXC start-failure request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &LXCContainerResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalLXCContainerModel("pve-1", 515)
	model.OSTemplate = types.StringValue("local:vztmpl/debian.tar.zst")
	model.RootFS = types.StringValue("local-lvm:8")
	model.Power = types.BoolValue(true)
	model.PowerShutdownTimeout = types.Int64Value(120)
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "start failed: apparmor denied") {
		t.Fatalf("expected start failure diagnostics: %v", resp.Diagnostics)
	}
	var partial lxcContainerModel
	if diags := resp.State.Get(context.Background(), &partial); diags.HasError() {
		t.Fatalf("decode partial LXC start-failure state: %v", diags)
	}
	if partial.ID.ValueString() != "pve-1/515" || partial.Node.ValueString() != "pve-1" || partial.VMID.ValueInt64() != 515 || !partial.Power.ValueBool() || partial.PowerShutdownTimeout.ValueInt64() != 120 {
		t.Fatalf("expected tracked identity and desired power after failed start, got: %#v", partial)
	}
	wantCalls := []string{
		"POST /api2/json/nodes/pve-1/lxc",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:lxc-create-startfail/status",
		"POST /api2/json/nodes/pve-1/lxc/515/status/start",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:vzstart:515/status",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("failed start must not delete or re-create the container: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestQemuVMResourceUpdatePowerRemovalConverges(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/443/config":
			handler.envelope(w, map[string]any{"name": "unpowered-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/443/status/current":
			handler.envelope(w, map[string]any{"status": "stopped", "uptime": 0})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/443/config":
			handler.envelope(w, nil)
		default:
			// No start/shutdown endpoints: removing `power` must leave the
			// guest untouched.
			handler.fail(w, "unexpected power-removal request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("pve-1", 443)
	stateModel.Power = types.BoolValue(true)
	stateModel.PowerShutdownTimeout = types.Int64Value(90)
	planModel := minimalQemuVMModel("pve-1", 443)
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  testResourcePlan(t, schema, planModel),
		State: testResourceState(t, schema, stateModel),
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("power-removal update diagnostics: %v", updateResp.Diagnostics)
	}
	// No status read may be added for a plan without `power`: the only
	// status GET belongs to the ordinary post-update state read.
	wantCalls := []string{
		"GET /api2/json/nodes/pve-1/qemu/443/config",
		"PUT /api2/json/nodes/pve-1/qemu/443/config",
		"GET /api2/json/nodes/pve-1/qemu/443/config",
		"GET /api2/json/nodes/pve-1/qemu/443/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("power-removal must not read status before the PUT or act on the guest: got %v want %v", calls, wantCalls)
	}
	var updated qemuVMModel
	if diags := updateResp.State.Get(context.Background(), &updated); diags.HasError() {
		t.Fatalf("decode power-removal state: %v", diags)
	}
	if !updated.Power.IsNull() || !updated.PowerShutdownTimeout.IsNull() {
		t.Fatalf("expected power and timeout to converge to null, got: %#v", updated)
	}
	handler.assert(t)
}

func TestLXCContainerResourceUpdatePowerRemovalConverges(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/518/config":
			handler.envelope(w, map[string]any{"rootfs": "local-lvm:vm-518-disk-0"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/518/status/current":
			handler.envelope(w, map[string]any{"status": "stopped", "uptime": 0})
		default:
			// No PUT, start, or shutdown endpoints: removing `power` without
			// config changes must leave the container untouched.
			handler.fail(w, "unexpected LXC power-removal request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &LXCContainerResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalLXCContainerModel("pve-1", 518)
	stateModel.Power = types.BoolValue(true)
	stateModel.PowerShutdownTimeout = types.Int64Value(90)
	planModel := minimalLXCContainerModel("pve-1", 518)
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  testResourcePlan(t, schema, planModel),
		State: testResourceState(t, schema, stateModel),
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("LXC power-removal update diagnostics: %v", updateResp.Diagnostics)
	}
	wantCalls := []string{
		"GET /api2/json/nodes/pve-1/lxc/518/config",
		"GET /api2/json/nodes/pve-1/lxc/518/config",
		"GET /api2/json/nodes/pve-1/lxc/518/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("LXC power-removal must not read status early or act on the container: got %v want %v", calls, wantCalls)
	}
	var updated lxcContainerModel
	if diags := updateResp.State.Get(context.Background(), &updated); diags.HasError() {
		t.Fatalf("decode LXC power-removal state: %v", diags)
	}
	if !updated.Power.IsNull() || !updated.PowerShutdownTimeout.IsNull() {
		t.Fatalf("expected power and timeout to converge to null, got: %#v", updated)
	}
	handler.assert(t)
}

func TestQemuVMResourceUpdateReconcilesOutOfBandStopViaRead(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	running := false
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/444/config":
			handler.envelope(w, map[string]any{"name": "out-of-band-vm"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/444/status/current":
			status := "stopped"
			if running {
				status = "running"
			}
			handler.envelope(w, map[string]any{"status": status, "uptime": 0})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/444/config":
			handler.envelope(w, nil)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/444/status/start":
			running = true
			handler.envelope(w, "UPID:pve-1:qmstart:444")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:qmstart:444/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected out-of-band stop reconcile request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalQemuVMModel("pve-1", 444)
	model.Power = types.BoolValue(true)

	// Refresh alone mirrors the out-of-band stop into state without acting;
	// with `power = true` configured, the next apply reconciles the guest
	// back to running: the config PUT completes first, then the start task
	// runs.
	state := testResourceState(t, schema, model)
	readResp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("refresh diagnostics: %v", readResp.Diagnostics)
	}
	var refreshed qemuVMModel
	if diags := readResp.State.Get(context.Background(), &refreshed); diags.HasError() {
		t.Fatalf("decode refreshed state: %v", diags)
	}
	if refreshed.Power.ValueBool() {
		t.Fatalf("refresh must mirror the out-of-band stop, got %#v", refreshed.Power)
	}

	// The Update consumes the real Read output as its prior state.
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  testResourcePlan(t, schema, model),
		State: readResp.State,
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("out-of-band stop reconcile update diagnostics: %v", updateResp.Diagnostics)
	}
	// Every request of the refresh -> reconcile sequence must appear exactly
	// once and in order: read (config + status), update (existence config,
	// reconcile status, PUT, start task + poll), post-update read (config +
	// status).
	wantCalls := []string{
		"GET /api2/json/nodes/pve-1/qemu/444/config",
		"GET /api2/json/nodes/pve-1/qemu/444/status/current",
		"GET /api2/json/nodes/pve-1/qemu/444/config",
		"GET /api2/json/nodes/pve-1/qemu/444/status/current",
		"PUT /api2/json/nodes/pve-1/qemu/444/config",
		"POST /api2/json/nodes/pve-1/qemu/444/status/start",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:qmstart:444/status",
		"GET /api2/json/nodes/pve-1/qemu/444/config",
		"GET /api2/json/nodes/pve-1/qemu/444/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected out-of-band stop reconcile call sequence: got %v want %v", calls, wantCalls)
	}
	var updated qemuVMModel
	if diags := updateResp.State.Get(context.Background(), &updated); diags.HasError() {
		t.Fatalf("decode reconciled state: %v", diags)
	}
	if !updated.Power.ValueBool() || updated.Status.ValueString() != "running" {
		t.Fatalf("expected reconciled running guest, got: %#v", updated)
	}
	handler.assert(t)
}

func TestQemuVMResourceUpdateShutsOutOfBandRunningViaRead(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	running := true
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/445/config":
			handler.envelope(w, map[string]any{"name": "out-of-band-running"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/445/status/current":
			status := "stopped"
			if running {
				status = "running"
			}
			handler.envelope(w, map[string]any{"status": status, "uptime": 4})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/445/status/shutdown":
			if !handler.form(w, r, url.Values{"forceStop": {"1"}, "timeout": {"90"}}) {
				return
			}
			running = false
			handler.envelope(w, "UPID:pve-1:qmshutdown:445")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:qmshutdown:445/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu/445/config":
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected out-of-band start reconcile request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalQemuVMModel("pve-1", 445)
	// The prior state desires stopped while the guest was started out of
	// band: only this seeding makes Read actually mirror the observed drift.
	stateModel.Power = types.BoolValue(false)
	stateModel.PowerShutdownTimeout = types.Int64Value(90)
	// Refresh mirrors the out-of-band start into state.
	state := testResourceState(t, schema, stateModel)
	readResp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("refresh diagnostics: %v", readResp.Diagnostics)
	}
	var refreshed qemuVMModel
	if diags := readResp.State.Get(context.Background(), &refreshed); diags.HasError() {
		t.Fatalf("decode refreshed state: %v", diags)
	}
	if !refreshed.Power.ValueBool() {
		t.Fatalf("refresh must mirror the out-of-band start, got %#v", refreshed.Power)
	}

	planModel := minimalQemuVMModel("pve-1", 445)
	planModel.Power = types.BoolValue(false)
	planModel.PowerShutdownTimeout = types.Int64Value(90)
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  testResourcePlan(t, schema, planModel),
		State: readResp.State,
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("out-of-band start reconcile update diagnostics: %v", updateResp.Diagnostics)
	}
	// Every request of the refresh -> reconcile sequence must appear exactly
	// once and in order: read (config + status), update (existence config,
	// reconcile status, shutdown task + poll, PUT), post-update read (config
	// + status) - the shutdown strictly before the config PUT.
	wantCalls := []string{
		"GET /api2/json/nodes/pve-1/qemu/445/config",
		"GET /api2/json/nodes/pve-1/qemu/445/status/current",
		"GET /api2/json/nodes/pve-1/qemu/445/config",
		"GET /api2/json/nodes/pve-1/qemu/445/status/current",
		"POST /api2/json/nodes/pve-1/qemu/445/status/shutdown",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:qmshutdown:445/status",
		"PUT /api2/json/nodes/pve-1/qemu/445/config",
		"GET /api2/json/nodes/pve-1/qemu/445/config",
		"GET /api2/json/nodes/pve-1/qemu/445/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected out-of-band start reconcile call sequence: got %v want %v", calls, wantCalls)
	}
	var updated qemuVMModel
	if diags := updateResp.State.Get(context.Background(), &updated); diags.HasError() {
		t.Fatalf("decode reconciled state: %v", diags)
	}
	if updated.Power.ValueBool() || updated.PowerShutdownTimeout.ValueInt64() != 90 || updated.Status.ValueString() != "stopped" {
		t.Fatalf("expected reconciled stopped guest with echoed timeout, got: %#v", updated)
	}
	handler.assert(t)
}

func TestLXCContainerResourceUpdateReconcilesOutOfBandStopViaRead(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	running := false
	hostname := ""
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/519/config":
			handler.envelope(w, map[string]any{"hostname": hostname, "rootfs": "local-lvm:vm-519-disk-0"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/519/status/current":
			status := "stopped"
			if running {
				status = "running"
			}
			handler.envelope(w, map[string]any{"status": status, "uptime": 0})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/519/config":
			hostname = "renamed-ct"
			handler.envelope(w, "UPID:pve-1:lxc-update-oob")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:lxc-update-oob/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/519/status/start":
			running = true
			handler.envelope(w, "UPID:pve-1:vzstart:519")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:vzstart:519/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected LXC out-of-band stop reconcile request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &LXCContainerResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := minimalLXCContainerModel("pve-1", 519)
	model.Hostname = types.StringValue("renamed-ct")
	model.Power = types.BoolValue(true)

	// The container was stopped out of band while the configuration desires
	// running. Refresh alone mirrors the observed stop into state without
	// acting; the next apply reconciles the container back to running: the
	// real config PUT completes first, then the start task runs.
	state := testResourceState(t, schema, model)
	readResp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("LXC refresh diagnostics: %v", readResp.Diagnostics)
	}
	var refreshed lxcContainerModel
	if diags := readResp.State.Get(context.Background(), &refreshed); diags.HasError() {
		t.Fatalf("decode refreshed LXC state: %v", diags)
	}
	if refreshed.Power.ValueBool() {
		t.Fatalf("refresh must mirror the out-of-band stop, got %#v", refreshed.Power)
	}

	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  testResourcePlan(t, schema, model),
		State: readResp.State,
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("LXC out-of-band stop reconcile update diagnostics: %v", updateResp.Diagnostics)
	}
	// Every request of the refresh -> reconcile sequence must appear exactly
	// once and in order: read (config + status), update (existence config,
	// reconcile status, PUT + poll, start task + poll), post-update read
	// (config + status) - the start strictly after the config PUT.
	wantCalls := []string{
		"GET /api2/json/nodes/pve-1/lxc/519/config",
		"GET /api2/json/nodes/pve-1/lxc/519/status/current",
		"GET /api2/json/nodes/pve-1/lxc/519/config",
		"GET /api2/json/nodes/pve-1/lxc/519/status/current",
		"PUT /api2/json/nodes/pve-1/lxc/519/config",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:lxc-update-oob/status",
		"POST /api2/json/nodes/pve-1/lxc/519/status/start",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:vzstart:519/status",
		"GET /api2/json/nodes/pve-1/lxc/519/config",
		"GET /api2/json/nodes/pve-1/lxc/519/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected LXC out-of-band stop reconcile call sequence: got %v want %v", calls, wantCalls)
	}
	var updated lxcContainerModel
	if diags := updateResp.State.Get(context.Background(), &updated); diags.HasError() {
		t.Fatalf("decode reconciled LXC state: %v", diags)
	}
	if !updated.Power.ValueBool() || updated.Hostname.ValueString() != "renamed-ct" || updated.Status.ValueString() != "running" {
		t.Fatalf("expected reconciled running container with applied config, got: %#v", updated)
	}
	handler.assert(t)
}

func TestLXCContainerResourceUpdateShutsOutOfBandRunningViaRead(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	running := true
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/520/config":
			handler.envelope(w, map[string]any{"rootfs": "local-lvm:vm-520-disk-0"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/520/status/current":
			status := "stopped"
			if running {
				status = "running"
			}
			handler.envelope(w, map[string]any{"status": status, "uptime": 6})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/520/status/shutdown":
			if !handler.form(w, r, url.Values{"forceStop": {"1"}, "timeout": {"45"}}) {
				return
			}
			running = false
			handler.envelope(w, "UPID:pve-1:vzshutdown:520")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:vzshutdown:520/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/520/config":
			handler.envelope(w, "UPID:pve-1:lxc-update-off")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:lxc-update-off/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected LXC out-of-band start reconcile request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &LXCContainerResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	stateModel := minimalLXCContainerModel("pve-1", 520)
	// The prior state desires stopped while the container was started out
	// of band: only this seeding makes Read actually mirror the drift.
	stateModel.Power = types.BoolValue(false)
	stateModel.PowerShutdownTimeout = types.Int64Value(45)
	state := testResourceState(t, schema, stateModel)
	readResp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("LXC refresh diagnostics: %v", readResp.Diagnostics)
	}
	var refreshed lxcContainerModel
	if diags := readResp.State.Get(context.Background(), &refreshed); diags.HasError() {
		t.Fatalf("decode refreshed LXC state: %v", diags)
	}
	if !refreshed.Power.ValueBool() {
		t.Fatalf("refresh must mirror the out-of-band start, got %#v", refreshed.Power)
	}

	planModel := minimalLXCContainerModel("pve-1", 520)
	planModel.Power = types.BoolValue(false)
	planModel.PowerShutdownTimeout = types.Int64Value(45)
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  testResourcePlan(t, schema, planModel),
		State: readResp.State,
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("LXC out-of-band start reconcile update diagnostics: %v", updateResp.Diagnostics)
	}
	// Every request of the refresh -> reconcile sequence must appear exactly
	// once and in order: read (config + status), update (existence config,
	// reconcile status, shutdown task + poll, PUT + poll), post-update read
	// (config + status) - the shutdown strictly before the config PUT.
	wantCalls := []string{
		"GET /api2/json/nodes/pve-1/lxc/520/config",
		"GET /api2/json/nodes/pve-1/lxc/520/status/current",
		"GET /api2/json/nodes/pve-1/lxc/520/config",
		"GET /api2/json/nodes/pve-1/lxc/520/status/current",
		"POST /api2/json/nodes/pve-1/lxc/520/status/shutdown",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:vzshutdown:520/status",
		"PUT /api2/json/nodes/pve-1/lxc/520/config",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:lxc-update-off/status",
		"GET /api2/json/nodes/pve-1/lxc/520/config",
		"GET /api2/json/nodes/pve-1/lxc/520/status/current",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected LXC out-of-band start reconcile call sequence: got %v want %v", calls, wantCalls)
	}
	var updated lxcContainerModel
	if diags := updateResp.State.Get(context.Background(), &updated); diags.HasError() {
		t.Fatalf("decode reconciled LXC state: %v", diags)
	}
	if updated.Power.ValueBool() || updated.PowerShutdownTimeout.ValueInt64() != 45 || updated.Status.ValueString() != "stopped" {
		t.Fatalf("expected reconciled stopped container with echoed timeout, got: %#v", updated)
	}
	handler.assert(t)
}

func TestClientLXCContainerPowerMethodsRejectMalformedAck(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch r.URL.EscapedPath() {
		case "/api2/json/nodes/pve-1/lxc/301/status/start", "/api2/json/nodes/pve-1/lxc/301/status/stop", "/api2/json/nodes/pve-1/lxc/301/status/shutdown":
			handler.envelope(w, "not-a-upid")
		default:
			handler.fail(w, "unexpected LXC malformed-ack request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := testLifecycleClient(t, server)
	for name, callErr := range map[string]error{
		"start":    client.StartLXCContainer(ctx, "pve-1", 301),
		"stop":     client.StopLXCContainer(ctx, "pve-1", 301),
		"shutdown": client.ShutdownLXCContainer(ctx, "pve-1", 301, 60),
	} {
		if callErr == nil || !strings.Contains(callErr.Error(), "invalid UPID") {
			t.Fatalf("expected malformed %s UPID error, got %v", name, callErr)
		}
	}
	// Malformed acknowledgements must not poll task status at all.
	wantCalls := []string{
		"POST /api2/json/nodes/pve-1/lxc/301/status/start",
		"POST /api2/json/nodes/pve-1/lxc/301/status/stop",
		"POST /api2/json/nodes/pve-1/lxc/301/status/shutdown",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("malformed acknowledgements must not poll task status: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestClientLXCContainerPowerMethodsTaskStatus404RemainsError(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	ctx := context.Background()
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/301/status/start":
			handler.envelope(w, "UPID:pve-1:vzstart:301")
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/301/status/stop":
			handler.envelope(w, "UPID:pve-1:vzstop:301")
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/lxc/301/status/shutdown":
			handler.envelope(w, "UPID:pve-1:vzshutdown:301")
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/api2/json/nodes/pve-1/tasks/"):
			http.NotFound(w, r)
		default:
			handler.fail(w, "unexpected LXC 404-poll request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := testLifecycleClient(t, server)
	for name, callErr := range map[string]error{
		"start":    client.StartLXCContainer(ctx, "pve-1", 301),
		"stop":     client.StopLXCContainer(ctx, "pve-1", 301),
		"shutdown": client.ShutdownLXCContainer(ctx, "pve-1", 301, 60),
	} {
		if callErr == nil || !strings.Contains(callErr.Error(), "unable to poll task") {
			t.Fatalf("expected task status 404 to remain a %s error, got %v", name, callErr)
		}
	}
	// Each valid UPID is polled exactly once before the error propagates;
	// none of the methods may report false success.
	wantCalls := []string{
		"POST /api2/json/nodes/pve-1/lxc/301/status/start",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:vzstart:301/status",
		"POST /api2/json/nodes/pve-1/lxc/301/status/stop",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:vzstop:301/status",
		"POST /api2/json/nodes/pve-1/lxc/301/status/shutdown",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:vzshutdown:301/status",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected 404-poll call order: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}
