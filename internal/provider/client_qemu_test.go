// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestClientQemuVMMethods(t *testing.T) {
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
		case r.URL.Path == "/api2/json/nodes/pve-1/qemu/101/config" && r.Method == http.MethodGet:
			handler.envelope(w, map[string]any{
				"name":        "api-vm",
				"description": "Managed by Terraform",
				"tags":        "prod,terraform",
				"template":    0,
				"pool":        "platform",
				"onboot":      1,
				"protection":  true,
				"scsihw":      "virtio-scsi-pci",
				"tablet":      true,
				"serial0":     "socket",
				"startup":     "order=2",
				"bios":        "ovmf",
				"machine":     "q35",
				"agent":       "enabled=1",
				"cores":       "4",
				"sockets":     2,
				"memory":      8192,
				"cpu":         "host",
				"numa":        1,
				"vcpus":       2,
				"cpuunits":    1024,
				"cpulimit":    1.5,
				"balloon":     2048,
				"shares":      2000,
				"hugepages":   "2",
				"ostype":      "l26",
				"boot":        "order=scsi0;net0",
			})
		case r.URL.Path == "/api2/json/nodes/pve-1/qemu/101/status/current" && r.Method == http.MethodGet:
			handler.envelope(w, map[string]any{
				"status": "running",
				"uptime": "300",
			})
		case r.URL.Path == "/api2/json/nodes/pve-1/qemu" && r.Method == http.MethodPost:
			if !handler.form(w, r, url.Values{
				"vmid":        {"101"},
				"name":        {"api-vm"},
				"description": {"Managed by Terraform"},
				"tags":        {"prod,terraform"},
				"pool":        {"platform"},
				"onboot":      {"1"},
				"protection":  {"1"},
				"scsihw":      {"virtio-scsi-pci"},
				"tablet":      {"1"},
				"startup":     {"order=2"},
				"bios":        {"ovmf"},
				"machine":     {"q35"},
				"agent":       {"enabled=1"},
				"cores":       {"4"},
				"sockets":     {"2"},
				"memory":      {"8192"},
				"cpu":         {"host"},
				"ostype":      {"l26"},
				"boot":        {"order=scsi0;net0"},
				"serial0":     {"socket"},
				"numa":        {"1"},
				"vcpus":       {"2"},
				"cpuunits":    {"1024"},
				"cpulimit":    {"1.5"},
				"balloon":     {"2048"},
				"shares":      {"2000"},
				"hugepages":   {"2"},
			}) {
				return
			}
			handler.envelope(w, "UPID:pve-1:qemu-create")
		case r.URL.Path == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:qemu-create/status" && r.Method == http.MethodGet:
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.URL.Path == "/api2/json/nodes/pve-1/qemu/101/config" && r.Method == http.MethodPut:
			if !handler.form(w, r, url.Values{
				"name":       {"api-vm"},
				"onboot":     {"0"},
				"protection": {"0"},
				"scsihw":     {"megasas"},
				"tablet":     {"0"},
				"memory":     {"4096"},
			}) {
				return
			}
			handler.envelope(w, nil)
		case r.URL.Path == "/api2/json/nodes/pve-1/qemu/101" && r.Method == http.MethodDelete:
			handler.envelope(w, "UPID:pve-1:qemu-delete")
		case r.URL.Path == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:qemu-delete/status" && r.Method == http.MethodGet:
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewClient(ctx, ClientConfig{
		Endpoint:       server.URL,
		APITokenID:     "terraform@pve!provider",
		APITokenSecret: "token-secret",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}

	config, err := client.GetQemuVMConfig(ctx, "pve-1", 101)
	if err != nil {
		t.Fatalf("GetQemuVMConfig() unexpected error: %v", err)
	}
	if config.Name != "api-vm" || config.Cores.Ptr() == nil || *config.Cores.Ptr() != 4 {
		t.Fatalf("unexpected qemu config: %#v", config)
	}
	if config.OnBoot.Ptr() == nil || !*config.OnBoot.Ptr() {
		t.Fatalf("expected onboot=true, got %#v", config.OnBoot)
	}
	if config.Protection.Ptr() == nil || !*config.Protection.Ptr() {
		t.Fatalf("expected protection=true, got %#v", config.Protection)
	}
	if config.SCSIHW != "virtio-scsi-pci" {
		t.Fatalf("expected scsihw=virtio-scsi-pci, got %q", config.SCSIHW)
	}
	if config.Tablet.Ptr() == nil || !*config.Tablet.Ptr() {
		t.Fatalf("expected tablet=true, got %#v", config.Tablet)
	}
	if got := config.Serial["serial0"]; got != "socket" {
		t.Fatalf("expected serial0=socket, got %#v", config.Serial)
	}
	if config.NUMA.Ptr() == nil || !*config.NUMA.Ptr() {
		t.Fatalf("expected numa=true, got %#v", config.NUMA)
	}
	if config.VCPUs.Ptr() == nil || *config.VCPUs.Ptr() != 2 {
		t.Fatalf("expected vcpus=2, got %#v", config.VCPUs)
	}
	if config.CPUUnits.Ptr() == nil || *config.CPUUnits.Ptr() != 1024 {
		t.Fatalf("expected cpuunits=1024, got %#v", config.CPUUnits)
	}
	if config.CPULimit.Ptr() == nil || *config.CPULimit.Ptr() != 1.5 {
		t.Fatalf("expected cpulimit=1.5, got %#v", config.CPULimit)
	}
	if config.Balloon.Ptr() == nil || *config.Balloon.Ptr() != 2048 {
		t.Fatalf("expected balloon=2048, got %#v", config.Balloon)
	}
	if config.Shares.Ptr() == nil || *config.Shares.Ptr() != 2000 {
		t.Fatalf("expected shares=2000, got %#v", config.Shares)
	}
	if config.Hugepages != "2" {
		t.Fatalf("expected hugepages=2, got %#v", config.Hugepages)
	}

	status, err := client.GetQemuVMStatus(ctx, "pve-1", 101)
	if err != nil {
		t.Fatalf("GetQemuVMStatus() unexpected error: %v", err)
	}
	if status.Status != "running" || status.Uptime.Ptr() == nil || *status.Uptime.Ptr() != 300 {
		t.Fatalf("unexpected qemu status: %#v", status)
	}

	if err := client.CreateQemuVM(ctx, "pve-1", CreateQemuVMRequest{
		VMID: 101,
		qemuVMConfigRequest: qemuVMConfigRequest{
			Name:        stringPtr("api-vm"),
			Description: stringPtr("Managed by Terraform"),
			Tags:        stringPtr("prod,terraform"),
			Pool:        stringPtr("platform"),
			OnBoot:      boolPtr(true),
			Protection:  boolPtr(true),
			SCSIHW:      stringPtr("virtio-scsi-pci"),
			Tablet:      boolPtr(true),
			Startup:     stringPtr("order=2"),
			Bios:        stringPtr("ovmf"),
			Machine:     stringPtr("q35"),
			Agent:       stringPtr("enabled=1"),
			Cores:       intPtr64(4),
			Sockets:     intPtr64(2),
			Memory:      intPtr64(8192),
			CPU:         stringPtr("host"),
			OSType:      stringPtr("l26"),
			Boot:        stringPtr("order=scsi0;net0"),
			Serial:      map[string]string{"serial0": "socket"},
			NUMA:        boolPtr(true),
			VCPUs:       intPtr64(2),
			CPUUnits:    intPtr64(1024),
			CPULimit:    float64Ptr(1.5),
			Balloon:     intPtr64(2048),
			Shares:      intPtr64(2000),
			Hugepages:   stringPtr("2"),
		},
	}); err != nil {
		t.Fatalf("CreateQemuVM() unexpected error: %v", err)
	}

	if err := client.UpdateQemuVM(ctx, "pve-1", 101, UpdateQemuVMRequest{
		qemuVMConfigRequest: qemuVMConfigRequest{
			Name:       stringPtr("api-vm"),
			OnBoot:     boolPtr(false),
			Protection: boolPtr(false),
			SCSIHW:     stringPtr("megasas"),
			Tablet:     boolPtr(false),
			Memory:     intPtr64(4096),
		},
	}); err != nil {
		t.Fatalf("UpdateQemuVM() unexpected error: %v", err)
	}

	if err := client.DeleteQemuVM(ctx, "pve-1", 101); err != nil {
		t.Fatalf("DeleteQemuVM() unexpected error: %v", err)
	}

	wantCalls := []string{
		"GET /api2/json/nodes/pve-1/qemu/101/config",
		"GET /api2/json/nodes/pve-1/qemu/101/status/current",
		"POST /api2/json/nodes/pve-1/qemu",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:qemu-create/status",
		"PUT /api2/json/nodes/pve-1/qemu/101/config",
		"DELETE /api2/json/nodes/pve-1/qemu/101",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:qemu-delete/status",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected QEMU client call order: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestClientCreateQemuVMTaskFailure(t *testing.T) {
	oldPollInterval := nodeTaskPollInterval
	nodeTaskPollInterval = 0
	defer func() { nodeTaskPollInterval = oldPollInterval }()

	const upid = "UPID:pve-1:0000008A:00002020:65F00000:qmcreate:101:root@pam:"
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/qemu":
			if !handler.form(w, r, url.Values{"vmid": {"101"}}) {
				return
			}
			handler.envelope(w, upid)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:0000008A:00002020:65F00000:qmcreate:101:root@pam:/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "ERROR: storage unavailable"})
		default:
			handler.fail(w, "unexpected QEMU task failure request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	err := testLifecycleClient(t, server).CreateQemuVM(context.Background(), "pve-1", CreateQemuVMRequest{VMID: 101})
	if err == nil || !strings.Contains(err.Error(), upid) || !strings.Contains(err.Error(), "ERROR: storage unavailable") {
		t.Fatalf("expected QEMU task identity and exit status, got %v", err)
	}
	wantCalls := []string{
		"POST /api2/json/nodes/pve-1/qemu",
		"GET /api2/json/nodes/pve-1/tasks/UPID:pve-1:0000008A:00002020:65F00000:qmcreate:101:root@pam:/status",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected QEMU task failure call order: got %v want %v", calls, wantCalls)
	}
	handler.assert(t)
}

