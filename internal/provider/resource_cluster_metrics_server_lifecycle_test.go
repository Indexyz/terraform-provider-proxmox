// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestClusterMetricsServerResourceInfluxLifecycle(t *testing.T) {
	exists := true
	getCount := 0
	serverState := map[string]any{"id": "influx main", "type": "influxdb", "server": "influx.example.test", "port": 8086, "disable": 0, "mtu": 1400, "influxdbproto": "https", "organization": "acme", "bucket": "metrics", "verify-certificate": 1, "max-body-size": 4096, "api-path-prefix": "/proxy"}
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.RawQuery != "" {
			handler.fail(w, "unexpected metrics query: %s", r.URL.RawQuery)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/cluster/metrics/server/influx%20main":
			want := url.Values{"id": {"influx main"}, "type": {"influxdb"}, "server": {"influx.example.test"}, "port": {"8086"}, "disable": {"0"}, "mtu": {"1400"}, "influxdbproto": {"https"}, "organization": {"acme"}, "bucket": {"metrics"}, "token": {"influx-secret"}, "verify-certificate": {"1"}, "max-body-size": {"4096"}, "api-path-prefix": {"/proxy"}}
			if !handler.form(w, r, want) {
				return
			}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/cluster/metrics/server/influx%20main":
			if !exists {
				http.Error(w, "missing metrics server", http.StatusNotFound)
				return
			}
			getCount++
			response := make(map[string]any, len(serverState)+1)
			for key, value := range serverState {
				response[key] = value
			}
			response["digest"] = "metrics-digest-" + strconv.Itoa(getCount)
			handler.envelope(w, response)
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/cluster/metrics/server/influx%20main":
			want := url.Values{"server": {"influx-2.example.test"}, "port": {"8087"}, "disable": {"1"}, "mtu": {"1400"}, "influxdbproto": {"https"}, "bucket": {"metrics"}, "token": {"influx-secret"}, "verify-certificate": {"1"}, "delete": {"api-path-prefix,max-body-size,organization"}, "digest": {"metrics-digest-2"}}
			if !handler.form(w, r, want) {
				return
			}
			serverState = map[string]any{"id": "influx main", "type": "influxdb", "server": "influx-2.example.test", "port": 8087, "disable": 1, "mtu": 1400, "influxdbproto": "https", "bucket": "metrics", "verify-certificate": 1}
			handler.envelope(w, nil)
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/cluster/metrics/server/influx%20main":
			if !handler.form(w, r, url.Values{}) {
				return
			}
			if !exists {
				http.Error(w, "missing metrics server", http.StatusNotFound)
				return
			}
			exists = false
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected metrics request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &ClusterMetricsServerResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := clusterMetricsServerModel{ServerID: types.StringValue("influx main"), Type: types.StringValue("influxdb"), Server: types.StringValue("influx.example.test"), Port: types.Int64Value(8086), Disable: types.BoolValue(false), MTU: types.Int64Value(1400), InfluxDBProtocol: types.StringValue("https"), Organization: types.StringValue("acme"), Bucket: types.StringValue("metrics"), Token: types.StringValue("influx-secret"), VerifyCertificate: types.BoolValue(true), MaxBodySize: types.Int64Value(4096), APIPathPrefix: types.StringValue("/proxy")}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial), Config: testResourceConfig(t, schema, initial)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("metrics create diagnostics: %v", createResp.Diagnostics)
	}
	var created clusterMetricsServerModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode metrics create state: %v", diags)
	}
	if created.Token.ValueString() != "influx-secret" || created.InfluxDBProtocol.ValueString() != "https" || created.Port.ValueInt64() != 8086 || created.Disable.ValueBool() {
		t.Fatalf("unexpected typed InfluxDB create state: %#v", created)
	}

	updated := clusterMetricsServerModel{ServerID: types.StringValue("influx main"), Type: types.StringValue("influxdb"), Server: types.StringValue("influx-2.example.test"), Port: types.Int64Value(8087), Disable: types.BoolValue(true), MTU: types.Int64Value(1400), InfluxDBProtocol: types.StringValue("https"), Bucket: types.StringValue("metrics"), Token: types.StringValue("influx-secret"), VerifyCertificate: types.BoolValue(true)}
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}, Private: createResp.Private}
	res.Update(context.Background(), resource.UpdateRequest{Config: testResourceConfig(t, schema, updated), State: createResp.State, Private: createResp.Private}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("metrics update diagnostics: %v", updateResp.Diagnostics)
	}
	var updatedState clusterMetricsServerModel
	if diags := updateResp.State.Get(context.Background(), &updatedState); diags.HasError() {
		t.Fatalf("decode metrics update state: %v", diags)
	}
	if updatedState.Token.ValueString() != "influx-secret" || updatedState.Server.ValueString() != "influx-2.example.test" || !updatedState.Organization.IsNull() || !updatedState.MaxBodySize.IsNull() || !updatedState.APIPathPrefix.IsNull() {
		t.Fatalf("unexpected typed InfluxDB update state: %#v", updatedState)
	}

	readResp := resource.ReadResponse{State: updateResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: updateResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("metrics read diagnostics: %v", readResp.Diagnostics)
	}
	assertStateString(t, readResp.State, path.Root("token"), "influx-secret")
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("metrics delete diagnostics: %v", deleteResp.Diagnostics)
	}
	missingResp := resource.ReadResponse{State: readResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: readResp.State}, &missingResp)
	if missingResp.Diagnostics.HasError() || !missingResp.State.Raw.IsNull() {
		t.Fatalf("missing metrics server was not removed: diagnostics=%v raw=%v", missingResp.Diagnostics, missingResp.State.Raw)
	}
	var idempotentDelete resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &idempotentDelete)
	if idempotentDelete.Diagnostics.HasError() {
		t.Fatalf("idempotent metrics delete diagnostics: %v", idempotentDelete.Diagnostics)
	}
	handler.assert(t)
	wantCalls := []string{"POST /api2/json/cluster/metrics/server/influx%20main", "GET /api2/json/cluster/metrics/server/influx%20main", "GET /api2/json/cluster/metrics/server/influx%20main", "PUT /api2/json/cluster/metrics/server/influx%20main", "GET /api2/json/cluster/metrics/server/influx%20main", "GET /api2/json/cluster/metrics/server/influx%20main", "DELETE /api2/json/cluster/metrics/server/influx%20main", "GET /api2/json/cluster/metrics/server/influx%20main", "DELETE /api2/json/cluster/metrics/server/influx%20main"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected metrics call order: got %v want %v", calls, wantCalls)
	}

	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, clusterMetricsServerModel{ID: types.StringNull(), ServerID: types.StringNull()})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "influx main"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("metrics import diagnostics: %v", importResp.Diagnostics)
	}
	assertStateString(t, importResp.State, path.Root("server_id"), "influx main")
}

