// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestClientReplicationJobCRUD(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.URL.Path == "/api2/json/cluster/replication" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{
				"comment":  {"replicate database"},
				"disable":  {"0"},
				"id":       {"101-0"},
				"rate":     {"12.5"},
				"schedule": {"*/30"},
				"target":   {"pve-2"},
				"type":     {"local"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/replication/101-0" && r.Method == http.MethodGet:
			writeEnvelope(t, w, map[string]any{
				"comment":  "replicate database",
				"digest":   "abc123",
				"disable":  0,
				"guest":    101,
				"id":       "101-0",
				"jobnum":   0,
				"rate":     12.5,
				"schedule": "*/30",
				"source":   "pve-1",
				"target":   "pve-2",
				"type":     "local",
			})
		case r.URL.Path == "/api2/json/cluster/replication/101-0" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{
				"delete":  {"comment,rate"},
				"digest":  {"abc123"},
				"disable": {"1"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/replication/101-0" && r.Method == http.MethodDelete:
			assertFormValues(t, r, url.Values{"force": {"1"}})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewClient(ctx, ClientConfig{Endpoint: server.URL, APITokenID: "terraform@pve!provider", APITokenSecret: "token-secret", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}
	rate := 12.5
	if err := client.CreateReplicationJob(ctx, "101-0", "pve-2", ReplicationJobRequest{
		Comment: stringPtr("replicate database"), Disable: boolPtr(false), Rate: &rate, Schedule: stringPtr("*/30"),
	}); err != nil {
		t.Fatalf("CreateReplicationJob() unexpected error: %v", err)
	}
	job, err := client.GetReplicationJob(ctx, "101-0")
	if err != nil {
		t.Fatalf("GetReplicationJob() unexpected error: %v", err)
	}
	if job.Digest != "abc123" || job.Disable.Ptr() == nil || *job.Disable.Ptr() || job.Rate == nil || *job.Rate != 12.5 {
		t.Fatalf("unexpected replication job: %#v", job)
	}
	if err := client.UpdateReplicationJob(ctx, "101-0", ReplicationJobRequest{
		Digest: stringPtr(job.Digest), Disable: boolPtr(true), Delete: []string{"rate", "comment"},
	}); err != nil {
		t.Fatalf("UpdateReplicationJob() unexpected error: %v", err)
	}
	if err := client.DeleteReplicationJob(ctx, "101-0"); err != nil {
		t.Fatalf("DeleteReplicationJob() unexpected error: %v", err)
	}
}