func TestDecodeQemuVMConfigProtectionBoolVariants(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		raw  json.RawMessage
		want bool
	}{
		{name: "json bool true", raw: json.RawMessage(`true`), want: true},
		{name: "json integer one", raw: json.RawMessage(`1`), want: true},
		{name: "json string false", raw: json.RawMessage(`"false"`), want: false},
		{name: "json string zero", raw: json.RawMessage(`"0"`), want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config, err := decodeQemuVMConfig(map[string]json.RawMessage{
				"protection": tc.raw,
				"hostpci0":   json.RawMessage(`"0000:00:1f.0"`),
			})
			if err != nil {
				t.Fatalf("decodeQemuVMConfig() unexpected error: %v", err)
			}
			if config.Protection.Ptr() == nil || *config.Protection.Ptr() != tc.want {
				t.Fatalf("unexpected protection value: got %#v want %v", config.Protection, tc.want)
			}
			if _, ok := config.ExtraConfig["protection"]; ok {
				t.Fatalf("expected protection to be decoded as typed field, got extra config %#v", config.ExtraConfig)
			}
			if got := config.ExtraConfig["hostpci0"]; got != "0000:00:1f.0" {
				t.Fatalf("expected unrelated raw key to remain raw, got %#v", config.ExtraConfig)
			}
		})
	}
}

