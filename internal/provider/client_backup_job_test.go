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

func TestCanonicalBackupPruneString(t *testing.T) {
	if got, want := canonicalBackupPruneString("keep-weekly=4, keep-daily=7,keep-monthly=6"), "keep-daily=7,keep-monthly=6,keep-weekly=4"; got != want {
		t.Fatalf("unexpected canonical prune string: got %q want %q", got, want)
	}
}

func TestCanonicalBackupPruneOptionsObject(t *testing.T) {
	got, err := canonicalBackupPruneOptions(json.RawMessage(`{"keep-all":false,"keep-daily":7}`))
	if err != nil {
		t.Fatalf("canonicalBackupPruneOptions() unexpected error: %v", err)
	}
	if want := "keep-all=0,keep-daily=7"; got != want {
		t.Fatalf("unexpected prune options: got %q want %q", got, want)
	}
}

func TestClientBackupJobCRUD(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/backup":
			assertFormValues(t, r, url.Values{
				"id":            {"nightly"},
				"all":           {"1"},
				"enabled":       {"1"},
				"mode":          {"snapshot"},
				"prune-backups": {"keep-daily=7,keep-last=3"},
				"schedule":      {"02:00"},
				"storage":       {"backup"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/backup/nightly":
			writeEnvelope(t, w, map[string]any{
				"id":            "nightly",
				"all":           1,
				"enabled":       1,
				"mode":          "snapshot",
				"next-run":      1_800_000_000,
				"prune-backups": "keep-daily=7,keep-last=3",
				"schedule":      "02:00",
				"storage":       "backup",
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/backup/nightly":
			assertFormValues(t, r, url.Values{
				"delete":  {"comment,pool"},
				"enabled": {"0"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/backup/nightly":
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

	if err := client.CreateBackupJob(ctx, "nightly", BackupJobRequest{
		All:          boolPtr(true),
		Enabled:      boolPtr(true),
		Mode:         stringPtr("snapshot"),
		PruneBackups: stringPtr("keep-daily=7,keep-last=3"),
		Schedule:     stringPtr("02:00"),
		Storage:      stringPtr("backup"),
	}); err != nil {
		t.Fatalf("CreateBackupJob() unexpected error: %v", err)
	}
	job, err := client.GetBackupJob(ctx, "nightly")
	if err != nil {
		t.Fatalf("GetBackupJob() unexpected error: %v", err)
	}
	if job.ID != "nightly" || job.NextRun.Ptr() == nil || *job.NextRun.Ptr() != 1_800_000_000 {
		t.Fatalf("unexpected backup job: %#v", job)
	}
	prune, err := canonicalBackupPruneOptions(job.PruneBackups)
	if err != nil {
		t.Fatalf("canonicalBackupPruneOptions() unexpected error: %v", err)
	}
	if prune != "keep-daily=7,keep-last=3" {
		t.Fatalf("unexpected prune options: %q", prune)
	}
	if err := client.UpdateBackupJob(ctx, "nightly", BackupJobRequest{
		Enabled: boolPtr(false),
		Delete:  []string{"pool", "comment"},
	}); err != nil {
		t.Fatalf("UpdateBackupJob() unexpected error: %v", err)
	}
	if err := client.DeleteBackupJob(ctx, "nightly"); err != nil {
		t.Fatalf("DeleteBackupJob() unexpected error: %v", err)
	}
}
