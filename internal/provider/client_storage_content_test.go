// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUploadStorageFileMultipartContract(t *testing.T) {
	payload := []byte("fake-seed-image-bytes")
	var gotPayload []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		if r.Method != http.MethodPost || r.URL.EscapedPath() != "/api2/json/nodes/pve%20one/storage/local%20iso/upload" || r.URL.RawQuery != "" {
			t.Fatalf("unexpected upload request: %s %s", r.Method, r.URL.String())
		}
		mediaType, ok := r.Header["Content-Type"]
		if !ok || len(mediaType) != 1 || !bytes.HasPrefix([]byte(mediaType[0]), []byte("multipart/form-data; boundary=")) {
			t.Fatalf("unexpected content type: %v", mediaType)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		if got := r.FormValue("content"); got != "iso" {
			t.Fatalf("unexpected content field: %q", got)
		}
		file, header, err := r.FormFile("filename")
		if err != nil {
			t.Fatalf("read filename file part: %v", err)
		}
		defer file.Close()
		if header.Filename != "seed 2026.iso" {
			t.Fatalf("unexpected file part name: %q", header.Filename)
		}
		gotPayload, err = io.ReadAll(file)
		if err != nil {
			t.Fatalf("read upload payload: %v", err)
		}
		writeEnvelope(t, w, "UPID:pve-two:000C1DE5:upload:seed 2026.iso:terraform@pve!provider")
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

	upid, taskNode, err := client.UploadStorageFile(context.Background(), UploadStorageFileRequest{
		Node:     "pve one",
		Storage:  "local iso",
		Content:  "iso",
		Filename: "seed 2026.iso",
		Data:     payload,
	})
	if err != nil {
		t.Fatalf("UploadStorageFile() unexpected error: %v", err)
	}
	if want := "UPID:pve-two:000C1DE5:upload:seed 2026.iso:terraform@pve!provider"; upid != want {
		t.Fatalf("unexpected UPID: got %q want %q", upid, want)
	}
	// The upload endpoint is not proxied to the requested node, so the returned
	// task owner (pve-two) must win over the destination node (pve one).
	if taskNode != "pve-two" {
		t.Fatalf("unexpected task owner node: %q", taskNode)
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Fatalf("upload payload mismatch: got %q want %q", gotPayload, payload)
	}
}

func TestUploadStorageFileRequiresTaskUPID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		writeEnvelope(t, w, nil)
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

	_, _, err = client.UploadStorageFile(context.Background(), UploadStorageFileRequest{Node: "pve", Storage: "local", Content: "iso", Filename: "seed.iso"})
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("returned no UPID")) {
		t.Fatalf("expected missing UPID error, got %v", err)
	}
}

func TestTaskOwnerNode(t *testing.T) {
	node, err := taskOwnerNode("UPID:pve-one:000C1DE5:00000000:6712A5F4:upload:seed.iso:terraform@pve!provider")
	if err != nil || node != "pve-one" {
		t.Fatalf("unexpected owner node %q err %v", node, err)
	}
	for _, malformed := range []string{"", "pve", "UPID:", "UPID::000C1DE5"} {
		if _, err := taskOwnerNode(malformed); err == nil {
			t.Fatalf("expected malformed UPID error for %q", malformed)
		}
	}
}

func TestNodeStoragesAndContentList(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/storage" && r.URL.RawQuery == "":
			writeEnvelope(t, w, []any{
				map[string]any{"storage": "local iso", "type": "dir", "content": "iso,vztmpl", "active": 1, "enabled": 1, "shared": 0},
				map[string]any{"storage": "local-zfs", "type": "zfspool", "content": "images,rootdir", "active": 1, "enabled": 1, "shared": 0},
			})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve%20one/storage/local%20iso/content":
			if got := r.URL.Query().Get("content"); got != "iso" {
				t.Fatalf("unexpected content query: %q", got)
			}
			writeEnvelope(t, w, []any{
				map[string]any{"volid": "local iso:iso/other.iso", "format": "iso", "size": 4096, "content": "iso"},
				map[string]any{"volid": "local iso:iso/seed.iso", "format": "iso", "size": 372736, "used": 372736, "content": "iso"},
			})
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

	storages, err := client.NodeStorages(ctx, "pve one")
	if err != nil {
		t.Fatalf("NodeStorages() unexpected error: %v", err)
	}
	if len(storages) != 2 {
		t.Fatalf("unexpected storages: %#v", storages)
	}
	if storages[0].Storage != "local iso" || storages[0].Type != "dir" || storages[0].Content != "iso,vztmpl" {
		t.Fatalf("unexpected first storage: %#v", storages[0])
	}
	if enabled := storages[0].Enabled.Ptr(); enabled == nil || !*enabled {
		t.Fatalf("expected enabled storage, got %#v", storages[0].Enabled)
	}
	if shared := storages[0].Shared.Ptr(); shared == nil || *shared {
		t.Fatalf("expected non-shared storage, got %#v", storages[0].Shared)
	}

	items, err := client.StorageContentList(ctx, "pve one", "local iso", "iso")
	if err != nil {
		t.Fatalf("StorageContentList() unexpected error: %v", err)
	}
	if len(items) != 2 || items[1].Volid != "local iso:iso/seed.iso" || items[1].Format != "iso" {
		t.Fatalf("unexpected content items: %#v", items)
	}
	if size := items[1].Size.Ptr(); size == nil || *size != 372736 {
		t.Fatalf("unexpected size: %#v", items[1].Size)
	}
}

func TestStorageContentListPreservesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":{"permission":"missing Datastore.Audit"}}`))
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

	_, err = client.StorageContentList(context.Background(), "pve", "local", "iso")
	var apiErr *APIError
	if err == nil || !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden || !bytes.Contains([]byte(err.Error()), []byte("missing Datastore.Audit")) {
		t.Fatalf("expected preserved API error, got %v", err)
	}
}

func TestGetNodeTaskStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api2/json/nodes/pve%20one/tasks/UPID:pve%20one:imgcopy-1/status" || r.URL.RawQuery != "" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		writeEnvelope(t, w, map[string]any{"status": "stopped", "exitstatus": "OK"})
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

	status, err := client.GetNodeTaskStatus(context.Background(), "pve one", "UPID:pve one:imgcopy-1")
	if err != nil {
		t.Fatalf("GetNodeTaskStatus() unexpected error: %v", err)
	}
	if status.Status != "stopped" || status.ExitStatus != "OK" {
		t.Fatalf("unexpected task status: %#v", status)
	}
}
