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
	"sort"
	"strconv"
	"strings"
)

type BackupJob struct {
	ID               string               `json:"id"`
	All              proxmoxOptionalBool  `json:"all"`
	BWLimit          proxmoxOptionalInt64 `json:"bwlimit"`
	Comment          string               `json:"comment"`
	Compress         string               `json:"compress"`
	Enabled          proxmoxOptionalBool  `json:"enabled"`
	ExcludeVMIDs     string               `json:"exclude"`
	Mode             string               `json:"mode"`
	NextRun          proxmoxOptionalInt64 `json:"next-run"`
	Node             string               `json:"node"`
	NotesTemplate    string               `json:"notes-template"`
	NotificationMode string               `json:"notification-mode"`
	Pool             string               `json:"pool"`
	Protected        proxmoxOptionalBool  `json:"protected"`
	PruneBackups     json.RawMessage      `json:"prune-backups"`
	Remove           proxmoxOptionalBool  `json:"remove"`
	RepeatMissed     proxmoxOptionalBool  `json:"repeat-missed"`
	Schedule         string               `json:"schedule"`
	Storage          string               `json:"storage"`
	VMIDs            string               `json:"vmid"`
}

type BackupJobRequest struct {
	All              *bool
	BWLimit          *int64
	Comment          *string
	Compress         *string
	Enabled          *bool
	ExcludeVMIDs     *string
	Mode             *string
	Node             *string
	NotesTemplate    *string
	NotificationMode *string
	Pool             *string
	Protected        *bool
	PruneBackups     *string
	Remove           *bool
	RepeatMissed     *bool
	Schedule         *string
	Storage          *string
	VMIDs            *string
	Delete           []string
}

func (c *Client) GetBackupJob(ctx context.Context, id string) (BackupJob, error) {
	var job BackupJob
	if err := c.do(ctx, http.MethodGet, "/cluster/backup/"+url.PathEscape(id), nil, nil, &job); err != nil {
		return BackupJob{}, err
	}
	return job, nil
}

func (c *Client) CreateBackupJob(ctx context.Context, id string, req BackupJobRequest) error {
	form := backupJobForm(req)
	form.Set("id", id)
	return c.do(ctx, http.MethodPost, "/cluster/backup", nil, form, nil)
}

func (c *Client) UpdateBackupJob(ctx context.Context, id string, req BackupJobRequest) error {
	form := backupJobForm(req)
	if len(req.Delete) > 0 {
		form.Set("delete", strings.Join(sortedStrings(req.Delete), ","))
	}
	return c.do(ctx, http.MethodPut, "/cluster/backup/"+url.PathEscape(id), nil, form, nil)
}

func (c *Client) DeleteBackupJob(ctx context.Context, id string) error {
	err := c.do(ctx, http.MethodDelete, "/cluster/backup/"+url.PathEscape(id), nil, nil, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

func backupJobForm(req BackupJobRequest) url.Values {
	form := url.Values{}
	setOptionalBool(form, "all", req.All)
	setOptionalInt64(form, "bwlimit", req.BWLimit)
	setOptionalString(form, "comment", req.Comment)
	setOptionalString(form, "compress", req.Compress)
	setOptionalBool(form, "enabled", req.Enabled)
	setOptionalString(form, "exclude", req.ExcludeVMIDs)
	setOptionalString(form, "mode", req.Mode)
	setOptionalString(form, "node", req.Node)
	setOptionalString(form, "notes-template", req.NotesTemplate)
	setOptionalString(form, "notification-mode", req.NotificationMode)
	setOptionalString(form, "pool", req.Pool)
	setOptionalBool(form, "protected", req.Protected)
	setOptionalString(form, "prune-backups", req.PruneBackups)
	setOptionalBool(form, "remove", req.Remove)
	setOptionalBool(form, "repeat-missed", req.RepeatMissed)
	setOptionalString(form, "schedule", req.Schedule)
	setOptionalString(form, "storage", req.Storage)
	setOptionalString(form, "vmid", req.VMIDs)
	return form
}

func canonicalBackupPruneOptions(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", fmt.Errorf("unable to decode prune-backups options: %w", err)
		}
		return canonicalBackupPruneString(value), nil
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return "", fmt.Errorf("unable to decode prune-backups options: %w", err)
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		var boolValue bool
		if err := json.Unmarshal(values[key], &boolValue); err == nil {
			value := "0"
			if boolValue {
				value = "1"
			}
			parts = append(parts, key+"="+value)
			continue
		}
		var intValue int64
		if err := json.Unmarshal(values[key], &intValue); err != nil {
			return "", fmt.Errorf("unable to decode prune-backups option %q: %w", key, err)
		}
		parts = append(parts, key+"="+strconv.FormatInt(intValue, 10))
	}
	return strings.Join(parts, ","), nil
}

func canonicalBackupPruneString(value string) string {
	parts := strings.Split(value, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
