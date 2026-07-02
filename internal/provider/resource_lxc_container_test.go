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

func TestLXCContainerResourceMetadata(t *testing.T) {
	t.Parallel()

	res := NewLXCContainerResource()
	var resp resource.MetadataResponse
	res.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "proxmox"}, &resp)

	if resp.TypeName != "proxmox_lxc_container" {
		t.Fatalf("unexpected resource name: %q", resp.TypeName)
	}
}

func TestLXCContainerResourceReadState(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)

		switch r.URL.Path {
		case "/api2/json/nodes/pve-1/lxc/101/config":
			writeEnvelope(t, w, map[string]any{"hostname": "api-ct", "memory": 1024, "rootfs": "local-lvm:vm-101-disk-0", "net0": "name=eth0,bridge=vmbr0,ip=dhcp,type=veth"})
		case "/api2/json/nodes/pve-1/lxc/101/status/current":
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

	res := &LXCContainerResource{client: client}
	state, diags := res.readLXCContainerState(context.Background(), "pve-1", 101, nil)
	if diags.HasError() {
		t.Fatalf("readLXCContainerState() unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "pve-1/101" || state.Hostname.ValueString() != "api-ct" || state.Memory.ValueInt64() != 1024 || state.Status.ValueString() != "running" || state.Uptime.ValueInt64() != 99 {
		t.Fatalf("unexpected resource state: %#v", state)
	}
}

func TestLXCContainerResourceReadStateMissing(t *testing.T) {
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

	res := &LXCContainerResource{client: client}
	state, diags := res.readLXCContainerState(context.Background(), "pve-1", 404, nil)
	if diags.HasError() {
		t.Fatalf("readLXCContainerState() unexpected diagnostics: %v", diags)
	}
	if !state.ID.IsNull() {
		t.Fatalf("expected null id for missing container, got %#v", state)
	}

	_, err = client.GetLXCContainerStatus(context.Background(), "pve-1", 404)
	if !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound for status lookup, got %v", err)
	}
}

func TestLXCContainerResourceSchemaAttributes(t *testing.T) {
	t.Parallel()

	attrs := lxcContainerResourceAttributes()
	for _, key := range []string{"node", "vm_id", "ostemplate", "rootfs", "network", "mount_point", "raw", "status", "uptime"} {
		if _, ok := attrs[key]; !ok {
			t.Fatalf("expected resource attribute %q", key)
		}
	}
}
