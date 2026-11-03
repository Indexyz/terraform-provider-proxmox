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
)

const testTFAURI = "otpauth://totp/root%40pam?secret=ABCDEF&issuer=Proxmox"

func TestClientUserTFACRUD(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/access/tfa/root@pam":
			assertFormValues(t, r, url.Values{
				"description": {"laptop"},
				"totp":        {testTFAURI},
				"type":        {"totp"},
			})
			writeEnvelope(t, w, map[string]any{"id": "abc123"})
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/access/tfa/root@pam/abc123":
			writeEnvelope(t, w, map[string]any{"id": "abc123", "type": "totp", "description": "laptop", "enable": 1, "created": 1700000000})
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/access/tfa/root@pam/abc123":
			assertFormValues(t, r, url.Values{"description": {"renamed"}, "enable": {"0"}})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/access/tfa/root@pam/abc123":
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	result, err := client.CreateUserTFA(ctx, "root@pam", UserTFACreateRequest{Type: "totp", Description: stringPtr("laptop"), TOTP: stringPtr(testTFAURI)})
	if err != nil || result.ID != "abc123" {
		t.Fatalf("CreateUserTFA() = %#v, %v", result, err)
	}
	entry, err := client.GetUserTFA(ctx, "root@pam", "abc123")
	if err != nil || entry.Enable.Ptr() == nil || !*entry.Enable.Ptr() || entry.Description != "laptop" {
		t.Fatalf("GetUserTFA() = %#v, %v", entry, err)
	}
	if err := client.UpdateUserTFA(ctx, "root@pam", "abc123", UserTFAUpdateRequest{Description: stringPtr("renamed"), Enable: boolPtr(false)}); err != nil {
		t.Fatalf("UpdateUserTFA() unexpected error: %v", err)
	}
	if err := client.DeleteUserTFA(ctx, "root@pam", "abc123"); err != nil {
		t.Fatalf("DeleteUserTFA() unexpected error: %v", err)
	}
}

func TestClientCreateUserTFACapturesRecoveryCodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, w, map[string]any{"id": "rec1", "recovery": []string{"c1", "c2"}})
	}))
	defer server.Close()

	result, err := newHAResourceTestClient(t, server.URL).CreateUserTFA(context.Background(), "root@pam", UserTFACreateRequest{Type: "recovery"})
	if err != nil || len(result.Recovery) != 2 {
		t.Fatalf("CreateUserTFA() = %#v, %v", result, err)
	}
}

func TestClientUserTFARejectsMissingIDAndToleratesMissingDelete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writeEnvelope(t, w, map[string]any{})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	if _, err := client.CreateUserTFA(context.Background(), "root@pam", UserTFACreateRequest{Type: "recovery"}); err == nil || !strings.Contains(err.Error(), "entry id") {
		t.Fatalf("CreateUserTFA() with no id error = %v", err)
	}
	if err := client.DeleteUserTFA(context.Background(), "root@pam", "gone"); err != nil {
		t.Fatalf("DeleteUserTFA() must be idempotent: %v", err)
	}
}