func TestClusterMetricsServerResourceGraphiteAndOpenTelemetryCreateRead(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		model      clusterMetricsServerModel
		wantForm   url.Values
		api        map[string]any
		assertType func(*testing.T, clusterMetricsServerModel)
	}{
		{name: "graphite", id: "graphite", model: clusterMetricsServerModel{ServerID: types.StringValue("graphite"), Type: types.StringValue("graphite"), Server: types.StringValue("graphite.example.test"), Port: types.Int64Value(2003), GraphitePath: types.StringValue("pve.cluster"), GraphiteProtocol: types.StringValue("tcp"), GraphiteTimeout: types.Int64Value(3), MTU: types.Int64Value(1450)}, wantForm: url.Values{"id": {"graphite"}, "type": {"graphite"}, "server": {"graphite.example.test"}, "port": {"2003"}, "path": {"pve.cluster"}, "proto": {"tcp"}, "timeout": {"3"}, "mtu": {"1450"}}, api: map[string]any{"id": "graphite", "type": "graphite", "server": "graphite.example.test", "port": 2003, "path": "pve.cluster", "proto": "tcp", "timeout": 3, "mtu": 1450}, assertType: func(t *testing.T, state clusterMetricsServerModel) {
			t.Helper()
			if state.GraphitePath.ValueString() != "pve.cluster" || state.GraphiteProtocol.ValueString() != "tcp" || state.GraphiteTimeout.ValueInt64() != 3 {
				t.Fatalf("unexpected Graphite state: %#v", state)
			}
		}},
		{name: "opentelemetry", id: "otel", model: clusterMetricsServerModel{ServerID: types.StringValue("otel"), Type: types.StringValue("opentelemetry"), Server: types.StringValue("otel.example.test"), Port: types.Int64Value(4318), OpenTelemetryCompress: types.StringValue("gzip"), OpenTelemetryHeaders: types.StringValue("eyJBdXRob3JpemF0aW9uIjoiQmVhcmVyIn0="), OpenTelemetryMaxBody: types.Int64Value(8192), OpenTelemetryPath: types.StringValue("/v1/metrics"), OpenTelemetryProtocol: types.StringValue("https"), OpenTelemetryResource: types.StringValue("eyJzZXJ2aWNlLm5hbWUiOiJwdmUifQ=="), OpenTelemetryTimeout: types.Int64Value(5), OpenTelemetryVerify: types.BoolValue(true)}, wantForm: url.Values{"id": {"otel"}, "type": {"opentelemetry"}, "server": {"otel.example.test"}, "port": {"4318"}, "otel-compression": {"gzip"}, "otel-headers": {"eyJBdXRob3JpemF0aW9uIjoiQmVhcmVyIn0="}, "otel-max-body-size": {"8192"}, "otel-path": {"/v1/metrics"}, "otel-protocol": {"https"}, "otel-resource-attributes": {"eyJzZXJ2aWNlLm5hbWUiOiJwdmUifQ=="}, "otel-timeout": {"5"}, "otel-verify-ssl": {"1"}}, api: map[string]any{"id": "otel", "type": "opentelemetry", "server": "otel.example.test", "port": 4318, "otel-compression": "gzip", "otel-headers": "eyJBdXRob3JpemF0aW9uIjoiQmVhcmVyIn0=", "otel-max-body-size": 8192, "otel-path": "/v1/metrics", "otel-protocol": "https", "otel-resource-attributes": "eyJzZXJ2aWNlLm5hbWUiOiJwdmUifQ==", "otel-timeout": 5, "otel-verify-ssl": 1}, assertType: func(t *testing.T, state clusterMetricsServerModel) {
			t.Helper()
			if state.OpenTelemetryCompress.ValueString() != "gzip" ||
				state.OpenTelemetryHeaders.ValueString() != "eyJBdXRob3JpemF0aW9uIjoiQmVhcmVyIn0=" ||
				state.OpenTelemetryMaxBody.ValueInt64() != 8192 ||
				state.OpenTelemetryPath.ValueString() != "/v1/metrics" ||
				state.OpenTelemetryProtocol.ValueString() != "https" ||
				state.OpenTelemetryResource.ValueString() != "eyJzZXJ2aWNlLm5hbWUiOiJwdmUifQ==" ||
				state.OpenTelemetryTimeout.ValueInt64() != 5 ||
				!state.OpenTelemetryVerify.ValueBool() {
				t.Fatalf("unexpected OpenTelemetry state: %#v", state)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := &lifecycleHandler{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !handler.auth(w, r) {
					return
				}
				if r.URL.RawQuery != "" {
					handler.fail(w, "unexpected type-specific metrics query: %s", r.URL.RawQuery)
					return
				}
				wantPath := "/api2/json/cluster/metrics/server/" + test.id
				switch r.Method {
				case http.MethodPost:
					if r.URL.EscapedPath() != wantPath || !handler.form(w, r, test.wantForm) {
						return
					}
					handler.envelope(w, nil)
				case http.MethodGet:
					if r.URL.EscapedPath() != wantPath {
						handler.fail(w, "unexpected type-specific metrics path: %s", r.URL.EscapedPath())
						return
					}
					handler.envelope(w, test.api)
				case http.MethodDelete:
					if r.URL.EscapedPath() != wantPath || !handler.form(w, r, url.Values{}) {
						return
					}
					handler.envelope(w, nil)
				default:
					handler.fail(w, "unexpected type-specific metrics request: %s", r.Method)
				}
			}))
			defer server.Close()
			res := &ClusterMetricsServerResource{client: testLifecycleClient(t, server)}
			schema := testResourceSchema(t, res)
			resp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
			initializeResourcePrivate(t, &resp)
			res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, test.model), Config: testResourceConfig(t, schema, test.model)}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("%s create diagnostics: %v", test.name, resp.Diagnostics)
			}
			var state clusterMetricsServerModel
			if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
				t.Fatalf("decode %s state: %v", test.name, diags)
			}
			test.assertType(t, state)
			var deleteResp resource.DeleteResponse
			res.Delete(context.Background(), resource.DeleteRequest{State: resp.State}, &deleteResp)
			if deleteResp.Diagnostics.HasError() {
				t.Fatalf("%s delete diagnostics: %v", test.name, deleteResp.Diagnostics)
			}
			handler.assert(t)
		})
	}
}

