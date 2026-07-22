// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
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

func testStringList(t *testing.T, values ...string) types.List {
	t.Helper()
	value, diags := types.ListValueFrom(context.Background(), types.StringType, values)
	if diags.HasError() {
		t.Fatalf("create string list: %v", diags)
	}
	return value
}

func assertStateString(t *testing.T, state tfsdk.State, attribute path.Path, want string) {
	t.Helper()
	var got types.String
	if diags := state.GetAttribute(context.Background(), attribute, &got); diags.HasError() {
		t.Fatalf("read state %s: %v", attribute, diags)
	}
	if got.ValueString() != want {
		t.Fatalf("unexpected state %s: got %q want %q", attribute, got.ValueString(), want)
	}
}

func stringListValues(t *testing.T, value types.List) []string {
	t.Helper()
	var got []string
	if diags := value.ElementsAs(context.Background(), &got, false); diags.HasError() {
		t.Fatalf("decode string list: %v", diags)
	}
	return got
}

func TestGroupResourceLifecycle(t *testing.T) {
	comment := "created"
	exists := true
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/access/groups":
			if !handler.form(w, r, url.Values{"groupid": {"devs"}, "comment": {"created"}}) {
				return
			}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/access/groups/devs":
			if !exists {
				writeAPIError(t, w, http.StatusNotFound, "missing group")
				return
			}
			handler.envelope(w, map[string]any{"comment": comment, "members": []string{"zoe@pve", "amy@pve"}})
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/access/groups/devs":
			if !handler.form(w, r, url.Values{"groupid": {"devs"}, "comment": {"updated"}}) {
				return
			}
			comment = "updated"
			handler.envelope(w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/access/groups/devs":
			exists = false
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &GroupResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	createdPlan := GroupResourceModel{GroupID: types.StringValue("devs"), Comment: types.StringValue("created"), Members: types.ListNull(types.StringType)}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, createdPlan)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	assertStateString(t, createResp.State, path.Root("members").AtListIndex(0), "amy@pve")

	readResp := resource.ReadResponse{State: createResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: createResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}

	updatedPlan := createdPlan
	updatedPlan.Comment = types.StringValue("updated")
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, updatedPlan), State: readResp.State}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", updateResp.Diagnostics)
	}
	assertStateString(t, updateResp.State, path.Root("comment"), "updated")

	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", deleteResp.Diagnostics)
	}
	handler.assert(t)
	if want := []string{"POST /api2/json/access/groups", "GET /api2/json/access/groups/devs", "GET /api2/json/access/groups/devs", "GET /api2/json/access/groups/devs", "PUT /api2/json/access/groups/devs", "GET /api2/json/access/groups/devs", "DELETE /api2/json/access/groups/devs"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected call order: got %v want %v", calls, want)
	}

	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, GroupResourceModel{ID: types.StringNull(), GroupID: types.StringNull(), Comment: types.StringNull(), Members: types.ListNull(types.StringType)})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "devs"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", importResp.Diagnostics)
	}
	assertStateString(t, importResp.State, path.Root("group_id"), "devs")
}