func TestDecodeQemuVMConfigSCSIHWIsTyped(t *testing.T) {
	t.Parallel()

	config, err := decodeQemuVMConfig(map[string]json.RawMessage{
		"scsihw":   json.RawMessage(`"virtio-scsi-single"`),
		"hostpci0": json.RawMessage(`"0000:00:1f.0"`),
	})
	if err != nil {
		t.Fatalf("decodeQemuVMConfig() unexpected error: %v", err)
	}
	if config.SCSIHW != "virtio-scsi-single" {
		t.Fatalf("expected typed scsihw, got %q", config.SCSIHW)
	}
	if _, ok := config.ExtraConfig["scsihw"]; ok {
		t.Fatalf("expected scsihw to be decoded as typed field, got extra config %#v", config.ExtraConfig)
	}
	if got := config.ExtraConfig["hostpci0"]; got != "0000:00:1f.0" {
		t.Fatalf("expected unrelated raw key to remain raw, got %#v", config.ExtraConfig)
	}
}

func TestDecodeQemuVMConfigTabletIsTyped(t *testing.T) {
	t.Parallel()

	config, err := decodeQemuVMConfig(map[string]json.RawMessage{
		"tablet":   json.RawMessage(`true`),
		"hostpci0": json.RawMessage(`"0000:00:1f.0"`),
	})
	if err != nil {
		t.Fatalf("decodeQemuVMConfig() unexpected error: %v", err)
	}
	if config.Tablet.Ptr() == nil || !*config.Tablet.Ptr() {
		t.Fatalf("expected typed tablet=true, got %#v", config.Tablet)
	}
	if _, ok := config.ExtraConfig["tablet"]; ok {
		t.Fatalf("expected tablet to be decoded as typed field, got extra config %#v", config.ExtraConfig)
	}
	if got := config.ExtraConfig["hostpci0"]; got != "0000:00:1f.0" {
		t.Fatalf("expected unrelated raw key to remain raw, got %#v", config.ExtraConfig)
	}
}

