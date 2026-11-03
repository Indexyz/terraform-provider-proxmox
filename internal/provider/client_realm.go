// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

type Realm struct {
	Realm         string               `json:"realm"`
	Type          string               `json:"type"`
	Comment       string               `json:"comment"`
	Default       proxmoxOptionalBool  `json:"default"`
	Server1       string               `json:"server1"`
	Server2       string               `json:"server2"`
	Port          proxmoxOptionalInt64 `json:"port"`
	Mode          string               `json:"mode"`
	Verify        proxmoxOptionalBool  `json:"verify"`
	CAPath        string               `json:"capath"`
	BaseDN        string               `json:"base_dn"`
	UserAttr      string               `json:"user_attr"`
	Domain        string               `json:"domain"`
	BindDN        string               `json:"bind_dn"`
	IssuerURL     string               `json:"issuer-url"`
	ClientID      string               `json:"client-id"`
	Autocreate    proxmoxOptionalBool  `json:"autocreate"`
	UsernameClaim string               `json:"username-claim"`
	Scopes        string               `json:"scopes"`
	Prompt        string               `json:"prompt"`
	QueryUserinfo proxmoxOptionalBool  `json:"query-userinfo"`
	ACRValues     string               `json:"acr-values"`
	Audiences     string               `json:"audiences"`
	Cert          string               `json:"cert"`
	CertKey       string               `json:"certkey"`
	Filter        string               `json:"filter"`
	SyncAttrs     string               `json:"sync_attributes"`
	UserClasses   string               `json:"user_classes"`
	GroupDN       string               `json:"group_dn"`
	GroupNameAttr string               `json:"group_name_attr"`
	GroupFilter   string               `json:"group_filter"`
	GroupClasses  string               `json:"group_classes"`
	SyncDefaults  string               `json:"sync-defaults-options"`
	CaseSensitive proxmoxOptionalBool  `json:"case-sensitive"`
	GroupsClaim   string               `json:"groups-claim"`
	GroupsCreate  proxmoxOptionalBool  `json:"groups-autocreate"`
	GroupsOverwr  proxmoxOptionalBool  `json:"groups-overwrite"`
	TFA           string               `json:"tfa"`
	Digest        string               `json:"digest"`
}

type RealmRequest struct {
	Realm         string
	Type          string
	Comment       *string
	Default       *bool
	Server1       *string
	Server2       *string
	Port          *int64
	Mode          *string
	Verify        *bool
	CAPath        *string
	BaseDN        *string
	UserAttr      *string
	Domain        *string
	BindDN        *string
	BindPassword  *string
	IssuerURL     *string
	ClientID      *string
	ClientKey     *string
	Autocreate    *bool
	UsernameClaim *string
	Scopes        *string
	Prompt        *string
	QueryUserinfo *bool
	ACRValues     *string
	Audiences     *string
	Cert          *string
	CertKey       *string
	Filter        *string
	SyncAttrs     *string
	UserClasses   *string
	GroupDN       *string
	GroupNameAttr *string
	GroupFilter   *string
	GroupClasses  *string
	SyncDefaults  *string
	CaseSensitive *bool
	GroupsClaim   *string
	GroupsCreate  *bool
	GroupsOverwr  *bool
	TFA           *string
	Digest        *string
	Delete        []string
}

func (r RealmRequest) IsEmpty() bool {
	return r.Comment == nil && r.Default == nil && r.Server1 == nil && r.Server2 == nil &&
		r.Port == nil && r.Mode == nil && r.Verify == nil && r.CAPath == nil && r.BaseDN == nil &&
		r.UserAttr == nil && r.Domain == nil && r.BindDN == nil && r.BindPassword == nil &&
		r.IssuerURL == nil && r.ClientID == nil && r.ClientKey == nil && r.Autocreate == nil &&
		r.UsernameClaim == nil && r.Scopes == nil && r.Prompt == nil && r.QueryUserinfo == nil &&
		r.ACRValues == nil && r.Audiences == nil && r.Cert == nil && r.CertKey == nil &&
		r.Filter == nil && r.SyncAttrs == nil && r.UserClasses == nil && r.GroupDN == nil &&
		r.GroupNameAttr == nil && r.GroupFilter == nil && r.GroupClasses == nil &&
		r.SyncDefaults == nil && r.CaseSensitive == nil && r.GroupsClaim == nil &&
		r.GroupsCreate == nil && r.GroupsOverwr == nil && r.TFA == nil && len(r.Delete) == 0
}

// RealmSummary is one entry of the world-readable realm index used by the login
// box. It carries no secrets.
type RealmSummary struct {
	Realm   string              `json:"realm"`
	Type    string              `json:"type"`
	Comment string              `json:"comment"`
	Default proxmoxOptionalBool `json:"default"`
	TFA     string              `json:"tfa"`
}

