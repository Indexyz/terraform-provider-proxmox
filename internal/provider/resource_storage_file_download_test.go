// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestStorageFileDownloadResourceSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewStorageFileDownloadResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 17; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	if !resp.Schema.Attributes["url"].IsSensitive() {
		t.Fatal("url must be sensitive because it may contain credentials")
	}
}

func TestValidateStorageFileDownloadConfig(t *testing.T) {
	valid := storageFileDownloadModel{
		Content:           types.StringValue("iso"),
		Filename:          types.StringValue("debian-13.0.iso"),
		URL:               types.StringValue("https://example.com/debian.iso"),
		Checksum:          types.StringValue("abc123"),
		ChecksumAlgorithm: types.StringValue("sha256"),
	}
	if diags := validateStorageFileDownloadConfig(valid); diags.HasError() {
		t.Fatalf("valid config diagnostics: %v", diags)
	}

	tests := []struct {
		name  string
		model storageFileDownloadModel
	}{
		{"content", storageFileDownloadModel{Content: types.StringValue("backup"), Filename: valid.Filename, URL: valid.URL}},
		{"filename", storageFileDownloadModel{Content: valid.Content, Filename: types.StringValue("../debian.iso"), URL: valid.URL}},
		{"url", storageFileDownloadModel{Content: valid.Content, Filename: valid.Filename, URL: types.StringValue("ftp://example.com/file")}},
		{"checksum pair", storageFileDownloadModel{Content: valid.Content, Filename: valid.Filename, URL: valid.URL, Checksum: types.StringValue("abc123")}},
		{"checksum algorithm", storageFileDownloadModel{Content: valid.Content, Filename: valid.Filename, URL: valid.URL, Checksum: types.StringValue("abc123"), ChecksumAlgorithm: types.StringValue("crc32")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if diags := validateStorageFileDownloadConfig(test.model); !diags.HasError() {
				t.Fatal("expected validation diagnostics")
			}
		})
	}
}

func TestStorageFileIdentifiers(t *testing.T) {
	volume := storageFileVolumeID("local", "iso", "debian.iso")
	if volume != "local:iso/debian.iso" {
		t.Fatalf("unexpected volume identifier: %q", volume)
	}
	if got, want := storageFileDownloadID("pve-1", "local", volume), "pve-1/local/local:iso/debian.iso"; got != want {
		t.Fatalf("unexpected resource identifier: got %q want %q", got, want)
	}
}

func TestStorageFileDownloadReadState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api2/json/nodes/pve-1/storage/local/content/local:iso/debian.iso" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		writeEnvelope(t, w, map[string]any{
			"format": "iso",
			"path":   "/var/lib/vz/template/iso/debian.iso",
			"size":   2048,
			"used":   1024,
		})
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
	resource := &StorageFileDownloadResource{client: client}
	prior := storageFileDownloadModel{
		Node:     types.StringValue("pve-1"),
		Storage:  types.StringValue("local"),
		VolumeID: types.StringValue("local:iso/debian.iso"),
		URL:      types.StringValue("https://example.com/debian.iso"),
	}
	state, diags := resource.readState(context.Background(), prior)
	if diags.HasError() {
		t.Fatalf("readState() unexpected diagnostics: %v", diags)
	}
	if state.Format.ValueString() != "iso" || state.Size.ValueInt64() != 2048 {
		t.Fatalf("unexpected refreshed state: %#v", state)
	}
	if state.URL.ValueString() != prior.URL.ValueString() {
		t.Fatal("readState() did not preserve create-only URL")
	}
}
