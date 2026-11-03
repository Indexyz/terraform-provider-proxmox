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
	"time"
)

type NodeCertificate struct {
	Filename      string           `json:"filename"`
	Fingerprint   string           `json:"fingerprint"`
	Issuer        string           `json:"issuer"`
	NotAfter      certificateEpoch `json:"notafter"`
	NotBefore     certificateEpoch `json:"notbefore"`
	PEM           string           `json:"pem"`
	PublicKeyBits int64            `json:"public-key-bits"`
	PublicKeyType string           `json:"public-key-type"`
	SAN           certificateSAN   `json:"san"`
	Subject       string           `json:"subject"`
}

const nodeCustomCertificateFile = "pveproxy-ssl.pem"

func nodeCertificatesPath(node string) string {
	return "/nodes/" + url.PathEscape(node) + "/certificates"
}

func (c *Client) NodeCertificates(ctx context.Context, node string) ([]NodeCertificate, error) {
	var certificates []NodeCertificate
	if err := c.do(ctx, http.MethodGet, nodeCertificatesPath(node)+"/info", nil, nil, &certificates); err != nil {
		return nil, err
	}
	return certificates, nil
}

func (c *Client) GetNodeCertificate(ctx context.Context, node, filename string) (NodeCertificate, error) {
	certificates, err := c.NodeCertificates(ctx, node)
	if err != nil {
		return NodeCertificate{}, err
	}
	for _, certificate := range certificates {
		if certificate.Filename == filename {
			return certificate, nil
		}
	}
	return NodeCertificate{}, errNotFound
}

// UploadNodeCertificate installs a custom certificate chain and key.
func (c *Client) UploadNodeCertificate(ctx context.Context, node, certificates, key string, force, restart bool) error {
	form := url.Values{"certificates": {certificates}, "key": {key}}
	if force {
		form.Set("force", "1")
	}
	if restart {
		form.Set("restart", "1")
	}
	err := c.do(ctx, http.MethodPost, nodeCertificatesPath(node)+"/custom", nil, form, nil)
	if errors.Is(err, errNotFound) {
		return fmt.Errorf("node %q does not exist: %w", node, err)
	}
	return err
}

