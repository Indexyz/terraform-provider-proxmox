// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestQemuVMResourceMetadata(t *testing.T) {
	t.Parallel()

	res := NewQemuVMResource()
	var resp resource.MetadataResponse
	res.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "proxmox"}, &resp)

	if resp.TypeName != "proxmox_qemu_vm" {
		t.Fatalf("unexpected resource name: %q", resp.TypeName)
	}
}

func TestQemuVMResourceReadState(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)

		switch r.URL.Path {
		case "/api2/json/nodes/pve-1/qemu/101/config":
			writeEnvelope(t, w, map[string]any{"name": "api-vm", "template": 0, "onboot": 1, "memory": 4096})
		case "/api2/json/nodes/pve-1/qemu/101/status/current":
			writeEnvelope(t, w, map[string]any{"status": "running", "uptime": 99})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), ClientConfig{
		Endpoint:       server.URL,
		APITokenID:     "terraform@pve!provider",
		APITokenSecret: "token-secret",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}

	res := &QemuVMResource{client: client}
	state, diags := res.readQemuVMState(context.Background(), "pve-1", 101, nil)
	if diags.HasError() {
		t.Fatalf("readQemuVMState() unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "pve-1/101" || state.Memory.ValueInt64() != 4096 || state.Uptime.ValueInt64() != 99 {
		t.Fatalf("unexpected resource state: %#v", state)
	}
}

func TestQemuVMResourceReadStateMissing(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), ClientConfig{
		Endpoint:       server.URL,
		APITokenID:     "terraform@pve!provider",
		APITokenSecret: "token-secret",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}

	res := &QemuVMResource{client: client}
	state, diags := res.readQemuVMState(context.Background(), "pve-1", 404, nil)
	if diags.HasError() {
		t.Fatalf("readQemuVMState() unexpected diagnostics: %v", diags)
	}
	if !state.ID.IsNull() {
		t.Fatalf("expected null id for missing VM, got %#v", state)
	}

	_, err = client.GetQemuVMStatus(context.Background(), "pve-1", 404)
	if !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound for status lookup, got %v", err)
	}
}
