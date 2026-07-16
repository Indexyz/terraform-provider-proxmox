// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

var errNotFound = errors.New("proxmox resource not found")

type ClientConfig struct {
	Endpoint       string
	Username       string
	Password       string
	OTP            string
	APITokenID     string
	APITokenSecret string
	Insecure       bool
	Timeout        time.Duration
	UserAgent      string
}

type Client struct {
	baseURL        *url.URL
	httpClient     *http.Client
	username       string
	password       string
	otp            string
	apiTokenID     string
	apiTokenSecret string
	authTicket     string
	csrfToken      string
	userAgent      string
}

type Version struct {
	Console string `json:"console"`
	Release string `json:"release"`
	RepoID  string `json:"repoid"`
	Version string `json:"version"`
}

type Node struct {
	CPU            float64 `json:"cpu"`
	Level          string  `json:"level"`
	MaxCPU         int64   `json:"maxcpu"`
	MaxMemory      int64   `json:"maxmem"`
	MemoryUsed     int64   `json:"mem"`
	Name           string  `json:"node"`
	SSLFingerprint string  `json:"ssl_fingerprint"`
	Status         string  `json:"status"`
	Uptime         int64   `json:"uptime"`
}

type NodeStatus struct {
	BootInfo      NodeBootInfo      `json:"boot-info"`
	CPU           float64           `json:"cpu"`
	CPUInfo       NodeCPUInfo       `json:"cpuinfo"`
	CurrentKernel NodeCurrentKernel `json:"current-kernel"`
	LoadAverage   []string          `json:"loadavg"`
	Memory        NodeMemory        `json:"memory"`
	PVEVersion    string            `json:"pveversion"`
	RootFS        NodeRootFS        `json:"rootfs"`
}

type NodeDNS struct {
	DNS1   string `json:"dns1"`
	DNS2   string `json:"dns2"`
	DNS3   string `json:"dns3"`
	Search string `json:"search"`
}

type NodeTime struct {
	LocalTime int64  `json:"localtime"`
	Time      int64  `json:"time"`
	Timezone  string `json:"timezone"`
}

type NodeBootInfo struct {
	Mode       string `json:"mode"`
	SecureBoot *bool  `json:"secureboot"`
}

type NodeCPUInfo struct {
	Cores   int64  `json:"cores"`
	CPUs    int64  `json:"cpus"`
	Model   string `json:"model"`
	Sockets int64  `json:"sockets"`
}

type NodeCurrentKernel struct {
	Machine string `json:"machine"`
	Release string `json:"release"`
	Sysname string `json:"sysname"`
	Version string `json:"version"`
}

type NodeMemory struct {
	Available int64 `json:"available"`
	Free      int64 `json:"free"`
	Total     int64 `json:"total"`
	Used      int64 `json:"used"`
}

type NodeRootFS struct {
	Available int64 `json:"avail"`
	Free      int64 `json:"free"`
	Total     int64 `json:"total"`
	Used      int64 `json:"used"`
}

type ClusterResource struct {
	CGroupMode  *int64   `json:"cgroup-mode"`
	Content     string   `json:"content"`
	CPU         *float64 `json:"cpu"`
	Disk        *int64   `json:"disk"`
	DiskRead    *int64   `json:"diskread"`
	DiskWrite   *int64   `json:"diskwrite"`
	HAState     string   `json:"hastate"`
	ID          string   `json:"id"`
	Level       string   `json:"level"`
	Lock        string   `json:"lock"`
	MaxCPU      *float64 `json:"maxcpu"`
	MaxDisk     *int64   `json:"maxdisk"`
	MaxMemory   *int64   `json:"maxmem"`
	MemoryUsed  *int64   `json:"mem"`
	MemoryHost  *int64   `json:"memhost"`
	Name        string   `json:"name"`
	NetIn       *int64   `json:"netin"`
	NetOut      *int64   `json:"netout"`
	Network     string   `json:"network"`
	NetworkType string   `json:"network-type"`
	Node        string   `json:"node"`
	PluginType  string   `json:"plugintype"`
	Pool        string   `json:"pool"`
	Protocol    string   `json:"protocol"`
	SDN         string   `json:"sdn"`
	Shared      *bool    `json:"shared"`
	Status      string   `json:"status"`
	Storage     string   `json:"storage"`
	Tags        string   `json:"tags"`
	Template    *bool    `json:"template"`
	Type        string   `json:"type"`
	Uptime      *int64   `json:"uptime"`
	VMID        *int64   `json:"vmid"`
	ZoneType    string   `json:"zone-type"`
}

