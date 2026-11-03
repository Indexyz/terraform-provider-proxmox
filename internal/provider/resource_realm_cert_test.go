// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const realmCertUPID = "UPID:node1:00001234:00000000:00000000:imgcopy:vol:root@pam:"

func TestValidateRealmCertContent(t *testing.T) {
	base := func() realmModel {
		return realmModel{
			Realm: types.StringValue("corp-ldap"), Type: types.StringValue("ldap"),
			Server1: types.StringValue("ldap.example.com"), BaseDN: types.StringValue("dc=example"),
			UserAttr: types.StringValue("uid"),
		}
	}
	valid := base()
	valid.CertStorage = types.StringValue("store")
	valid.CertContent = types.StringValue("-----BEGIN CERTIFICATE-----")
	valid.CertKeyContent = types.StringValue("-----BEGIN KEY-----")
	if diags := validateRealmConfig(valid); diags.HasError() {
		t.Fatalf("valid content diagnostics: %v", diags)
	}
	paths := base()
	paths.Cert = types.StringValue("/etc/pve/priv/realm/corp.crt")
	paths.CertKey = types.StringValue("/etc/pve/priv/realm/corp.key")
	if diags := validateRealmConfig(paths); diags.HasError() {
		t.Fatalf("valid paths diagnostics: %v", diags)
	}

	conflict := valid
	conflict.Cert = types.StringValue("/etc/pve/priv/realm/corp.crt")
	if diags := validateRealmConfig(conflict); !diags.HasError() {
		t.Fatal("paths plus content must conflict")
	}
	withoutKey := valid
	withoutKey.CertKeyContent = types.StringNull()
	if diags := validateRealmConfig(withoutKey); !diags.HasError() {
		t.Fatal("content without key must be rejected")
	}
	withoutStorage := valid
	withoutStorage.CertStorage = types.StringNull()
	if diags := validateRealmConfig(withoutStorage); !diags.HasError() {
		t.Fatal("content without storage must be rejected")
	}
	storageOnly := base()
	storageOnly.CertStorage = types.StringValue("store")
	if diags := validateRealmConfig(storageOnly); !diags.HasError() {
		t.Fatal("storage without content must be rejected")
	}

	// The OpenID variant keeps rejecting the directory-only cert fields.
	openid := realmModel{
		Realm: types.StringValue("corp-sso"), Type: types.StringValue("openid"),
		IssuerURL: types.StringValue("https://id.example.com"), ClientID: types.StringValue("x"),
		CertContent: types.StringValue("c"), CertKeyContent: types.StringValue("k"), CertStorage: types.StringValue("store"),
	}
	if diags := validateRealmConfig(openid); !diags.HasError() {
		t.Fatal("openid must reject certificate content")
	}
}

func TestRealmCertUploadTargetRejectsUnusableStorage(t *testing.T) {
	ctx := context.Background()
	cases := map[string]map[string]any{
		"no path":  {"storage": "store", "type": "dir", "content": "iso"},
		"no iso":   {"storage": "store", "type": "dir", "content": "images", "path": "/srv/store"},
		"disabled": {"storage": "store", "type": "dir", "content": "iso", "path": "/srv/store", "disable": 1},
	}
	for name, fixture := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeEnvelope(t, w, fixture)
		}))
		_, err := uploadRealmClientCert(ctx, newHAResourceTestClient(t, server.URL), "corp", "store", []byte("c"), []byte("k"))
		server.Close()
		if err == nil {
			t.Fatalf("%s: expected a target error", name)
		}
	}
}

