// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"
)

func TestClusterMetricsServerTypeForms(t *testing.T) {
	tests := []struct {
		name string
		req  ClusterMetricsServerRequest
		want url.Values
	}{
		{
			name: "graphite",
			req:  ClusterMetricsServerRequest{Port: 2003, Server: "graphite.example.com", Path: stringPtr("proxmox.cluster"), Protocol: stringPtr("tcp"), Timeout: int64Pointer(2)},
			want: url.Values{"port": {"2003"}, "server": {"graphite.example.com"}, "path": {"proxmox.cluster"}, "proto": {"tcp"}, "timeout": {"2"}},
		},
		{
			name: "opentelemetry",
			req:  ClusterMetricsServerRequest{Port: 4318, Server: "otel.example.com", OpenTelemetryCompress: stringPtr("gzip"), OpenTelemetryPath: stringPtr("/v1/metrics"), OpenTelemetryProtocol: stringPtr("https"), OpenTelemetryTimeout: int64Pointer(5), OpenTelemetryVerify: boolPtr(true)},
			want: url.Values{"port": {"4318"}, "server": {"otel.example.com"}, "otel-compression": {"gzip"}, "otel-path": {"/v1/metrics"}, "otel-protocol": {"https"}, "otel-timeout": {"5"}, "otel-verify-ssl": {"1"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := clusterMetricsServerForm(test.req); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("unexpected form: got %#v want %#v", got, test.want)
			}
		})
	}
}

func TestClientClusterMetricsServerCRUD(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		if r.URL.Path != "/api2/json/cluster/metrics/server/influx-main" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodPost:
			assertFormValues(t, r, url.Values{
				"bucket":             {"pve"},
				"id":                 {"influx-main"},
				"influxdbproto":      {"https"},
				"organization":       {"infra"},
				"port":               {"8086"},
				"server":             {"influx.example.com"},
				"token":              {"secret"},
				"type":               {"influxdb"},
				"verify-certificate": {"1"},
			})
			writeEnvelope(t, w, nil)
		case http.MethodGet:
			writeEnvelope(t, w, map[string]any{
				"bucket":             "pve",
				"digest":             "abc123",
				"disable":            0,
				"id":                 "influx-main",
				"influxdbproto":      "https",
				"organization":       "infra",
				"port":               8086,
				"server":             "influx.example.com",
				"type":               "influxdb",
				"verify-certificate": 1,
			})
		case http.MethodPut:
			assertFormValues(t, r, url.Values{
				"delete":  {"bucket,organization,token"},
				"digest":  {"abc123"},
				"disable": {"1"},
				"port":    {"8086"},
				"server":  {"influx.example.com"},
			})
			writeEnvelope(t, w, nil)
		case http.MethodDelete:
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
	createReq := ClusterMetricsServerRequest{
		Bucket:            stringPtr("pve"),
		InfluxDBProtocol:  stringPtr("https"),
		Organization:      stringPtr("infra"),
		Port:              8086,
		Server:            "influx.example.com",
		Token:             stringPtr("secret"),
		Type:              "influxdb",
		VerifyCertificate: boolPtr(true),
	}
	if err := client.CreateClusterMetricsServer(ctx, "influx-main", createReq); err != nil {
		t.Fatalf("CreateClusterMetricsServer() unexpected error: %v", err)
	}
	metricsServer, err := client.GetClusterMetricsServer(ctx, "influx-main")
	if err != nil {
		t.Fatalf("GetClusterMetricsServer() unexpected error: %v", err)
	}
	if metricsServer.Digest != "abc123" || metricsServer.Token != "" {
		t.Fatalf("unexpected metrics server: %#v", metricsServer)
	}
	if err := client.UpdateClusterMetricsServer(ctx, "influx-main", ClusterMetricsServerRequest{
		Digest:  stringPtr(metricsServer.Digest),
		Disable: boolPtr(true),
		Port:    8086,
		Server:  "influx.example.com",
		Delete:  []string{"token", "organization", "bucket"},
	}); err != nil {
		t.Fatalf("UpdateClusterMetricsServer() unexpected error: %v", err)
	}
	if err := client.DeleteClusterMetricsServer(ctx, "influx-main"); err != nil {
		t.Fatalf("DeleteClusterMetricsServer() unexpected error: %v", err)
	}
}
