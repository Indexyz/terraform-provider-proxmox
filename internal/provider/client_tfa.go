// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// UserTFA is one TFA entry of a Proxmox user. Proxmox never returns the secret
// material of an entry, only its public metadata.
type UserTFA struct {
	ID          string               `json:"id"`
	Type        string               `json:"type"`
	Description string               `json:"description"`
	Created     proxmoxOptionalInt64 `json:"created"`
	Enable      proxmoxOptionalBool  `json:"enable"`
}

type UserTFACreateRequest struct {
	Type        string
	Description *string
	TOTP        *string
	Value       *string
	Password    *string
}

type UserTFAUpdateRequest struct {
	Description *string
	Enable      *bool
	Password    *string
}

type UserTFACreateResult struct {
	ID       string   `json:"id"`
	Recovery []string `json:"recovery"`
}

func userTFAPath(userID string) string {
	return "/access/tfa/" + url.PathEscape(userID)
}

func (c *Client) GetUserTFA(ctx context.Context, userID, entryID string) (UserTFA, error) {
	var entry UserTFA
	if err := c.do(ctx, http.MethodGet, userTFAPath(userID)+"/"+url.PathEscape(entryID), nil, nil, &entry); err != nil {
		return UserTFA{}, err
	}
	if entry.ID == "" {
		entry.ID = entryID
	}
	return entry, nil
}

func (c *Client) CreateUserTFA(ctx context.Context, userID string, req UserTFACreateRequest) (UserTFACreateResult, error) {
	form := url.Values{"type": {req.Type}}
	setOptionalString(form, "description", req.Description)
	setOptionalString(form, "totp", req.TOTP)
	setOptionalString(form, "value", req.Value)
	setOptionalString(form, "password", req.Password)
	var result UserTFACreateResult
	if err := c.do(ctx, http.MethodPost, userTFAPath(userID), nil, form, &result); err != nil {
		return UserTFACreateResult{}, err
	}
	if result.ID == "" {
		return UserTFACreateResult{}, fmt.Errorf("proxmox did not return a TFA entry id")
	}
	return result, nil
}

func (c *Client) UpdateUserTFA(ctx context.Context, userID, entryID string, req UserTFAUpdateRequest) error {
	form := url.Values{}
	setOptionalString(form, "description", req.Description)
	setOptionalBool(form, "enable", req.Enable)
	setOptionalString(form, "password", req.Password)
	return c.do(ctx, http.MethodPut, userTFAPath(userID)+"/"+url.PathEscape(entryID), nil, form, nil)
}

func (c *Client) DeleteUserTFA(ctx context.Context, userID, entryID string, password *string) error {
	form := url.Values{}
	setOptionalString(form, "password", password)
	err := c.do(ctx, http.MethodDelete, userTFAPath(userID)+"/"+url.PathEscape(entryID), nil, form, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

// UnlockUserTFA clears the TFA lockout of a user.
func (c *Client) UnlockUserTFA(ctx context.Context, userID string) (bool, error) {
	form := url.Values{"userid": {userID}}
	var result proxmoxOptionalInt64
	err := c.do(ctx, http.MethodPut, "/access/users/"+url.PathEscape(userID)+"/unlock-tfa", nil, form, &result)
	if err != nil {
		return false, err
	}
	return result.Ptr() != nil && *result.Ptr() != 0, nil
}
