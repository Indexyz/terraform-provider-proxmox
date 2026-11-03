// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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
	Cert                types.String `tfsdk:"cert"`
	CertKey             types.String `tfsdk:"cert_key"`
	CertStorage         types.String `tfsdk:"cert_storage"`
	CertContent         types.String `tfsdk:"cert_content"`
	CertKeyContent      types.String `tfsdk:"cert_key_content"`
	Filter              types.String `tfsdk:"filter"`
	SyncAttrs           types.String `tfsdk:"sync_attributes"`
	UserClasses         types.String `tfsdk:"user_classes"`
	GroupDN             types.String `tfsdk:"group_dn"`
	GroupNameAttr       types.String `tfsdk:"group_name_attr"`
	GroupFilter         types.String `tfsdk:"group_filter"`
	GroupClasses        types.String `tfsdk:"group_classes"`
	SyncDefaults        types.String `tfsdk:"sync_defaults_options"`
	CaseSensitive       types.Bool   `tfsdk:"case_sensitive"`
	GroupsClaim         types.String `tfsdk:"groups_claim"`
	GroupsAutocreate    types.Bool   `tfsdk:"groups_autocreate"`
	GroupsOverwrite     types.Bool   `tfsdk:"groups_overwrite"`
	TFA                 types.Object `tfsdk:"tfa"`
}

func realmTFAAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"type":   types.StringType,
		"id":     types.StringType,
		"key":    types.StringType,
		"url":    types.StringType,
		"digits": types.Int64Type,
		"step":   types.Int64Type,
	}
}

var realmSyncAttributesPattern = regexp.MustCompile(`^\w+=[^,]+(,\s*\w+=[^,]+)*$`)
var realmAttrListPattern = regexp.MustCompile(`^[\w.-]+(,\s*[\w.-]+)*$`)

func realmTFAAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		Computed:            true,
		MarkdownDescription: "Realm-level two-factor requirement. Proxmox stores this as a property string and only LDAP/AD realms support it; the Yubico `key` is kept in `/etc/pve/domains.cfg` in plaintext.",
		Attributes: map[string]schema.Attribute{
			"type":   schema.StringAttribute{Required: true, MarkdownDescription: "Second factor: `oath` (TOTP) or `yubico`."},
			"id":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Yubico API ID."},
			"key":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Yubico API key."},
			"url":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Yubico API URL."},
			"digits": schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "TOTP digit count between 6 and 8."},
			"step":   schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "TOTP time period in seconds, at least 10."},
		},
	}
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
	validateRealmCertContent(&diags, config)
	validateRealmSecretPair(&diags, "client_key", config.ClientKey, "client_key_version", config.ClientKeyVersion)
	validateRealmTextLimits(&diags, config)
	validateRealmSyncLists(&diags, config)
	validateRealmTFA(&diags, config.TFA)
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
		addOpenIDGroupFields(&incompatible, config)
	case "ad":
		requireRealmString(&diags, "server1", config.Server1)
		requireRealmString(&diags, "domain", config.Domain)
		addOpenIDRealmFields(&incompatible, config)
		addOpenIDGroupFields(&incompatible, config)
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
		addDirectoryRealmFields(&incompatible, config)
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

func validateRealmTextLimits(diags *diag.Diagnostics, config realmModel) {
	for _, field := range []struct {
		name  string
		value types.String
		max   int
	}{
		{"filter", config.Filter, 2048},
		{"group_filter", config.GroupFilter, 2048},
		{"group_dn", config.GroupDN, 256},
		{"group_name_attr", config.GroupNameAttr, 256},
	} {
		if field.value.IsNull() || field.value.IsUnknown() {
			continue
		}
		if len(field.value.ValueString()) > field.max {
			diags.AddAttributeError(path.Root(field.name), "Invalid realm field length", fmt.Sprintf("%s must not exceed %d characters.", field.name, field.max))
		}
	}
}