func TestDecodeQemuVMConfigSerialSlotsAreTyped(t *testing.T) {
	t.Parallel()

	config, err := decodeQemuVMConfig(map[string]json.RawMessage{
		"serial0":  json.RawMessage(`"socket"`),
		"serial1":  json.RawMessage(`"/dev/ttyS0"`),
		"hostpci0": json.RawMessage(`"0000:00:1f.0"`),
	})
	if err != nil {
		t.Fatalf("decodeQemuVMConfig() unexpected error: %v", err)
	}
	if got := config.Serial["serial0"]; got != "socket" {
		t.Fatalf("expected serial0=socket typed, got %#v", config.Serial)
	}
	if got := config.Serial["serial1"]; got != "/dev/ttyS0" {
		t.Fatalf("expected serial1=/dev/ttyS0 typed, got %#v", config.Serial)
	}
	if _, ok := config.ExtraConfig["serial0"]; ok {
		t.Fatalf("expected serial0 to be decoded as typed slot, got extra config %#v", config.ExtraConfig)
	}
	if _, ok := config.ExtraConfig["serial1"]; ok {
		t.Fatalf("expected serial1 to be decoded as typed slot, got extra config %#v", config.ExtraConfig)
	}
	if got := config.ExtraConfig["hostpci0"]; got != "0000:00:1f.0" {
		t.Fatalf("expected unrelated raw key to remain raw, got %#v", config.ExtraConfig)
	}
}

func TestDecodeQemuVMConfigCPUFieldsAreTyped(t *testing.T) {
	t.Parallel()

	config, err := decodeQemuVMConfig(map[string]json.RawMessage{
		"numa":     json.RawMessage(`1`),
		"vcpus":    json.RawMessage(`"2"`),
		"cpuunits": json.RawMessage(`1024`),
		"cpulimit": json.RawMessage(`"1.5"`),
		"hostpci0": json.RawMessage(`"0000:00:1f.0"`),
	})
	if err != nil {
		t.Fatalf("decodeQemuVMConfig() unexpected error: %v", err)
	}
	if config.NUMA.Ptr() == nil || !*config.NUMA.Ptr() {
		t.Fatalf("expected typed numa=true, got %#v", config.NUMA)
	}
	if config.VCPUs.Ptr() == nil || *config.VCPUs.Ptr() != 2 {
		t.Fatalf("expected typed vcpus=2, got %#v", config.VCPUs)
	}
	if config.CPUUnits.Ptr() == nil || *config.CPUUnits.Ptr() != 1024 {
		t.Fatalf("expected typed cpuunits=1024, got %#v", config.CPUUnits)
	}
	if config.CPULimit.Ptr() == nil || *config.CPULimit.Ptr() != 1.5 {
		t.Fatalf("expected typed cpulimit=1.5, got %#v", config.CPULimit)
	}
	for _, key := range []string{"numa", "vcpus", "cpuunits", "cpulimit"} {
		if _, ok := config.ExtraConfig[key]; ok {
			t.Fatalf("expected %s to be decoded as typed field, got extra config %#v", key, config.ExtraConfig)
		}
	}
	if got := config.ExtraConfig["hostpci0"]; got != "0000:00:1f.0" {
		t.Fatalf("expected unrelated raw key to remain raw, got %#v", config.ExtraConfig)
	}
}