type Pool struct {
	PoolID  string       `json:"poolid"`
	Comment string       `json:"comment"`
	Members []PoolMember `json:"members"`
}

type Group struct {
	GroupID string   `json:"groupid"`
	Comment string   `json:"comment"`
	Members []string `json:"members"`
}

type PoolMember struct {
	ID      string `json:"id"`
	Node    string `json:"node"`
	Storage string `json:"storage"`
	Type    string `json:"type"`
	VMID    *int64 `json:"vmid"`
}

type UpdatePoolRequest struct {
	PoolID     string
	Comment    *string
	AllowMove  bool
	Delete     bool
	StorageIDs []string
	VMIDs      []int64
}

type groupIndexResponse struct {
	GroupID string `json:"groupid"`
	Comment string `json:"comment"`
	Users   string `json:"users"`
}

type ticketResponse struct {
	CSRFPreventionToken string `json:"CSRFPreventionToken"`
	Ticket              string `json:"ticket"`
	Username            string `json:"username"`
}

type apiResponseEnvelope struct {
	Data   json.RawMessage `json:"data"`
	Errors any             `json:"errors"`
}

type APIError struct {
	StatusCode int
	Message    string
	Body       string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("proxmox API request failed with status %d: %s", e.StatusCode, e.Message)
	}
	if e.Body != "" {
		return fmt.Sprintf("proxmox API request failed with status %d: %s", e.StatusCode, e.Body)
	}
	return fmt.Sprintf("proxmox API request failed with status %d", e.StatusCode)
}

func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	baseURL, err := normalizeEndpoint(cfg.Endpoint)
	if err != nil {
		return nil, err
	}

	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("unexpected default transport type %T", http.DefaultTransport)
	}

	transport := baseTransport.Clone()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.InsecureSkipVerify = cfg.Insecure

	client := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
		},
		username:       cfg.Username,
		password:       cfg.Password,
		otp:            cfg.OTP,
		apiTokenID:     cfg.APITokenID,
		apiTokenSecret: cfg.APITokenSecret,
		userAgent:      cfg.UserAgent,
	}

	if client.usesTicketAuth() {
		if err := client.login(ctx); err != nil {
			return nil, err
		}
	}

	return client, nil
}

func (c *Client) Version(ctx context.Context) (Version, error) {
	var version Version
	err := c.do(ctx, http.MethodGet, "/version", nil, nil, &version)
	return version, err
}

func (c *Client) Nodes(ctx context.Context) ([]Node, error) {
	var nodes []Node
	err := c.do(ctx, http.MethodGet, "/nodes", nil, nil, &nodes)
	return nodes, err
}

func (c *Client) NodeStatus(ctx context.Context, node string) (NodeStatus, error) {
	var status NodeStatus
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/status", url.PathEscape(node)), nil, nil, &status)
	return status, err
}

func (c *Client) NodeDNS(ctx context.Context, node string) (NodeDNS, error) {
	var dns NodeDNS
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/dns", url.PathEscape(node)), nil, nil, &dns)
	return dns, err
}

func (c *Client) NodeTime(ctx context.Context, node string) (NodeTime, error) {
	var nodeTime NodeTime
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/time", url.PathEscape(node)), nil, nil, &nodeTime)
	return nodeTime, err
}

