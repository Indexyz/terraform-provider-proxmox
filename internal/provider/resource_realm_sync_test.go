// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const realmSyncUPID = "UPID:node1:00001234:00000000:00000000:realm-sync:corp:root@pam:"

func TestClientSyncRealm(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		if r.Method != http.MethodPost || r.URL.Path != "/api2/json/access/domains/corp/sync" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		assertFormValues(t, r, url.Values{
			"dry-run":         {"1"},
			"enable-new":      {"0"},
			"remove-vanished": {"acl;entry"},
			"scope":           {"both"},
		})
		writeEnvelope(t, w, realmSyncUPID)
	}))
	defer server.Close()

	upid, err := newHAResourceTestClient(t, server.URL).SyncRealm(context.Background(), RealmSyncRequest{
		Realm: "corp", Scope: "both", RemoveVanished: "acl;entry",
		EnableNew: boolPtr(false), DryRun: boolPtr(true),
	})
	if err != nil || upid != realmSyncUPID {
		t.Fatalf("SyncRealm() = %q, %v", upid, err)
	}
}

func TestClientSyncRealmRejectsEmptyUPID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, nil)
	}))
	defer server.Close()

	if _, err := newHAResourceTestClient(t, server.URL).SyncRealm(context.Background(), RealmSyncRequest{Realm: "corp", Scope: "users", RemoveVanished: "none"}); err == nil || !strings.Contains(err.Error(), "no UPID") {
		t.Fatalf("SyncRealm() with an empty acknowledgement error = %v", err)
	}
}

func TestValidateRealmSyncConfig(t *testing.T) {
	valid := realmSyncModel{
		Realm: types.StringValue("corp"), Scope: types.StringValue("both"),
		RemoveVanished: types.StringValue("acl;properties;entry"), Triggers: testStringMap(t, map[string]string{"r": "1"}),
	}
	if diags := validateRealmSyncConfig(valid); diags.HasError() {
		t.Fatalf("valid diagnostics: %v", diags)
	}
	none := realmSyncModel{Realm: types.StringValue("corp"), Scope: types.StringValue("users"), RemoveVanished: types.StringValue("none"), Triggers: testStringMap(t, map[string]string{"r": "1"})}
	if diags := validateRealmSyncConfig(none); diags.HasError() {
		t.Fatalf("none diagnostics: %v", diags)
	}
	if diags := validateRealmSyncConfig(realmSyncModel{Realm: types.StringUnknown(), Scope: types.StringUnknown(), RemoveVanished: types.StringUnknown(), Triggers: types.MapUnknown(types.StringType)}); diags.HasError() {
		t.Fatalf("unknown values must defer: %v", diags)
	}

	invalid := map[string]realmSyncModel{
		"built-in realm": {Realm: types.StringValue("pam"), Scope: types.StringValue("users"), Triggers: testStringMap(t, map[string]string{"r": "1"})},
		"bad realm":      {Realm: types.StringValue("corp-sso!x"), Scope: types.StringValue("users")},
		"bad scope":      {Realm: types.StringValue("corp"), Scope: types.StringValue("all")},
		"bad remove list": {
			Realm: types.StringValue("corp"), Scope: types.StringValue("users"), RemoveVanished: types.StringValue("acl;mail"),
		},
		"empty triggers": {Realm: types.StringValue("corp"), Scope: types.StringValue("users"), Triggers: types.MapValueMust(types.StringType, nil)},
	}
	for name, config := range invalid {
		if diags := validateRealmSyncConfig(config); !diags.HasError() {
			t.Fatalf("%s: expected invalid diagnostics: %#v", name, config)
		}
	}
}

func TestRealmSyncLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	var synced int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch r.Method + " " + r.URL.EscapedPath() {
		case "GET /api2/json/access/domains/corp":
			handler.envelope(w, map[string]any{"realm": "corp", "type": "ldap", "digest": "d"})
		case "POST /api2/json/access/domains/corp/sync":
			if err := r.ParseForm(); err != nil {
				handler.fail(w, "parse form: %v", err)
			}
			if r.Form.Get("scope") != "users" || r.Form.Get("remove-vanished") != "none" {
				handler.fail(w, "unexpected sync form: %v", r.Form)
			}
			synced++
			handler.envelope(w, realmSyncUPID)
		case "GET /api2/json/nodes/node1/tasks/" + realmSyncUPID + "/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &RealmSyncResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	plan := realmSyncModel{
		Realm: types.StringValue("corp"), Scope: types.StringValue("users"),
		RemoveVanished: types.StringValue("none"), EnableNew: types.BoolValue(true), DryRun: types.BoolValue(false),
		Triggers: testStringMap(t, map[string]string{"revision": "1"}),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, plan)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created realmSyncModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() || created.UPID.ValueString() != realmSyncUPID || created.ID.ValueString() != "corp" {
		t.Fatalf("unexpected state: %#v (%v)", created, diags)
	}

	deleteResp := resource.DeleteResponse{State: createResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: createResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() || synced != 1 {
		t.Fatalf("delete must not sync again: synced=%d diags=%v", synced, deleteResp.Diagnostics)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

func TestRealmSyncRejectsNonLDAPRealm(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		handler.envelope(w, map[string]any{"realm": "sso", "type": "openid"})
	}))
	defer server.Close()

	res := &RealmSyncResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	plan := realmSyncModel{
		Realm: types.StringValue("sso"), Scope: types.StringValue("users"), RemoveVanished: types.StringValue("none"),
		EnableNew: types.BoolValue(true), DryRun: types.BoolValue(false), Triggers: testStringMap(t, map[string]string{"r": "1"}),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, plan)}, &createResp)
	if !createResp.Diagnostics.HasError() || !strings.Contains(createResp.Diagnostics[0].Detail(), "openid") {
		t.Fatalf("non-LDAP realm was synced: %v", createResp.Diagnostics)
	}
}

func TestRealmSyncImportAndMissingRealm(t *testing.T) {
	var importResp resource.ImportStateResponse
	ctx := context.Background()
	importResp.State = tfsdk.State{Raw: tftypes.NewValue(testResourceSchema(t, &RealmSyncResource{}).Schema.Type().TerraformType(ctx), nil), Schema: testResourceSchema(t, &RealmSyncResource{}).Schema}
	(&RealmSyncResource{}).ImportState(context.Background(), resource.ImportStateRequest{ID: "corp"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", importResp.Diagnostics)
	}

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	res := &RealmSyncResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	state := realmSyncModel{ID: types.StringValue("corp"), Realm: types.StringValue("corp"), Triggers: types.MapNull(types.StringType)}
	readResp := resource.ReadResponse{State: testResourceState(t, schema, state)}
	res.Read(ctx, resource.ReadRequest{State: readResp.State}, &readResp)
	if readResp.Diagnostics.HasError() || !readResp.State.Raw.IsNull() {
		t.Fatalf("missing realm must remove the sync from state: %v %v", readResp.Diagnostics, readResp.State.Raw)
	}
}
