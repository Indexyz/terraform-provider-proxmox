// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

func (c *Client) SetNodeTimezone(ctx context.Context, node, timezone string) error {
	form := url.Values{"timezone": {timezone}}
	return c.do(ctx, http.MethodPut, "/nodes/"+url.PathEscape(node)+"/time", nil, form, nil)
}

func (c *Client) SetNodeDNS(ctx context.Context, node string, dns NodeDNS) error {
	form := url.Values{}
	if dns.DNS1 != "" {
		form.Set("dns1", dns.DNS1)
	}
	if dns.DNS2 != "" {
		form.Set("dns2", dns.DNS2)
	}
	if dns.DNS3 != "" {
		form.Set("dns3", dns.DNS3)
	}
	if dns.Search != "" {
		form.Set("search", dns.Search)
	}
	return c.do(ctx, http.MethodPut, "/nodes/"+url.PathEscape(node)+"/dns", nil, form, nil)
}

type NodeSyslogEntry struct {
	Line int64  `json:"n"`
	Text string `json:"t"`
}

func (c *Client) NodeSyslog(ctx context.Context, node string, limit int64, service, since, until string, start int64) ([]NodeSyslogEntry, error) {
	query := url.Values{"limit": {strconv.FormatInt(limit, 10)}}
	if service != "" {
		query.Set("service", service)
	}
	if since != "" {
		query.Set("since", since)
	}
	if until != "" {
		query.Set("until", until)
	}
	if start > 0 {
		query.Set("start", strconv.FormatInt(start, 10))
	}
	var entries []NodeSyslogEntry
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/syslog", query, nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (c *Client) NodeJournal(ctx context.Context, node string, lastEntries int64, service, since, until string) ([]string, error) {
	query := url.Values{"lastentries": {strconv.FormatInt(lastEntries, 10)}}
	if service != "" {
		query.Set("service", service)
	}
	if since != "" {
		query.Set("since", since)
	}
	if until != "" {
		query.Set("until", until)
	}
	var lines []string
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/journal", query, nil, &lines); err != nil {
		return nil, err
	}
	return lines, nil
}

var _ = fmt.Sprintf
