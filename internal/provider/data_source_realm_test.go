// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

func TestRealmDataSourceMetadataAndSchema(t *testing.T) {
	ds := NewRealmDataSource()
	var metadataResp datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "proxmox"}, &metadataResp)
	if metadataResp.TypeName != "proxmox_realm" {
		t.Fatalf("unexpected data source name: %q", metadataResp.TypeName)
	}

	var schemaResp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", schemaResp.Diagnostics)
	}
	if got, want := len(schemaResp.Schema.Attributes), 24; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	if !schemaResp.Schema.Attributes["realm"].IsRequired() {
		t.Fatal("realm must be required")
	}
	for _, name := range []string{"id", "type", "comment", "default", "server1", "issuer_url", "audiences"} {
		if !schemaResp.Schema.Attributes[name].IsComputed() {
			t.Fatalf("%s must be computed", name)
		}
	}
	for _, name := range []string{"bind_password", "bind_password_version", "client_key", "client_key_version", "password", "certkey", "tfa", "digest", "raw"} {
		if _, ok := schemaResp.Schema.Attributes[name]; ok {
			t.Fatalf("realm data source must not expose %q", name)
		}
	}
}

func TestRealmDataSourceReadState(t *testing.T) {
	fixtures := map[string]map[string]any{
		"corp-ldap": {
			"realm":     "ignored-response-id",
			"type":      "ldap",
			"comment":   "Corporate LDAP",
			"default":   1,
			"server1":   "ldap.example.com",
			"server2":   "ldap-backup.example.com",
			"port":      636,
			"mode":      "ldaps",
			"verify":    1,
			"capath":    "/etc/ssl/certs",
			"base_dn":   "dc=example,dc=com",
			"user_attr": "uid",
			"bind_dn":   "cn=terraform,dc=example,dc=com",
			"password":  "must-not-enter-state",
		},
		"corp-ad": {
			"type":      "ad",
			"server1":   "ad.example.com",
			"domain":    "example.com",
			"base_dn":   "dc=example,dc=com",
			"user_attr": "sAMAccountName",
		},
		"corp-sso": {
			"type":           "openid",
			"comment":        "Corporate SSO",
			"issuer-url":     "https://id.example.com",
			"client-id":      "proxmox",
			"client-key":     "must-not-enter-state",
			"autocreate":     1,
			"username-claim": "email",
			"scopes":         "openid email profile",
			"prompt":         "login",
			"query-userinfo": 1,
			"acr-values":     "urn:mace:incommon:iap:silver",
			"audiences":      "proxmox,api",
			"certkey":        "must-not-enter-state",
			"tfa":            "type=yubico,key=must-not-enter-state",
		},
		"pam": {
			"type":    "pam",
			"comment": "Linux PAM standard authentication",
		},
		"pve": {
			"type":    "pve",
			"comment": "Proxmox VE authentication server",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		if r.Method != http.MethodGet || r.URL.RawQuery != "" || !strings.HasPrefix(r.URL.Path, "/api2/json/access/domains/") {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		realm := strings.TrimPrefix(r.URL.Path, "/api2/json/access/domains/")
		fixture, ok := fixtures[realm]
		if !ok {
			t.Fatalf("unexpected realm request: %q", realm)
		}
		writeEnvelope(t, w, fixture)
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
	ds := &RealmDataSource{client: client}

	for realm := range fixtures {
		t.Run(realm, func(t *testing.T) {
			state, diags := ds.readState(context.Background(), realm)
			if diags.HasError() {
				t.Fatalf("readState() unexpected diagnostics: %v", diags)
			}
			if state.ID.ValueString() != realm || state.Realm.ValueString() != realm {
				t.Fatalf("unexpected realm identity: %#v", state)
			}
			switch realm {
			case "corp-ldap":
				if state.Type.ValueString() != "ldap" || state.Server1.ValueString() != "ldap.example.com" || state.Port.ValueInt64() != 636 || !state.Verify.ValueBool() || state.BindDN.ValueString() != "cn=terraform,dc=example,dc=com" {
					t.Fatalf("unexpected LDAP state: %#v", state)
				}
			case "corp-ad":
				if state.Type.ValueString() != "ad" || state.Domain.ValueString() != "example.com" || state.UserAttr.ValueString() != "sAMAccountName" {
					t.Fatalf("unexpected AD state: %#v", state)
				}
			case "corp-sso":
				if state.Type.ValueString() != "openid" || state.IssuerURL.ValueString() != "https://id.example.com" || state.ClientID.ValueString() != "proxmox" || !state.Autocreate.ValueBool() || state.Audiences.ValueString() != "proxmox,api" {
					t.Fatalf("unexpected OpenID state: %#v", state)
				}
			case "pam", "pve":
				if state.Type.ValueString() != realm || !state.Server1.IsNull() || !state.IssuerURL.IsNull() {
					t.Fatalf("unexpected built-in realm state: %#v", state)
				}
			}
		})
	}
}

func TestRealmDataSourceReadStatePreservesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"data":null,"errors":{"permission":"missing Sys.Audit"}}`)
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
	_, diags := (&RealmDataSource{client: client}).readState(context.Background(), "corp")
	if !diags.HasError() {
		t.Fatal("expected read diagnostics")
	}
	detail := diags[0].Detail()
	for _, want := range []string{"status 403", "missing Sys.Audit"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("diagnostic lost API error context %q: %s", want, detail)
		}
	}
}
