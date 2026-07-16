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

type HAResource struct {
	SID           string               `json:"sid"`
	State         string               `json:"state"`
	Comment       string               `json:"comment"`
	Failback      proxmoxOptionalBool  `json:"failback"`
	AutoRebalance proxmoxOptionalBool  `json:"auto-rebalance"`
	MaxRestart    proxmoxOptionalInt64 `json:"max_restart"`
	MaxRelocate   proxmoxOptionalInt64 `json:"max_relocate"`
	Digest        string               `json:"digest"`
}

type HAResourceRequest struct {
	State         string
	Comment       *string
	Failback      *bool
	AutoRebalance *bool
	MaxRestart    *int64
	MaxRelocate   *int64
	Digest        *string
	Delete        []string
}

func (c *Client) HAResources(ctx context.Context) ([]HAResource, error) {
	var resources []HAResource
	if err := c.do(ctx, http.MethodGet, "/cluster/ha/resources", nil, nil, &resources); err != nil {
		return nil, err
	}
	return resources, nil
}

func (c *Client) GetHAResource(ctx context.Context, sid string) (HAResource, error) {
	resources, err := c.HAResources(ctx)
	if err != nil {
		return HAResource{}, err
	}
	for _, resource := range resources {
		if resource.SID == sid {
			return resource, nil
		}
	}
	return HAResource{}, errNotFound
}

func (c *Client) CreateHAResource(ctx context.Context, sid string, req HAResourceRequest) error {
	form := haResourceForm(req)
	form.Set("sid", sid)
	return c.do(ctx, http.MethodPost, "/cluster/ha/resources", nil, form, nil)
}

func (c *Client) UpdateHAResource(ctx context.Context, sid string, req HAResourceRequest) error {
	form := haResourceForm(req)
	setOptionalString(form, "digest", req.Digest)
	if len(req.Delete) > 0 {
		form.Set("delete", strings.Join(sortedStrings(req.Delete), ","))
	}
	return c.do(ctx, http.MethodPut, "/cluster/ha/resources/"+url.PathEscape(sid), nil, form, nil)
}

func (c *Client) DeleteHAResource(ctx context.Context, sid string) error {
	if _, err := c.GetHAResource(ctx, sid); err != nil {
		if errors.Is(err, errNotFound) {
			return nil
		}
		return err
	}
	form := url.Values{"purge": {"0"}}
	return c.do(ctx, http.MethodDelete, "/cluster/ha/resources/"+url.PathEscape(sid), nil, form, nil)
}

func haResourceForm(req HAResourceRequest) url.Values {
	form := url.Values{}
	if req.State != "" {
		form.Set("state", req.State)
	}
	setOptionalString(form, "comment", req.Comment)
	setOptionalBool(form, "failback", req.Failback)
	setOptionalBool(form, "auto-rebalance", req.AutoRebalance)
	setOptionalInt64(form, "max_restart", req.MaxRestart)
	setOptionalInt64(form, "max_relocate", req.MaxRelocate)
	return form
}