func validateRealmSyncLists(diags *diag.Diagnostics, config realmModel) {
	if !config.SyncAttrs.IsNull() && !config.SyncAttrs.IsUnknown() && !realmSyncAttributesPattern.MatchString(config.SyncAttrs.ValueString()) {
		diags.AddAttributeError(path.Root("sync_attributes"), "Invalid sync attribute mapping", "sync_attributes must use `target=ldapAttribute` pairs separated by commas, for example `email=mail,firstname=givenName`.")
	}
	for _, field := range []struct {
		name  string
		value types.String
	}{
		{"user_classes", config.UserClasses},
		{"group_classes", config.GroupClasses},
	} {
		if field.value.IsNull() || field.value.IsUnknown() {
			continue
		}
		if !realmAttrListPattern.MatchString(field.value.ValueString()) {
			diags.AddAttributeError(path.Root(field.name), "Invalid LDAP object classes", fmt.Sprintf("%s must be a comma separated list of attribute names.", field.name))
		}
	}
}

func validateRealmTFA(diags *diag.Diagnostics, value types.Object) {
	if value.IsNull() || value.IsUnknown() {
		return
	}
	attrs := value.Attributes()
	tfaType, _ := attrs["type"].(types.String)
	if tfaType.IsNull() || tfaType.IsUnknown() {
		diags.AddAttributeError(path.Root("tfa").AtName("type"), "Missing TFA type", "tfa.type must be `oath` or `yubico`.")
		return
	}
	if !slices.Contains([]string{"oath", "yubico"}, tfaType.ValueString()) {
		diags.AddAttributeError(path.Root("tfa").AtName("type"), "Invalid TFA type", "tfa.type must be `oath` or `yubico`.")
		return
	}

	digits, _ := attrs["digits"].(types.Int64)
	step, _ := attrs["step"].(types.Int64)
	if !digits.IsNull() && !digits.IsUnknown() && (digits.ValueInt64() < 6 || digits.ValueInt64() > 8) {
		diags.AddAttributeError(path.Root("tfa").AtName("digits"), "Invalid TOTP digit count", "digits must be between 6 and 8.")
	}
	if !step.IsNull() && !step.IsUnknown() && step.ValueInt64() < 10 {
		diags.AddAttributeError(path.Root("tfa").AtName("step"), "Invalid TOTP time period", "step must be at least 10 seconds.")
	}

	var incompatible []string
	for _, name := range []string{"id", "key", "url"} {
		if attribute, ok := attrs[name].(types.String); ok && !attribute.IsNull() && !attribute.IsUnknown() {
			incompatible = append(incompatible, name)
		}
	}
	if tfaType.ValueString() == "yubico" {
		if !digits.IsNull() && !digits.IsUnknown() {
			incompatible = append(incompatible, "digits")
		}
		if !step.IsNull() && !step.IsUnknown() {
			incompatible = append(incompatible, "step")
		}
	}
	if len(incompatible) > 0 {
		diags.AddError("Invalid TFA fields", fmt.Sprintf("tfa.type %q does not support: %s", tfaType.ValueString(), strings.Join(incompatible, ", ")))
	}
}

func addOpenIDGroupFields(fields *[]string, config realmModel) {
	for _, value := range []struct {
		name    string
		present bool
	}{
		{"groups_claim", !config.GroupsClaim.IsNull()},
		{"groups_autocreate", !config.GroupsAutocreate.IsNull()},
		{"groups_overwrite", !config.GroupsOverwrite.IsNull()},
	} {
		if value.present {
			*fields = append(*fields, value.name)
		}
	}
}

