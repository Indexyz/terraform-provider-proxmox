// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

func TestRealmsDataSourceMetadataAndSchema(t *testing.T) {
	t.Parallel()

	ds := NewRealmsDataSource()
	var meta datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "proxmox"}, &meta)
	if meta.TypeName != "proxmox_realms" {
		t.Fatalf("unexpected data source name: %q", meta.TypeName)
	}

	var schemaResp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
	for _, key := range []string{"id", "realms"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("expected attribute %q", key)
		}
	}
	record, ok := schemaResp.Schema.Attributes["realms"].(datasourceschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("realms must be a nested list, got %T", schemaResp.Schema.Attributes["realms"])
	}
	for _, key := range []string{"realm", "type", "comment", "default", "tfa"} {
		if _, ok := record.NestedObject.Attributes[key]; !ok {
			t.Fatalf("expected record attribute %q", key)
		}
	}
	for _, excluded := range []string{"password", "client_key", "bind_password", "digest", "certkey", "tfa_key"} {
		if _, ok := record.NestedObject.Attributes[excluded]; ok {
			t.Fatalf("index must not expose %q", excluded)
		}
	}
}

func TestRealmsDataSourceReadsAndSorts(t *testing.T) {
	t.Parallel()

	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api2/json/access/domains" {
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
			return
		}
		handler.envelope(w, []map[string]any{
			{"realm": "corp", "type": "openid", "comment": "Corporate SSO", "tfa": "yubico"},
			{"realm": "pam", "type": "pam"},
			{"realm": "corp-ad", "type": "ad", "default": 1},
		})
	}))
	defer server.Close()

	ds := NewRealmsDataSource()
	var schemaResp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
	configurable, ok := ds.(datasource.DataSourceWithConfigure)
	if !ok {
		t.Fatalf("%T does not implement DataSourceWithConfigure", ds)
	}
	var configureResp datasource.ConfigureResponse
	configurable.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: testLifecycleClient(t, server)}, &configureResp)
	if configureResp.Diagnostics.HasError() {
		t.Fatalf("configure diagnostics: %v", configureResp.Diagnostics)
	}

	readResp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	ds.Read(context.Background(), datasource.ReadRequest{Config: testDataSourceConfig(t, schemaResp, map[string]any{})}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}

	var state RealmsDataSourceModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("get state: %v", diags)
	}
	if state.ID.ValueString() != "realms" || len(state.Realms) != 3 {
		t.Fatalf("unexpected state: %#v", state)
	}
	got := make([]string, 0, len(state.Realms))
	for _, record := range state.Realms {
		got = append(got, fmt.Sprintf("%s|%s|%v|%v", record.Realm.ValueString(), record.Type.ValueString(), record.Comment.IsNull(), record.Default.IsNull()))
	}
	want := []string{"corp|openid|false|true", "corp-ad|ad|true|false", "pam|pam|true|true"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected projection:\n got %v\nwant %v", got, want)
	}
	if state.Realms[0].TFA.IsNull() || state.Realms[0].TFA.ValueString() != "yubico" {
		t.Fatalf("TFA type was dropped: %#v", state.Realms[0])
	}
	if state.Realms[1].Default.IsNull() || !state.Realms[1].Default.ValueBool() {
		t.Fatalf("default flag was not mapped: %#v", state.Realms[1])
	}
}

func TestRealmsDataSourceEmptyAndAPIError(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		handler := &lifecycleHandler{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !handler.auth(w, r) {
				return
			}
			handler.envelope(w, []any{})
		}))
		defer server.Close()

		ds := NewRealmsDataSource()
		var schemaResp datasource.SchemaResponse
		ds.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
		configurable, ok := ds.(datasource.DataSourceWithConfigure)
		if !ok {
			t.Fatalf("%T does not implement DataSourceWithConfigure", ds)
		}
		var configureResp datasource.ConfigureResponse
		configurable.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: testLifecycleClient(t, server)}, &configureResp)
		readResp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
		ds.Read(context.Background(), datasource.ReadRequest{Config: testDataSourceConfig(t, schemaResp, map[string]any{})}, &readResp)
		if readResp.Diagnostics.HasError() {
			t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
		}
		var state RealmsDataSourceModel
		if diags := readResp.State.Get(context.Background(), &state); diags.HasError() || len(state.Realms) != 0 {
			t.Fatalf("empty index must stay an empty list: %#v (%v)", state.Realms, diags)
		}
	})

	t.Run("api error", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"realm":"permission denied"},"data":null}`))
		}))
		defer server.Close()

		ds := NewRealmsDataSource()
		var schemaResp datasource.SchemaResponse
		ds.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
		configurable, ok := ds.(datasource.DataSourceWithConfigure)
		if !ok {
			t.Fatalf("%T does not implement DataSourceWithConfigure", ds)
		}
		var configureResp datasource.ConfigureResponse
		configurable.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: testLifecycleClient(t, server)}, &configureResp)
		readResp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
		ds.Read(context.Background(), datasource.ReadRequest{Config: testDataSourceConfig(t, schemaResp, map[string]any{})}, &readResp)
		if len(readResp.Diagnostics) == 0 || readResp.Diagnostics[0].Detail() == "" {
			t.Fatal("API error was not surfaced")
		}
		if !strings.Contains(readResp.Diagnostics[0].Detail(), "status 500") || !strings.Contains(readResp.Diagnostics[0].Detail(), "permission denied") {
			t.Fatalf("API error context was not preserved: %v", readResp.Diagnostics)
		}
	})
}
