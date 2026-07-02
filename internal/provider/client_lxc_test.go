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
	"strings"
	"testing"
	"time"
)

func TestClientLXCContainerMethods(t *testing.T) {
	ctx := context.Background()
	seen := map[string]bool{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)

		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/lxc/101/config" && r.Method == http.MethodGet:
			seen["get-config"] = true
			writeEnvelope(t, w, map[string]any{
				"hostname":             "ct-101",
				"description":          "Managed by Terraform",
				"tags":                 "prod,terraform",
				"arch":                 "amd64",
				"cores":                "2",
				"memory":               512,
				"swap":                 "128",
				"onboot":               1,
				"protection":           true,
				"startup":              "order=2",
				"unprivileged":         "1",
				"features":             "nesting=1",
				"ostype":               "debian",
				"rootfs":               "local-lvm:vm-101-disk-0,size=8G",
				"nameserver":           "1.1.1.1",
				"searchdomain":         "example.internal",
				"timezone":             "host",
				"net0":                 "name=eth0,bridge=vmbr0,ip=dhcp",
				"mp0":                  "local-lvm:1,mp=/data",
				"lxc.apparmor.profile": "unconfined",
			})
		case r.URL.Path == "/api2/json/nodes/pve-1/lxc/101/status/current" && r.Method == http.MethodGet:
			seen["get-status"] = true
			writeEnvelope(t, w, map[string]any{
				"status": "running",
				"uptime": "300",
			})
		case r.URL.Path == "/api2/json/nodes/pve-1/lxc" && r.Method == http.MethodPost:
			seen["create"] = true
			assertFormValues(t, r, url.Values{
				"vmid":                 {"101"},
				"ostemplate":           {"local:vztmpl/debian-12.tar.zst"},
				"hostname":             {"ct-101"},
				"description":          {"Managed by Terraform"},
				"tags":                 {"prod,terraform"},
				"arch":                 {"amd64"},
				"cores":                {"2"},
				"memory":               {"512"},
				"swap":                 {"128"},
				"onboot":               {"1"},
				"protection":           {"1"},
				"startup":              {"order=2"},
				"unprivileged":         {"1"},
				"features":             {"nesting=1"},
				"ostype":               {"debian"},
				"rootfs":               {"local-lvm:8"},
				"nameserver":           {"1.1.1.1"},
				"searchdomain":         {"example.internal"},
				"timezone":             {"host"},
				"net0":                 {"name=eth0,bridge=vmbr0,ip=dhcp"},
				"mp0":                  {"local-lvm:1,mp=/data"},
				"lxc.apparmor.profile": {"unconfined"},
			})
			writeEnvelope(t, w, "UPID:pve-1:0001:create:101:")
		case isLXCContainerTaskRequest(r, "UPID:pve-1:0001:create:101:"):
			seen["create-task"] = true
			writeEnvelope(t, w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.URL.Path == "/api2/json/nodes/pve-1/lxc/101/config" && r.Method == http.MethodPut:
			seen["update"] = true
			assertFormValues(t, r, url.Values{
				"hostname": {"ct-updated"},
				"memory":   {"1024"},
				"net0":     {"name=eth0,bridge=vmbr1,ip=dhcp"},
				"delete":   {"mp0,tags"},
			})
			writeEnvelope(t, w, "UPID:pve-1:0002:update:101:")
		case isLXCContainerTaskRequest(r, "UPID:pve-1:0002:update:101:"):
			seen["update-task"] = true
			writeEnvelope(t, w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.URL.Path == "/api2/json/nodes/pve-1/lxc/101" && r.Method == http.MethodDelete:
			seen["delete"] = true
			writeEnvelope(t, w, "UPID:pve-1:0003:destroy:101:")
		case isLXCContainerTaskRequest(r, "UPID:pve-1:0003:destroy:101:"):
			seen["delete-task"] = true
			writeEnvelope(t, w, map[string]any{"status": "stopped", "exitstatus": "OK"})
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

	config, err := client.GetLXCContainerConfig(ctx, "pve-1", 101)
	if err != nil {
		t.Fatalf("GetLXCContainerConfig() unexpected error: %v", err)
	}
	if config.Hostname != "ct-101" || config.Description != "Managed by Terraform" || config.Tags != "prod,terraform" {
		t.Fatalf("unexpected basic lxc config fields: %#v", config)
	}
	if config.Arch != "amd64" || config.Startup != "order=2" || config.Features != "nesting=1" || config.OSType != "debian" {
		t.Fatalf("unexpected lxc config string fields: %#v", config)
	}
	if config.RootFS != "local-lvm:vm-101-disk-0,size=8G" || config.Nameserver != "1.1.1.1" || config.Searchdomain != "example.internal" || config.Timezone != "host" {
		t.Fatalf("unexpected lxc storage/dns fields: %#v", config)
	}
	if config.Cores.Ptr() == nil || *config.Cores.Ptr() != 2 || config.Memory.Ptr() == nil || *config.Memory.Ptr() != 512 || config.Swap.Ptr() == nil || *config.Swap.Ptr() != 128 {
		t.Fatalf("unexpected lxc integer fields: %#v", config)
	}
	if config.OnBoot.Ptr() == nil || !*config.OnBoot.Ptr() || config.Protection.Ptr() == nil || !*config.Protection.Ptr() || config.Unprivileged.Ptr() == nil || !*config.Unprivileged.Ptr() {
		t.Fatalf("unexpected lxc bool fields: %#v", config)
	}
	if got := config.Network["net0"]; got != "name=eth0,bridge=vmbr0,ip=dhcp" {
		t.Fatalf("unexpected network map: %#v", config.Network)
	}
	if got := config.MountPoint["mp0"]; got != "local-lvm:1,mp=/data" {
		t.Fatalf("unexpected mount point map: %#v", config.MountPoint)
	}
	if got := config.ExtraConfig["lxc.apparmor.profile"]; got != "unconfined" {
		t.Fatalf("unexpected extra config: %#v", config.ExtraConfig)
	}

	status, err := client.GetLXCContainerStatus(ctx, "pve-1", 101)
	if err != nil {
		t.Fatalf("GetLXCContainerStatus() unexpected error: %v", err)
	}
	if status.Status != "running" || status.Uptime.Ptr() == nil || *status.Uptime.Ptr() != 300 {
		t.Fatalf("unexpected lxc status: %#v", status)
	}

	if err := client.CreateLXCContainer(ctx, "pve-1", CreateLXCContainerRequest{
		VMID:       101,
		OSTemplate: stringPtr("local:vztmpl/debian-12.tar.zst"),
		lxcContainerConfigRequest: lxcContainerConfigRequest{
			Hostname:     stringPtr("ct-101"),
			Description:  stringPtr("Managed by Terraform"),
			Tags:         stringPtr("prod,terraform"),
			Arch:         stringPtr("amd64"),
			Cores:        intPtr64(2),
			Memory:       intPtr64(512),
			Swap:         intPtr64(128),
			OnBoot:       boolPtr(true),
			Protection:   boolPtr(true),
			Startup:      stringPtr("order=2"),
			Unprivileged: boolPtr(true),
			Features:     stringPtr("nesting=1"),
			OSType:       stringPtr("debian"),
			RootFS:       stringPtr("local-lvm:8"),
			Nameserver:   stringPtr("1.1.1.1"),
			Searchdomain: stringPtr("example.internal"),
			Timezone:     stringPtr("host"),
			Network: map[string]string{
				"net0": "name=eth0,bridge=vmbr0,ip=dhcp",
			},
			MountPoint: map[string]string{
				"mp0": "local-lvm:1,mp=/data",
			},
			ExtraConfig: map[string]string{
				"lxc.apparmor.profile": "unconfined",
			},
			Delete: []string{"hostname"},
		},
	}); err != nil {
		t.Fatalf("CreateLXCContainer() unexpected error: %v", err)
	}

	if err := client.UpdateLXCContainer(ctx, "pve-1", 101, UpdateLXCContainerRequest{
		lxcContainerConfigRequest: lxcContainerConfigRequest{
			Hostname:     stringPtr("ct-updated"),
			Arch:         stringPtr("arm64"),
			Memory:       intPtr64(1024),
			Unprivileged: boolPtr(true),
			RootFS:       stringPtr("local-lvm:16"),
			Network: map[string]string{
				"net0": "name=eth0,bridge=vmbr1,ip=dhcp",
			},
			Delete: []string{"tags", "mp0"},
		},
	}); err != nil {
		t.Fatalf("UpdateLXCContainer() unexpected error: %v", err)
	}

	if err := client.DeleteLXCContainer(ctx, "pve-1", 101); err != nil {
		t.Fatalf("DeleteLXCContainer() unexpected error: %v", err)
	}

	for _, name := range []string{"get-config", "get-status", "create", "create-task", "update", "update-task", "delete", "delete-task"} {
		if !seen[name] {
			t.Fatalf("expected request %q to run", name)
		}
	}
}

func TestDecodeLXCContainerConfigClassifiesMapsAndRaw(t *testing.T) {
	config, err := decodeLXCContainerConfig(map[string]json.RawMessage{
		"net0":                 json.RawMessage(`"name=eth0,bridge=vmbr0,ip=dhcp"`),
		"mp0":                  json.RawMessage(`"local-lvm:1,mp=/data"`),
		"rootfs":               json.RawMessage(`"local-lvm:vm-101-disk-0,size=8G"`),
		"lxc.apparmor.profile": json.RawMessage(`"unconfined"`),
	})
	if err != nil {
		t.Fatalf("decodeLXCContainerConfig() unexpected error: %v", err)
	}
	if got := config.Network["net0"]; got != "name=eth0,bridge=vmbr0,ip=dhcp" {
		t.Fatalf("unexpected network map: %#v", config.Network)
	}
	if got := config.MountPoint["mp0"]; got != "local-lvm:1,mp=/data" {
		t.Fatalf("unexpected mount point map: %#v", config.MountPoint)
	}
	if config.RootFS != "local-lvm:vm-101-disk-0,size=8G" {
		t.Fatalf("unexpected typed rootfs: %q", config.RootFS)
	}
	if _, ok := config.ExtraConfig["rootfs"]; ok {
		t.Fatalf("expected rootfs to stay typed, got extra config %#v", config.ExtraConfig)
	}
	if got := config.ExtraConfig["lxc.apparmor.profile"]; got != "unconfined" {
		t.Fatalf("unexpected raw extra config: %#v", config.ExtraConfig)
	}
}

func TestClientLXCContainerTaskFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(context.Context, *Client) error
		upid string
	}{
		{
			name: "create",
			upid: "UPID:pve-1:1001:create:101:",
			run: func(ctx context.Context, client *Client) error {
				return client.CreateLXCContainer(ctx, "pve-1", CreateLXCContainerRequest{VMID: 101})
			},
		},
		{
			name: "update",
			upid: "UPID:pve-1:1002:update:101:",
			run: func(ctx context.Context, client *Client) error {
				return client.UpdateLXCContainer(ctx, "pve-1", 101, UpdateLXCContainerRequest{})
			},
		},
		{
			name: "delete",
			upid: "UPID:pve-1:1003:destroy:101:",
			run: func(ctx context.Context, client *Client) error {
				return client.DeleteLXCContainer(ctx, "pve-1", 101)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assertTokenAuth(t, r)

				switch {
				case r.URL.Path == "/api2/json/nodes/pve-1/lxc" && r.Method == http.MethodPost:
					writeEnvelope(t, w, tc.upid)
				case r.URL.Path == "/api2/json/nodes/pve-1/lxc/101/config" && r.Method == http.MethodPut:
					writeEnvelope(t, w, tc.upid)
				case r.URL.Path == "/api2/json/nodes/pve-1/lxc/101" && r.Method == http.MethodDelete:
					writeEnvelope(t, w, tc.upid)
				case isLXCContainerTaskRequest(r, tc.upid):
					writeEnvelope(t, w, map[string]any{"status": "stopped", "exitstatus": "ERROR: boom"})
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

			err = tc.run(ctx, client)
			if err == nil {
				t.Fatalf("expected task failure error")
			}
			if !strings.Contains(err.Error(), tc.upid) || !strings.Contains(err.Error(), "ERROR: boom") {
				t.Fatalf("expected error to include upid and exit status, got %v", err)
			}
		})
	}
}

func TestClientLXCContainerNoTaskWhenUPIDMissing(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)

		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/lxc" && r.Method == http.MethodPost:
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/nodes/pve-1/lxc/101/config" && r.Method == http.MethodPut:
			writeJSON(t, w, map[string]any{})
		case r.URL.Path == "/api2/json/nodes/pve-1/lxc/101" && r.Method == http.MethodDelete:
			writeEnvelope(t, w, "")
		case strings.Contains(r.URL.Path, "/tasks/"):
			t.Fatalf("did not expect task polling request: %s %s", r.Method, r.URL.String())
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

	if err := client.CreateLXCContainer(ctx, "pve-1", CreateLXCContainerRequest{VMID: 101}); err != nil {
		t.Fatalf("CreateLXCContainer() unexpected error: %v", err)
	}
	if err := client.UpdateLXCContainer(ctx, "pve-1", 101, UpdateLXCContainerRequest{}); err != nil {
		t.Fatalf("UpdateLXCContainer() unexpected error: %v", err)
	}
	if err := client.DeleteLXCContainer(ctx, "pve-1", 101); err != nil {
		t.Fatalf("DeleteLXCContainer() unexpected error: %v", err)
	}
}