func (c *Client) Realms(ctx context.Context) ([]RealmSummary, error) {
	var realms []RealmSummary
	if err := c.do(ctx, http.MethodGet, "/access/domains", nil, nil, &realms); err != nil {
		return nil, err
	}
	return realms, nil
}

func (c *Client) GetRealm(ctx context.Context, realm string) (Realm, error) {
	var config Realm
	if err := c.do(ctx, http.MethodGet, "/access/domains/"+url.PathEscape(realm), nil, nil, &config); err != nil {
		return Realm{}, err
	}
	config.Realm = realm
	return config, nil
}

func (c *Client) CreateRealm(ctx context.Context, req RealmRequest) error {
	form := realmForm(req)
	form.Set("realm", req.Realm)
	form.Set("type", req.Type)
	return c.do(ctx, http.MethodPost, "/access/domains", nil, form, nil)
}

func (c *Client) UpdateRealm(ctx context.Context, realm string, req RealmRequest) error {
	form := realmForm(req)
	setOptionalString(form, "digest", req.Digest)
	if len(req.Delete) > 0 {
		form.Set("delete", strings.Join(sortedStrings(req.Delete), ","))
	}
	return c.do(ctx, http.MethodPut, "/access/domains/"+url.PathEscape(realm), nil, form, nil)
}

func (c *Client) DeleteRealm(ctx context.Context, realm string) error {
	err := c.do(ctx, http.MethodDelete, "/access/domains/"+url.PathEscape(realm), nil, nil, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

func realmForm(req RealmRequest) url.Values {
	form := url.Values{}
	setOptionalString(form, "comment", req.Comment)
	setOptionalBool(form, "default", req.Default)
	setOptionalString(form, "server1", req.Server1)
	setOptionalString(form, "server2", req.Server2)
	setOptionalInt64(form, "port", req.Port)
	setOptionalString(form, "mode", req.Mode)
	setOptionalBool(form, "verify", req.Verify)
	setOptionalString(form, "capath", req.CAPath)
	setOptionalString(form, "base_dn", req.BaseDN)
	setOptionalString(form, "user_attr", req.UserAttr)
	setOptionalString(form, "domain", req.Domain)
	setOptionalString(form, "bind_dn", req.BindDN)
	setOptionalString(form, "password", req.BindPassword)
	setOptionalString(form, "issuer-url", req.IssuerURL)
	setOptionalString(form, "client-id", req.ClientID)
	setOptionalString(form, "client-key", req.ClientKey)
	setOptionalBool(form, "autocreate", req.Autocreate)
	setOptionalString(form, "username-claim", req.UsernameClaim)
	setOptionalString(form, "scopes", req.Scopes)
	setOptionalString(form, "prompt", req.Prompt)
	setOptionalBool(form, "query-userinfo", req.QueryUserinfo)
	setOptionalString(form, "acr-values", req.ACRValues)
	setOptionalString(form, "audiences", req.Audiences)
	setOptionalString(form, "cert", req.Cert)
	setOptionalString(form, "certkey", req.CertKey)
	setOptionalString(form, "filter", req.Filter)
	setOptionalString(form, "sync_attributes", req.SyncAttrs)
	setOptionalString(form, "user_classes", req.UserClasses)
	setOptionalString(form, "group_dn", req.GroupDN)
	setOptionalString(form, "group_name_attr", req.GroupNameAttr)
	setOptionalString(form, "group_filter", req.GroupFilter)
	setOptionalString(form, "group_classes", req.GroupClasses)
	setOptionalString(form, "sync-defaults-options", req.SyncDefaults)
	setOptionalBool(form, "case-sensitive", req.CaseSensitive)
	setOptionalString(form, "groups-claim", req.GroupsClaim)
	setOptionalBool(form, "groups-autocreate", req.GroupsCreate)
	setOptionalBool(form, "groups-overwrite", req.GroupsOverwr)
	setOptionalString(form, "tfa", req.TFA)
	return form
}

// SDN-style sync options for LDAP/AD realms.
type RealmSyncRequest struct {
	Realm          string
	Scope          string
	RemoveVanished string
	EnableNew      *bool
	DryRun         *bool
}

// SyncRealm starts an LDAP/AD user/group sync and returns the worker UPID.
func (c *Client) SyncRealm(ctx context.Context, req RealmSyncRequest) (string, error) {
	form := url.Values{
		"scope":           {req.Scope},
		"remove-vanished": {req.RemoveVanished},
	}
	setOptionalBool(form, "enable-new", req.EnableNew)
	setOptionalBool(form, "dry-run", req.DryRun)
	var upid string
	if err := c.do(ctx, http.MethodPost, "/access/domains/"+url.PathEscape(req.Realm)+"/sync", nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "realm sync"); err != nil {
		return "", err
	}
	return upid, nil
}
