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

func TestClientUserTokenMethods(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.URL.Path == "/api2/json/access/users/deploy@pve/token/ci" && r.Method == http.MethodPost:
			writeEnvelope(t, w, map[string]any{
				"full-tokenid": "deploy@pve!ci",
				"value":        "abc123secret",
				"info": map[string]any{
					"comment": "CI runner",
					"expire":  0,
					"privsep": 1,
				},
			})
		case r.URL.Path == "/api2/json/access/users/deploy@pve/token/ci" && r.Method == http.MethodGet:
			writeEnvelope(t, w, map[string]any{
				"comment": "CI runner",
				"expire":  0,
				"privsep": 1,
			})
		case r.URL.Path == "/api2/json/access/users/deploy@pve/token/ci" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{
				"comment": {"CI runner updated"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/access/users/deploy@pve/token/ci" && r.Method == http.MethodDelete:
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

	// Create returns value
	token, err := client.CreateUserToken(ctx, "deploy@pve", "ci", UserTokenRequest{
		Comment: stringPtr("CI runner"),
	})
	if err != nil {
		t.Fatalf("CreateUserToken() unexpected error: %v", err)
	}
	if token.FullTokenID != "deploy@pve!ci" || token.Value != "abc123secret" {
		t.Fatalf("unexpected token create response: %#v", token)
	}

	// Read does NOT return value
	read, err := client.GetUserToken(ctx, "deploy@pve", "ci")
	if err != nil {
		t.Fatalf("GetUserToken() unexpected error: %v", err)
	}
	if read.Value != "" {
		t.Fatalf("expected empty value on read, got %q", read.Value)
	}
	if read.Comment != "CI runner" {
		t.Fatalf("expected comment, got %q", read.Comment)
	}

	// Update
	if err := client.UpdateUserToken(ctx, "deploy@pve", "ci", UserTokenRequest{
		Comment: stringPtr("CI runner updated"),
	}); err != nil {
		t.Fatalf("UpdateUserToken() unexpected error: %v", err)
	}

	// Delete
	if err := client.DeleteUserToken(ctx, "deploy@pve", "ci"); err != nil {
		t.Fatalf("DeleteUserToken() unexpected error: %v", err)
	}
}