func TestRoleResourceLifecycle(t *testing.T) {
	privs := "Sys.Audit"
	exists := true
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		wantPath := "/api2/json/access/roles/UnitRole"
		if r.Method == http.MethodPost {
			wantPath = "/api2/json/access/roles"
		}
		if r.URL.EscapedPath() != wantPath {
			handler.fail(w, "unexpected role path: got %s want %s", r.URL.EscapedPath(), wantPath)
			return
		}
		if err := r.ParseForm(); err != nil {
			handler.fail(w, "parse role form: %v", err)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath()+" "+r.Form.Encode())
		switch r.Method {
		case http.MethodPost:
			if !reflect.DeepEqual(r.Form, url.Values{"roleid": {"UnitRole"}, "privs": {"Sys.Audit"}}) {
				handler.fail(w, "unexpected role create form: %v", r.Form)
				return
			}
			handler.envelope(w, nil)
		case http.MethodGet:
			if !exists {
				writeAPIError(t, w, http.StatusNotFound, "missing role")
				return
			}
			values := map[string]any{}
			for _, value := range splitProxmoxList(privs) {
				values[value] = 1
			}
			handler.envelope(w, values)
		case http.MethodPut:
			if !reflect.DeepEqual(r.Form, url.Values{"privs": {"Pool.Audit,Sys.Audit"}}) {
				handler.fail(w, "unexpected role update form: %v", r.Form)
				return
			}
			privs = "Pool.Audit,Sys.Audit"
			handler.envelope(w, nil)
		case http.MethodDelete:
			if len(r.Form) != 0 {
				handler.fail(w, "unexpected role delete form: %v", r.Form)
				return
			}
			exists = false
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected role method: %s", r.Method)
		}
	}))
	defer server.Close()
	res := &RoleResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := roleResourceModel{RoleID: types.StringValue("UnitRole"), Privs: types.StringValue("Sys.Audit")}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial)}, &createResp)
	updated := initial
	updated.Privs = types.StringValue("Pool.Audit,Sys.Audit")
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, updated), State: createResp.State}, &updateResp)
	if createResp.Diagnostics.HasError() || updateResp.Diagnostics.HasError() {
		t.Fatalf("role lifecycle diagnostics: create=%v update=%v", createResp.Diagnostics, updateResp.Diagnostics)
	}
	assertStateString(t, updateResp.State, path.Root("privs"), "Pool.Audit,Sys.Audit")
	readResp := resource.ReadResponse{State: updateResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: updateResp.State}, &readResp)
	assertStateString(t, readResp.State, path.Root("privs"), "Pool.Audit,Sys.Audit")
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &deleteResp)
	if readResp.Diagnostics.HasError() || deleteResp.Diagnostics.HasError() {
		t.Fatalf("role read/delete diagnostics: read=%v delete=%v", readResp.Diagnostics, deleteResp.Diagnostics)
	}
	handler.assert(t)
	wantCalls := []string{
		"POST /api2/json/access/roles privs=Sys.Audit&roleid=UnitRole",
		"GET /api2/json/access/roles/UnitRole ",
		"PUT /api2/json/access/roles/UnitRole privs=Pool.Audit%2CSys.Audit",
		"GET /api2/json/access/roles/UnitRole ",
		"GET /api2/json/access/roles/UnitRole ",
		"DELETE /api2/json/access/roles/UnitRole ",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected role calls: got %v want %v", calls, wantCalls)
	}
	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, roleResourceModel{ID: types.StringNull(), RoleID: types.StringNull(), Privs: types.StringNull()})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "UnitRole"}, &importResp)
	assertStateString(t, importResp.State, path.Root("role_id"), "UnitRole")
}