func (c *Client) ClusterResources(ctx context.Context, resourceType string) ([]ClusterResource, error) {
	query := url.Values{}
	if resourceType != "" {
		query.Set("type", resourceType)
	}

	var resources []ClusterResource
	err := c.do(ctx, http.MethodGet, "/cluster/resources", query, nil, &resources)
	return resources, err
}

func (c *Client) GetPool(ctx context.Context, poolID string) (Pool, error) {
	query := url.Values{}
	query.Set("poolid", poolID)

	var pools []Pool
	if err := c.do(ctx, http.MethodGet, "/pools", query, nil, &pools); err != nil {
		return Pool{}, err
	}

	if len(pools) == 0 {
		return Pool{}, errNotFound
	}

	return pools[0], nil
}

func (c *Client) Pools(ctx context.Context) ([]Pool, error) {
	var pools []Pool
	err := c.do(ctx, http.MethodGet, "/pools", nil, nil, &pools)
	return pools, err
}

func (c *Client) CreatePool(ctx context.Context, poolID string, comment *string) error {
	form := url.Values{}
	form.Set("poolid", poolID)
	if comment != nil {
		form.Set("comment", *comment)
	}

	return c.do(ctx, http.MethodPost, "/pools", nil, form, nil)
}

func (c *Client) UpdatePool(ctx context.Context, req UpdatePoolRequest) error {
	form := url.Values{}
	form.Set("poolid", req.PoolID)

	if req.Comment != nil {
		form.Set("comment", *req.Comment)
	}

	if req.Delete {
		form.Set("delete", "1")
	}

	if req.AllowMove {
		form.Set("allow-move", "1")
	}

	if len(req.StorageIDs) > 0 {
		form.Set("storage", strings.Join(sortedStrings(req.StorageIDs), ","))
	}

	if len(req.VMIDs) > 0 {
		form.Set("vms", joinInt64s(sortedInt64s(req.VMIDs)))
	}

	return c.do(ctx, http.MethodPut, "/pools", nil, form, nil)
}

func (c *Client) DeletePool(ctx context.Context, poolID string) error {
	form := url.Values{}
	form.Set("poolid", poolID)
	return c.do(ctx, http.MethodDelete, "/pools", nil, form, nil)
}

func (c *Client) GetGroup(ctx context.Context, groupID string) (Group, error) {
	var group Group
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/access/groups/%s", url.PathEscape(groupID)), nil, nil, &group); err != nil {
		return Group{}, err
	}

	group.GroupID = groupID
	group.Members = sortedStrings(group.Members)
	return group, nil
}

func (c *Client) Groups(ctx context.Context) ([]Group, error) {
	var response []groupIndexResponse
	if err := c.do(ctx, http.MethodGet, "/access/groups", nil, nil, &response); err != nil {
		return nil, err
	}

	groups := make([]Group, 0, len(response))
	for _, item := range response {
		groups = append(groups, Group{
			GroupID: item.GroupID,
			Comment: item.Comment,
			Members: splitProxmoxList(item.Users),
		})
	}

	return groups, nil
}

func (c *Client) CreateGroup(ctx context.Context, groupID string, comment *string) error {
	form := url.Values{}
	form.Set("groupid", groupID)
	if comment != nil {
		form.Set("comment", *comment)
	}

	return c.do(ctx, http.MethodPost, "/access/groups", nil, form, nil)
}

func (c *Client) UpdateGroup(ctx context.Context, groupID string, comment *string) error {
	form := url.Values{}
	form.Set("groupid", groupID)
	if comment != nil {
		form.Set("comment", *comment)
	}

	return c.do(ctx, http.MethodPut, fmt.Sprintf("/access/groups/%s", url.PathEscape(groupID)), nil, form, nil)
}

func (c *Client) DeleteGroup(ctx context.Context, groupID string) error {
	form := url.Values{}
	form.Set("groupid", groupID)
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/access/groups/%s", url.PathEscape(groupID)), nil, form, nil)
}

