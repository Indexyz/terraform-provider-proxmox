// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var realmIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]+$`)

type realmModel struct {
	ID                  types.String `tfsdk:"id"`
	Realm               types.String `tfsdk:"realm"`
	Type                types.String `tfsdk:"type"`
	Comment             types.String `tfsdk:"comment"`
	Default             types.Bool   `tfsdk:"default"`
	Server1             types.String `tfsdk:"server1"`
	Server2             types.String `tfsdk:"server2"`
	Port                types.Int64  `tfsdk:"port"`
	Mode                types.String `tfsdk:"mode"`
	Verify              types.Bool   `tfsdk:"verify"`
	CAPath              types.String `tfsdk:"ca_path"`
	BaseDN              types.String `tfsdk:"base_dn"`
	UserAttr            types.String `tfsdk:"user_attr"`
	Domain              types.String `tfsdk:"domain"`
	BindDN              types.String `tfsdk:"bind_dn"`
	BindPassword        types.String `tfsdk:"bind_password"`
	BindPasswordVersion types.Int64  `tfsdk:"bind_password_version"`
	IssuerURL           types.String `tfsdk:"issuer_url"`
	ClientID            types.String `tfsdk:"client_id"`
	ClientKey           types.String `tfsdk:"client_key"`
	ClientKeyVersion    types.Int64  `tfsdk:"client_key_version"`
	Autocreate          types.Bool   `tfsdk:"autocreate"`
	UsernameClaim       types.String `tfsdk:"username_claim"`
	Scopes              types.String `tfsdk:"scopes"`
	Prompt              types.String `tfsdk:"prompt"`
	QueryUserinfo       types.Bool   `tfsdk:"query_userinfo"`
	ACRValues           types.String `tfsdk:"acr_values"`
	Audiences           types.String `tfsdk:"audiences"`
}

func validateRealmConfig(config realmModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if !config.Realm.IsNull() && !config.Realm.IsUnknown() {
		realm := config.Realm.ValueString()
		switch {
		case realm == "pve" || realm == "pam":
			diags.AddAttributeError(path.Root("realm"), "Unsupported built-in realm", "The built-in pve and pam realms cannot be managed by this resource.")
		case len(realm) > 32 || !realmIDPattern.MatchString(realm):
			diags.AddAttributeError(path.Root("realm"), "Invalid realm identifier", "realm must start with a letter, contain only letters, numbers, dots, hyphens, or underscores, and be at most 32 characters.")
		}
	}
	if !config.Port.IsNull() && !config.Port.IsUnknown() && (config.Port.ValueInt64() < 1 || config.Port.ValueInt64() > 65535) {
		diags.AddAttributeError(path.Root("port"), "Invalid realm server port", "port must be between 1 and 65535.")
	}
	if !config.Mode.IsNull() && !config.Mode.IsUnknown() && !slices.Contains([]string{"ldap", "ldaps", "ldap+starttls"}, config.Mode.ValueString()) {
		diags.AddAttributeError(path.Root("mode"), "Invalid LDAP mode", "mode must be ldap, ldaps, or ldap+starttls.")
	}
	validateRealmSecretPair(&diags, "bind_password", config.BindPassword, "bind_password_version", config.BindPasswordVersion)
	validateRealmSecretPair(&diags, "client_key", config.ClientKey, "client_key_version", config.ClientKeyVersion)
	if config.Type.IsNull() || config.Type.IsUnknown() {
		return diags
	}

	realmType := config.Type.ValueString()
	if !slices.Contains([]string{"ldap", "ad", "openid"}, realmType) {
		diags.AddAttributeError(path.Root("type"), "Invalid realm type", "type must be ldap, ad, or openid.")
		return diags
	}

	var incompatible []string
	addString := func(name string, value types.String) {
		if !value.IsNull() {
			incompatible = append(incompatible, name)
		}
	}
	addInt64 := func(name string, value types.Int64) {
		if !value.IsNull() {
			incompatible = append(incompatible, name)
		}
	}
	addBool := func(name string, value types.Bool) {
		if !value.IsNull() {
			incompatible = append(incompatible, name)
		}
	}

	switch realmType {
	case "ldap":
		requireRealmString(&diags, "server1", config.Server1)
		requireRealmString(&diags, "base_dn", config.BaseDN)
		requireRealmString(&diags, "user_attr", config.UserAttr)
		addString("domain", config.Domain)
		addOpenIDRealmFields(&incompatible, config)
	case "ad":
		requireRealmString(&diags, "server1", config.Server1)
		requireRealmString(&diags, "domain", config.Domain)
		addOpenIDRealmFields(&incompatible, config)
	case "openid":
		requireRealmString(&diags, "issuer_url", config.IssuerURL)
		requireRealmString(&diags, "client_id", config.ClientID)
		addString("server1", config.Server1)
		addString("server2", config.Server2)
		addInt64("port", config.Port)
		addString("mode", config.Mode)
		addBool("verify", config.Verify)
		addString("ca_path", config.CAPath)
		addString("base_dn", config.BaseDN)
		addString("user_attr", config.UserAttr)
		addString("domain", config.Domain)
		addString("bind_dn", config.BindDN)
		addString("bind_password", config.BindPassword)
		addInt64("bind_password_version", config.BindPasswordVersion)
	}
	if len(incompatible) > 0 {
		slices.Sort(incompatible)
		diags.AddError("Invalid realm fields", fmt.Sprintf("type %q does not support: %s", realmType, strings.Join(incompatible, ", ")))
	}
	return diags
}