func TestUserResourceLifecyclePreservesPassword(t *testing.T) {
	comment := "created"
	exists := true
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		wantPath := "/api2/json/access/users/unit@pve"
		if r.Method == http.MethodPost {
			wantPath = "/api2/json/access/users"
		}
		if r.URL.EscapedPath() != wantPath {
			handler.fail(w, "unexpected user path: got %s want %s", r.URL.EscapedPath(), wantPath)
			return
		}
		if err := r.ParseForm(); err != nil {
			handler.fail(w, "parse user form: %v", err)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath()+" "+r.Form.Encode())
		switch r.Method {
		case http.MethodPost:
			want := url.Values{"userid": {"unit@pve"}, "comment": {"created"}, "email": {"unit@example.test"}, "enable": {"1"}, "expire": {"0"}, "groups": {"devs"}, "password": {"initial-secret"}}
			if !reflect.DeepEqual(r.Form, want) {
				handler.fail(w, "unexpected user create form: got %v want %v", r.Form, want)
				return
			}
			handler.envelope(w, nil)
		case http.MethodGet:
			if !exists {
				writeAPIError(t, w, http.StatusNotFound, "missing user")
				return
			}
			handler.envelope(w, map[string]any{"comment": comment, "email": "unit@example.test", "enable": 1, "expire": 0, "groups": []string{"devs"}})
		case http.MethodPut:
			want := url.Values{"comment": {"updated"}, "email": {"unit@example.test"}, "enable": {"1"}, "expire": {"0"}, "groups": {"devs"}}
			if !reflect.DeepEqual(r.Form, want) {
				handler.fail(w, "unexpected user update form: got %v want %v", r.Form, want)
				return
			}
			comment = "updated"
			handler.envelope(w, nil)
		case http.MethodDelete:
			if len(r.Form) != 0 {
				handler.fail(w, "unexpected user delete form: %v", r.Form)
				return
			}
			if !exists {
				writeAPIError(t, w, http.StatusNotFound, "missing user")
				return
			}
			exists = false
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected user method: %s", r.Method)
		}
	}))
	defer server.Close()
	res := &UserResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := userResourceModel{UserID: types.StringValue("unit@pve"), Comment: types.StringValue("created"), Email: types.StringValue("unit@example.test"), Enable: types.BoolValue(true), Expire: types.Int64Value(0), Groups: types.StringValue("devs"), Password: types.StringValue("initial-secret")}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial)}, &createResp)
	updated := initial
	updated.Comment = types.StringValue("updated")
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, updated), State: createResp.State}, &updateResp)
	if createResp.Diagnostics.HasError() || updateResp.Diagnostics.HasError() {
		t.Fatalf("user lifecycle diagnostics: create=%v update=%v", createResp.Diagnostics, updateResp.Diagnostics)
	}
	assertStateString(t, updateResp.State, path.Root("password"), "initial-secret")
	readResp := resource.ReadResponse{State: updateResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: updateResp.State}, &readResp)
	assertStateString(t, readResp.State, path.Root("comment"), "updated")
	assertStateString(t, readResp.State, path.Root("groups"), "devs")
	assertStateString(t, readResp.State, path.Root("password"), "initial-secret")
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &deleteResp)
	if readResp.Diagnostics.HasError() || deleteResp.Diagnostics.HasError() {
		t.Fatalf("user read/delete diagnostics: read=%v delete=%v", readResp.Diagnostics, deleteResp.Diagnostics)
	}
	var idempotentDeleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &idempotentDeleteResp)
	if idempotentDeleteResp.Diagnostics.HasError() {
		t.Fatalf("idempotent user delete diagnostics: %v", idempotentDeleteResp.Diagnostics)
	}
	missingResp := resource.ReadResponse{State: readResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: readResp.State}, &missingResp)
	if missingResp.Diagnostics.HasError() || !missingResp.State.Raw.IsNull() {
		t.Fatalf("missing user was not removed from state: diagnostics=%v raw=%v", missingResp.Diagnostics, missingResp.State.Raw)
	}
	handler.assert(t)
	wantCalls := []string{
		"POST /api2/json/access/users comment=created&email=unit%40example.test&enable=1&expire=0&groups=devs&password=initial-secret&userid=unit%40pve",
		"GET /api2/json/access/users/unit@pve ",
		"PUT /api2/json/access/users/unit@pve comment=updated&email=unit%40example.test&enable=1&expire=0&groups=devs",
		"GET /api2/json/access/users/unit@pve ",
		"GET /api2/json/access/users/unit@pve ",
		"DELETE /api2/json/access/users/unit@pve ",
		"DELETE /api2/json/access/users/unit@pve ",
		"GET /api2/json/access/users/unit@pve ",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected user calls: got %v want %v", calls, wantCalls)
	}
	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, userResourceModel{ID: types.StringNull(), UserID: types.StringNull()})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "unit@pve"}, &importResp)
	assertStateString(t, importResp.State, path.Root("user_id"), "unit@pve")
}

