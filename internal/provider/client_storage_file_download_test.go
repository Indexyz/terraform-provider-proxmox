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
	"time"
)

func TestClientStorageFileDownloadLifecycle(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve-1/storage/local/download-url":
			assertFormValues(t, r, url.Values{
				"checksum":            {"abc123"},
				"checksum-algorithm":  {"sha256"},
				"compression":         {"zstd"},
				"content":             {"iso"},
				"filename":            {"debian.iso"},
				"url":                 {"https://example.com/debian.iso.zst"},
				"verify-certificates": {"1"},
			})
			writeEnvelope(t, w, "UPID:pve-1:download")
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tasks/UPID:pve-1:download/status"):
			writeEnvelope(t, w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve-1/storage/local/content/local:iso/debian.iso":
			if !strings.Contains(r.URL.RawPath, "local:iso%2Fdebian.iso") {
				t.Fatalf("volume must be encoded as one path segment, raw path: %q", r.URL.RawPath)
			}
			writeEnvelope(t, w, map[string]any{"format": "iso", "path": "/var/lib/vz/template/iso/debian.iso", "size": 1024, "used": 1024})
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/nodes/pve-1/storage/local/content/local:iso/debian.iso":
			if !strings.Contains(r.URL.RawPath, "local:iso%2Fdebian.iso") {
				t.Fatalf("volume must be encoded as one path segment, raw path: %q", r.URL.RawPath)
			}
			writeEnvelope(t, w, "UPID:pve-1:delete")
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tasks/UPID:pve-1:delete/status"):
			writeEnvelope(t, w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			t.Fatalf("unexpected request: %s %s raw=%q", r.Method, r.URL.String(), r.URL.RawPath)
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

	volume, err := client.DownloadStorageFile(ctx, DownloadStorageFileRequest{
		Node:               "pve-1",
		Storage:            "local",
		Content:            "iso",
		Filename:           "debian.iso",
		URL:                "https://example.com/debian.iso.zst",
		Checksum:           stringPtr("abc123"),
		ChecksumAlgorithm:  stringPtr("sha256"),
		Compression:        stringPtr("zstd"),
		VerifyCertificates: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("DownloadStorageFile() unexpected error: %v", err)
	}
	if volume != "local:iso/debian.iso" {
		t.Fatalf("unexpected volume: %q", volume)
	}
	file, err := client.GetStorageFile(ctx, "pve-1", "local", volume)
	if err != nil {
		t.Fatalf("GetStorageFile() unexpected error: %v", err)
	}
	if file.Format != "iso" || file.Size.Ptr() == nil || *file.Size.Ptr() != 1024 {
		t.Fatalf("unexpected storage file: %#v", file)
	}
	if err := client.DeleteStorageFile(ctx, "pve-1", "local", volume); err != nil {
		t.Fatalf("DeleteStorageFile() unexpected error: %v", err)
	}
}
