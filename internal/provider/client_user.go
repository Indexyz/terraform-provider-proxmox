// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

type User struct {
	UserID    string
	Comment   string
	Email     string
	Enable    proxmoxOptionalBool
	Expire    proxmoxOptionalInt64
	Firstname string
	Lastname  string
	Groups    string
	Keys      string
}

type userConfigKnown struct {
	Comment   string               `json:"comment"`
	Email     string               `json:"email"`
	Enable    proxmoxOptionalBool  `json:"enable"`
	Expire    proxmoxOptionalInt64 `json:"expire"`
	Firstname string               `json:"firstname"`
	Lastname  string               `json:"lastname"`
	Groups    string               `json:"groups"`
	Keys      string               `json:"keys"`
}

func (c *Client) GetUser(ctx context.Context, userID string) (User, error) {
	var raw map[string]json.RawMessage
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/access/users/%s", url.PathEscape(userID)), nil, nil, &raw); err != nil {
		return User{}, err
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return User{}, fmt.Errorf("unable to marshal raw user config: %w", err)
	}
	var known userConfigKnown
	if err := json.Unmarshal(payload, &known); err != nil {
		return User{}, fmt.Errorf("unable to decode user config: %w", err)
	}
	return User{
		UserID:    userID,
		Comment:   known.Comment,
		Email:     known.Email,
		Enable:    known.Enable,
		Expire:    known.Expire,
		Firstname: known.Firstname,
		Lastname:  known.Lastname,
		Groups:    known.Groups,
		Keys:      known.Keys,
	}, nil
}

type UserRequest struct {
	Comment   *string
	Email     *string
	Enable    *bool
	Expire    *int64
	Firstname *string
	Lastname  *string
	Groups    *string
	Keys      *string
	Password  *string
}

func (c *Client) CreateUser(ctx context.Context, userID string, req UserRequest) error {
	form := url.Values{}
	form.Set("userid", userID)
	setOptionalString(form, "comment", req.Comment)
	setOptionalString(form, "email", req.Email)
	setOptionalBool(form, "enable", req.Enable)
	setOptionalInt64(form, "expire", req.Expire)
	setOptionalString(form, "firstname", req.Firstname)
	setOptionalString(form, "lastname", req.Lastname)
	setOptionalString(form, "groups", req.Groups)
	setOptionalString(form, "keys", req.Keys)
	setOptionalString(form, "password", req.Password)
	return c.do(ctx, http.MethodPost, "/access/users", nil, form, nil)
}

func (c *Client) UpdateUser(ctx context.Context, userID string, req UserRequest) error {
	form := url.Values{}
	setOptionalString(form, "comment", req.Comment)
	setOptionalString(form, "email", req.Email)
	setOptionalBool(form, "enable", req.Enable)
	setOptionalInt64(form, "expire", req.Expire)
	setOptionalString(form, "firstname", req.Firstname)
	setOptionalString(form, "lastname", req.Lastname)
	setOptionalString(form, "groups", req.Groups)
	setOptionalString(form, "keys", req.Keys)
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/access/users/%s", url.PathEscape(userID)), nil, form, nil)
}

func (c *Client) DeleteUser(ctx context.Context, userID string) error {
	err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/access/users/%s", url.PathEscape(userID)), nil, nil, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

type userIndexEntry struct {
	UserID    string               `json:"userid"`
	Comment   string               `json:"comment"`
	Email     string               `json:"email"`
	Firstname string               `json:"firstname"`
	Lastname  string               `json:"lastname"`
	Enable    proxmoxOptionalBool  `json:"enable"`
	Expire    proxmoxOptionalInt64 `json:"expire"`
	Groups    string               `json:"groups"`
}

func (c *Client) Users(ctx context.Context) ([]User, error) {
	var entries []userIndexEntry
	if err := c.do(ctx, http.MethodGet, "/access/users", nil, nil, &entries); err != nil {
		return nil, err
	}
	result := make([]User, 0, len(entries))
	for _, e := range entries {
		result = append(result, User{
			UserID:    e.UserID,
			Comment:   e.Comment,
			Email:     e.Email,
			Enable:    e.Enable,
			Expire:    e.Expire,
			Firstname: e.Firstname,
			Lastname:  e.Lastname,
			Groups:    e.Groups,
		})
	}
	return result, nil
}