func (c *Client) DeleteNodeCertificate(ctx context.Context, node string, restart bool) error {
	form := url.Values{}
	if restart {
		form.Set("restart", "1")
	}
	err := c.do(ctx, http.MethodDelete, nodeCertificatesPath(node)+"/custom", nil, form, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

type ACMEAccount struct {
	Name      string            `json:"name"`
	Directory string            `json:"directory"`
	TOS       string            `json:"tos"`
	Location  string            `json:"location"`
	Details   ACMEAccountDetail `json:"account"`
}

type ACMEAccountDetail struct {
	Status  string   `json:"status"`
	Contact []string `json:"contact"`
}

func (c *Client) ACMEAccounts(ctx context.Context) ([]ACMEAccount, error) {
	var accounts []ACMEAccount
	if err := c.do(ctx, http.MethodGet, "/cluster/acme/account", nil, nil, &accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

func (c *Client) GetACMEAccount(ctx context.Context, name string) (ACMEAccount, error) {
	// The endpoint has answered both a single-element array and an object.
	var accounts acmeAccountResponse
	if err := c.do(ctx, http.MethodGet, "/cluster/acme/account/"+url.PathEscape(name), nil, nil, &accounts); err != nil {
		return ACMEAccount{}, err
	}
	if len(accounts) == 0 {
		return ACMEAccount{}, errNotFound
	}
	account := accounts[0]
	if account.Name == "" {
		account.Name = name
	}
	return account, nil
}

func (c *Client) CreateACMEAccount(ctx context.Context, account ACMEAccount, eabKID, eabHMAC string) (string, error) {
	form := url.Values{"contact": {strings.Join(account.Details.Contact, ",")}}
	if account.Name != "" {
		form.Set("name", account.Name)
	}
	if account.Directory != "" {
		form.Set("directory", account.Directory)
	}
	if account.TOS != "" {
		form.Set("tos_url", account.TOS)
	}
	if eabKID != "" {
		form.Set("eab-kid", eabKID)
	}
	if eabHMAC != "" {
		form.Set("eab-hmac-key", eabHMAC)
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, "/cluster/acme/account", nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "ACME account registration"); err != nil {
		return "", err
	}
	return upid, nil
}

func (c *Client) UpdateACMEAccount(ctx context.Context, name, contact string) (string, error) {
	form := url.Values{"contact": {contact}, "name": {name}}
	var upid string
	if err := c.do(ctx, http.MethodPut, "/cluster/acme/account/"+url.PathEscape(name), nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "ACME account update"); err != nil {
		return "", err
	}
	return upid, nil
}

func (c *Client) DeleteACMEAccount(ctx context.Context, name string) (string, error) {
	var upid string
	if err := c.do(ctx, http.MethodDelete, "/cluster/acme/account/"+url.PathEscape(name), nil, nil, &upid); err != nil {
		if errors.Is(err, errNotFound) {
			return "", nil
		}
		return "", err
	}
	if err := validateQemuTaskAck(upid, "ACME account removal"); err != nil {
		return "", err
	}
	return upid, nil
}

type ACMEPlugin struct {
	ID              string              `json:"plugin"`
	Type            string              `json:"type"`
	API             string              `json:"api"`
	Data            string              `json:"data"`
	Nodes           string              `json:"nodes"`
	ValidationDelay int64               `json:"validation-delay"`
	Disable         proxmoxOptionalBool `json:"disable"`
	Digest          string
}

func (c *Client) ACMEPlugins(ctx context.Context) ([]ACMEPlugin, error) {
	var plugins []ACMEPlugin
	if err := c.do(ctx, http.MethodGet, "/cluster/acme/plugins", nil, nil, &plugins); err != nil {
		return nil, err
	}
	return plugins, nil
}

func (c *Client) GetACMEPlugin(ctx context.Context, id string) (ACMEPlugin, error) {
	plugins, err := c.ACMEPlugins(ctx)
	if err != nil {
		return ACMEPlugin{}, err
	}
	for _, plugin := range plugins {
		if plugin.ID == id {
			return plugin, nil
		}
	}
	return ACMEPlugin{}, errNotFound
}

func (c *Client) CreateACMEPlugin(ctx context.Context, plugin ACMEPlugin, disabled *bool, validationDelay *int64) error {
	form := url.Values{"id": {plugin.ID}, "type": {plugin.Type}}
	if plugin.API != "" {
		form.Set("api", plugin.API)
	}
	if plugin.Data != "" {
		form.Set("data", plugin.Data)
	}
	if plugin.Nodes != "" {
		form.Set("nodes", plugin.Nodes)
	}
	setOptionalBool(form, "disable", disabled)
	setOptionalInt64(form, "validation-delay", validationDelay)
	return c.do(ctx, http.MethodPost, "/cluster/acme/plugins", nil, form, nil)
}

func (c *Client) UpdateACMEPlugin(ctx context.Context, id string, plugin ACMEPlugin, disabled *bool, validationDelay *int64, deleted []string, digest string) error {
	form := url.Values{"id": {id}}
	if plugin.API != "" {
		form.Set("api", plugin.API)
	}
	if plugin.Data != "" {
		form.Set("data", plugin.Data)
	}
	if plugin.Nodes != "" {
		form.Set("nodes", plugin.Nodes)
	}
	setOptionalBool(form, "disable", disabled)
	setOptionalInt64(form, "validation-delay", validationDelay)
	if len(deleted) > 0 {
		form.Set("delete", joinComma(deleted))
	}
	if digest != "" {
		form.Set("digest", digest)
	}
	return c.do(ctx, http.MethodPut, "/cluster/acme/plugins/"+url.PathEscape(id), nil, form, nil)
}

func (c *Client) DeleteACMEPlugin(ctx context.Context, id string) error {
	err := c.do(ctx, http.MethodDelete, "/cluster/acme/plugins/"+url.PathEscape(id), nil, nil, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

func (c *Client) OrderNodeACMECertificate(ctx context.Context, node string, force bool) (string, error) {
	form := url.Values{}
	if force {
		form.Set("force", "1")
	}
	var upid string
	if err := c.do(ctx, http.MethodPost, nodeCertificatesPath(node)+"/acme/certificate", nil, form, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "ACME certificate order"); err != nil {
		return "", err
	}
	return upid, nil
}

func (c *Client) RevokeNodeACMECertificate(ctx context.Context, node string) (string, error) {
	var upid string
	if err := c.do(ctx, http.MethodDelete, nodeCertificatesPath(node)+"/acme/certificate", nil, nil, &upid); err != nil {
		return "", err
	}
	if err := validateQemuTaskAck(upid, "ACME certificate revoke"); err != nil {
		return "", err
	}
	return upid, nil
}

func joinComma(values []string) string {
	joined := ""
	for index, value := range values {
		if index > 0 {
			joined += ","
		}
		joined += value
	}
	return joined
}

var _ = strconv.Itoa

// certificateSAN accepts both the array and the joined string form Proxmox
// returns for subject alternative names.
type certificateSAN []string

func (san *certificateSAN) UnmarshalJSON(raw []byte) error {
	if len(raw) > 0 && raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		for _, entry := range strings.Split(text, ",") {
			if entry = strings.TrimSpace(strings.TrimPrefix(entry, "DNS:")); entry != "" {
				*san = append(*san, entry)
			}
		}
		return nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return err
	}
	*san = list
	return nil
}

// certificateEpoch accepts epoch numbers and RFC3339 strings.
type certificateEpoch int64

func (value *certificateEpoch) UnmarshalJSON(raw []byte) error {
	if len(raw) > 0 && raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		parsed, err := time.Parse(time.RFC3339, text)
		if err != nil {
			parsed, err = time.Parse("2006-01-02", text)
			if err != nil {
				return err
			}
		}
		*value = certificateEpoch(parsed.Unix())
		return nil
	}
	var epoch int64
	if err := json.Unmarshal(raw, &epoch); err != nil {
		return err
	}
	*value = certificateEpoch(epoch)
	return nil
}

type acmeAccountResponse []ACMEAccount

func (response *acmeAccountResponse) UnmarshalJSON(raw []byte) error {
	if len(raw) > 0 && raw[0] == '[' {
		var list []ACMEAccount
		if err := json.Unmarshal(raw, &list); err != nil {
			return err
		}
		*response = list
		return nil
	}
	var single ACMEAccount
	if err := json.Unmarshal(raw, &single); err != nil {
		return err
	}
	if len(single.Details.Contact) > 0 || single.Directory != "" || single.Location != "" {
		*response = append(*response, single)
	}
	return nil
}

type NodeACMESettings struct {
	ACME   string `json:"acme"`
	Digest string `json:"digest"`
}

func (c *Client) NodeACMESettings(ctx context.Context, node string) (NodeACMESettings, error) {
	var settings NodeACMESettings
	if err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/config", nil, nil, &settings); err != nil {
		return NodeACMESettings{}, err
	}
	return settings, nil
}

// ConfigureNodeACME writes the node ACME settings and the per-domain plugins.
func (c *Client) ConfigureNodeACME(ctx context.Context, node, acme string, domainPlugins map[string]string, digest string) error {
	form := url.Values{"acme": {acme}}
	domains := make([]string, 0, len(domainPlugins))
	for domain := range domainPlugins {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	for index, domain := range domains {
		entry := "domain=" + domain
		if plugin := domainPlugins[domain]; plugin != "" {
			entry += ",plugin=" + plugin
		}
		form.Set(fmt.Sprintf("acmedomain%d", index), entry)
	}
	if digest != "" {
		form.Set("digest", digest)
	}
	return c.do(ctx, http.MethodPut, "/nodes/"+url.PathEscape(node)+"/config", nil, form, nil)
}