func TestUserTokenResourceLifecyclePreservesSecret(t *testing.T) {
	comment := "created"
	handler := &lifecycleHandler{}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.EscapedPath() != "/api2/json/access/users/unit@pve/token/ci" {
			handler.fail(w, "unexpected token path: %s", r.URL.EscapedPath())
			return
		}
		if err := r.ParseForm(); err != nil {
			handler.fail(w, "parse token form: %v", err)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath()+" "+r.Form.Encode())
		switch r.Method {
		case http.MethodPost:
			want := url.Values{"comment": {"created"}, "expire": {"0"}, "privsep": {"1"}}
			if !reflect.DeepEqual(r.Form, want) {
				handler.fail(w, "unexpected token create form: got %v want %v", r.Form, want)
				return
			}
			handler.envelope(w, map[string]any{"full-tokenid": "unit@pve!ci", "value": "token-secret", "info": map[string]any{"comment": "created", "expire": 0, "privsep": 1}})
		case http.MethodGet:
			handler.envelope(w, map[string]any{"comment": comment, "expire": 0, "privsep": 1})
		case http.MethodPut:
			want := url.Values{"comment": {"updated"}, "expire": {"0"}, "privsep": {"1"}}
			if !reflect.DeepEqual(r.Form, want) {
				handler.fail(w, "unexpected token update form: got %v want %v", r.Form, want)
				return
			}
			comment = "updated"
			handler.envelope(w, nil)
		case http.MethodDelete:
			if len(r.Form) != 0 {
				handler.fail(w, "unexpected token delete form: %v", r.Form)
				return
			}
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected token method: %s", r.Method)
		}
	}))
	defer server.Close()
	res := &UserTokenResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := userTokenResourceModel{UserID: types.StringValue("unit@pve"), TokenID: types.StringValue("ci"), Comment: types.StringValue("created"), Expire: types.Int64Value(0), Privsep: types.BoolValue(true)}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial)}, &createResp)
	updated := initial
	updated.Comment = types.StringValue("updated")
	updated.Value = types.StringValue("token-secret")
	updated.FullTokenID = types.StringValue("unit@pve!ci")
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, updated), State: createResp.State}, &updateResp)
	if createResp.Diagnostics.HasError() || updateResp.Diagnostics.HasError() {
		t.Fatalf("token lifecycle diagnostics: create=%v update=%v", createResp.Diagnostics, updateResp.Diagnostics)
	}
	assertStateString(t, updateResp.State, path.Root("value"), "token-secret")
	readResp := resource.ReadResponse{State: updateResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: updateResp.State}, &readResp)
	assertStateString(t, readResp.State, path.Root("comment"), "updated")
	assertStateString(t, readResp.State, path.Root("value"), "token-secret")
	assertStateString(t, readResp.State, path.Root("full_token_id"), "unit@pve!ci")
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &deleteResp)
	if readResp.Diagnostics.HasError() || deleteResp.Diagnostics.HasError() {
		t.Fatalf("token read/delete diagnostics: read=%v delete=%v", readResp.Diagnostics, deleteResp.Diagnostics)
	}
	handler.assert(t)
	wantCalls := []string{
		"POST /api2/json/access/users/unit@pve/token/ci comment=created&expire=0&privsep=1",
		"GET /api2/json/access/users/unit@pve/token/ci ",
		"PUT /api2/json/access/users/unit@pve/token/ci comment=updated&expire=0&privsep=1",
		"GET /api2/json/access/users/unit@pve/token/ci ",
		"GET /api2/json/access/users/unit@pve/token/ci ",
		"DELETE /api2/json/access/users/unit@pve/token/ci ",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected token calls: got %v want %v", calls, wantCalls)
	}
	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, userTokenResourceModel{ID: types.StringNull(), UserID: types.StringNull(), TokenID: types.StringNull()})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "unit@pve/ci"}, &importResp)
	assertStateString(t, importResp.State, path.Root("token_id"), "ci")
}

