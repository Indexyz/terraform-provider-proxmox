// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestClientRoleMethods(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.URL.Path == "/api2/json/access/roles" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{
				"roleid": {"TerraformManage"},
				"privs":  {"VM.Allocate,VM.Audit,Datastore.AllocateSpace"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/access/roles/TerraformManage" && r.Method == http.MethodGet:
			writeEnvelope(t, w, map[string]any{
				"VM.Allocate":             1,
				"VM.Audit":                1,
				"Datastore.AllocateSpace": 1,
			})
		case r.URL.Path == "/api2/json/access/roles/TerraformManage" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{
				"privs": {"VM.Allocate,VM.Audit"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/access/roles/TerraformManage" && r.Method == http.MethodDelete:
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewClient(ctx, ClientConfig{
		Endpoint:       server.URL,
		APITokenID:     "terraform@pve!provider",
		APITokenSecret: "token-secret",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}

	if err := client.CreateRole(ctx, "TerraformManage", "VM.Allocate,VM.Audit,Datastore.AllocateSpace"); err != nil {
		t.Fatalf("CreateRole() unexpected error: %v", err)
	}

	role, err := client.GetRole(ctx, "TerraformManage")
	if err != nil {
		t.Fatalf("GetRole() unexpected error: %v", err)
	}
	if role.RoleID != "TerraformManage" || role.Privs != "Datastore.AllocateSpace,VM.Allocate,VM.Audit" {
		t.Fatalf("unexpected role: %#v", role)
	}

	if err := client.UpdateRole(ctx, "TerraformManage", "VM.Allocate,VM.Audit"); err != nil {
		t.Fatalf("UpdateRole() unexpected error: %v", err)
	}

	if err := client.DeleteRole(ctx, "TerraformManage"); err != nil {
		t.Fatalf("DeleteRole() unexpected error: %v", err)
	}
}

func TestClientUserMethods(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.URL.Path == "/api2/json/access/users" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{
				"userid":    {"deploy@pve"},
				"firstname": {"Terraform"},
				"lastname":  {"Bot"},
				"email":     {"bot@example.com"},
				"password":  {"secret123"},
				"groups":    {"admins"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/access/users/deploy@pve" && r.Method == http.MethodGet:
			writeEnvelope(t, w, map[string]any{
				"userid":    "deploy@pve",
				"firstname": "Terraform",
				"lastname":  "Bot",
				"email":     "bot@example.com",
				"groups":    []string{"admins"},
				"enable":    1,
				"expire":    0,
			})
		case r.URL.Path == "/api2/json/access/users/deploy@pve" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{
				"lastname": {"Bot-Updated"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/access/users/deploy@pve" && r.Method == http.MethodDelete:
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewClient(ctx, ClientConfig{
		Endpoint:       server.URL,
		APITokenID:     "terraform@pve!provider",
		APITokenSecret: "token-secret",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}

	if err := client.CreateUser(ctx, "deploy@pve", UserRequest{
		Firstname: stringPtr("Terraform"),
		Lastname:  stringPtr("Bot"),
		Email:     stringPtr("bot@example.com"),
		Password:  stringPtr("secret123"),
		Groups:    stringPtr("admins"),
	}); err != nil {
		t.Fatalf("CreateUser() unexpected error: %v", err)
	}

	user, err := client.GetUser(ctx, "deploy@pve")
	if err != nil {
		t.Fatalf("GetUser() unexpected error: %v", err)
	}
	if user.UserID != "deploy@pve" || user.Firstname != "Terraform" || user.Email != "bot@example.com" || user.Groups != "admins" {
		t.Fatalf("unexpected user: %#v", user)
	}
	if user.Enable.Ptr() == nil || !*user.Enable.Ptr() {
		t.Fatalf("expected enable=true, got %#v", user.Enable)
	}

	if err := client.UpdateUser(ctx, "deploy@pve", UserRequest{
		Lastname: stringPtr("Bot-Updated"),
	}); err != nil {
		t.Fatalf("UpdateUser() unexpected error: %v", err)
	}

	if err := client.DeleteUser(ctx, "deploy@pve"); err != nil {
		t.Fatalf("DeleteUser() unexpected error: %v", err)
	}
}

func TestDecodeUserConfigHandlesNullValues(t *testing.T) {
	t.Parallel()

	// Verify decode path via GetUser against a test server returning empty strings.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, map[string]any{
			"userid":    "test@pve",
			"firstname": "",
			"email":     "",
		})
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), ClientConfig{
		Endpoint:       server.URL,
		APITokenID:     "terraform@pve!provider",
		APITokenSecret: "token-secret",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}
	config, err := client.GetUser(context.Background(), "test@pve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Firstname != "" || config.Email != "" {
		t.Fatalf("expected empty strings, got %#v", config)
	}
}