func validateRealmSecretPair(diags *diag.Diagnostics, secretName string, secret types.String, versionName string, version types.Int64) {
	if secret.IsNull() != version.IsNull() {
		diags.AddError("Invalid realm secret configuration", fmt.Sprintf("%s and %s must be configured together.", secretName, versionName))
	}
	if !version.IsNull() && !version.IsUnknown() && version.ValueInt64() < 1 {
		diags.AddAttributeError(path.Root(versionName), "Invalid realm secret version", fmt.Sprintf("%s must be at least 1.", versionName))
	}
}

func requireRealmString(diags *diag.Diagnostics, name string, value types.String) {
	if value.IsNull() || (!value.IsUnknown() && strings.TrimSpace(value.ValueString()) == "") {
		diags.AddAttributeError(path.Root(name), "Missing realm field", fmt.Sprintf("%s must be configured for this realm type.", name))
	}
}

func addOpenIDRealmFields(fields *[]string, config realmModel) {
	values := []struct {
		name    string
		present bool
	}{
		{"issuer_url", !config.IssuerURL.IsNull()},
		{"client_id", !config.ClientID.IsNull()},
		{"client_key", !config.ClientKey.IsNull()},
		{"client_key_version", !config.ClientKeyVersion.IsNull()},
		{"autocreate", !config.Autocreate.IsNull()},
		{"username_claim", !config.UsernameClaim.IsNull()},
		{"scopes", !config.Scopes.IsNull()},
		{"prompt", !config.Prompt.IsNull()},
		{"query_userinfo", !config.QueryUserinfo.IsNull()},
		{"acr_values", !config.ACRValues.IsNull()},
		{"audiences", !config.Audiences.IsNull()},
	}
	for _, value := range values {
		if value.present {
			*fields = append(*fields, value.name)
		}
	}
}

func realmStateFromAPI(config Realm, prior *realmModel) realmModel {
	bindPasswordVersion := types.Int64Null()
	clientKeyVersion := types.Int64Null()
	if prior != nil {
		if !prior.BindPasswordVersion.IsUnknown() {
			bindPasswordVersion = prior.BindPasswordVersion
		}
		if !prior.ClientKeyVersion.IsUnknown() {
			clientKeyVersion = prior.ClientKeyVersion
		}
	}
	return realmModel{
		ID:                  types.StringValue(config.Realm),
		Realm:               types.StringValue(config.Realm),
		Type:                types.StringValue(config.Type),
		Comment:             stringOrNull(config.Comment),
		Default:             boolOrNull(config.Default.Ptr()),
		Server1:             stringOrNull(config.Server1),
		Server2:             stringOrNull(config.Server2),
		Port:                int64OrNull(config.Port.Ptr()),
		Mode:                stringOrNull(config.Mode),
		Verify:              boolOrNull(config.Verify.Ptr()),
		CAPath:              stringOrNull(config.CAPath),
		BaseDN:              stringOrNull(config.BaseDN),
		UserAttr:            stringOrNull(config.UserAttr),
		Domain:              stringOrNull(config.Domain),
		BindDN:              stringOrNull(config.BindDN),
		BindPassword:        types.StringNull(),
		BindPasswordVersion: bindPasswordVersion,
		IssuerURL:           stringOrNull(config.IssuerURL),
		ClientID:            stringOrNull(config.ClientID),
		ClientKey:           types.StringNull(),
		ClientKeyVersion:    clientKeyVersion,
		Autocreate:          boolOrNull(config.Autocreate.Ptr()),
		UsernameClaim:       stringOrNull(config.UsernameClaim),
		Scopes:              stringOrNull(config.Scopes),
		Prompt:              stringOrNull(config.Prompt),
		QueryUserinfo:       boolOrNull(config.QueryUserinfo.Ptr()),
		ACRValues:           stringOrNull(config.ACRValues),
		Audiences:           stringOrNull(config.Audiences),
	}
}

