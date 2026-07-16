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

func TestReplicationJobResourceSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewReplicationJobResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 11; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	if !resp.Schema.Attributes["job_id"].IsRequired() || !resp.Schema.Attributes["target"].IsRequired() {
		t.Fatal("job_id and target must be required")
	}
}

func TestValidateReplicationJobConfig(t *testing.T) {
	valid := replicationJobModel{
		JobID:    types.StringValue("101-0"),
		Target:   types.StringValue("pve-2"),
		Rate:     types.Float64Value(10.5),
		Schedule: types.StringValue("*/15"),
	}
	if diags := validateReplicationJobConfig(valid); diags.HasError() {
		t.Fatalf("valid config diagnostics: %v", diags)
	}
	invalid := []replicationJobModel{
		{JobID: types.StringValue("101"), Target: valid.Target},
		{JobID: valid.JobID, Target: types.StringValue("")},
		{JobID: valid.JobID, Target: valid.Target, Rate: types.Float64Value(0.5)},
		{JobID: valid.JobID, Target: valid.Target, Schedule: types.StringValue("")},
		{JobID: valid.JobID, Target: valid.Target, Comment: types.StringValue(string(make([]byte, 4097)))},
	}
	for _, config := range invalid {
		if diags := validateReplicationJobConfig(config); !diags.HasError() {
			t.Fatalf("expected invalid config diagnostics: %#v", config)
		}
	}
}

func TestReplicationJobManagedFields(t *testing.T) {
	config := replicationJobModel{Disable: types.BoolValue(true), Schedule: types.StringValue("*/30")}
	got := replicationJobDeleteKeys(config, []string{"comment", "disable", "rate", "schedule"})
	want := []string{"comment", "rate"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("unexpected delete keys: got %v want %v", got, want)
	}
	if got := replicationJobDeleteKeys(config, nil); len(got) != 0 {
		t.Fatalf("imported unmanaged fields must not be deleted: %v", got)
	}
}

func TestReplicationJobReadState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api2/json/cluster/replication/101-0" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		writeEnvelope(t, w, map[string]any{
			"disable":  1,
			"guest":    101,
			"id":       "101-0",
			"jobnum":   0,
			"schedule": "*/15",
			"source":   "pve-1",
			"target":   "pve-2",
			"type":     "local",
		})
	}))
	defer server.Close()
	client, err := NewClient(context.Background(), ClientConfig{Endpoint: server.URL, APITokenID: "terraform@pve!provider", APITokenSecret: "token-secret", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}
	state, diags := (&ReplicationJobResource{client: client}).readState(context.Background(), "101-0")
	if diags.HasError() {
		t.Fatalf("readState() unexpected diagnostics: %v", diags)
	}
	if state.Target.ValueString() != "pve-2" || !state.Disable.ValueBool() || state.GuestID.ValueInt64() != 101 || state.Type.ValueString() != "local" {
		t.Fatalf("unexpected replication state: %#v", state)
	}
}
