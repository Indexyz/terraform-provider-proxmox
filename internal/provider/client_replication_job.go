// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type ReplicationJob struct {
	Comment   string              `json:"comment"`
	Digest    string              `json:"digest"`
	Disable   proxmoxOptionalBool `json:"disable"`
	GuestID   int64               `json:"guest"`
	ID        string              `json:"id"`
	JobNumber int64               `json:"jobnum"`
	Rate      *float64            `json:"rate"`
	Schedule  string              `json:"schedule"`
	Source    string              `json:"source"`
	Target    string              `json:"target"`
	Type      string              `json:"type"`
}

type ReplicationJobRequest struct {
	Comment  *string
	Digest   *string
	Disable  *bool
	Rate     *float64
	Schedule *string
	Delete   []string
}

func (c *Client) GetReplicationJob(ctx context.Context, id string) (ReplicationJob, error) {
	var job ReplicationJob
	if err := c.do(ctx, http.MethodGet, "/cluster/replication/"+url.PathEscape(id), nil, nil, &job); err != nil {
		return ReplicationJob{}, err
	}
	if job.ID == "" {
		job.ID = id
	}
	return job, nil
}

func (c *Client) CreateReplicationJob(ctx context.Context, id, target string, req ReplicationJobRequest) error {
	form := replicationJobForm(req)
	form.Set("id", id)
	form.Set("target", target)
	form.Set("type", "local")
	return c.do(ctx, http.MethodPost, "/cluster/replication", nil, form, nil)
}

func (c *Client) UpdateReplicationJob(ctx context.Context, id string, req ReplicationJobRequest) error {
	form := replicationJobForm(req)
	setOptionalString(form, "digest", req.Digest)
	if len(req.Delete) > 0 {
		form.Set("delete", strings.Join(sortedStrings(req.Delete), ","))
	}
	return c.do(ctx, http.MethodPut, "/cluster/replication/"+url.PathEscape(id), nil, form, nil)
}

func (c *Client) DeleteReplicationJob(ctx context.Context, id string) error {
	form := url.Values{"force": {"1"}}
	err := c.do(ctx, http.MethodDelete, "/cluster/replication/"+url.PathEscape(id), nil, form, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

func replicationJobForm(req ReplicationJobRequest) url.Values {
	form := url.Values{}
	setOptionalString(form, "comment", req.Comment)
	setOptionalBool(form, "disable", req.Disable)
	if req.Rate != nil {
		form.Set("rate", strconv.FormatFloat(*req.Rate, 'f', -1, 64))
	}
	setOptionalString(form, "schedule", req.Schedule)
	return form
}