func TestClientLXCContainerTaskContextCanceled(t *testing.T) {
	withLXCContainerTaskTiming(t, time.Millisecond, time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)

		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/lxc" && r.Method == http.MethodPost:
			writeEnvelope(t, w, "UPID:pve-1:2001:create:101:")
		case isLXCContainerTaskRequest(r, "UPID:pve-1:2001:create:101:"):
			cancel()
			writeEnvelope(t, w, map[string]any{"status": "running"})
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

	err = client.CreateLXCContainer(ctx, "pve-1", CreateLXCContainerRequest{VMID: 101})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation to be preserved, got %v", err)
	}
}

func TestClientLXCContainerTaskTimeoutCap(t *testing.T) {
	withLXCContainerTaskTiming(t, time.Millisecond, 5*time.Millisecond)

	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)

		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/lxc" && r.Method == http.MethodPost:
			writeEnvelope(t, w, "UPID:pve-1:3001:create:101:")
		case isLXCContainerTaskRequest(r, "UPID:pve-1:3001:create:101:"):
			writeEnvelope(t, w, map[string]any{"status": "running"})
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

	err = client.CreateLXCContainer(ctx, "pve-1", CreateLXCContainerRequest{VMID: 101})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
	if !strings.Contains(err.Error(), "UPID:pve-1:3001:create:101:") {
		t.Fatalf("expected timeout error to name UPID, got %v", err)
	}
}

func isLXCContainerTaskRequest(r *http.Request, upid string) bool {
	if r.Method != http.MethodGet {
		return false
	}
	prefix := "/api2/json/nodes/pve-1/tasks/"
	if !strings.HasPrefix(r.URL.Path, prefix) || !strings.HasSuffix(r.URL.Path, "/status") {
		return false
	}
	encoded := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), "/status")
	decoded := encoded
	for range 2 {
		value, err := url.PathUnescape(decoded)
		if err != nil || value == decoded {
			break
		}
		decoded = value
	}
	return decoded == upid
}

func withLXCContainerTaskTiming(t *testing.T, pollInterval, timeoutCap time.Duration) {
	t.Helper()
	originalPollInterval := lxcContainerTaskPollInterval
	originalTimeoutCap := lxcContainerTaskTimeoutCap
	lxcContainerTaskPollInterval = pollInterval
	lxcContainerTaskTimeoutCap = timeoutCap
	t.Cleanup(func() {
		lxcContainerTaskPollInterval = originalPollInterval
		lxcContainerTaskTimeoutCap = originalTimeoutCap
	})
}
