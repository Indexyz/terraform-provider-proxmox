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

func TestClusterMetricsServerResourceSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewClusterMetricsServerResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 25; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	if !resp.Schema.Attributes["token"].IsSensitive() || !resp.Schema.Attributes["opentelemetry_headers"].IsSensitive() {
		t.Fatal("metrics credentials must be sensitive")
	}
}

func TestValidateClusterMetricsServerConfig(t *testing.T) {
	valid := clusterMetricsServerModel{
		Type:             types.StringValue("influxdb"),
		Server:           types.StringValue("influx.example.com"),
		Port:             types.Int64Value(8086),
		InfluxDBProtocol: types.StringValue("https"),
	}
	if diags := validateClusterMetricsServerConfig(valid); diags.HasError() {
		t.Fatalf("valid config diagnostics: %v", diags)
	}
	for _, invalid := range []clusterMetricsServerModel{
		{Type: types.StringValue("prometheus"), Server: valid.Server, Port: valid.Port},
		{Type: valid.Type, Server: types.StringValue(""), Port: valid.Port},
		{Type: valid.Type, Server: valid.Server, Port: types.Int64Value(0)},
		{Type: valid.Type, Server: valid.Server, Port: valid.Port, InfluxDBProtocol: types.StringValue("tcp")},
		{Type: types.StringValue("graphite"), Server: valid.Server, Port: valid.Port, Bucket: types.StringValue("pve")},
	} {
		if diags := validateClusterMetricsServerConfig(invalid); !diags.HasError() {
			t.Fatalf("expected invalid config diagnostics: %#v", invalid)
		}
	}
}

func TestClusterMetricsManagedFields(t *testing.T) {
	priorManaged := []string{"bucket", "disable", "organization", "token"}
	config := clusterMetricsServerModel{Disable: types.BoolValue(true)}
	got := clusterMetricsDeleteKeys(config, priorManaged)
	want := []string{"bucket", "organization", "token"}
	if len(got) != len(want) {
		t.Fatalf("unexpected delete keys: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected delete keys: got %v want %v", got, want)
		}
	}
	if got := clusterMetricsDeleteKeys(config, nil); len(got) != 0 {
		t.Fatalf("imported unmanaged fields must not be deleted: %v", got)
	}
}

func TestClusterMetricsServerReadPreservesToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, map[string]any{
			"id":      "influx-main",
			"type":    "influxdb",
			"server":  "influx.example.com",
			"port":    8086,
			"disable": 0,
			"bucket":  "pve",
			"token":   nil,
			"digest":  "abc123",
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
	resource := &ClusterMetricsServerResource{client: client}
	prior := clusterMetricsServerModel{Token: types.StringValue("secret")}
	state, diags := resource.readState(context.Background(), "influx-main", "influxdb", prior)
	if diags.HasError() {
		t.Fatalf("readState() unexpected diagnostics: %v", diags)
	}
	if state.Token.ValueString() != "secret" {
		t.Fatal("readState() did not preserve write-only token")
	}
}