func (c *Client) usesTicketAuth() bool {
	return c.apiTokenID == "" && c.apiTokenSecret == ""
}

func (c *Client) login(ctx context.Context) error {
	form := url.Values{}
	form.Set("username", c.username)
	form.Set("password", c.password)
	if c.otp != "" {
		form.Set("otp", c.otp)
	}

	var response ticketResponse
	if err := c.do(ctx, http.MethodPost, "/access/ticket", nil, form, &response); err != nil {
		return fmt.Errorf("unable to login to Proxmox VE: %w", err)
	}

	c.authTicket = response.Ticket
	c.csrfToken = response.CSRFPreventionToken
	return nil
}

func (c *Client) do(ctx context.Context, method, apiPath string, query url.Values, form url.Values, out any) error {
	requestURL := *c.baseURL
	escapedPath := path.Join(c.baseURL.EscapedPath(), apiPath)
	requestPath, err := url.PathUnescape(escapedPath)
	if err != nil {
		return fmt.Errorf("unable to decode Proxmox API path %q: %w", escapedPath, err)
	}
	requestURL.Path = requestPath
	requestURL.RawPath = escapedPath
	if method == http.MethodGet && len(query) > 0 {
		requestURL.RawQuery = query.Encode()
	}

	var body io.Reader
	if method != http.MethodGet && form != nil {
		encoded := form.Encode()
		body = strings.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	if method != http.MethodGet && form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	if c.apiTokenID != "" && c.apiTokenSecret != "" {
		req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.apiTokenID, c.apiTokenSecret))
	} else if c.authTicket != "" {
		req.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: c.authTicket})
		if method != http.MethodGet && c.csrfToken != "" {
			req.Header.Set("CSRFPreventionToken", c.csrfToken)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNotFound {
		return errNotFound
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(resp.StatusCode, bodyBytes)
	}

	if out == nil {
		return nil
	}

	var envelope apiResponseEnvelope
	if err := json.Unmarshal(bodyBytes, &envelope); err != nil {
		return fmt.Errorf("unable to decode Proxmox API response: %w", err)
	}

	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}

	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("unable to decode Proxmox API payload: %w", err)
	}

	return nil
}

func normalizeEndpoint(rawEndpoint string) (*url.URL, error) {
	if strings.TrimSpace(rawEndpoint) == "" {
		return nil, errors.New("endpoint must not be empty")
	}

	endpoint, err := url.Parse(rawEndpoint)
	if err != nil {
		return nil, fmt.Errorf("unable to parse Proxmox endpoint: %w", err)
	}

	if endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, errors.New("endpoint must be a full URL such as https://pve.example.com:8006")
	}

	if endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, errors.New("endpoint must not include a query string or fragment")
	}

	cleanPath := strings.TrimRight(endpoint.Path, "/")
	if cleanPath == "" {
		cleanPath = "/api2/json"
	} else if !strings.HasSuffix(cleanPath, "/api2/json") {
		cleanPath += "/api2/json"
	}

	endpoint.Path = cleanPath
	return endpoint, nil
}

func decodeAPIError(statusCode int, body []byte) error {
	var envelope apiResponseEnvelope
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Errors != nil {
		errorBytes, marshalErr := json.Marshal(envelope.Errors)
		if marshalErr == nil {
			return &APIError{
				StatusCode: statusCode,
				Message:    string(errorBytes),
				Body:       string(body),
			}
		}
	}

	return &APIError{
		StatusCode: statusCode,
		Body:       strings.TrimSpace(string(body)),
	}
}

func sortedStrings(values []string) []string {
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	return copied
}

func sortedInt64s(values []int64) []int64 {
	copied := append([]int64(nil), values...)
	sort.Slice(copied, func(i, j int) bool { return copied[i] < copied[j] })
	return copied
}

func joinInt64s(values []int64) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%d", value))
	}
	return strings.Join(parts, ",")
}

func splitProxmoxList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		result = append(result, part)
	}

	return sortedStrings(result)
}
