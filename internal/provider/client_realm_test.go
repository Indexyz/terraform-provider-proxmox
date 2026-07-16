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

func TestClientRealmMethods(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.URL.Path == "/api2/json/access/domains/corp" && r.Method == http.MethodGet:
			writeEnvelope(t, w, map[string]any{
				"realm":          "corp",
				"type":           "openid",
				"comment":        "Corporate SSO",
				"default":        1,
				"issuer-url":     "https://id.example.com",
				"client-id":      "proxmox",
				"client-key":     "must-not-enter-state",
				"username-claim": "email",
				"audiences":      "proxmox,api",
				"tfa":            "type=yubico,key=must-not-enter-state",
				"digest":         "abc123",
			})
		case r.URL.Path == "/api2/json/access/domains" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{
				"realm":          {"corp"},
				"type":           {"openid"},
				"comment":        {"Corporate SSO"},
				"default":        {"1"},
				"issuer-url":     {"https://id.example.com"},
				"client-id":      {"proxmox"},
				"client-key":     {"oidc-secret"},
				"autocreate":     {"1"},
				"username-claim": {"email"},
				"scopes":         {"openid email profile"},
				"prompt":         {"login"},
				"query-userinfo": {"1"},
				"acr-values":     {"urn:mace:incommon:iap:silver"},
				"audiences":      {"proxmox,api"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/access/domains/corp" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{
				"comment": {"Updated SSO"},
				"delete":  {"client-key,prompt"},
				"digest":  {"abc123"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/access/domains/corp" && r.Method == http.MethodDelete:
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

	realm, err := client.GetRealm(ctx, "corp")
	if err != nil {
		t.Fatalf("GetRealm() unexpected error: %v", err)
	}
	if realm.Realm != "corp" || realm.Type != "openid" || realm.IssuerURL != "https://id.example.com" || realm.Audiences != "proxmox,api" || realm.Digest != "abc123" {
		t.Fatalf("unexpected realm: %#v", realm)
	}

	if err := client.CreateRealm(ctx, RealmRequest{
		Realm:         "corp",
		Type:          "openid",
		Comment:       stringPtr("Corporate SSO"),
		Default:       boolPtr(true),
		IssuerURL:     stringPtr("https://id.example.com"),
		ClientID:      stringPtr("proxmox"),
		ClientKey:     stringPtr("oidc-secret"),
		Autocreate:    boolPtr(true),
		UsernameClaim: stringPtr("email"),
		Scopes:        stringPtr("openid email profile"),
		Prompt:        stringPtr("login"),
		QueryUserinfo: boolPtr(true),
		ACRValues:     stringPtr("urn:mace:incommon:iap:silver"),
		Audiences:     stringPtr("proxmox,api"),
	}); err != nil {
		t.Fatalf("CreateRealm() unexpected error: %v", err)
	}

	if err := client.UpdateRealm(ctx, "corp", RealmRequest{
		Comment: stringPtr("Updated SSO"),
		Digest:  stringPtr("abc123"),
		Delete:  []string{"prompt", "client-key"},
	}); err != nil {
		t.Fatalf("UpdateRealm() unexpected error: %v", err)
	}

	if err := client.DeleteRealm(ctx, "corp"); err != nil {
		t.Fatalf("DeleteRealm() unexpected error: %v", err)
	}
}

func TestClientCreateLDAPAndADRealms(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		if r.Method != http.MethodPost || r.URL.Path != "/api2/json/access/domains" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		switch r.Form.Get("type") {
		case "ldap":
			assertFormValues(t, r, url.Values{
				"realm":     {"corp-ldap"},
				"type":      {"ldap"},
				"server1":   {"ldap.example.com"},
				"server2":   {"ldap-backup.example.com"},
				"port":      {"636"},
				"mode":      {"ldaps"},
				"verify":    {"1"},
				"capath":    {"/etc/ssl/certs"},
				"base_dn":   {"dc=example,dc=com"},
				"user_attr": {"uid"},
				"bind_dn":   {"cn=terraform,dc=example,dc=com"},
				"password":  {"ldap-secret"},
			})
		case "ad":
			assertFormValues(t, r, url.Values{
				"realm":   {"corp-ad"},
				"type":    {"ad"},
				"server1": {"ad.example.com"},
				"domain":  {"example.com"},
				"mode":    {"ldap+starttls"},
			})
		default:
			t.Fatalf("unexpected realm type: %q", r.Form.Get("type"))
		}
		writeEnvelope(t, w, nil)
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
	port := int64(636)
	if err := client.CreateRealm(context.Background(), RealmRequest{
		Realm:        "corp-ldap",
		Type:         "ldap",
		Server1:      stringPtr("ldap.example.com"),
		Server2:      stringPtr("ldap-backup.example.com"),
		Port:         &port,
		Mode:         stringPtr("ldaps"),
		Verify:       boolPtr(true),
		CAPath:       stringPtr("/etc/ssl/certs"),
		BaseDN:       stringPtr("dc=example,dc=com"),
		UserAttr:     stringPtr("uid"),
		BindDN:       stringPtr("cn=terraform,dc=example,dc=com"),
		BindPassword: stringPtr("ldap-secret"),
	}); err != nil {
		t.Fatalf("CreateRealm(ldap) unexpected error: %v", err)
	}
	if err := client.CreateRealm(context.Background(), RealmRequest{
		Realm:   "corp-ad",
		Type:    "ad",
		Server1: stringPtr("ad.example.com"),
		Domain:  stringPtr("example.com"),
		Mode:    stringPtr("ldap+starttls"),
	}); err != nil {
		t.Fatalf("CreateRealm(ad) unexpected error: %v", err)
	}
}

func TestClientDeleteRealmNotFoundIsIdempotent(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
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
	if err := client.DeleteRealm(context.Background(), "missing"); err != nil {
		t.Fatalf("DeleteRealm() unexpected error: %v", err)
	}
}
