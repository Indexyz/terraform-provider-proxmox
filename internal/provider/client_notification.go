// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

type NotificationEndpoint struct {
	Name       string              `json:"name"`
	Comment    string              `json:"comment"`
	Author     string              `json:"author"`
	Server     string              `json:"server"`
	From       string              `json:"from-address"`
	Mode       string              `json:"mode"`
	Username   string              `json:"username"`
	Method     string              `json:"method"`
	URL        string              `json:"url"`
	Body       string              `json:"body"`
	Mailto     []string            `json:"mailto"`
	MailtoUser []string            `json:"mailto-user"`
	Header     []string            `json:"header"`
	Secret     []string            `json:"secret"`
	Port       int64               `json:"port"`
	Disable    proxmoxOptionalBool `json:"disable"`
	Digest     string              `json:"digest"`
}

func notificationEndpointPath(kind string) string {
	return "/cluster/notifications/endpoints/" + kind
}

func (c *Client) NotificationEndpoints(ctx context.Context, kind string) ([]NotificationEndpoint, error) {
	var entries []NotificationEndpoint
	if err := c.do(ctx, http.MethodGet, notificationEndpointPath(kind), nil, nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (c *Client) GetNotificationEndpoint(ctx context.Context, kind, name string) (NotificationEndpoint, error) {
	entries, err := c.NotificationEndpoints(ctx, kind)
	if err != nil {
		return NotificationEndpoint{}, err
	}
	for _, entry := range entries {
		if entry.Name == name {
			return entry, nil
		}
	}
	return NotificationEndpoint{}, errNotFound
}

func (c *Client) CreateNotificationEndpoint(ctx context.Context, kind string, form url.Values) error {
	return c.do(ctx, http.MethodPost, notificationEndpointPath(kind), nil, form, nil)
}

func (c *Client) UpdateNotificationEndpoint(ctx context.Context, kind, name string, form url.Values, digest string) error {
	if digest != "" {
		form.Set("digest", digest)
	}
	return c.do(ctx, http.MethodPut, notificationEndpointPath(kind)+"/"+url.PathEscape(name), nil, form, nil)
}

func (c *Client) DeleteNotificationEndpoint(ctx context.Context, kind, name string) error {
	// The endpoint DELETE schema has no digest parameter.
	err := c.do(ctx, http.MethodDelete, notificationEndpointPath(kind)+"/"+url.PathEscape(name), nil, nil, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

type NotificationMatcher struct {
	Name          string              `json:"name"`
	Comment       string              `json:"comment"`
	Mode          string              `json:"mode"`
	Target        []string            `json:"target"`
	MatchField    []string            `json:"match-field"`
	MatchSeverity []string            `json:"match-severity"`
	MatchCalendar []string            `json:"match-calendar"`
	InvertMatch   proxmoxOptionalBool `json:"invert-match"`
	Disable       proxmoxOptionalBool `json:"disable"`
	Digest        string              `json:"digest"`
}

func (c *Client) NotificationMatchers(ctx context.Context) ([]NotificationMatcher, error) {
	var entries []NotificationMatcher
	if err := c.do(ctx, http.MethodGet, "/cluster/notifications/matchers", nil, nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (c *Client) GetNotificationMatcher(ctx context.Context, name string) (NotificationMatcher, error) {
	entries, err := c.NotificationMatchers(ctx)
	if err != nil {
		return NotificationMatcher{}, err
	}
	for _, entry := range entries {
		if entry.Name == name {
			return entry, nil
		}
	}
	return NotificationMatcher{}, errNotFound
}

func (c *Client) CreateNotificationMatcher(ctx context.Context, form url.Values) error {
	return c.do(ctx, http.MethodPost, "/cluster/notifications/matchers", nil, form, nil)
}

func (c *Client) UpdateNotificationMatcher(ctx context.Context, name string, form url.Values, digest string) error {
	if digest != "" {
		form.Set("digest", digest)
	}
	return c.do(ctx, http.MethodPut, "/cluster/notifications/matchers/"+url.PathEscape(name), nil, form, nil)
}

func (c *Client) DeleteNotificationMatcher(ctx context.Context, name string) error {
	err := c.do(ctx, http.MethodDelete, "/cluster/notifications/matchers/"+url.PathEscape(name), nil, nil, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

func (c *Client) TestNotificationTarget(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/cluster/notifications/targets/"+url.PathEscape(name)+"/test", nil, url.Values{}, nil)
}