func TestClusterMetricsServerResourceReadPreservesAPIError(t *testing.T) {
	handler := &lifecycleHandler{}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls++
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api2/json/cluster/metrics/server/metrics%20error" || r.URL.RawQuery != "" {
			handler.fail(w, "unexpected metrics error request: %s %s", r.Method, r.URL.String())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":{"permission":"missing Sys.Modify for metrics server"}}`))
	}))
	defer server.Close()
	res := &ClusterMetricsServerResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := testResourceState(t, schema, clusterMetricsServerModel{ID: types.StringValue("metrics error"), ServerID: types.StringValue("metrics error"), Type: types.StringValue("influxdb")})
	resp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "status 403") || !containsDiagnostic(resp.Diagnostics, "missing Sys.Modify for metrics server") {
		t.Fatalf("expected preserved metrics API detail, got %v", resp.Diagnostics)
	}
	if calls != 1 || !resp.State.Raw.Equal(state.Raw) {
		t.Fatalf("metrics API error caused unexpected calls or state mutation: calls=%d raw=%v", calls, resp.State.Raw)
	}
	handler.assert(t)
}

func TestClusterMetricsServerUpdateRejectsMalformedPrivateState(t *testing.T) {
	handler := &lifecycleHandler{}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		handler.fail(w, "malformed private state must stop before HTTP: %s %s", r.Method, r.URL.String())
	}))
	defer server.Close()
	res := &ClusterMetricsServerResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := clusterMetricsServerModel{ID: types.StringValue("influx"), ServerID: types.StringValue("influx"), Type: types.StringValue("influxdb"), Server: types.StringValue("influx.example.test"), Port: types.Int64Value(8086)}
	resp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &resp)
	if diags := resp.Private.SetKey(context.Background(), clusterMetricsManagedFieldsKey, []byte("{}")); diags.HasError() {
		t.Fatalf("set malformed metrics private state: %v", diags)
	}
	res.Update(context.Background(), resource.UpdateRequest{Config: testResourceConfig(t, schema, state), State: testResourceState(t, schema, state), Private: resp.Private}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "unable to decode managed fields") || !containsDiagnostic(resp.Diagnostics, "cannot unmarshal object") {
		t.Fatalf("expected malformed metrics private-state diagnostic, got %v", resp.Diagnostics)
	}
	if calls != 0 || !resp.State.Raw.IsNull() {
		t.Fatalf("malformed metrics private state caused HTTP or state mutation: calls=%d raw=%v", calls, resp.State.Raw)
	}
	handler.assert(t)
}
