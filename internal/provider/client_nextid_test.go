// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientGetNextVMID(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.Path != "/api2/json/cluster/nextid" || r.Method != http.MethodGet {
			handler.fail(w, "unexpected nextid request: %s %s", r.Method, r.URL.String())
			return
		}
		switch r.URL.RawQuery {
		case "":
			handler.envelope(w, 105)
		case "vmid=200":
			handler.envelope(w, 200)
		case "vmid=201":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			if err := json.NewEncoder(w).Encode(map[string]any{
				"errors": map[string]string{"vmid": "VM 201 already exists"},
				"data":   nil,
			}); err != nil {
				handler.fail(w, "encode response: %v", err)
			}
		default:
			handler.fail(w, "unexpected nextid query: %q", r.URL.RawQuery)
		}
	}))
	defer server.Close()

	client := testLifecycleClient(t, server)

	vmID, err := client.GetNextVMID(ctx, nil)
	if err != nil {
		t.Fatalf("GetNextVMID() unexpected error: %v", err)
	}
	if vmID != 105 {
		t.Fatalf("GetNextVMID() = %d, want 105", vmID)
	}

	assertID := int64(200)
	vmID, err = client.GetNextVMID(ctx, &assertID)
	if err != nil {
		t.Fatalf("GetNextVMID(assert) unexpected error: %v", err)
	}
	if vmID != 200 {
		t.Fatalf("GetNextVMID(assert) = %d, want 200", vmID)
	}

	takenID := int64(201)
	vmID, err = client.GetNextVMID(ctx, &takenID)
	if err == nil {
		t.Fatalf("GetNextVMID(taken) expected error, got vmID %d", vmID)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("GetNextVMID(taken) error = %v, want HTTP 400 APIError", err)
	}
	handler.assert(t)
}

func TestClientGetNextVMIDStringResponse(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		// Older Proxmox VE versions return the VMID as a JSON string.
		handler.envelope(w, "107")
	}))
	defer server.Close()

	vmID, err := testLifecycleClient(t, server).GetNextVMID(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetNextVMID() unexpected error: %v", err)
	}
	if vmID != 107 {
		t.Fatalf("GetNextVMID() = %d, want 107", vmID)
	}
	handler.assert(t)
}

func TestClientGetNextVMIDInvalidResponse(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		handler.envelope(w, "not-a-number")
	}))
	defer server.Close()

	vmID, err := testLifecycleClient(t, server).GetNextVMID(context.Background(), nil)
	if err == nil {
		t.Fatalf("GetNextVMID() expected error, got vmID %d", vmID)
	}
	if vmID != 0 {
		t.Fatalf("GetNextVMID() = %d, want 0 on error", vmID)
	}
	handler.assert(t)
}
