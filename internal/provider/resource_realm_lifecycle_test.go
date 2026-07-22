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

func initializeResourcePrivate(t *testing.T, response any) {
	t.Helper()
	field := reflect.ValueOf(response).Elem().FieldByName("Private")
	if !field.IsValid() || field.Kind() != reflect.Pointer {
		t.Fatalf("%T has no Private pointer field", response)
	}
	field.Set(reflect.New(field.Type().Elem()))
}

func realmLifecycleConfig(secret string, version int64, comment any) map[string]any {
	return map[string]any{
		"realm":                 "corp",
		"type":                  "ldap",
		"comment":               comment,
		"server1":               "ldap.example.test",
		"base_dn":               "dc=example,dc=test",
		"user_attr":             "uid",
		"bind_password":         secret,
		"bind_password_version": version,
	}
}

func TestRealmResourceLifecycleSecretsManagedDeletionAndImport(t *testing.T) {
	comment := "created"
	exists := true
	handler := &lifecycleHandler{}
	var calls []string
	var forms []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if err := r.ParseForm(); err != nil {
			handler.fail(w, "parse realm form: %v", err)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/access/domains":
			want := url.Values{"realm": {"corp"}, "type": {"ldap"}, "comment": {"created"}, "server1": {"ldap.example.test"}, "base_dn": {"dc=example,dc=test"}, "user_attr": {"uid"}, "password": {"first-secret"}}
			if !reflect.DeepEqual(r.Form, want) {
				handler.fail(w, "unexpected realm create form: got %v want %v", r.Form, want)
				return
			}
			forms = append(forms, r.Form.Encode())
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/access/domains/corp":
			if !exists {
				writeAPIError(t, w, http.StatusNotFound, "missing realm")
				return
			}
			handler.envelope(w, map[string]any{"type": "ldap", "comment": comment, "server1": "ldap.example.test", "base_dn": "dc=example,dc=test", "user_attr": "uid", "digest": "digest-1", "password": "must-not-enter-state"})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/access/domains/corp":
			forms = append(forms, r.Form.Encode())
			if r.Form.Has("delete") {
				comment = ""
			} else if value := r.Form.Get("comment"); value != "" {
				comment = value
			}
			handler.envelope(w, nil)
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/access/domains/corp":
			if len(r.Form) != 0 {
				handler.fail(w, "unexpected realm delete form: %v", r.Form)
				return
			}
			if !exists {
				writeAPIError(t, w, http.StatusNotFound, "missing realm")
				return
			}
			exists = false
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected realm request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &RealmResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(context.Background(), resource.CreateRequest{Config: testResourceConfigValues(t, schema, realmLifecycleConfig("first-secret", 1, "created"))}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("realm create diagnostics: %v", createResp.Diagnostics)
	}
	var createdState realmModel
	if diags := createResp.State.Get(context.Background(), &createdState); diags.HasError() {
		t.Fatalf("decode created realm state: %v", diags)
	}
	if !createdState.BindPassword.IsNull() || createdState.BindPasswordVersion.ValueInt64() != 1 {
		t.Fatalf("realm create state leaked secret or lost version: %#v", createdState)
	}

	unchangedResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}, Private: createResp.Private}
	res.Update(context.Background(), resource.UpdateRequest{Config: testResourceConfigValues(t, schema, realmLifecycleConfig("first-secret", 1, "created")), State: createResp.State, Private: createResp.Private}, &unchangedResp)
	if unchangedResp.Diagnostics.HasError() {
		t.Fatalf("realm unchanged update diagnostics: %v", unchangedResp.Diagnostics)
	}

	rotatedResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}, Private: unchangedResp.Private}
	res.Update(context.Background(), resource.UpdateRequest{Config: testResourceConfigValues(t, schema, realmLifecycleConfig("second-secret", 2, "created")), State: unchangedResp.State, Private: unchangedResp.Private}, &rotatedResp)
	if rotatedResp.Diagnostics.HasError() {
		t.Fatalf("realm rotation diagnostics: %v", rotatedResp.Diagnostics)
	}
	var rotatedState realmModel
	if diags := rotatedResp.State.Get(context.Background(), &rotatedState); diags.HasError() {
		t.Fatalf("decode rotated realm state: %v", diags)
	}
	if !rotatedState.BindPassword.IsNull() || rotatedState.BindPasswordVersion.ValueInt64() != 2 {
		t.Fatalf("realm rotation state leaked secret or lost version: %#v", rotatedState)
	}

	managedDeleteResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}, Private: rotatedResp.Private}
	res.Update(context.Background(), resource.UpdateRequest{Config: testResourceConfigValues(t, schema, realmLifecycleConfig("second-secret", 2, nil)), State: rotatedResp.State, Private: rotatedResp.Private}, &managedDeleteResp)
	if managedDeleteResp.Diagnostics.HasError() {
		t.Fatalf("realm managed delete diagnostics: %v", managedDeleteResp.Diagnostics)
	}

	readResp := resource.ReadResponse{State: managedDeleteResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: managedDeleteResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("realm read diagnostics: %v", readResp.Diagnostics)
	}
	var readState realmModel
	if diags := readResp.State.Get(context.Background(), &readState); diags.HasError() {
		t.Fatalf("decode read realm state: %v", diags)
	}
	if !readState.BindPassword.IsNull() || readState.BindPasswordVersion.ValueInt64() != 2 || readState.Comment.ValueString() != "" {
		t.Fatalf("unexpected filtered realm read state: %#v", readState)
	}
	assertStateString(t, readResp.State, path.Root("server1"), "ldap.example.test")

	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("realm delete diagnostics: %v", deleteResp.Diagnostics)
	}
	missingResp := resource.ReadResponse{State: readResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: readResp.State}, &missingResp)
	if missingResp.Diagnostics.HasError() || !missingResp.State.Raw.IsNull() {
		t.Fatalf("missing realm was not removed from state: diagnostics=%v raw=%v", missingResp.Diagnostics, missingResp.State.Raw)
	}
	var idempotentDeleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &idempotentDeleteResp)
	if idempotentDeleteResp.Diagnostics.HasError() {
		t.Fatalf("idempotent realm delete diagnostics: %v", idempotentDeleteResp.Diagnostics)
	}
	handler.assert(t)

	wantForms := []string{
		"base_dn=dc%3Dexample%2Cdc%3Dtest&comment=created&password=first-secret&realm=corp&server1=ldap.example.test&type=ldap&user_attr=uid",
		"base_dn=dc%3Dexample%2Cdc%3Dtest&comment=created&digest=digest-1&server1=ldap.example.test&user_attr=uid",
		"base_dn=dc%3Dexample%2Cdc%3Dtest&comment=created&digest=digest-1&password=second-secret&server1=ldap.example.test&user_attr=uid",
		"base_dn=dc%3Dexample%2Cdc%3Dtest&delete=comment&digest=digest-1&server1=ldap.example.test&user_attr=uid",
	}
	if !reflect.DeepEqual(forms, wantForms) {
		t.Fatalf("unexpected realm form sequence: got %v want %v", forms, wantForms)
	}
	if len(calls) != 15 {
		t.Fatalf("unexpected realm call count/order: %v", calls)
	}

	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, realmModel{ID: types.StringNull(), Realm: types.StringNull()})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "corp"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("realm import diagnostics: %v", importResp.Diagnostics)
	}
	assertStateString(t, importResp.State, path.Root("realm"), "corp")
}

func TestRealmResourceDeletePreservesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeAPIError(t, w, http.StatusForbidden, "missing Realm.Allocate")
	}))
	defer server.Close()
	res := &RealmResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := realmModel{ID: types.StringValue("corp"), Realm: types.StringValue("corp"), Type: types.StringValue("ldap")}
	var resp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: testResourceState(t, schema, state)}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "status 403") || !containsDiagnostic(resp.Diagnostics, "missing Realm.Allocate") {
		t.Fatalf("expected preserved realm API error, got %v", resp.Diagnostics)
	}
}
