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
	"testing"
	"time"
)

func TestClientQemuVMMethods(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)

		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/qemu/101/config" && r.Method == http.MethodGet:
			writeEnvelope(t, w, map[string]any{
				"name":        "api-vm",
				"description": "Managed by Terraform",
				"tags":        "prod,terraform",
				"template":    0,
				"pool":        "platform",
				"onboot":      1,
				"protection":  true,
				"scsihw":      "virtio-scsi-pci",
				"tablet":      true,
				"startup":     "order=2",
				"bios":        "ovmf",
				"machine":     "q35",
				"agent":       "enabled=1",
				"cores":       "4",
				"sockets":     2,
				"memory":      8192,
				"cpu":         "host",
				"ostype":      "l26",
				"boot":        "order=scsi0;net0",
			})
		case r.URL.Path == "/api2/json/nodes/pve-1/qemu/101/status/current" && r.Method == http.MethodGet:
			writeEnvelope(t, w, map[string]any{
				"status": "running",
				"uptime": "300",
			})
		case r.URL.Path == "/api2/json/nodes/pve-1/qemu" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{
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
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/nodes/pve-1/qemu/101/config" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{
				"name":       {"api-vm"},
				"onboot":     {"0"},
				"protection": {"0"},
				"scsihw":     {"megasas"},
				"tablet":     {"0"},
				"memory":     {"4096"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/nodes/pve-1/qemu/101" && r.Method == http.MethodDelete:
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
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

func stringPtr(v string) *string { return &v }
func boolPtr(v bool) *bool       { return &v }
func intPtr64(v int64) *int64    { return &v }
