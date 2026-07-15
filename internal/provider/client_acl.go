// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/url"
)

type ACLEntry struct {
	Path      string
	Propagate proxmoxOptionalBool
	RoleID    string
	Type      string // "user" or "group"
	UGID      string // user or group identifier
}

type aclListEntry struct {
	Path      string              `json:"path"`
	Propagate proxmoxOptionalBool `json:"propagate"`
	RoleID    string              `json:"roleid"`
	Type      string              `json:"type"`
	UGID      string              `json:"ugid"`
}

type ACLRequest struct {
	Path      string
	Roles     string
	Users     string
	Groups    string
	Propagate *bool
	Delete    bool
}

func (c *Client) GetACL(ctx context.Context) ([]ACLEntry, error) {
	var entries []aclListEntry
	if err := c.do(ctx, http.MethodGet, "/access/acl", nil, nil, &entries); err != nil {
		return nil, err
	}
	result := make([]ACLEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, ACLEntry(e))
	}
	return result, nil
}

func (c *Client) SetACL(ctx context.Context, req ACLRequest) error {
	form := url.Values{}
	form.Set("path", req.Path)
	form.Set("roles", req.Roles)
	setOptionalString(form, "users", stringPtrIfNotEmpty(req.Users))
	setOptionalString(form, "groups", stringPtrIfNotEmpty(req.Groups))
	if req.Propagate != nil {
		form.Set("propagate", boolToFormValue(*req.Propagate))
	}
	if req.Delete {
		form.Set("delete", "1")
	}
	return c.do(ctx, http.MethodPut, "/access/acl", nil, form, nil)
}

func boolToFormValue(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
