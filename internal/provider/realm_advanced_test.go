// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func realmTFAForTest(t *testing.T, values map[string]any) types.Object {
	t.Helper()
	attrs := map[string]attr.Value{
		"type":   types.StringNull(),
		"id":     types.StringNull(),
		"key":    types.StringNull(),
		"url":    types.StringNull(),
		"digits": types.Int64Null(),
		"step":   types.Int64Null(),
	}
	for key, value := range values {
		switch typed := value.(type) {
		case string:
			attrs[key] = types.StringValue(typed)
		case int64:
			attrs[key] = types.Int64Value(typed)
		default:
			t.Fatalf("unexpected TFA test value %#v", value)
		}
	}
	object, diags := types.ObjectValue(realmTFAAttrTypes(), attrs)
	if diags.HasError() {
		t.Fatalf("create TFA object: %v", diags)
	}
	return object
}

func validLDAPRealmConfig(t *testing.T) realmModel {
	t.Helper()
	return realmModel{
		Realm:         types.StringValue("corp-ldap"),
		Type:          types.StringValue("ldap"),
		Server1:       types.StringValue("ldap.example.com"),
		BaseDN:        types.StringValue("dc=example,dc=com"),
		UserAttr:      types.StringValue("uid"),
		Cert:          types.StringValue("/etc/pve/priv/realm/corp.crt"),
		CertKey:       types.StringValue("/etc/pve/priv/realm/corp.key"),
		Filter:        types.StringValue("(objectClass=person)"),
		SyncAttrs:     types.StringValue("email=mail"),
		UserClasses:   types.StringValue("inetorgperson, posixaccount"),
		GroupDN:       types.StringValue("ou=groups,dc=example,dc=com"),
		GroupNameAttr: types.StringValue("cn"),
		GroupFilter:   types.StringValue("(objectClass=groupOfNames)"),
		GroupClasses:  types.StringValue("groupOfNames, group"),
		SyncDefaults:  types.StringValue("remove-vanished=entry"),
		CaseSensitive: types.BoolValue(false),
		TFA:           realmTFAForTest(t, map[string]any{"type": "oath", "digits": int64(8), "step": int64(60)}),
	}
}

func TestValidateRealmAdvancedFields(t *testing.T) {
	if diags := validateRealmConfig(validLDAPRealmConfig(t)); diags.HasError() {
		t.Fatalf("valid advanced LDAP config diagnostics: %v", diags)
	}
	openid := realmModel{
		Realm:            types.StringValue("corp-sso"),
		Type:             types.StringValue("openid"),
		IssuerURL:        types.StringValue("https://id.example.com"),
		ClientID:         types.StringValue("proxmox"),
		GroupsClaim:      types.StringValue("groups"),
		GroupsAutocreate: types.BoolValue(true),
		GroupsOverwrite:  types.BoolValue(true),
	}
	if diags := validateRealmConfig(openid); diags.HasError() {
		t.Fatalf("valid OpenID group config diagnostics: %v", diags)
	}

	invalid := map[string]func(*realmModel){}
	addLDAP := func(name string, mutate func(*realmModel)) {
		config := validLDAPRealmConfig(t)
		mutate(&config)
		invalid[name] = func(m *realmModel) { *m = config }
	}
	addLDAP("sync attributes format", func(m *realmModel) { m.SyncAttrs = types.StringValue("mail") })
	addLDAP("user classes format", func(m *realmModel) { m.UserClasses = types.StringValue("inetorgperson;") })
	addLDAP("filter too long", func(m *realmModel) { m.Filter = types.StringValue(strings.Repeat("x", 2049)) })
	addLDAP("group dn too long", func(m *realmModel) { m.GroupDN = types.StringValue(strings.Repeat("x", 257)) })
	addLDAP("tfa without type", func(m *realmModel) { m.TFA = realmTFAForTest(t, map[string]any{"digits": int64(6)}) })
	addLDAP("tfa unknown type", func(m *realmModel) { m.TFA = realmTFAForTest(t, map[string]any{"type": "sms"}) })
	addLDAP("oath digits too low", func(m *realmModel) { m.TFA = realmTFAForTest(t, map[string]any{"type": "oath", "digits": int64(5)}) })
	addLDAP("oath step too low", func(m *realmModel) { m.TFA = realmTFAForTest(t, map[string]any{"type": "oath", "step": int64(5)}) })
	addLDAP("oath with yubico field", func(m *realmModel) {
		m.TFA = realmTFAForTest(t, map[string]any{"type": "oath", "id": "1234"})
	})
	addLDAP("yubico with digits", func(m *realmModel) {
		m.TFA = realmTFAForTest(t, map[string]any{"type": "yubico", "digits": int64(6)})
	})
	addLDAP("openid group field on ldap", func(m *realmModel) { m.GroupsClaim = types.StringValue("groups") })

	config := openid
	config.Cert = types.StringValue("/etc/pve/priv/realm/corp.crt")
	invalid["cert on openid"] = func(m *realmModel) { *m = config }
	config = openid
	config.TFA = realmTFAForTest(t, map[string]any{"type": "oath"})
	invalid["tfa on openid"] = func(m *realmModel) { *m = config }

	for name, build := range invalid {
		var subject realmModel
		build(&subject)
		if diags := validateRealmConfig(subject); !diags.HasError() {
			t.Fatalf("%s: expected invalid diagnostics: %#v", name, subject)
		}
	}
}

