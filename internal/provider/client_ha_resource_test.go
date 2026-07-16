// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientHAResourceCRUDUsesCollectionDigestAndSafeDelete(t *testing.T) {
	ctx := context.Background()
	var collectionReads atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.URL.Path == "/api2/json/cluster/ha/resources" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{
				"auto-rebalance": {"0"},
				"comment":        {"database guest"},
				"failback":       {"1"},
				"max_relocate":   {"3"},
				"max_restart":    {"2"},
				"sid":            {"vm:120"},
				"state":          {"ignored"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/ha/resources" && r.Method == http.MethodGet:
			collectionReads.Add(1)
			writeEnvelope(t, w, []map[string]any{
				{
					"auto-rebalance": "0",
					"comment":        "database guest",
					"digest":         "abc123",
					"failback":       1,
					"max_relocate":   3,
					"max_restart":    "2",
					"sid":            "vm:120",
					"state":          "ignored",
				},
				{"sid": "ct:121", "state": "disabled"},
			})
		case r.URL.Path == "/api2/json/cluster/ha/resources/vm:120" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{
				"auto-rebalance": {"1"},
				"delete":         {"comment,max_restart"},
				"digest":         {"abc123"},
				"max_relocate":   {"5"},
				"state":          {"disabled"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/ha/resources/vm:120" && r.Method == http.MethodDelete:
			assertFormValues(t, r, url.Values{"purge": {"0"}})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	if err := client.CreateHAResource(ctx, "vm:120", HAResourceRequest{
		State:         "ignored",
		Comment:       stringPtr("database guest"),
		Failback:      boolPtr(true),
		AutoRebalance: boolPtr(false),
		MaxRestart:    intPtr64(2),
		MaxRelocate:   intPtr64(3),
	}); err != nil {
		t.Fatalf("CreateHAResource() unexpected error: %v", err)
	}
	resource, err := client.GetHAResource(ctx, "vm:120")
	if err != nil {
		t.Fatalf("GetHAResource() unexpected error: %v", err)
	}
	if resource.Digest != "abc123" || resource.Failback.Ptr() == nil || !*resource.Failback.Ptr() || resource.AutoRebalance.Ptr() == nil || *resource.AutoRebalance.Ptr() {
		t.Fatalf("unexpected HA booleans/digest: %#v", resource)
	}
	if resource.MaxRestart.Ptr() == nil || *resource.MaxRestart.Ptr() != 2 || resource.MaxRelocate.Ptr() == nil || *resource.MaxRelocate.Ptr() != 3 {
		t.Fatalf("unexpected HA limits: %#v", resource)
	}
	if err := client.UpdateHAResource(ctx, "vm:120", HAResourceRequest{
		State:         "disabled",
		AutoRebalance: boolPtr(true),
		MaxRelocate:   intPtr64(5),
		Digest:        stringPtr(resource.Digest),
		Delete:        []string{"max_restart", "comment"},
	}); err != nil {
		t.Fatalf("UpdateHAResource() unexpected error: %v", err)
	}
	if err := client.DeleteHAResource(ctx, "vm:120"); err != nil {
		t.Fatalf("DeleteHAResource() unexpected error: %v", err)
	}
	if got, want := collectionReads.Load(), int64(2); got != want {
		t.Fatalf("unexpected collection read count: got %d want %d", got, want)
	}
}

func TestClientHAResourceMissingUsesCollectionAndDeleteIsIdempotent(t *testing.T) {
	ctx := context.Background()
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api2/json/cluster/ha/resources" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		writeEnvelope(t, w, []any{})
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	if _, err := client.GetHAResource(ctx, "ct:404"); !errors.Is(err, errNotFound) {
		t.Fatalf("GetHAResource() error = %v, want errNotFound", err)
	}
	if err := client.DeleteHAResource(ctx, "ct:404"); err != nil {
		t.Fatalf("DeleteHAResource() unexpected error: %v", err)
	}
	if got, want := requests.Load(), int64(2); got != want {
		t.Fatalf("unexpected request count: got %d want %d", got, want)
	}
}

func TestClientHAResourcePreservesCollectionAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":{"sid":"permission denied"},"data":null}`))
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	_, err := client.GetHAResource(context.Background(), "vm:120")
	if err == nil || !strings.Contains(err.Error(), "status 500") || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("GetHAResource() did not preserve API error: %v", err)
	}
}

func newHAResourceTestClient(t *testing.T, endpoint string) *Client {
	t.Helper()
	client, err := NewClient(context.Background(), ClientConfig{
		Endpoint:       endpoint,
		APITokenID:     "terraform@pve!provider",
		APITokenSecret: "token-secret",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}
	return client
}
