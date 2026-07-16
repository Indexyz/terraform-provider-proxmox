// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRealmResourceSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewRealmResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := len(resp.Schema.Attributes), 28; got != want {
		t.Fatalf("unexpected attribute count: got %d want %d", got, want)
	}
	if _, ok := resp.Schema.Attributes["raw"]; ok {
		t.Fatal("realm schema must not expose a raw secret escape hatch")
	}
	for _, name := range []string{"bind_password", "client_key"} {
		attribute := resp.Schema.Attributes[name]
		if !attribute.IsSensitive() || !attribute.IsWriteOnly() {
			t.Fatalf("%s must be sensitive and write-only", name)
		}
	}
}

func TestValidateRealmConfig(t *testing.T) {
	validLDAP := realmModel{
		Realm:               types.StringValue("corp-ldap"),
		Type:                types.StringValue("ldap"),
		Server1:             types.StringValue("ldap.example.com"),
		BaseDN:              types.StringValue("dc=example,dc=com"),
		UserAttr:            types.StringValue("uid"),
		Mode:                types.StringValue("ldap+starttls"),
		Port:                types.Int64Value(389),
		BindDN:              types.StringValue("cn=terraform,dc=example,dc=com"),
		BindPassword:        types.StringValue("secret"),
		BindPasswordVersion: types.Int64Value(1),
	}
	validAD := realmModel{
		Realm:   types.StringValue("corp-ad"),
		Type:    types.StringValue("ad"),
		Server1: types.StringValue("ad.example.com"),
		Domain:  types.StringValue("example.com"),
	}
	validOpenID := realmModel{
		Realm:            types.StringValue("corp-sso"),
		Type:             types.StringValue("openid"),
		IssuerURL:        types.StringValue("https://id.example.com"),
		ClientID:         types.StringValue("proxmox"),
		ClientKey:        types.StringValue("secret"),
		ClientKeyVersion: types.Int64Value(1),
		Audiences:        types.StringValue("proxmox,api"),
	}
	for _, config := range []realmModel{validLDAP, validAD, validOpenID} {
		if diags := validateRealmConfig(config); diags.HasError() {
			t.Fatalf("valid config diagnostics for %#v: %v", config, diags)
		}
	}

	invalid := []realmModel{
		{Realm: types.StringValue("pve"), Type: validLDAP.Type, Server1: validLDAP.Server1, BaseDN: validLDAP.BaseDN, UserAttr: validLDAP.UserAttr},
		{Realm: types.StringValue("bad realm"), Type: validLDAP.Type, Server1: validLDAP.Server1, BaseDN: validLDAP.BaseDN, UserAttr: validLDAP.UserAttr},
		{Realm: validLDAP.Realm, Type: types.StringValue("pam")},
		{Realm: validLDAP.Realm, Type: validLDAP.Type, Server1: validLDAP.Server1, BaseDN: validLDAP.BaseDN},
		{Realm: validAD.Realm, Type: validAD.Type, Server1: validAD.Server1},
		{Realm: validOpenID.Realm, Type: validOpenID.Type, IssuerURL: validOpenID.IssuerURL},
		{Realm: validLDAP.Realm, Type: validLDAP.Type, Server1: validLDAP.Server1, BaseDN: validLDAP.BaseDN, UserAttr: validLDAP.UserAttr, Domain: validAD.Domain},
		{Realm: validOpenID.Realm, Type: validOpenID.Type, IssuerURL: validOpenID.IssuerURL, ClientID: validOpenID.ClientID, Server1: validLDAP.Server1},
		{Realm: validLDAP.Realm, Type: validLDAP.Type, Server1: validLDAP.Server1, BaseDN: validLDAP.BaseDN, UserAttr: validLDAP.UserAttr, Port: types.Int64Value(0)},
		{Realm: validLDAP.Realm, Type: validLDAP.Type, Server1: validLDAP.Server1, BaseDN: validLDAP.BaseDN, UserAttr: validLDAP.UserAttr, Mode: types.StringValue("tls")},
		{Realm: validLDAP.Realm, Type: validLDAP.Type, Server1: validLDAP.Server1, BaseDN: validLDAP.BaseDN, UserAttr: validLDAP.UserAttr, BindPassword: types.StringValue("secret")},
		{Realm: validLDAP.Realm, Type: validLDAP.Type, Server1: validLDAP.Server1, BaseDN: validLDAP.BaseDN, UserAttr: validLDAP.UserAttr, BindPasswordVersion: types.Int64Value(1)},
		{Realm: validOpenID.Realm, Type: validOpenID.Type, IssuerURL: validOpenID.IssuerURL, ClientID: validOpenID.ClientID, ClientKey: types.StringValue("secret")},
		{Realm: validOpenID.Realm, Type: validOpenID.Type, IssuerURL: validOpenID.IssuerURL, ClientID: validOpenID.ClientID, ClientKey: types.StringValue("secret"), ClientKeyVersion: types.Int64Value(0)},
	}
	for _, config := range invalid {
		if diags := validateRealmConfig(config); !diags.HasError() {
			t.Fatalf("expected invalid config diagnostics: %#v", config)
		}
	}
}

