// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
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
				"name":   {"api-vm"},
				"onboot": {"0"},
				"memory": {"4096"},
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
			Name:   stringPtr("api-vm"),
			OnBoot: boolPtr(false),
			Memory: intPtr64(4096),
		},
	}); err != nil {
		t.Fatalf("UpdateQemuVM() unexpected error: %v", err)
	}

	if err := client.DeleteQemuVM(ctx, "pve-1", 101); err != nil {
		t.Fatalf("DeleteQemuVM() unexpected error: %v", err)
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
