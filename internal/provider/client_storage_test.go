// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestClientStorageMethods(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.URL.Path == "/api2/json/storage/local-lvm" && r.Method == http.MethodGet:
			writeEnvelope(t, w, map[string]any{
				"storage":  "local-lvm",
				"type":     "lvmthin",
				"content":  "images,rootdir",
				"vgname":   "pve",
				"thinpool": "data",
			})
		case r.URL.Path == "/api2/json/storage" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{
				"storage": {"local-dir"},
				"type":    {"dir"},
				"content": {"images,iso,vztmpl"},
				"path":    {"/mnt/data"},
				"nodes":   {"pve-1,pve-2"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/storage/local-dir" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{
				"content": {"images,iso"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/storage/local-dir" && r.Method == http.MethodDelete:
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

	// Read
	config, err := client.GetStorage(ctx, "local-lvm")
	if err != nil {
		t.Fatalf("GetStorage() unexpected error: %v", err)
	}
	if config.Storage != "local-lvm" || config.Type != "lvmthin" || config.Content != "images,rootdir" {
		t.Fatalf("unexpected storage config: %#v", config)
	}
	if config.VGName != "pve" || config.ThinPool != "data" {
		t.Fatalf("unexpected lvm fields: %#v", config)
	}

	// Create
	if err := client.CreateStorage(ctx, StorageRequest{
		Storage: "local-dir",
		Type:    "dir",
		Content: stringPtr("images,iso,vztmpl"),
		Path:    stringPtr("/mnt/data"),
		Nodes:   stringPtr("pve-1,pve-2"),
	}); err != nil {
		t.Fatalf("CreateStorage() unexpected error: %v", err)
	}

	// Update
	if err := client.UpdateStorage(ctx, "local-dir", StorageRequest{
		Content: stringPtr("images,iso"),
	}); err != nil {
		t.Fatalf("UpdateStorage() unexpected error: %v", err)
	}

	// Delete
	if err := client.DeleteStorage(ctx, "local-dir"); err != nil {
		t.Fatalf("DeleteStorage() unexpected error: %v", err)
	}
}

func TestDecodeStorageConfigExtraConfigFallback(t *testing.T) {
	t.Parallel()

	config, err := decodeStorageConfig(map[string]json.RawMessage{
		"storage":               json.RawMessage(`"my-store"`),
		"type":                  json.RawMessage(`"dir"`),
		"max-protected-backups": json.RawMessage(`"5"`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Storage != "my-store" || config.Type != "dir" {
		t.Fatalf("unexpected config: %#v", config)
	}
	if config.ExtraConfig["max-protected-backups"] != "5" {
		t.Fatalf("expected untyped key in extra_config, got %#v", config.ExtraConfig)
	}
}