func TestRealmRequestSecretVersions(t *testing.T) {
	config := realmModel{
		Realm:               types.StringValue("corp-ldap"),
		Type:                types.StringValue("ldap"),
		Server1:             types.StringValue("ldap.example.com"),
		BindPassword:        types.StringValue("first-secret"),
		BindPasswordVersion: types.Int64Value(1),
	}
	create := realmRequestFromModels(config, realmModel{}, true)
	if create.BindPassword == nil || *create.BindPassword != "first-secret" {
		t.Fatalf("create did not include bind password: %#v", create)
	}
	unchanged := realmRequestFromModels(config, config, false)
	if unchanged.BindPassword != nil {
		t.Fatalf("unchanged secret version must not resend password: %#v", unchanged)
	}
	rotated := config
	rotated.BindPassword = types.StringValue("second-secret")
	rotated.BindPasswordVersion = types.Int64Value(2)
	update := realmRequestFromModels(rotated, config, false)
	if update.BindPassword == nil || *update.BindPassword != "second-secret" {
		t.Fatalf("changed secret version did not send password: %#v", update)
	}

	openid := realmModel{
		Realm:            types.StringValue("corp-sso"),
		Type:             types.StringValue("openid"),
		ClientKey:        types.StringValue("oidc-secret"),
		ClientKeyVersion: types.Int64Value(2),
	}
	request := realmRequestFromModels(openid, realmModel{ClientKeyVersion: types.Int64Value(1)}, false)
	if request.ClientKey == nil || *request.ClientKey != "oidc-secret" {
		t.Fatalf("changed client key version did not send key: %#v", request)
	}
}

func TestRealmStateFiltersSecrets(t *testing.T) {
	prior := realmModel{
		BindPasswordVersion: types.Int64Value(3),
		ClientKeyVersion:    types.Int64Value(4),
	}
	state := realmStateFromAPI(Realm{
		Realm:     "corp-sso",
		Type:      "openid",
		IssuerURL: "https://id.example.com",
		ClientID:  "proxmox",
		Audiences: "proxmox,api",
	}, &prior)
	if !state.BindPassword.IsNull() || !state.ClientKey.IsNull() {
		t.Fatalf("secrets entered state: %#v", state)
	}
	if state.BindPasswordVersion.ValueInt64() != 3 || state.ClientKeyVersion.ValueInt64() != 4 {
		t.Fatalf("secret versions were not preserved: %#v", state)
	}
}

func TestRealmImportRejectsBuiltInRealms(t *testing.T) {
	for _, realm := range []string{"pam", "pve"} {
		var resp resource.ImportStateResponse
		(&RealmResource{}).ImportState(context.Background(), resource.ImportStateRequest{ID: realm}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected import diagnostic for built-in realm %q", realm)
		}
	}
}

func TestRealmManagedFieldDeletion(t *testing.T) {
	config := realmModel{
		Comment:          types.StringValue("managed"),
		ClientKeyVersion: types.Int64Null(),
	}
	got := realmDeleteKeys(config, []string{"client-key", "comment", "prompt"})
	want := []string{"client-key", "prompt"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("unexpected delete keys: got %v want %v", got, want)
	}
	if got := realmDeleteKeys(config, nil); len(got) != 0 {
		t.Fatalf("imported unmanaged fields must not be deleted: %v", got)
	}
}