func TestRealmCertContentLifecycle(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	var mu sync.Mutex
	volumes := map[string]bool{}
	var createdPaths url.Values
	deletedVolumes := []string{}
	realmExists := false
	realm := map[string]any{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method + " " + r.URL.EscapedPath() {
		case "GET /api2/json/storage/store":
			handler.envelope(w, map[string]any{"storage": "store", "type": "dir", "content": "iso", "path": "/srv/store", "nodes": "node1"})
		case "GET /api2/json/nodes/node1/storage":
			handler.envelope(w, []map[string]any{{"storage": "store", "content": "iso", "enabled": 1, "active": 1}})
		case "POST /api2/json/nodes/node1/storage/store/upload":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				handler.fail(w, "parse upload: %v", err)
				return
			}
			volumes["store:iso/"+r.MultipartForm.File["filename"][0].Filename] = true
			handler.envelope(w, realmCertUPID)
		case "GET /api2/json/nodes/node1/tasks/" + realmCertUPID + "/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case "POST /api2/json/access/domains":
			if err := r.ParseForm(); err != nil {
				handler.fail(w, "parse realm form: %v", err)
				return
			}
			createdPaths = r.Form
			realm = map[string]any{"realm": "corp", "type": "ldap", "cert": r.Form.Get("cert"), "certkey": r.Form.Get("certkey")}
			realmExists = true
			handler.envelope(w, nil)
		case "GET /api2/json/access/domains/corp":
			if !realmExists {
				writeAPIError(t, w, http.StatusNotFound, "missing realm")
				return
			}
			handler.envelope(w, realm)
		case "PUT /api2/json/access/domains/corp":
			if err := r.ParseForm(); err != nil {
				handler.fail(w, "parse realm update: %v", err)
				return
			}
			if cert := r.Form.Get("cert"); cert != "" {
				realm["cert"] = cert
			}
			handler.envelope(w, nil)
		case "DELETE /api2/json/access/domains/corp":
			realmExists = false
			handler.envelope(w, nil)
		default:
			if r.Method == http.MethodDelete && strings.Contains(r.URL.EscapedPath(), "/storage/store/content/") {
				volume := strings.TrimPrefix(r.URL.EscapedPath(), "/api2/json/nodes/node1/storage/store/content/")
				deletedVolumes = append(deletedVolumes, volume)
				handler.envelope(w, realmCertUPID)
				return
			}
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &RealmResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := realmModel{
		Realm: types.StringValue("corp"), Type: types.StringValue("ldap"),
		Server1: types.StringValue("ldap.example.com"), BaseDN: types.StringValue("dc=example"), UserAttr: types.StringValue("uid"),
		CertStorage: types.StringValue("store"), CertContent: types.StringValue("CERT"), CertKeyContent: types.StringValue("KEY"),
		TFA: types.ObjectNull(realmTFAAttrTypes()),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}

	if got, want := createdPaths.Get("cert"), "/srv/store/template/iso/terraform-realm-corp-cert.img"; got != want {
		t.Fatalf("unexpected cert path %q want %q", got, want)
	}
	if got, want := createdPaths.Get("certkey"), "/srv/store/template/iso/terraform-realm-corp-key.img"; got != want {
		t.Fatalf("unexpected key path %q want %q", got, want)
	}
	if len(volumes) != 2 {
		t.Fatalf("expected both files uploaded: %#v", volumes)
	}

	var created realmModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() {
		t.Fatalf("decode created state: %v", diags)
	}
	if created.Cert.ValueString() != createdPaths.Get("cert") || created.CertContent.ValueString() != "CERT" || created.CertStorage.ValueString() != "store" {
		t.Fatalf("unexpected created state: %#v", created)
	}
	attachment, privateDiags := readRealmClientCert(ctx, createResp.Private)
	if privateDiags.HasError() || attachment.CertVolID != "store:iso/terraform-realm-corp-cert.img" {
		t.Fatalf("attachment not tracked: %#v (%v)", attachment, privateDiags)
	}

	deleteResp := resource.DeleteResponse{State: createResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: createResp.State, Private: createResp.Private}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", deleteResp.Diagnostics)
	}
	if len(deletedVolumes) != 2 {
		t.Fatalf("expected both files deleted after the realm: %#v", deletedVolumes)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

func TestRealmCertContentCreateFailureCleansUpFiles(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	var mu sync.Mutex
	var deleted int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/storage/store":
			handler.envelope(w, map[string]any{"storage": "store", "type": "dir", "content": "iso", "path": "/srv/store", "nodes": "node1"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/node1/storage":
			handler.envelope(w, []map[string]any{{"storage": "store", "content": "iso", "enabled": 1, "active": 1}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.EscapedPath(), "/upload"):
			handler.envelope(w, realmCertUPID)
		case r.Method == http.MethodGet && strings.Contains(r.URL.EscapedPath(), "/tasks/"):
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api2/json/access/domains":
			writeAPIError(t, w, http.StatusBadRequest, "realm exists")
		case r.Method == http.MethodDelete && strings.Contains(r.URL.EscapedPath(), "/content/"):
			deleted++
			handler.envelope(w, realmCertUPID)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &RealmResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := realmModel{
		Realm: types.StringValue("corp"), Type: types.StringValue("ldap"),
		Server1: types.StringValue("ldap.example.com"), BaseDN: types.StringValue("dc=example"), UserAttr: types.StringValue("uid"),
		CertStorage: types.StringValue("store"), CertContent: types.StringValue("CERT"), CertKeyContent: types.StringValue("KEY"),
		TFA: types.ObjectNull(realmTFAAttrTypes()),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if !createResp.Diagnostics.HasError() || !strings.Contains(createResp.Diagnostics[0].Detail(), "realm exists") {
		t.Fatalf("create failure was not reported: %v", createResp.Diagnostics)
	}
	if deleted != 2 {
		t.Fatalf("uploaded files were not cleaned up: %d", deleted)
	}
}

func TestRealmCertContentHealsDeletedFiles(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	var mu sync.Mutex
	exists := true
	var updatedCert string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/access/domains/corp":
			handler.envelope(w, map[string]any{"realm": "corp", "type": "ldap", "cert": "/srv/store/template/iso/terraform-realm-corp-cert.img", "certkey": "/srv/store/template/iso/terraform-realm-corp-key.img", "digest": "d"})
		case r.Method == http.MethodGet && strings.Contains(r.URL.EscapedPath(), "/content"):
			items := []map[string]any{}
			if exists {
				items = append(items, map[string]any{"volid": "store:iso/terraform-realm-corp-cert.img"}, map[string]any{"volid": "store:iso/terraform-realm-corp-key.img"})
			}
			handler.envelope(w, items)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/storage/store":
			handler.envelope(w, map[string]any{"storage": "store", "type": "dir", "content": "iso", "path": "/srv/store", "nodes": "node1"})
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/node1/storage":
			handler.envelope(w, []map[string]any{{"storage": "store", "content": "iso", "enabled": 1, "active": 1}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.EscapedPath(), "/upload"):
			handler.envelope(w, realmCertUPID)
		case r.Method == http.MethodGet && strings.Contains(r.URL.EscapedPath(), "/tasks/"):
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api2/json/access/domains/corp":
			if err := r.ParseForm(); err != nil {
				handler.fail(w, "parse update: %v", err)
				return
			}
			updatedCert = r.Form.Get("cert")
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &RealmResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	attachment := realmClientCert{
		Node: "node1", Storage: "store",
		CertPath: "/srv/store/template/iso/terraform-realm-corp-cert.img", KeyPath: "/srv/store/template/iso/terraform-realm-corp-key.img",
		CertVolID: "store:iso/terraform-realm-corp-cert.img", KeyVolID: "store:iso/terraform-realm-corp-key.img",
	}
	state := realmModel{
		ID: types.StringValue("corp"), Realm: types.StringValue("corp"), Type: types.StringValue("ldap"),
		Cert: types.StringValue(attachment.CertPath), CertKey: types.StringValue(attachment.KeyPath),
		CertStorage: types.StringValue("store"), CertContent: types.StringValue("CERT"), CertKeyContent: types.StringValue("KEY"),
		TFA: types.ObjectNull(realmTFAAttrTypes()),
	}
	readResp := resource.ReadResponse{State: testResourceState(t, schema, state)}
	initializeResourcePrivate(t, &readResp)
	if diags := writeRealmClientCert(ctx, readResp.Private, attachment); diags.HasError() {
		t.Fatalf("seed attachment: %v", diags)
	}
	exists = false
	res.Read(ctx, resource.ReadRequest{State: readResp.State, Private: readResp.Private}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	if updatedCert == "" {
		t.Fatalf("deleted files were not restored: %v", readResp.Diagnostics)
	}
	healed, healDiags := readRealmClientCert(ctx, readResp.Private)
	if healDiags.HasError() || healed.CertVolID != attachment.CertVolID {
		t.Fatalf("healed attachment was not persisted: %#v (%v)", healed, healDiags)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}
