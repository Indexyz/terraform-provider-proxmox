// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type NotBackedUpGuest struct {
	Name string `json:"name"`
	Type string `json:"type"`
	VMID int64  `json:"vmid"`
}

func (c *Client) NotBackedUpGuests(ctx context.Context) ([]NotBackedUpGuest, error) {
	var guests []NotBackedUpGuest
	if err := c.do(ctx, http.MethodGet, "/cluster/backup-info/not-backed-up", nil, nil, &guests); err != nil {
		return nil, err
	}
	return guests, nil
}

type BackupVolumeNode struct {
	Type     string              `json:"type"`
	Leaf     proxmoxOptionalBool `json:"leaf"`
	Name     string              `json:"name"`
	ID       int64               `json:"id"`
	Children []BackupVolumeNode  `json:"children"`
}

func (c *Client) BackupJobIncludedVolumes(ctx context.Context, id string) ([]BackupVolumeNode, error) {
	var result struct {
		Children []BackupVolumeNode `json:"children"`
	}
	if err := c.do(ctx, http.MethodGet, "/cluster/backup/"+url.PathEscape(id)+"/included_volumes", nil, nil, &result); err != nil {
		return nil, err
	}
	return result.Children, nil
}

type RealmSyncJob struct {
	ID             string               `json:"id"`
	Realm          string               `json:"realm"`
	Scope          string               `json:"scope"`
	Schedule       string               `json:"schedule"`
	Enabled        proxmoxOptionalBool  `json:"enabled"`
	Comment        string               `json:"comment"`
	RemoveVanished string               `json:"remove-vanished"`
	LastRun        proxmoxOptionalInt64 `json:"last-run"`
	NextRun        proxmoxOptionalInt64 `json:"next-run"`
}

func (c *Client) RealmSyncJobs(ctx context.Context) ([]RealmSyncJob, error) {
	var jobs []RealmSyncJob
	if err := c.do(ctx, http.MethodGet, "/cluster/jobs/realm-sync", nil, nil, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// RunVzdump starts a backup and returns the worker UPID.
func (c *Client) RunVzdump(ctx context.Context, node string, vmid int64, storage, mode, compress, notesTemplate string, protected, quiet bool) (string, error) {
	form := url.Values{"vmid": {fmt.Sprintf("%d", vmid)}, "storage": {storage}}
	if mode != "" {
		form.Set("mode", mode)
	}
	if compress != "" {
		form.Set("compress", compress)
	}
	if notesTemplate != "" {
		form.Set("notes-template", notesTemplate)
	}
	if protected {
		form.Set("protected", "1")
	}
	if quiet {
		form.Set("quiet", "1")
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/nodes/"+url.PathEscape(node)+"/vzdump", nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "backup"); err != nil {
		return "", err
	}
	return upid, nil
}

// PruneBackups prunes backups of one guest and returns the worker UPID.
func (c *Client) PruneBackups(ctx context.Context, node, storage, prune, guestType string, vmid int64) (string, error) {
	form := url.Values{"vmid": {fmt.Sprintf("%d", vmid)}}
	if prune != "" {
		form.Set("prune-backups", prune)
	}
	if guestType != "" {
		form.Set("type", guestType)
	}
	var upid string
	apiPath := fmt.Sprintf("/nodes/%s/storage/%s/prunebackups", url.PathEscape(node), url.PathEscape(storage))
	if err := c.do(ctx, http.MethodDelete, apiPath, nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "backup prune"); err != nil {
		return "", err
	}
	return upid, nil
}