func realmRequestFromModels(config, prior realmModel, create bool) RealmRequest {
	request := RealmRequest{
		Realm:         config.Realm.ValueString(),
		Type:          config.Type.ValueString(),
		Comment:       stringPointer(config.Comment),
		Default:       boolPointerValue(config.Default),
		Server1:       stringPointer(config.Server1),
		Server2:       stringPointer(config.Server2),
		Port:          int64PointerValue(config.Port),
		Mode:          stringPointer(config.Mode),
		Verify:        boolPointerValue(config.Verify),
		CAPath:        stringPointer(config.CAPath),
		BaseDN:        stringPointer(config.BaseDN),
		UserAttr:      stringPointer(config.UserAttr),
		Domain:        stringPointer(config.Domain),
		BindDN:        stringPointer(config.BindDN),
		IssuerURL:     stringPointer(config.IssuerURL),
		ClientID:      stringPointer(config.ClientID),
		Autocreate:    boolPointerValue(config.Autocreate),
		UsernameClaim: stringPointer(config.UsernameClaim),
		Scopes:        stringPointer(config.Scopes),
		Prompt:        stringPointer(config.Prompt),
		QueryUserinfo: boolPointerValue(config.QueryUserinfo),
		ACRValues:     stringPointer(config.ACRValues),
		Audiences:     stringPointer(config.Audiences),
	}
	if create || realmSecretVersionChanged(config.BindPasswordVersion, prior.BindPasswordVersion) {
		request.BindPassword = stringPointer(config.BindPassword)
	}
	if create || realmSecretVersionChanged(config.ClientKeyVersion, prior.ClientKeyVersion) {
		request.ClientKey = stringPointer(config.ClientKey)
	}
	return request
}

func realmSecretVersionChanged(current, prior types.Int64) bool {
	if current.IsNull() || current.IsUnknown() {
		return false
	}
	return prior.IsNull() || prior.IsUnknown() || current.ValueInt64() != prior.ValueInt64()
}

func realmManagedFields(config realmModel) []string {
	var fields []string
	addString := func(key string, value types.String) {
		if !value.IsNull() {
			fields = append(fields, key)
		}
	}
	addInt64 := func(key string, value types.Int64) {
		if !value.IsNull() {
			fields = append(fields, key)
		}
	}
	addBool := func(key string, value types.Bool) {
		if !value.IsNull() {
			fields = append(fields, key)
		}
	}
	addString("comment", config.Comment)
	addBool("default", config.Default)
	addString("server1", config.Server1)
	addString("server2", config.Server2)
	addInt64("port", config.Port)
	addString("mode", config.Mode)
	addBool("verify", config.Verify)
	addString("capath", config.CAPath)
	addString("base_dn", config.BaseDN)
	addString("user_attr", config.UserAttr)
	addString("domain", config.Domain)
	addString("bind_dn", config.BindDN)
	if !config.BindPasswordVersion.IsNull() {
		fields = append(fields, "password")
	}
	addString("issuer-url", config.IssuerURL)
	addString("client-id", config.ClientID)
	if !config.ClientKeyVersion.IsNull() {
		fields = append(fields, "client-key")
	}
	addBool("autocreate", config.Autocreate)
	addString("username-claim", config.UsernameClaim)
	addString("scopes", config.Scopes)
	addString("prompt", config.Prompt)
	addBool("query-userinfo", config.QueryUserinfo)
	addString("acr-values", config.ACRValues)
	addString("audiences", config.Audiences)
	slices.Sort(fields)
	return fields
}

func realmDeleteKeys(config realmModel, previouslyManaged []string) []string {
	current := realmManagedFields(config)
	currentSet := make(map[string]struct{}, len(current))
	for _, key := range current {
		currentSet[key] = struct{}{}
	}
	var deleted []string
	for _, key := range previouslyManaged {
		if _, ok := currentSet[key]; !ok {
			deleted = append(deleted, key)
		}
	}
	slices.Sort(deleted)
	return deleted
}