func addDirectoryRealmFields(fields *[]string, config realmModel) {
	for _, value := range []struct {
		name    string
		present bool
	}{
		{"cert", !config.Cert.IsNull()},
		{"cert_key", !config.CertKey.IsNull()},
		{"cert_storage", !config.CertStorage.IsNull()},
		{"cert_content", !config.CertContent.IsNull()},
		{"cert_key_content", !config.CertKeyContent.IsNull()},
		{"filter", !config.Filter.IsNull()},
		{"sync_attributes", !config.SyncAttrs.IsNull()},
		{"user_classes", !config.UserClasses.IsNull()},
		{"group_dn", !config.GroupDN.IsNull()},
		{"group_name_attr", !config.GroupNameAttr.IsNull()},
		{"group_filter", !config.GroupFilter.IsNull()},
		{"group_classes", !config.GroupClasses.IsNull()},
		{"sync_defaults_options", !config.SyncDefaults.IsNull()},
		{"case_sensitive", !config.CaseSensitive.IsNull()},
		{"tfa", !config.TFA.IsNull()},
	} {
		if value.present {
			*fields = append(*fields, value.name)
		}
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

func realmStateFromAPI(config Realm, prior *realmModel) (realmModel, diag.Diagnostics) {
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
	tfa, diags := realmTFAObject(config.TFA)
	certStorage := types.StringNull()
	certContent := types.StringNull()
	certKeyContent := types.StringNull()
	if prior != nil {
		certStorage = prior.CertStorage
		certContent = prior.CertContent
		certKeyContent = prior.CertKeyContent
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
		Cert:                stringOrNull(config.Cert),
		CertKey:             stringOrNull(config.CertKey),
		Filter:              stringOrNull(config.Filter),
		SyncAttrs:           stringOrNull(config.SyncAttrs),
		UserClasses:         stringOrNull(config.UserClasses),
		GroupDN:             stringOrNull(config.GroupDN),
		GroupNameAttr:       stringOrNull(config.GroupNameAttr),
		GroupFilter:         stringOrNull(config.GroupFilter),
		GroupClasses:        stringOrNull(config.GroupClasses),
		SyncDefaults:        stringOrNull(config.SyncDefaults),
		CaseSensitive:       boolOrNull(config.CaseSensitive.Ptr()),
		GroupsClaim:         stringOrNull(config.GroupsClaim),
		GroupsAutocreate:    boolOrNull(config.GroupsCreate.Ptr()),
		GroupsOverwrite:     boolOrNull(config.GroupsOverwr.Ptr()),
		CertStorage:         certStorage,
		CertContent:         certContent,
		CertKeyContent:      certKeyContent,
		TFA:                 tfa,
	}, diags
}

// realmTFAObject parses Proxmox' TFA property string into the typed block.
// Unknown keys are errors so a newer Proxmox field never disappears silently.
func realmTFAObject(raw string) (types.Object, diag.Diagnostics) {
	if raw == "" {
		return types.ObjectNull(realmTFAAttrTypes()), nil
	}
	values := map[string]attr.Value{
		"type":   types.StringNull(),
		"id":     types.StringNull(),
		"key":    types.StringNull(),
		"url":    types.StringNull(),
		"digits": types.Int64Null(),
		"step":   types.Int64Null(),
	}
	var diags diag.Diagnostics
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		key, value, found := strings.Cut(pair, "=")
		if !found {
			diags.AddError("Unable to Decode Proxmox Realm TFA Config", fmt.Sprintf("unexpected TFA entry %q.", pair))
			return types.ObjectNull(realmTFAAttrTypes()), diags
		}
		switch key {
		case "type", "id", "key", "url":
			values[key] = types.StringValue(value)
		case "digits", "step":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				diags.AddError("Unable to Decode Proxmox Realm TFA Config", fmt.Sprintf("unable to decode TFA %s value %q: %s", key, value, err))
				return types.ObjectNull(realmTFAAttrTypes()), diags
			}
			values[key] = types.Int64Value(parsed)
		default:
			diags.AddError("Unable to Decode Proxmox Realm TFA Config", fmt.Sprintf("unknown TFA entry %q.", key))
			return types.ObjectNull(realmTFAAttrTypes()), diags
		}
	}
	if values["type"].IsNull() {
		diags.AddError("Unable to Decode Proxmox Realm TFA Config", fmt.Sprintf("TFA config %q has no type.", raw))
		return types.ObjectNull(realmTFAAttrTypes()), diags
	}
	object, objectDiags := types.ObjectValue(realmTFAAttrTypes(), values)
	diags.Append(objectDiags...)
	return object, diags
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
		Cert:          stringPointer(config.Cert),
		CertKey:       stringPointer(config.CertKey),
		Filter:        stringPointer(config.Filter),
		SyncAttrs:     stringPointer(config.SyncAttrs),
		UserClasses:   stringPointer(config.UserClasses),
		GroupDN:       stringPointer(config.GroupDN),
		GroupNameAttr: stringPointer(config.GroupNameAttr),
		GroupFilter:   stringPointer(config.GroupFilter),
		GroupClasses:  stringPointer(config.GroupClasses),
		SyncDefaults:  stringPointer(config.SyncDefaults),
		CaseSensitive: boolPointerValue(config.CaseSensitive),
		GroupsClaim:   stringPointer(config.GroupsClaim),
		GroupsCreate:  boolPointerValue(config.GroupsAutocreate),
		GroupsOverwr:  boolPointerValue(config.GroupsOverwrite),
		TFA:           realmTFAString(config.TFA),
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
	addString("cert", config.Cert)
	addString("certkey", config.CertKey)
	if isConfiguredValue(config.CertContent) {
		fields = append(fields, "cert", "certkey")
	}
	addString("filter", config.Filter)
	addString("sync_attributes", config.SyncAttrs)
	addString("user_classes", config.UserClasses)
	addString("group_dn", config.GroupDN)
	addString("group_name_attr", config.GroupNameAttr)
	addString("group_filter", config.GroupFilter)
	addString("group_classes", config.GroupClasses)
	addString("sync-defaults-options", config.SyncDefaults)
	addBool("case-sensitive", config.CaseSensitive)
	addString("groups-claim", config.GroupsClaim)
	addBool("groups-autocreate", config.GroupsAutocreate)
	addBool("groups-overwrite", config.GroupsOverwrite)
	if !config.TFA.IsNull() && !config.TFA.IsUnknown() {
		fields = append(fields, "tfa")
	}
	slices.Sort(fields)
	return fields
}