func TestValidateUserTFAConfig(t *testing.T) {
	if diags := validateUserTFAConfig(userTFAModel{UserID: types.StringValue("root@pam"), Type: types.StringValue("totp"), TOTP: types.StringValue(testTFAURI)}); diags.HasError() {
		t.Fatalf("valid totp diagnostics: %v", diags)
	}
	if diags := validateUserTFAConfig(userTFAModel{UserID: types.StringValue("root@pam"), Type: types.StringValue("yubico"), Value: types.StringValue("cccccc")}); diags.HasError() {
		t.Fatalf("valid yubico diagnostics: %v", diags)
	}
	if diags := validateUserTFAConfig(userTFAModel{UserID: types.StringValue("root@pam"), Type: types.StringValue("recovery")}); diags.HasError() {
		t.Fatalf("valid recovery diagnostics: %v", diags)
	}
	if diags := validateUserTFAConfig(userTFAModel{UserID: types.StringUnknown(), Type: types.StringUnknown()}); diags.HasError() {
		t.Fatalf("unknown values must defer: %v", diags)
	}

	invalid := map[string]userTFAModel{
		"bare user":          {UserID: types.StringValue("root"), Type: types.StringValue("recovery")},
		"webauthn":           {UserID: types.StringValue("root@pam"), Type: types.StringValue("webauthn")},
		"totp without uri":   {UserID: types.StringValue("root@pam"), Type: types.StringValue("totp")},
		"bad totp uri":       {UserID: types.StringValue("root@pam"), Type: types.StringValue("totp"), TOTP: types.StringValue("secret")},
		"yubico without otp": {UserID: types.StringValue("root@pam"), Type: types.StringValue("yubico")},
		"totp on recovery":   {UserID: types.StringValue("root@pam"), Type: types.StringValue("recovery"), TOTP: types.StringValue(testTFAURI)},
		"long description":   {UserID: types.StringValue("root@pam"), Type: types.StringValue("recovery"), Description: types.StringValue(strings.Repeat("x", 256))},
	}
	for name, config := range invalid {
		if diags := validateUserTFAConfig(config); !diags.HasError() {
			t.Fatalf("%s: expected invalid diagnostics: %#v", name, config)
		}
	}
}

func TestUserTFALifecyclePreservesSecretInputs(t *testing.T) {
	entries := map[string]map[string]any{}
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch r.Method {
		case http.MethodPost:
			if err := r.ParseForm(); err != nil {
				handler.fail(w, "parse form: %v", err)
				return
			}
			if r.Form.Get("type") != "recovery" {
				handler.fail(w, "unexpected create form: %v", r.Form)
				return
			}
			entries["rec1"] = map[string]any{"id": "rec1", "type": "recovery", "enable": 1}
			handler.envelope(w, map[string]any{"id": "rec1", "recovery": []string{"c1", "c2"}})
		case http.MethodGet:
			entry, ok := entries["rec1"]
			if !ok {
				http.NotFound(w, r)
				return
			}
			handler.envelope(w, entry)
		case http.MethodPut:
			entries["rec1"]["description"] = "codes"
			handler.envelope(w, nil)
		case http.MethodDelete:
			delete(entries, "rec1")
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &UserTFAResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := userTFAModel{
		UserID: types.StringValue("root@pam"), Type: types.StringValue("recovery"),
		Enable: types.BoolValue(true), TOTP: types.StringNull(), Value: types.StringNull(), Password: types.StringNull(),
		RecoveryCodes: types.ListNull(types.StringType),
	}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(ctx, resource.CreateRequest{Config: testResourceConfig(t, schema, config), Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created userTFAModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() {
		t.Fatalf("decode created state: %v", diags)
	}
	if created.ID.ValueString() != "root@pam/rec1" || created.EntryID.ValueString() != "rec1" {
		t.Fatalf("unexpected identity: %#v", created)
	}
	var codes []string
	if diags := created.RecoveryCodes.ElementsAs(ctx, &codes, false); diags.HasError() || len(codes) != 2 {
		t.Fatalf("recovery codes were not captured: %#v (%v)", codes, diags)
	}

	updated := config
	updated.Description = types.StringValue("codes")
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(ctx, resource.UpdateRequest{Config: testResourceConfig(t, schema, updated), Plan: testResourcePlan(t, schema, updated), State: createResp.State}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", updateResp.Diagnostics)
	}
	var refreshed userTFAModel
	if diags := updateResp.State.Get(ctx, &refreshed); diags.HasError() {
		t.Fatalf("decode updated state: %v", diags)
	}
	if refreshed.Description.ValueString() != "codes" || refreshed.RecoveryCodes.IsNull() {
		t.Fatalf("update lost description or codes: %#v", refreshed)
	}

	deleteResp := resource.DeleteResponse{State: updateResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", deleteResp.Diagnostics)
	}
	readResp := resource.ReadResponse{State: updateResp.State}
	res.Read(ctx, resource.ReadRequest{State: updateResp.State}, &readResp)
	if readResp.Diagnostics.HasError() || !readResp.State.Raw.IsNull() {
		t.Fatalf("deleted entry was not removed: %v %v", readResp.Diagnostics, readResp.State.Raw)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}
