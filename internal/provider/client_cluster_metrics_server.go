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

type ClusterMetricsServer struct {
	APIPathPrefix         string              `json:"api-path-prefix"`
	Bucket                string              `json:"bucket"`
	Digest                string              `json:"digest"`
	Disable               proxmoxOptionalBool `json:"disable"`
	ID                    string              `json:"id"`
	InfluxDBProtocol      string              `json:"influxdbproto"`
	MaxBodySize           *int64              `json:"max-body-size"`
	MTU                   *int64              `json:"mtu"`
	OpenTelemetryCompress string              `json:"otel-compression"`
	OpenTelemetryHeaders  string              `json:"otel-headers"`
	OpenTelemetryMaxBody  *int64              `json:"otel-max-body-size"`
	OpenTelemetryPath     string              `json:"otel-path"`
	OpenTelemetryProtocol string              `json:"otel-protocol"`
	OpenTelemetryResource string              `json:"otel-resource-attributes"`
	OpenTelemetryTimeout  *int64              `json:"otel-timeout"`
	OpenTelemetryVerify   proxmoxOptionalBool `json:"otel-verify-ssl"`
	Organization          string              `json:"organization"`
	Path                  string              `json:"path"`
	Port                  *int64              `json:"port"`
	Protocol              string              `json:"proto"`
	Server                string              `json:"server"`
	Timeout               *int64              `json:"timeout"`
	Token                 string              `json:"token"`
	Type                  string              `json:"type"`
	VerifyCertificate     proxmoxOptionalBool `json:"verify-certificate"`
}

type ClusterMetricsServerRequest struct {
	APIPathPrefix         *string
	Bucket                *string
	Digest                *string
	Disable               *bool
	InfluxDBProtocol      *string
	MaxBodySize           *int64
	MTU                   *int64
	OpenTelemetryCompress *string
	OpenTelemetryHeaders  *string
	OpenTelemetryMaxBody  *int64
	OpenTelemetryPath     *string
	OpenTelemetryProtocol *string
	OpenTelemetryResource *string
	OpenTelemetryTimeout  *int64
	OpenTelemetryVerify   *bool
	Organization          *string
	Path                  *string
	Port                  int64
	Protocol              *string
	Server                string
	Timeout               *int64
	Token                 *string
	Type                  string
	VerifyCertificate     *bool
	Delete                []string
}

func (c *Client) ClusterMetricsServers(ctx context.Context) ([]ClusterMetricsServer, error) {
	var servers []ClusterMetricsServer
	err := c.do(ctx, http.MethodGet, "/cluster/metrics/server", nil, nil, &servers)
	return servers, err
}

func (c *Client) GetClusterMetricsServer(ctx context.Context, id string) (ClusterMetricsServer, error) {
	var server ClusterMetricsServer
	if err := c.do(ctx, http.MethodGet, "/cluster/metrics/server/"+url.PathEscape(id), nil, nil, &server); err != nil {
		return ClusterMetricsServer{}, err
	}
	return server, nil
}

func (c *Client) CreateClusterMetricsServer(ctx context.Context, id string, req ClusterMetricsServerRequest) error {
	form := clusterMetricsServerForm(req)
	form.Set("id", id)
	form.Set("type", req.Type)
	return c.do(ctx, http.MethodPost, "/cluster/metrics/server/"+url.PathEscape(id), nil, form, nil)
}

func (c *Client) UpdateClusterMetricsServer(ctx context.Context, id string, req ClusterMetricsServerRequest) error {
	form := clusterMetricsServerForm(req)
	setOptionalString(form, "digest", req.Digest)
	if len(req.Delete) > 0 {
		form.Set("delete", strings.Join(sortedStrings(req.Delete), ","))
	}
	return c.do(ctx, http.MethodPut, "/cluster/metrics/server/"+url.PathEscape(id), nil, form, nil)
}

func (c *Client) DeleteClusterMetricsServer(ctx context.Context, id string) error {
	err := c.do(ctx, http.MethodDelete, "/cluster/metrics/server/"+url.PathEscape(id), nil, nil, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

func clusterMetricsServerForm(req ClusterMetricsServerRequest) url.Values {
	form := url.Values{
		"port":   {strconv.FormatInt(req.Port, 10)},
		"server": {req.Server},
	}
	setOptionalString(form, "api-path-prefix", req.APIPathPrefix)
	setOptionalString(form, "bucket", req.Bucket)
	setOptionalBool(form, "disable", req.Disable)
	setOptionalString(form, "influxdbproto", req.InfluxDBProtocol)
	setOptionalInt64(form, "max-body-size", req.MaxBodySize)
	setOptionalInt64(form, "mtu", req.MTU)
	setOptionalString(form, "otel-compression", req.OpenTelemetryCompress)
	setOptionalString(form, "otel-headers", req.OpenTelemetryHeaders)
	setOptionalInt64(form, "otel-max-body-size", req.OpenTelemetryMaxBody)
	setOptionalString(form, "otel-path", req.OpenTelemetryPath)
	setOptionalString(form, "otel-protocol", req.OpenTelemetryProtocol)
	setOptionalString(form, "otel-resource-attributes", req.OpenTelemetryResource)
	setOptionalInt64(form, "otel-timeout", req.OpenTelemetryTimeout)
	setOptionalBool(form, "otel-verify-ssl", req.OpenTelemetryVerify)
	setOptionalString(form, "organization", req.Organization)
	setOptionalString(form, "path", req.Path)
	setOptionalString(form, "proto", req.Protocol)
	setOptionalInt64(form, "timeout", req.Timeout)
	setOptionalString(form, "token", req.Token)
	setOptionalBool(form, "verify-certificate", req.VerifyCertificate)
	return form
}
