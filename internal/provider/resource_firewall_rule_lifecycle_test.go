// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestFirewallRuleResourceScopedLifecycles(t *testing.T) {
	cases := []struct {
		name   string
		model  firewallRuleModel
		path   string
		update bool
	}{
		{name: "cluster", model: firewallRuleModel{Scope: types.StringValue("cluster")}, path: "/api2/json/cluster/firewall/rules", update: true},
		{name: "node", model: firewallRuleModel{Scope: types.StringValue("node"), Node: types.StringValue("pve one")}, path: "/api2/json/nodes/pve%20one/firewall/rules", update: true},
		{name: "qemu guest", model: firewallRuleModel{Scope: types.StringValue("guest"), Node: types.StringValue("pve one"), GuestType: types.StringValue("qemu"), VMID: types.Int64Value(101)}, path: "/api2/json/nodes/pve%20one/qemu/101/firewall/rules", update: true},
		{name: "security group", model: firewallRuleModel{Scope: types.StringValue("security_group"), SecurityGroup: types.StringValue("web-group")}, path: "/api2/json/cluster/firewall/groups/web-group", update: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			handler := &lifecycleHandler{}
			rules := []map[string]any{}
			postCount := 0
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !handler.auth(w, r) {
					return
				}
				switch r.Method {
				case http.MethodGet, http.MethodPost:
					if r.URL.EscapedPath() != test.path {
						handler.fail(w, "unexpected rule collection path for %s: %s", r.Method, r.URL.EscapedPath())
						return
					}
				case http.MethodPut, http.MethodDelete:
					if r.URL.EscapedPath() != test.path+"/3" {
						handler.fail(w, "unexpected positioned rule path for %s: %s", r.Method, r.URL.EscapedPath())
						return
					}
				default:
					handler.fail(w, "unexpected rule method: %s", r.Method)
					return
				}
				if r.Method == http.MethodDelete {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						handler.fail(w, "read rule delete form: %v", err)
						return
					}
					r.Form, err = url.ParseQuery(string(body))
					if err != nil {
						handler.fail(w, "parse rule delete form: %v", err)
						return
					}
				} else if err := r.ParseForm(); err != nil {
					handler.fail(w, "parse rule form: %v", err)
					return
				}
				calls = append(calls, r.Method+" "+r.URL.EscapedPath()+" "+r.Form.Encode())
				switch r.Method {
				case http.MethodGet:
					handler.envelope(w, rules)
				case http.MethodPost:
					postCount++
					want := url.Values{"type": {"in"}, "action": {"ACCEPT"}, "enable": {"0"}, "comment": {"managed"}, "source": {"10.0.0.0/24"}, "proto": {"tcp"}, "dport": {"443"}, "log": {"nolog"}}
					if postCount > 1 {
						want = url.Values{"type": {"in"}, "action": {"ACCEPT"}, "enable": {"1"}, "source": {"10.0.0.0/24"}, "proto": {"tcp"}, "dport": {"443"}}
					}
					if !reflect.DeepEqual(r.Form, want) {
						handler.fail(w, "unexpected rule create/recreate form: got %v want %v", r.Form, want)
						return
					}
					rules = []map[string]any{{"pos": 3, "type": "in", "action": "ACCEPT", "enable": want.Get("enable"), "comment": want.Get("comment"), "source": "10.0.0.0/24", "proto": "tcp", "dport": "443", "log": want.Get("log"), "digest": "digest-1"}}
					handler.envelope(w, nil)
				case http.MethodPut:
					want := url.Values{"type": {"in"}, "action": {"ACCEPT"}, "enable": {"1"}, "source": {"10.0.0.0/24"}, "proto": {"tcp"}, "dport": {"443"}, "delete": {"comment,log"}, "digest": {"digest-1"}}
					if !reflect.DeepEqual(r.Form, want) {
						handler.fail(w, "unexpected rule update form: got %v want %v", r.Form, want)
						return
					}
					rules = []map[string]any{{"pos": 3, "type": "in", "action": "ACCEPT", "enable": 1, "source": "10.0.0.0/24", "proto": "tcp", "dport": "443", "digest": "digest-1"}}
					handler.envelope(w, nil)
				case http.MethodDelete:
					want := url.Values{"digest": {"digest-1"}}
					if !reflect.DeepEqual(r.Form, want) {
						handler.fail(w, "unexpected rule delete form: got %v want %v", r.Form, want)
						return
					}
					rules = nil
					handler.envelope(w, nil)
				default:
					handler.fail(w, "unexpected rule method: %s", r.Method)
				}
			}))
			defer server.Close()

			res := &FirewallRuleResource{client: testLifecycleClient(t, server)}
			schema := testResourceSchema(t, res)
			initial := test.model
			initial.Type = types.StringValue("in")
			initial.Action = types.StringValue("ACCEPT")
			initial.Enable = types.Int64Value(0)
			initial.Comment = types.StringValue("managed")
			initial.Source = types.StringValue("10.0.0.0/24")
			initial.Proto = types.StringValue("tcp")
			initial.DPort = types.StringValue("443")
			initial.Log = types.StringValue("nolog")
			createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
			initializeResourcePrivate(t, &createResp)
			res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial), Config: testResourceConfig(t, schema, initial)}, &createResp)
			if createResp.Diagnostics.HasError() {
				t.Fatalf("rule create diagnostics: %v", createResp.Diagnostics)
			}
			var created firewallRuleModel
			if diags := createResp.State.Get(context.Background(), &created); diags.HasError() || created.Enable.ValueInt64() != 0 || created.Pos.ValueInt64() != 3 {
				t.Fatalf("unexpected created rule state: %#v diagnostics=%v", created, diags)
			}
			readResp := resource.ReadResponse{State: createResp.State}
			res.Read(context.Background(), resource.ReadRequest{State: createResp.State}, &readResp)
			if readResp.Diagnostics.HasError() {
				t.Fatalf("rule read diagnostics: %v", readResp.Diagnostics)
			}

			finalState := readResp.State
			finalPrivate := createResp.Private
			var updated firewallRuleModel
			if test.update {
				updated = initial
				updated.Enable = types.Int64Value(1)
				updated.Comment = types.StringNull()
				updated.Log = types.StringNull()
				updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}, Private: createResp.Private}
				res.Update(context.Background(), resource.UpdateRequest{Config: testResourceConfig(t, schema, updated), State: readResp.State, Private: createResp.Private}, &updateResp)
				if updateResp.Diagnostics.HasError() {
					t.Fatalf("rule update diagnostics: %v", updateResp.Diagnostics)
				}
				var state firewallRuleModel
				if diags := updateResp.State.Get(context.Background(), &state); diags.HasError() || state.Enable.ValueInt64() != 1 || !state.Comment.IsNull() || state.Pos.ValueInt64() != 3 {
					t.Fatalf("unexpected updated rule state: %#v diagnostics=%v", state, diags)
				}
				finalState, finalPrivate = updateResp.State, updateResp.Private
			}
			var deleteResp resource.DeleteResponse
			res.Delete(context.Background(), resource.DeleteRequest{State: finalState}, &deleteResp)
			if deleteResp.Diagnostics.HasError() {
				t.Fatalf("rule delete diagnostics: %v", deleteResp.Diagnostics)
			}
			if test.update {
				recreateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}, Private: finalPrivate}
				res.Update(context.Background(), resource.UpdateRequest{Config: testResourceConfig(t, schema, updated), State: finalState, Private: finalPrivate}, &recreateResp)
				if recreateResp.Diagnostics.HasError() {
					t.Fatalf("missing rule recreation diagnostics: %v", recreateResp.Diagnostics)
				}
				assertStateString(t, recreateResp.State, path.Root("proto"), "tcp")
			}
			handler.assert(t)
			wantCalls := []string{
				"GET " + test.path + " ",
				"POST " + test.path + " action=ACCEPT&comment=managed&dport=443&enable=0&log=nolog&proto=tcp&source=10.0.0.0%2F24&type=in",
				"GET " + test.path + " ",
				"GET " + test.path + " ",
				"GET " + test.path + " ",
				"PUT " + test.path + "/3 action=ACCEPT&delete=comment%2Clog&digest=digest-1&dport=443&enable=1&proto=tcp&source=10.0.0.0%2F24&type=in",
				"GET " + test.path + " ",
				"GET " + test.path + " ",
				"DELETE " + test.path + "/3 digest=digest-1",
				"GET " + test.path + " ",
				"POST " + test.path + " action=ACCEPT&dport=443&enable=1&proto=tcp&source=10.0.0.0%2F24&type=in",
				"GET " + test.path + " ",
			}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("unexpected rule lifecycle call sequence:\n got %v\nwant %v", calls, wantCalls)
			}
		})
	}
}

func TestFirewallRuleResourceRejectsRemoteDuplicate(t *testing.T) {
	handler := &lifecycleHandler{}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		requests++
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api2/json/cluster/firewall/rules" {
			handler.fail(w, "unexpected duplicate check request: %s %s", r.Method, r.URL.EscapedPath())
			return
		}
		handler.envelope(w, []map[string]any{{"pos": 7, "type": "in", "action": "ACCEPT"}})
	}))
	defer server.Close()
	res := &FirewallRuleResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := firewallRuleModel{Scope: types.StringValue("cluster"), Type: types.StringValue("in"), Action: types.StringValue("ACCEPT")}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &resp)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model), Config: testResourceConfig(t, schema, model)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "already exists at pos 7") || requests != 1 {
		t.Fatalf("expected duplicate rejection without POST: requests=%d diagnostics=%v", requests, resp.Diagnostics)
	}
	handler.assert(t)
}
