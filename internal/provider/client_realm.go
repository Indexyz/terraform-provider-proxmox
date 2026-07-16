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
	Digest        *string
	Delete        []string
}

func (r RealmRequest) IsEmpty() bool {
	return r.Comment == nil && r.Default == nil && r.Server1 == nil && r.Server2 == nil &&
		r.Port == nil && r.Mode == nil && r.Verify == nil && r.CAPath == nil && r.BaseDN == nil &&
		r.UserAttr == nil && r.Domain == nil && r.BindDN == nil && r.BindPassword == nil &&
		r.IssuerURL == nil && r.ClientID == nil && r.ClientKey == nil && r.Autocreate == nil &&
		r.UsernameClaim == nil && r.Scopes == nil && r.Prompt == nil && r.QueryUserinfo == nil &&
		r.ACRValues == nil && r.Audiences == nil && len(r.Delete) == 0
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
	return form
}