func TestDecodeQemuVMConfigMemoryFieldsAreTyped(t *testing.T) {
	t.Parallel()

	config, err := decodeQemuVMConfig(map[string]json.RawMessage{
		"balloon":   json.RawMessage(`2048`),
		"shares":    json.RawMessage(`"2000"`),
		"hugepages": json.RawMessage(`"2"`),
		"hostpci0":  json.RawMessage(`"0000:00:1f.0"`),
	})
	if err != nil {
		t.Fatalf("decodeQemuVMConfig() unexpected error: %v", err)
	}
	if config.Balloon.Ptr() == nil || *config.Balloon.Ptr() != 2048 {
		t.Fatalf("expected typed balloon=2048, got %#v", config.Balloon)
	}
	if config.Shares.Ptr() == nil || *config.Shares.Ptr() != 2000 {
		t.Fatalf("expected typed shares=2000, got %#v", config.Shares)
	}
	if config.Hugepages != "2" {
		t.Fatalf("expected typed hugepages=2, got %#v", config.Hugepages)
	}
	for _, key := range []string{"balloon", "shares", "hugepages"} {
		if _, ok := config.ExtraConfig[key]; ok {
			t.Fatalf("expected %s to be decoded as typed field, got extra config %#v", key, config.ExtraConfig)
		}
	}
	if got := config.ExtraConfig["hostpci0"]; got != "0000:00:1f.0" {
		t.Fatalf("expected unrelated raw key to remain raw, got %#v", config.ExtraConfig)
	}
}

func TestClientQemuVMConfigNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := NewClient(ctx, ClientConfig{
		Endpoint:       server.URL,
		APITokenID:     "terraform@pve!provider",
		APITokenSecret: "token-secret",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}

	_, err = client.GetQemuVMConfig(ctx, "pve-1", 404)
	if !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}

func TestClientQemuVMConfigPVE9MissingResponse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"data":null,"message":"Configuration file 'nodes/pve-1/qemu-server/948674.conf' does not exist\n"}`))
	}))
	defer server.Close()

	client, err := NewClient(ctx, ClientConfig{
		Endpoint:       server.URL,
		APITokenID:     "terraform@pve!provider",
		APITokenSecret: "token-secret",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}

	_, err = client.GetQemuVMConfig(ctx, "pve-1", 948674)
	if !errors.Is(err, errNotFound) {
		t.Fatalf("expected PVE 9 missing config response to match errNotFound, got %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusInternalServerError || !strings.Contains(apiErr.Body, "948674.conf") {
		t.Fatalf("expected underlying PVE API error detail, got %v", err)
	}
}

func TestClientQemuVMConfigOtherPVE9ErrorsRemainAPIErrors(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"different vm":    `{"data":null,"message":"Configuration file 'nodes/pve-1/qemu-server/123.conf' does not exist\n"}`,
		"different node":  `{"data":null,"message":"Configuration file 'nodes/pve-2/qemu-server/948674.conf' does not exist\n"}`,
		"generic failure": `{"data":null,"message":"QEMU configuration service unavailable"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()

			client, err := NewClient(ctx, ClientConfig{
				Endpoint:       server.URL,
				APITokenID:     "terraform@pve!provider",
				APITokenSecret: "token-secret",
				Timeout:        time.Second,
			})
			if err != nil {
				t.Fatalf("NewClient() unexpected error: %v", err)
			}

			_, err = client.GetQemuVMConfig(ctx, "pve-1", 948674)
			if errors.Is(err, errNotFound) {
				t.Fatalf("unexpected errNotFound classification: %v", err)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusInternalServerError {
				t.Fatalf("expected underlying PVE API error, got %v", err)
			}
		})
	}
}

func stringPtr(v string) *string    { return &v }
func boolPtr(v bool) *bool          { return &v }
func intPtr64(v int64) *int64       { return &v }
func float64Ptr(v float64) *float64 { return &v }
