// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/url"
)

type NodeCertificateInfo struct {
	Filename      string   `json:"filename"`
	Fingerprint   string   `json:"fingerprint"`
	Issuer        string   `json:"issuer"`
	NotAfter      int64    `json:"notafter"`
	NotBefore     int64    `json:"notbefore"`
	PEM           string   `json:"pem"`
	PublicKeyBits int64    `json:"public-key-bits"`
	PublicKeyType string   `json:"public-key-type"`
	SAN           []string `json:"san"`
	Subject       string   `json:"subject"`
}

func (c *Client) NodeCertificateInfos(ctx context.Context, node string) ([]NodeCertificateInfo, error) {
	var infos []NodeCertificateInfo
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/certificates/info", nil, nil, &infos); err != nil {
		return nil, err
	}
	return infos, nil
}

type NodeSubscription struct {
	CheckTime   int64  `json:"checktime"`
	Key         string `json:"key"`
	Level       string `json:"level"`
	Message     string `json:"message"`
	NextDueDate string `json:"nextduedate"`
	ProductName string `json:"productname"`
	RegDate     string `json:"regdate"`
	ServerID    string `json:"serverid"`
	Signature   string `json:"signature"`
	Sockets     int64  `json:"sockets"`
	Status      string `json:"status"`
	URL         string `json:"url"`
}

func (c *Client) NodeSubscription(ctx context.Context, node string) (*NodeSubscription, error) {
	var subscription NodeSubscription
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/subscription", nil, nil, &subscription); err != nil {
		return nil, err
	}
	return &subscription, nil
}
