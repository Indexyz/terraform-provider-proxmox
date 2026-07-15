// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type Role struct {
	RoleID string
	Privs  string
}

type roleResponse struct {
	RoleID string `json:"roleid"`
	Privs  string `json:"privs"`
}

func (c *Client) GetRole(ctx context.Context, roleID string) (Role, error) {
	var raw map[string]interface{}
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/access/roles/%s", url.PathEscape(roleID)), nil, nil, &raw); err != nil {
		return Role{}, err
	}
	privs, _ := raw["privs"].(string)
	return Role{RoleID: roleID, Privs: privs}, nil
}

func (c *Client) CreateRole(ctx context.Context, roleID string, privs string) error {
	form := url.Values{}
	form.Set("roleid", roleID)
	if strings.TrimSpace(privs) != "" {
		form.Set("privs", privs)
	}
	return c.do(ctx, http.MethodPost, "/access/roles", nil, form, nil)
}

func (c *Client) UpdateRole(ctx context.Context, roleID string, privs string) error {
	form := url.Values{}
	if strings.TrimSpace(privs) != "" {
		form.Set("privs", privs)
	}
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/access/roles/%s", url.PathEscape(roleID)), nil, form, nil)
}

func (c *Client) DeleteRole(ctx context.Context, roleID string) error {
	err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/access/roles/%s", url.PathEscape(roleID)), nil, nil, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

func (c *Client) Roles(ctx context.Context) ([]Role, error) {
	var entries []roleResponse
	if err := c.do(ctx, http.MethodGet, "/access/roles", nil, nil, &entries); err != nil {
		return nil, err
	}
	result := make([]Role, 0, len(entries))
	for _, e := range entries {
		result = append(result, Role(e))
	}
	return result, nil
}