func TestACLApplySortsBindingsAndPropagation(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.Method != http.MethodPut || r.URL.EscapedPath() != "/api2/json/access/acl" {
			handler.fail(w, "unexpected ACL request: %s %s", r.Method, r.URL.EscapedPath())
			return
		}
		if !handler.form(w, r, url.Values{
			"path":      {"/pool/unit"},
			"roles":     {"PVEAdmin,PVEAuditor"},
			"users":     {"amy@pve,zoe@pve"},
			"groups":    {"admins,devs"},
			"propagate": {"0"},
		}) {
			return
		}
		handler.envelope(w, nil)
	}))
	defer server.Close()
	res := &ACLResource{client: testLifecycleClient(t, server)}
	model := aclResourceModel{
		Path:      types.StringValue("/pool/unit"),
		Propagate: types.BoolValue(false),
		Roles:     testStringList(t, "PVEAuditor", "PVEAdmin"),
		Users:     testStringList(t, "zoe@pve", "amy@pve"),
		Groups:    testStringList(t, "devs", "admins"),
	}
	if err := res.applyACL(context.Background(), model, false); err != nil {
		t.Fatalf("applyACL() unexpected error: %v", err)
	}
	handler.assert(t)
}

func TestACLResourceLifecycleAndBindingReconciliation(t *testing.T) {
	type binding struct {
		role      string
		kind      string
		principal string
		propagate int
	}
	bindings := []binding{}
	handler := &lifecycleHandler{}
	var calls []string
	var updates []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.EscapedPath() != "/api2/json/access/acl" {
			handler.fail(w, "unexpected ACL path: %s", r.URL.EscapedPath())
			return
		}
		if err := r.ParseForm(); err != nil {
			handler.fail(w, "parse ACL form: %v", err)
			return
		}
		calls = append(calls, r.Method+" "+r.Form.Encode())
		switch r.Method {
		case http.MethodPut:
			if r.Form.Get("path") != "/pool/unit" {
				handler.fail(w, "unexpected ACL form: %v", r.Form)
				return
			}
			updates = append(updates, r.Form.Encode())
			roles := splitProxmoxList(r.Form.Get("roles"))
			principals := map[string][]string{"user": splitProxmoxList(r.Form.Get("users")), "group": splitProxmoxList(r.Form.Get("groups"))}
			if r.Form.Get("delete") == "1" {
				remaining := bindings[:0]
				for _, existing := range bindings {
					remove := false
					for _, role := range roles {
						for kind, values := range principals {
							for _, principal := range values {
								remove = remove || existing.role == role && existing.kind == kind && existing.principal == principal
							}
						}
					}
					if !remove {
						remaining = append(remaining, existing)
					}
				}
				bindings = remaining
			} else {
				propagate := 0
				if r.Form.Get("propagate") == "1" {
					propagate = 1
				}
				for _, role := range roles {
					for kind, values := range principals {
						for _, principal := range values {
							candidate := binding{role: role, kind: kind, principal: principal, propagate: propagate}
							found := false
							for i := range bindings {
								if bindings[i].role == role && bindings[i].kind == kind && bindings[i].principal == principal {
									bindings[i] = candidate
									found = true
								}
							}
							if !found {
								bindings = append(bindings, candidate)
							}
						}
					}
				}
			}
			handler.envelope(w, nil)
		case http.MethodGet:
			entries := make([]map[string]any, 0, len(bindings))
			for _, value := range bindings {
				entries = append(entries, map[string]any{"path": "/pool/unit", "roleid": value.role, "type": value.kind, "ugid": value.principal, "propagate": value.propagate})
			}
			handler.envelope(w, entries)
		default:
			handler.fail(w, "unexpected ACL method: %s", r.Method)
		}
	}))
	defer server.Close()
	res := &ACLResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := aclResourceModel{Path: types.StringValue("/pool/unit"), Propagate: types.BoolValue(false), Roles: testStringList(t, "PVEAuditor"), Users: testStringList(t, "amy@pve"), Groups: testStringList(t, "devs")}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("ACL create diagnostics: %v", createResp.Diagnostics)
	}
	readResp := resource.ReadResponse{State: createResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: createResp.State}, &readResp)
	updated := initial
	updated.Groups = testStringList(t)
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, updated), State: readResp.State}, &updateResp)
	if readResp.Diagnostics.HasError() || updateResp.Diagnostics.HasError() {
		t.Fatalf("ACL read/update diagnostics: read=%v update=%v", readResp.Diagnostics, updateResp.Diagnostics)
	}
	var updatedState aclResourceModel
	if diags := updateResp.State.Get(context.Background(), &updatedState); diags.HasError() {
		t.Fatalf("decode updated ACL state: %v", diags)
	}
	if got, want := stringListValues(t, updatedState.Roles), []string{"PVEAuditor"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected updated ACL roles: got %v want %v", got, want)
	}
	if got, want := stringListValues(t, updatedState.Users), []string{"amy@pve"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected updated ACL users: got %v want %v", got, want)
	}
	if got := stringListValues(t, updatedState.Groups); len(got) != 0 {
		t.Fatalf("removed ACL group reappeared in state: %v", got)
	}
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("ACL delete diagnostics: %v", deleteResp.Diagnostics)
	}
	handler.assert(t)
	wantUpdates := []string{
		"groups=devs&path=%2Fpool%2Funit&propagate=0&roles=PVEAuditor&users=amy%40pve",
		"delete=1&groups=devs&path=%2Fpool%2Funit&propagate=1&roles=PVEAuditor",
		"path=%2Fpool%2Funit&propagate=0&roles=PVEAuditor&users=amy%40pve",
		"delete=1&path=%2Fpool%2Funit&propagate=0&roles=PVEAuditor&users=amy%40pve",
	}
	if !reflect.DeepEqual(updates, wantUpdates) {
		t.Fatalf("unexpected ACL update sequence: got %v want %v; all calls %v", updates, wantUpdates, calls)
	}
	wantCalls := []string{
		"PUT groups=devs&path=%2Fpool%2Funit&propagate=0&roles=PVEAuditor&users=amy%40pve",
		"GET ",
		"GET ",
		"PUT delete=1&groups=devs&path=%2Fpool%2Funit&propagate=1&roles=PVEAuditor",
		"PUT path=%2Fpool%2Funit&propagate=0&roles=PVEAuditor&users=amy%40pve",
		"GET ",
		"PUT delete=1&path=%2Fpool%2Funit&propagate=0&roles=PVEAuditor&users=amy%40pve",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected complete ACL call sequence: got %v want %v", calls, wantCalls)
	}
	removed := res.diffRemovedBindings(context.Background(), updated, initial)
	if len(removed) != 1 || removed[0].Type != "group" || removed[0].UGID != "devs" {
		t.Fatalf("unexpected removed ACL bindings: %#v", removed)
	}
	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, aclResourceModel{ID: types.StringNull(), Path: types.StringNull(), Propagate: types.BoolNull(), Roles: types.ListNull(types.StringType), Users: types.ListNull(types.StringType), Groups: types.ListNull(types.StringType)})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "/pool/unit"}, &importResp)
	assertStateString(t, importResp.State, path.Root("path"), "/pool/unit")
}

func TestACLResourceReadPreservesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeAPIError(t, w, http.StatusForbidden, "missing Permissions.Modify")
	}))
	defer server.Close()
	res := &ACLResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := testResourceState(t, schema, aclResourceModel{ID: types.StringValue("/pool/unit"), Path: types.StringValue("/pool/unit"), Propagate: types.BoolValue(true), Roles: testStringList(t, "PVEAuditor"), Users: testStringList(t, "amy@pve"), Groups: testStringList(t)})
	resp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "status 403") || !containsDiagnostic(resp.Diagnostics, "missing Permissions.Modify") {
		t.Fatalf("expected preserved ACL API error, got %v", resp.Diagnostics)
	}
	if !resp.State.Raw.Equal(state.Raw) {
		t.Fatalf("ACL API error unexpectedly mutated state: %v", resp.State.Raw)
	}
}

func writeAPIError(t *testing.T, w http.ResponseWriter, status int, message string) {
	t.Helper()
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"message":%q}`, message)
}