// realmTFAString serializes the typed TFA block into Proxmox' property string.
// Validation already rejected unknown variants, so this only assembles set keys.
func realmTFAString(value types.Object) *string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	attrs := value.Attributes()
	parts := make([]string, 0, 6)
	for _, key := range []string{"type", "id", "key", "url", "digits", "step"} {
		switch attribute := attrs[key].(type) {
		case types.String:
			if !attribute.IsNull() && !attribute.IsUnknown() {
				parts = append(parts, key+"="+attribute.ValueString())
			}
		case types.Int64:
			if !attribute.IsNull() && !attribute.IsUnknown() {
				parts = append(parts, fmt.Sprintf("%s=%d", key, attribute.ValueInt64()))
			}
		}
	}
	joined := strings.Join(parts, ",")
	if joined == "" {
		return nil
	}
	return &joined
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

// validateRealmCertContent enforces the mutual exclusion between certificate
// paths and uploaded certificate content.
func validateRealmCertContent(diags *diag.Diagnostics, config realmModel) {
	hasContent := isConfiguredValue(config.CertContent)
	hasKeyContent := isConfiguredValue(config.CertKeyContent)
	hasStorage := isConfiguredValue(config.CertStorage)
	// Terraform evaluates the config again with unknown values; only judge
	// combinations where all three fields are known.
	if config.CertContent.IsUnknown() || config.CertKeyContent.IsUnknown() || config.CertStorage.IsUnknown() {
		return
	}
	if hasContent != hasKeyContent {
		diags.AddError("Incomplete realm client certificate", "cert_content and cert_key_content must be configured together.")
	}
	if (hasContent || hasKeyContent) && !hasStorage {
		diags.AddAttributeError(path.Root("cert_storage"), "Missing certificate storage", "cert_storage is required when certificate content is uploaded.")
	}
	if hasStorage && !hasContent && !hasKeyContent {
		diags.AddAttributeError(path.Root("cert_storage"), "Unexpected certificate storage", "cert_storage is only used together with cert_content.")
	}
	if (hasContent || hasKeyContent) && isConfiguredValue(config.Cert) {
		diags.AddAttributeError(path.Root("cert"), "Conflicting client certificate", "configure either cert/cert_key paths or cert_content/cert_key_content, not both.")
	}
}