func TestRealmAdvancedRequestUsesExplicitConfiguration(t *testing.T) {
	config := validLDAPRealmConfig(t)
	config.TFA = types.ObjectNull(realmTFAAttrTypes())
	plan := config
	plan.TFA = realmTFAForTest(t, map[string]any{"type": "yubico", "id": "42", "key": "sekret"})

	request := realmRequestFromModels(config, realmModel{}, true)
	if request.TFA != nil {
		t.Fatalf("null tfa must not be sent: %#v", request.TFA)
	}
	if request.Cert == nil || *request.Cert != "/etc/pve/priv/realm/corp.crt" || request.GroupDN == nil || request.CaseSensitive == nil || *request.CaseSensitive {
		t.Fatalf("request omitted explicit advanced configuration: %#v", request)
	}

	config.TFA = realmTFAForTest(t, map[string]any{"type": "yubico", "id": "42", "key": "sekret"})
	plan.TFA = config.TFA
	request = realmRequestFromModels(config, realmModel{}, true)
	if request.TFA == nil || *request.TFA != "type=yubico,id=42,key=sekret" {
		t.Fatalf("unexpected TFA encoding: %#v", request.TFA)
	}

	empty := realmModel{Realm: types.StringValue("corp"), Type: types.StringValue("openid")}
	request = realmRequestFromModels(empty, empty, true)
	if request.Cert != nil || request.TFA != nil {
		t.Fatalf("computed-only values must not be sent: %#v", request)
	}
}

func TestRealmStateDecodesAdvancedFieldsAndTFA(t *testing.T) {
	state, diags := realmStateFromAPI(Realm{
		Realm:         "corp",
		Type:          "ldap",
		Cert:          "/etc/pve/priv/realm/corp.crt",
		CertKey:       "/etc/pve/priv/realm/corp.key",
		Filter:        "(objectClass=person)",
		GroupDN:       "ou=groups",
		CaseSensitive: proxmoxOptionalBool{value: boolPtr(false)},
		TFA:           "type=oath,digits=8,step=60",
	}, nil)
	if diags.HasError() {
		t.Fatalf("realmStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if state.Cert.ValueString() != "/etc/pve/priv/realm/corp.crt" || state.CertKey.ValueString() != "/etc/pve/priv/realm/corp.key" || state.Filter.ValueString() != "(objectClass=person)" || state.GroupDN.ValueString() != "ou=groups" {
		t.Fatalf("advanced fields were not mapped: %#v", state)
	}
	if state.CaseSensitive.IsNull() || state.CaseSensitive.ValueBool() {
		t.Fatalf("case_sensitive must be false: %#v", state.CaseSensitive)
	}
	attrs := state.TFA.Attributes()
	tfaType, _ := attrs["type"].(types.String)
	digits, _ := attrs["digits"].(types.Int64)
	step, _ := attrs["step"].(types.Int64)
	if tfaType.ValueString() != "oath" || digits.ValueInt64() != 8 || step.ValueInt64() != 60 {
		t.Fatalf("TFA config was not decoded: %#v", state.TFA)
	}
	if !attrs["id"].IsNull() || !attrs["key"].IsNull() || !attrs["url"].IsNull() {
		t.Fatalf("unset TFA fields must stay null: %#v", state.TFA)
	}

	nullTFA, diags := realmStateFromAPI(Realm{Realm: "corp", Type: "ldap"}, nil)
	if diags.HasError() || !nullTFA.TFA.IsNull() || !nullTFA.Cert.IsNull() {
		t.Fatalf("absent values must stay null: %#v (%v)", nullTFA, diags)
	}
}

func TestRealmTFADecodeRejectsGarbage(t *testing.T) {
	for _, raw := range []string{"type", "type=oath,unknown=1", "type=oath,digits=x", "digits=6"} {
		if _, diags := realmTFAObject(raw); !diags.HasError() {
			t.Fatalf("TFA payload %q was accepted", raw)
		}
	}
}

func TestRealmAdvancedManagedFieldDeletion(t *testing.T) {
	config := realmModel{
		Comment: types.StringValue("managed"),
		Cert:    types.StringValue("/etc/pve/priv/realm/corp.crt"),
		TFA:     realmTFAForTest(t, map[string]any{"type": "oath"}),
	}
	got := realmDeleteKeys(config, []string{"cert", "certkey", "tfa", "filter", "group_dn"})
	if len(got) != 3 || got[0] != "certkey" || got[1] != "filter" || got[2] != "group_dn" {
		t.Fatalf("unexpected delete keys: %v", got)
	}
	config.TFA = types.ObjectNull(realmTFAAttrTypes())
	got = realmDeleteKeys(config, []string{"tfa"})
	if len(got) != 1 || got[0] != "tfa" {
		t.Fatalf("removed tfa must be deleted: %v", got)
	}
}
