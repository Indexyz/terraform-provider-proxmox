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

type UserToken struct {
	UserID      string
	TokenID     string
	FullTokenID string
	Value       string
	Comment     string
	Expire      proxmoxOptionalInt64
	Privsep     proxmoxOptionalBool
}

type userTokenConfigKnown struct {
	Comment string               `json:"comment"`
	Expire  proxmoxOptionalInt64 `json:"expire"`
	Privsep proxmoxOptionalBool  `json:"privsep"`
}

type userTokenCreateResponse struct {
	FullTokenID string               `json:"full-tokenid"`
	Value       string               `json:"value"`
	Info        userTokenConfigKnown `json:"info"`
}

type UserTokenRequest struct {
	Comment *string
	Expire  *int64
	Privsep *bool
}

func (c *Client) GetUserToken(ctx context.Context, userID, tokenID string) (UserToken, error) {
	var raw map[string]json.RawMessage
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/access/users/%s/token/%s", url.PathEscape(userID), url.PathEscape(tokenID)), nil, nil, &raw); err != nil {
		return UserToken{}, err
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return UserToken{}, fmt.Errorf("unable to marshal raw user token config: %w", err)
	}
	var known userTokenConfigKnown
	if err := json.Unmarshal(payload, &known); err != nil {
		return UserToken{}, fmt.Errorf("unable to decode user token config: %w", err)
	}
	return UserToken{
		UserID:  userID,
		TokenID: tokenID,
		Comment: known.Comment,
		Expire:  known.Expire,
		Privsep: known.Privsep,
	}, nil
}

func (c *Client) CreateUserToken(ctx context.Context, userID, tokenID string, req UserTokenRequest) (UserToken, error) {
	form := url.Values{}
	setOptionalString(form, "comment", req.Comment)
	setOptionalInt64(form, "expire", req.Expire)
	setOptionalBool(form, "privsep", req.Privsep)

	var resp userTokenCreateResponse
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/access/users/%s/token/%s", url.PathEscape(userID), url.PathEscape(tokenID)), nil, form, &resp); err != nil {
		return UserToken{}, err
	}
	return UserToken{
		UserID:      userID,
		TokenID:     tokenID,
		FullTokenID: resp.FullTokenID,
		Value:       resp.Value,
		Comment:     resp.Info.Comment,
		Expire:      resp.Info.Expire,
		Privsep:     resp.Info.Privsep,
	}, nil
}

func (c *Client) UpdateUserToken(ctx context.Context, userID, tokenID string, req UserTokenRequest) error {
	form := url.Values{}
	setOptionalString(form, "comment", req.Comment)
	setOptionalInt64(form, "expire", req.Expire)
	setOptionalBool(form, "privsep", req.Privsep)
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/access/users/%s/token/%s", url.PathEscape(userID), url.PathEscape(tokenID)), nil, form, nil)
}

func (c *Client) DeleteUserToken(ctx context.Context, userID, tokenID string) error {
	err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/access/users/%s/token/%s", url.PathEscape(userID), url.PathEscape(tokenID)), nil, nil, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}
