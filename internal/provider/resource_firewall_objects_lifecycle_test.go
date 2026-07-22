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

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type firewallObjectLifecycleCase struct {
	name              string
	resource          resource.Resource
	initial           any
	updated           any
	collectionPath    string
	getPath           string
	createPath        string
	updatePath        string
	deletePath        string
	createMethod      string
	updateMethod      string
	createForm        url.Values
	updateForm        url.Values
	deleteForm        url.Values
	initialData       any
	updatedData       any
	missingData       any
	importID          string
	importAttribute   path.Path
	importWant        string
	expectedCalls     []string
	updatedAssertions func(*testing.T, tfsdk.State)
}

func TestClusterFirewallObjectResourceLifecycles(t *testing.T) {
	cases := []firewallObjectLifecycleCase{
		{
			name: "alias", resource: &ClusterFirewallAliasResource{},
			initial:        clusterFirewallAliasModel{Name: types.StringValue("office-net"), CIDR: types.StringValue("10.0.0.0/24"), Comment: types.StringValue("managed")},
			updated:        clusterFirewallAliasModel{Name: types.StringValue("office-net"), CIDR: types.StringValue("10.1.0.0/24"), Comment: types.StringNull()},
			collectionPath: "/api2/json/cluster/firewall/aliases", getPath: "/api2/json/cluster/firewall/aliases/office-net", createPath: "/api2/json/cluster/firewall/aliases", updatePath: "/api2/json/cluster/firewall/aliases/office-net", deletePath: "/api2/json/cluster/firewall/aliases/office-net",
			createMethod: http.MethodPost, updateMethod: http.MethodPut,
			createForm: url.Values{"name": {"office-net"}, "cidr": {"10.0.0.0/24"}, "comment": {"managed"}},
			updateForm: url.Values{"cidr": {"10.1.0.0/24"}, "comment": {""}, "digest": {"digest-1"}}, deleteForm: url.Values{"digest": {"digest-1"}},
			initialData: map[string]any{"name": "office-net", "cidr": "10.0.0.0/24", "comment": "managed", "digest": "digest-1"},
			updatedData: map[string]any{"name": "office-net", "cidr": "10.1.0.0/24", "comment": "", "digest": "digest-1"},
			importID:    "office-net", importAttribute: path.Root("name"), importWant: "office-net",
			expectedCalls:     []string{"POST /api2/json/cluster/firewall/aliases", "GET /api2/json/cluster/firewall/aliases/office-net", "GET /api2/json/cluster/firewall/aliases/office-net", "GET /api2/json/cluster/firewall/aliases/office-net", "PUT /api2/json/cluster/firewall/aliases/office-net", "GET /api2/json/cluster/firewall/aliases/office-net", "GET /api2/json/cluster/firewall/aliases/office-net", "DELETE /api2/json/cluster/firewall/aliases/office-net", "GET /api2/json/cluster/firewall/aliases/office-net"},
			updatedAssertions: func(t *testing.T, state tfsdk.State) { assertStateString(t, state, path.Root("cidr"), "10.1.0.0/24") },
		},
		{
			name: "ip_set", resource: &ClusterFirewallIPSetResource{},
			initial:        clusterFirewallIPSetModel{Name: types.StringValue("trusted-set"), Comment: types.StringValue("managed")},
			updated:        clusterFirewallIPSetModel{Name: types.StringValue("trusted-set"), Comment: types.StringNull()},
			collectionPath: "/api2/json/cluster/firewall/ipset", getPath: "/api2/json/cluster/firewall/ipset", createPath: "/api2/json/cluster/firewall/ipset", updatePath: "/api2/json/cluster/firewall/ipset", deletePath: "/api2/json/cluster/firewall/ipset/trusted-set",
			createMethod: http.MethodPost, updateMethod: http.MethodPost,
			createForm: url.Values{"name": {"trusted-set"}, "comment": {"managed"}},
			updateForm: url.Values{"name": {"trusted-set"}, "rename": {"trusted-set"}, "comment": {""}, "digest": {"digest-1"}}, deleteForm: url.Values{},
			initialData: []map[string]any{{"name": "trusted-set", "comment": "managed", "digest": "digest-1"}},
			updatedData: []map[string]any{{"name": "trusted-set", "comment": "", "digest": "digest-1"}}, missingData: []any{},
			importID: "trusted-set", importAttribute: path.Root("name"), importWant: "trusted-set",
			expectedCalls: []string{"POST /api2/json/cluster/firewall/ipset", "GET /api2/json/cluster/firewall/ipset", "GET /api2/json/cluster/firewall/ipset", "GET /api2/json/cluster/firewall/ipset", "POST /api2/json/cluster/firewall/ipset", "GET /api2/json/cluster/firewall/ipset", "DELETE /api2/json/cluster/firewall/ipset/trusted-set", "GET /api2/json/cluster/firewall/ipset"},
		},
		{
			name: "ip_set_entry", resource: &ClusterFirewallIPSetEntryResource{},
			initial:        clusterFirewallIPSetEntryModel{IPSet: types.StringValue("trusted-set"), CIDR: types.StringValue("10.0.0.0/24"), Comment: types.StringValue("managed"), NoMatch: types.BoolValue(true)},
			updated:        clusterFirewallIPSetEntryModel{IPSet: types.StringValue("trusted-set"), CIDR: types.StringValue("10.0.0.0/24"), Comment: types.StringNull(), NoMatch: types.BoolValue(false)},
			collectionPath: "/api2/json/cluster/firewall/ipset/trusted-set", getPath: "/api2/json/cluster/firewall/ipset/trusted-set/10.0.0.0%2F24", createPath: "/api2/json/cluster/firewall/ipset/trusted-set", updatePath: "/api2/json/cluster/firewall/ipset/trusted-set/10.0.0.0%2F24", deletePath: "/api2/json/cluster/firewall/ipset/trusted-set/10.0.0.0%2F24",
			createMethod: http.MethodPost, updateMethod: http.MethodPut,
			createForm: url.Values{"cidr": {"10.0.0.0/24"}, "comment": {"managed"}, "nomatch": {"1"}},
			updateForm: url.Values{"comment": {""}, "digest": {"digest-1"}, "nomatch": {"0"}}, deleteForm: url.Values{"digest": {"digest-1"}},
			initialData: map[string]any{"cidr": "10.0.0.0/24", "comment": "managed", "nomatch": 1, "digest": "digest-1"},
			updatedData: map[string]any{"cidr": "10.0.0.0/24", "comment": "", "nomatch": 0, "digest": "digest-1"},
			importID:    "trusted-set/10.0.0.0/24", importAttribute: path.Root("cidr"), importWant: "10.0.0.0/24",
			expectedCalls: []string{"POST /api2/json/cluster/firewall/ipset/trusted-set", "GET /api2/json/cluster/firewall/ipset/trusted-set/10.0.0.0%2F24", "GET /api2/json/cluster/firewall/ipset/trusted-set/10.0.0.0%2F24", "GET /api2/json/cluster/firewall/ipset/trusted-set/10.0.0.0%2F24", "PUT /api2/json/cluster/firewall/ipset/trusted-set/10.0.0.0%2F24", "GET /api2/json/cluster/firewall/ipset/trusted-set/10.0.0.0%2F24", "GET /api2/json/cluster/firewall/ipset/trusted-set/10.0.0.0%2F24", "DELETE /api2/json/cluster/firewall/ipset/trusted-set/10.0.0.0%2F24", "GET /api2/json/cluster/firewall/ipset/trusted-set/10.0.0.0%2F24"},
			updatedAssertions: func(t *testing.T, state tfsdk.State) {
				var value types.Bool
				if diags := state.GetAttribute(context.Background(), path.Root("nomatch"), &value); diags.HasError() || value.ValueBool() {
					t.Fatalf("expected explicit false nomatch, got %v diagnostics=%v", value, diags)
				}
			},
		},
		{
			name: "security_group", resource: &ClusterFirewallSecurityGroupResource{},
			initial:        clusterFirewallSecurityGroupModel{Name: types.StringValue("web-group"), Comment: types.StringValue("managed")},
			updated:        clusterFirewallSecurityGroupModel{Name: types.StringValue("web-group"), Comment: types.StringNull()},
			collectionPath: "/api2/json/cluster/firewall/groups", getPath: "/api2/json/cluster/firewall/groups", createPath: "/api2/json/cluster/firewall/groups", updatePath: "/api2/json/cluster/firewall/groups", deletePath: "/api2/json/cluster/firewall/groups/web-group",
			createMethod: http.MethodPost, updateMethod: http.MethodPost,
			createForm: url.Values{"group": {"web-group"}, "comment": {"managed"}},
			updateForm: url.Values{"group": {"web-group"}, "rename": {"web-group"}, "comment": {""}, "digest": {"digest-1"}}, deleteForm: url.Values{},
			initialData: []map[string]any{{"group": "web-group", "comment": "managed", "digest": "digest-1"}},
			updatedData: []map[string]any{{"group": "web-group", "comment": "", "digest": "digest-1"}}, missingData: []any{},
			importID: "web-group", importAttribute: path.Root("name"), importWant: "web-group",
			expectedCalls: []string{"POST /api2/json/cluster/firewall/groups", "GET /api2/json/cluster/firewall/groups", "GET /api2/json/cluster/firewall/groups", "GET /api2/json/cluster/firewall/groups", "POST /api2/json/cluster/firewall/groups", "GET /api2/json/cluster/firewall/groups", "DELETE /api2/json/cluster/firewall/groups/web-group", "GET /api2/json/cluster/firewall/groups"},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			handler := &lifecycleHandler{}
			exists := false
			updated := false
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !handler.auth(w, r) {
					return
				}
				calls = append(calls, r.Method+" "+r.URL.EscapedPath())
				switch {
				case r.Method == http.MethodGet && r.URL.EscapedPath() == test.getPath:
					if !exists && test.missingData == nil {
						writeAPIError(t, w, http.StatusNotFound, "missing firewall object")
						return
					}
					if !exists {
						handler.envelope(w, test.missingData)
						return
					}
					if updated {
						handler.envelope(w, test.updatedData)
					} else {
						handler.envelope(w, test.initialData)
					}
				case r.Method == test.createMethod && r.URL.EscapedPath() == test.createPath:
					if err := r.ParseForm(); err != nil {
						handler.fail(w, "parse object create form: %v", err)
						return
					}
					want := test.createForm
					if r.Form.Has("rename") {
						want = test.updateForm
						updated = true
					} else {
						exists = true
					}
					if !reflect.DeepEqual(r.Form, want) {
						handler.fail(w, "unexpected object POST form: got %v want %v", r.Form, want)
						return
					}
					handler.envelope(w, nil)
				case r.Method == test.updateMethod && r.URL.EscapedPath() == test.updatePath:
					if !handler.form(w, r, test.updateForm) {
						return
					}
					updated = true
					handler.envelope(w, nil)
				case r.Method == http.MethodDelete && r.URL.EscapedPath() == test.deletePath:
					if !handler.form(w, r, test.deleteForm) {
						return
					}
					exists = false
					handler.envelope(w, nil)
				default:
					handler.fail(w, "unexpected object request: %s %s", r.Method, r.URL.EscapedPath())
				}
			}))
			defer server.Close()

			schema := testResourceSchema(t, test.resource)
			configurable, ok := test.resource.(resource.ResourceWithConfigure)
			if !ok {
				t.Fatalf("%T does not implement ResourceWithConfigure", test.resource)
			}
			var configureResp resource.ConfigureResponse
			configurable.Configure(context.Background(), resource.ConfigureRequest{ProviderData: testLifecycleClient(t, server)}, &configureResp)
			if configureResp.Diagnostics.HasError() {
				t.Fatalf("configure diagnostics: %v", configureResp.Diagnostics)
			}
			createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
			initializeResourcePrivate(t, &createResp)
			test.resource.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, test.initial), Config: testResourceConfig(t, schema, test.initial)}, &createResp)
			if createResp.Diagnostics.HasError() {
				t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
			}
			readResp := resource.ReadResponse{State: createResp.State}
			test.resource.Read(context.Background(), resource.ReadRequest{State: createResp.State}, &readResp)
			if readResp.Diagnostics.HasError() {
				t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
			}
			updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}, Private: createResp.Private}
			test.resource.Update(context.Background(), resource.UpdateRequest{Config: testResourceConfig(t, schema, test.updated), State: readResp.State, Private: createResp.Private}, &updateResp)
			if updateResp.Diagnostics.HasError() {
				t.Fatalf("update diagnostics: %v", updateResp.Diagnostics)
			}
			var comment types.String
			if diags := updateResp.State.GetAttribute(context.Background(), path.Root("comment"), &comment); diags.HasError() || !comment.IsNull() {
				t.Fatalf("managed comment was not cleared: %v diagnostics=%v", comment, diags)
			}
			if test.updatedAssertions != nil {
				test.updatedAssertions(t, updateResp.State)
			}
			var deleteResp resource.DeleteResponse
			test.resource.Delete(context.Background(), resource.DeleteRequest{State: updateResp.State}, &deleteResp)
			if deleteResp.Diagnostics.HasError() {
				t.Fatalf("delete diagnostics: %v", deleteResp.Diagnostics)
			}
			missingResp := resource.ReadResponse{State: updateResp.State}
			test.resource.Read(context.Background(), resource.ReadRequest{State: updateResp.State}, &missingResp)
			if missingResp.Diagnostics.HasError() || !missingResp.State.Raw.IsNull() {
				t.Fatalf("missing object not removed: diagnostics=%v raw=%v", missingResp.Diagnostics, missingResp.State.Raw)
			}
			handler.assert(t)
			if !reflect.DeepEqual(calls, test.expectedCalls) {
				t.Fatalf("unexpected calls: got %v want %v", calls, test.expectedCalls)
			}

			importer, ok := test.resource.(resource.ResourceWithImportState)
			if !ok {
				t.Fatalf("%T does not implement ResourceWithImportState", test.resource)
			}
			importResp := resource.ImportStateResponse{State: testResourceState(t, schema, test.initial)}
			importer.ImportState(context.Background(), resource.ImportStateRequest{ID: test.importID}, &importResp)
			if importResp.Diagnostics.HasError() {
				t.Fatalf("import diagnostics: %v", importResp.Diagnostics)
			}
			assertStateString(t, importResp.State, test.importAttribute, test.importWant)
		})
	}
}

func TestClusterFirewallAliasReadPreservesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeAPIError(t, w, http.StatusForbidden, "missing Sys.Modify")
	}))
	defer server.Close()
	res := &ClusterFirewallAliasResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := testResourceState(t, schema, clusterFirewallAliasModel{ID: types.StringValue("office"), Name: types.StringValue("office"), CIDR: types.StringValue("10.0.0.0/24")})
	resp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "status 403") || !containsDiagnostic(resp.Diagnostics, "missing Sys.Modify") {
		t.Fatalf("expected preserved API error, got %v", resp.Diagnostics)
	}
	if !resp.State.Raw.Equal(state.Raw) {
		t.Fatalf("firewall alias API error unexpectedly mutated state: %v", resp.State.Raw)
	}
}
