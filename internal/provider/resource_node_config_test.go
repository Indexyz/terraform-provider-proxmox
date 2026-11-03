// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestNodeConfigClientRequests(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/time" && r.Method == http.MethodPut:
			if !handler.form(w, r, url.Values{"timezone": {"Europe/Berlin"}}) {
				return
			}
			handler.envelope(w, nil)
		case r.URL.Path == "/api2/json/nodes/pve-1/time" && r.Method == http.MethodGet:
			handler.envelope(w, map[string]any{"timezone": "Europe/Berlin", "time": 100, "localtime": 200})
		case r.URL.Path == "/api2/json/nodes/pve-1/dns" && r.Method == http.MethodPut:
			if !handler.form(w, r, url.Values{"dns1": {"1.1.1.1"}, "dns2": {"9.9.9.9"}, "search": {"example.com"}}) {
				return
			}
			handler.envelope(w, nil)
		case r.URL.Path == "/api2/json/nodes/pve-1/dns" && r.Method == http.MethodGet:
			handler.envelope(w, map[string]any{"dns1": "1.1.1.1", "dns2": "9.9.9.9", "search": "example.com"})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)
	if err := client.SetNodeTimezone(context.Background(), "pve-1", "Europe/Berlin"); err != nil {
		t.Fatalf("SetNodeTimezone() unexpected error: %v", err)
	}
	timeInfo, err := client.NodeTime(context.Background(), "pve-1")
	if err != nil || timeInfo.Timezone != "Europe/Berlin" || timeInfo.LocalTime != 200 {
		t.Fatalf("unexpected time info: %#v (%v)", timeInfo, err)
	}
	if err := client.SetNodeDNS(context.Background(), "pve-1", NodeDNS{DNS1: "1.1.1.1", DNS2: "9.9.9.9", Search: "example.com"}); err != nil {
		t.Fatalf("SetNodeDNS() unexpected error: %v", err)
	}
	if dns, err := client.NodeDNS(context.Background(), "pve-1"); err != nil || dns.DNS1 != "1.1.1.1" {
		t.Fatalf("unexpected dns: %#v (%v)", dns, err)
	}
	handler.assert(t)
}

func TestNodeTimeLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			handler.envelope(w, nil)
		default:
			handler.envelope(w, map[string]any{"timezone": "Europe/Berlin", "time": 100, "localtime": 200})
		}
	}))
	defer server.Close()
	res := &NodeTimeResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := nodeTimeModel{Node: types.StringValue("pve-1"), Timezone: types.StringValue("Europe/Berlin")}
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("time diagnostics: %v", resp.Diagnostics)
	}
	var state nodeTimeModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.ID.ValueString() != "pve-1" || state.Time.ValueInt64() != 100 || state.Localtime.ValueInt64() != 200 {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestNodeDNSLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			handler.envelope(w, nil)
			return
		}
		handler.envelope(w, map[string]any{"dns1": "1.1.1.1"})
	}))
	defer server.Close()
	res := &NodeDNSResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := nodeDNSModel{Node: types.StringValue("pve-1"), DNS1: types.StringValue("1.1.1.1")}
	resp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("dns diagnostics: %v", resp.Diagnostics)
	}
	var state nodeDNSModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if state.DNS1.ValueString() != "1.1.1.1" || !state.DNS2.IsNull() {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestNodeConfigImportState(t *testing.T) {
	for _, resourceUnderTest := range []resource.Resource{&NodeTimeResource{}, &NodeDNSResource{}} {
		importable, ok := resourceUnderTest.(resource.ResourceWithImportState)
		if !ok {
			t.Fatalf("%T does not implement import", resourceUnderTest)
		}
		schema := testResourceSchema(t, resourceUnderTest)
		tfType := schema.Schema.Type().TerraformType(context.Background())
		resp := resource.ImportStateResponse{State: tfsdk.State{Schema: schema.Schema, Raw: tftypes.NewValue(tfType, nil)}}
		importable.ImportState(context.Background(), resource.ImportStateRequest{ID: "pve-1"}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("%T import diagnostics: %v", resourceUnderTest, resp.Diagnostics)
		}
	}
}

func TestNodeSyslogDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/syslog?limit=100": []map[string]any{{"n": 2, "t": "second"}, {"n": 1, "t": "first"}},
	}
	readResp := runSDNDataSource(t, NewNodeSyslogDataSource(), responses, map[string]any{"node": "pve-1"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state nodeSyslogModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() || len(state.Entries) != 2 || state.Entries[0].Text.ValueString() != "second" {
		t.Fatalf("unexpected projection: %#v (%v)", state.Entries, diags)
	}
}

func TestNodeJournalDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/journal?lastentries=2": []string{"a", "b"},
	}
	readResp := runSDNDataSource(t, NewNodeJournalDataSource(), responses, map[string]any{"node": "pve-1", "limit": int64(2)})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state nodeJournalModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() || len(state.Lines.Elements()) != 2 {
		t.Fatalf("unexpected state: %#v (%v)", state, diags)
	}
}
