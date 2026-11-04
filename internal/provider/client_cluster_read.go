// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type ClusterLogEntry struct {
	Message string `json:"msg"`
	User    string `json:"user"`
	Time    int64  `json:"time"`
	ID      string `json:"id"`
	Tag     string `json:"tag"`
	PRI     int64  `json:"pri"`
	PID     int64  `json:"pid"`
	Node    string `json:"node"`
	UID     string `json:"uid"`
}

func (c *Client) ClusterLog(ctx context.Context, limit int64) ([]ClusterLogEntry, error) {
	query := url.Values{}
	if limit > 0 {
		query.Set("max", strconv.FormatInt(limit, 10))
	}
	var entries []ClusterLogEntry
	if err := c.do(ctx, http.MethodGet, "/cluster/log", query, nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

type ClusterTask struct {
	UPID      string `json:"upid"`
	Node      string `json:"node"`
	Type      string `json:"type"`
	ID        string `json:"id"`
	Status    string `json:"status"`
	User      string `json:"user"`
	StartTime int64  `json:"starttime"`
	EndTime   int64  `json:"endtime"`
	Saved     string `json:"saved"`
}

func (c *Client) ClusterTasks(ctx context.Context) ([]ClusterTask, error) {
	var tasks []ClusterTask
	if err := c.do(ctx, http.MethodGet, "/cluster/tasks", nil, nil, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

type ClusterStatusNode struct {
	ID      string              `json:"id"`
	Name    string              `json:"name"`
	Type    string              `json:"type"`
	IP      string              `json:"ip"`
	Level   string              `json:"level"`
	Local   proxmoxOptionalBool `json:"local"`
	NodeID  int64               `json:"nodeid"`
	Nodes   int64               `json:"nodes"`
	Online  proxmoxOptionalBool `json:"online"`
	Quorate proxmoxOptionalBool `json:"quorate"`
	Version int64               `json:"version"`
}

func (c *Client) ClusterStatus(ctx context.Context) ([]ClusterStatusNode, error) {
	var status []ClusterStatusNode
	if err := c.do(ctx, http.MethodGet, "/cluster/status", nil, nil, &status); err != nil {
		return nil, err
	}
	return status, nil
}

type SchedulePoint struct {
	Timestamp int64  `json:"timestamp"`
	UTC       string `json:"utc"`
}

func (c *Client) AnalyzeSchedule(ctx context.Context, schedule string, iterations, startTime int64) ([]SchedulePoint, error) {
	query := url.Values{"schedule": {schedule}}
	if iterations > 0 {
		query.Set("iterations", strconv.FormatInt(iterations, 10))
	}
	if startTime > 0 {
		query.Set("starttime", strconv.FormatInt(startTime, 10))
	}
	var points []SchedulePoint
	if err := c.do(ctx, http.MethodGet, "/cluster/jobs/schedule-analyze", query, nil, &points); err != nil {
		return nil, err
	}
	return points, nil
}

type CPUFlag struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	SupportedOn []string `json:"supported-on"`
}

func (c *Client) CPUFlags(ctx context.Context, arch, accel string) ([]CPUFlag, error) {
	query := url.Values{}
	if arch != "" {
		query.Set("arch", arch)
	}
	if accel != "" {
		query.Set("accel", accel)
	}
	var flags []CPUFlag
	if err := c.do(ctx, http.MethodGet, "/cluster/qemu/cpu-flags", query, nil, &flags); err != nil {
		return nil, err
	}
	return flags, nil
}

type CustomCPUModel struct {
	CPUType       string              `json:"cputype"`
	Digest        string              `json:"digest"`
	Flags         string              `json:"flags"`
	GuestPhysBits int64               `json:"guest-phys-bits"`
	Hidden        proxmoxOptionalBool `json:"hidden"`
	HVVendorID    string              `json:"hv-vendor-id"`
	Level         int64               `json:"level"`
	PhysBits      string              `json:"phys-bits"`
	ReportedModel string              `json:"reported-model"`
}

func (c *Client) CustomCPUModels(ctx context.Context) ([]CustomCPUModel, error) {
	var models []CustomCPUModel
	if err := c.do(ctx, http.MethodGet, "/cluster/qemu/custom-cpu-models", nil, nil, &models); err != nil {
		return nil, err
	}
	return models, nil
}

type HAStatusEntry struct {
	Type          string              `json:"type"`
	ID            string              `json:"id"`
	Node          string              `json:"node"`
	Status        string              `json:"status"`
	State         string              `json:"state"`
	RequestState  string              `json:"request_state"`
	SID           string              `json:"sid"`
	Quorate       proxmoxOptionalBool `json:"quorate"`
	ArmedState    string              `json:"armed-state"`
	Timestamp     int64               `json:"timestamp"`
	MaxRelocate   int64               `json:"max_relocate"`
	MaxRestart    int64               `json:"max_restart"`
	AutoRebalance proxmoxOptionalBool `json:"auto-rebalance"`
	Failback      proxmoxOptionalBool `json:"failback"`
	CRMState      string              `json:"crm_state"`
	ResourceMode  string              `json:"resource_mode"`
}

func (c *Client) HAStatus(ctx context.Context) ([]HAStatusEntry, error) {
	var status []HAStatusEntry
	if err := c.do(ctx, http.MethodGet, "/cluster/ha/status/current", nil, nil, &status); err != nil {
		return nil, err
	}
	return status, nil
}

func (c *Client) HAManagerStatus(ctx context.Context) (string, error) {
	var status map[string]any
	if err := c.do(ctx, http.MethodGet, "/cluster/ha/status/manager_status", nil, nil, &status); err != nil {
		return "", err
	}
	data, err := json.Marshal(status)
	if err != nil {
		return "", fmt.Errorf("encode ha manager status: %w", err)
	}
	return string(data), nil
}

type HAGroup struct {
	Group      string              `json:"group"`
	Comment    string              `json:"comment"`
	Restricted proxmoxOptionalBool `json:"restricted"`
	NoFailback proxmoxOptionalBool `json:"nofailback"`
	Nodes      string              `json:"nodes"`
}

// HAGroups lists legacy HA groups; PVE 9 rejects the endpoint once groups were migrated.
func (c *Client) HAGroups(ctx context.Context) ([]HAGroup, bool, error) {
	var groups []HAGroup
	err := c.do(ctx, http.MethodGet, "/cluster/ha/groups", nil, nil, &groups)
	if err != nil && isMigratedError(err) {
		return nil, true, nil
	}
	return groups, false, err
}

func isMigratedError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "migrated to rules")
}

func (c *Client) CustomCPUModel(ctx context.Context, name string) (*CustomCPUModel, error) {
	var model CustomCPUModel
	if err := c.do(ctx, http.MethodGet, "/cluster/qemu/custom-cpu-models/"+url.PathEscape(name), nil, nil, &model); err != nil {
		return nil, err
	}
	return &model, nil
}

func (c *Client) CreateCustomCPUModel(ctx context.Context, model CustomCPUModel) error {
	form := customCPUModelForm(model)
	return c.do(ctx, http.MethodPost, "/cluster/qemu/custom-cpu-models", nil, form, nil)
}

func (c *Client) UpdateCustomCPUModel(ctx context.Context, name string, model CustomCPUModel, deleteKeys []string) error {
	form := customCPUModelForm(model)
	if model.Digest != "" {
		form.Set("digest", model.Digest)
	}
	for _, key := range deleteKeys {
		form.Add("delete", key)
	}
	return c.do(ctx, http.MethodPut, "/cluster/qemu/custom-cpu-models/"+url.PathEscape(name), nil, form, nil)
}

func (c *Client) DeleteCustomCPUModel(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/cluster/qemu/custom-cpu-models/"+url.PathEscape(name), nil, nil, nil)
}

func customCPUModelForm(model CustomCPUModel) url.Values {
	form := url.Values{"cputype": {model.CPUType}}
	if model.Flags != "" {
		form.Set("flags", model.Flags)
	}
	if model.GuestPhysBits > 0 {
		form.Set("guest-phys-bits", strconv.FormatInt(model.GuestPhysBits, 10))
	}
	if model.Hidden.Ptr() != nil {
		form.Set("hidden", map[bool]string{true: "1", false: "0"}[*model.Hidden.Ptr()])
	}
	if model.HVVendorID != "" {
		form.Set("hv-vendor-id", model.HVVendorID)
	}
	if model.Level > 0 {
		form.Set("level", strconv.FormatInt(model.Level, 10))
	}
	if model.PhysBits != "" {
		form.Set("phys-bits", model.PhysBits)
	}
	if model.ReportedModel != "" {
		form.Set("reported-model", model.ReportedModel)
	}
	return form
}
