// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:     "base URL appends api path",
			input:    "https://pve.example.com:8006",
			expected: "https://pve.example.com:8006/api2/json",
		},
		{
			name:     "existing api path preserved",
			input:    "https://pve.example.com:8006/api2/json",
			expected: "https://pve.example.com:8006/api2/json",
		},
		{
			name:        "query string rejected",
			input:       "https://pve.example.com:8006?foo=bar",
			expectError: true,
		},
		{
			name:        "scheme required",
			input:       "pve.example.com:8006",
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			endpoint, err := normalizeEndpoint(test.input)
			if test.expectError {
				if err == nil {
					t.Fatalf("expected an error for %q", test.input)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error for %q: %v", test.input, err)
			}

			if got := endpoint.String(); got != test.expected {
				t.Fatalf("unexpected normalized endpoint: got %q want %q", got, test.expected)
			}
		})
	}
}

func TestNewClientTicketAuthAndGroupRequests(t *testing.T) {
	ctx := context.Background()
	var sawLogin bool
	var sawCreate bool
	var sawList bool
	var sawRead bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/access/ticket":
			sawLogin = true
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected login method: %s", r.Method)
			}
			assertFormValues(t, r, url.Values{
				"username": {"root@pam"},
				"password": {"secret"},
				"otp":      {"123456"},
			})
			writeEnvelope(t, w, map[string]any{
				"ticket":              "ticket-1",
				"CSRFPreventionToken": "csrf-1",
				"username":            "root@pam",
			})
		case r.URL.Path == "/api2/json/access/groups" && r.Method == http.MethodPost:
			sawCreate = true
			assertCookie(t, r, "PVEAuthCookie", "ticket-1")
			if got := r.Header.Get("CSRFPreventionToken"); got != "csrf-1" {
				t.Fatalf("unexpected csrf header: %q", got)
			}
			assertFormValues(t, r, url.Values{
				"groupid": {"ops"},
				"comment": {"Ops team"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/access/groups" && r.Method == http.MethodGet:
			sawList = true
			assertCookie(t, r, "PVEAuthCookie", "ticket-1")
			if got := r.Header.Get("CSRFPreventionToken"); got != "" {
				t.Fatalf("expected no csrf header on GET, got %q", got)
			}
			writeEnvelope(t, w, []map[string]any{{
				"groupid": "ops",
				"comment": "Ops team",
				"users":   "bob@pve,alice@pve",
			}})
		case r.URL.Path == "/api2/json/access/groups/ops" && r.Method == http.MethodGet:
			sawRead = true
			assertCookie(t, r, "PVEAuthCookie", "ticket-1")
			writeEnvelope(t, w, map[string]any{
				"comment": "Ops team",
				"members": []string{"bob@pve", "alice@pve"},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewClient(ctx, ClientConfig{
		Endpoint:  server.URL,
		Username:  "root@pam",
		Password:  "secret",
		OTP:       "123456",
		Timeout:   time.Second,
		UserAgent: "terraform-provider-proxmox/test",
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}

	comment := "Ops team"
	if err := client.CreateGroup(ctx, "ops", &comment); err != nil {
		t.Fatalf("CreateGroup() unexpected error: %v", err)
	}

	groups, err := client.Groups(ctx)
	if err != nil {
		t.Fatalf("Groups() unexpected error: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("unexpected group count: %d", len(groups))
	}
	if got, want := groups[0].Members, []string{"alice@pve", "bob@pve"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected group users: got %v want %v", got, want)
	}

	group, err := client.GetGroup(ctx, "ops")
	if err != nil {
		t.Fatalf("GetGroup() unexpected error: %v", err)
	}
	if got, want := group.Members, []string{"alice@pve", "bob@pve"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected group members: got %v want %v", got, want)
	}

	if !sawLogin || !sawCreate || !sawList || !sawRead {
		t.Fatalf("expected all ticket auth requests to run, got login=%t create=%t list=%t read=%t", sawLogin, sawCreate, sawList, sawRead)
	}
}

func TestClientTokenAuthPoolAndGroupMethods(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)

		switch {
		case r.URL.Path == "/api2/json/pools" && r.Method == http.MethodGet && r.URL.Query().Get("poolid") == "platform":
			writeEnvelope(t, w, []map[string]any{{
				"poolid":  "platform",
				"comment": "Managed by Terraform",
				"members": []map[string]any{{
					"id":   "qemu/101",
					"node": "pve-1",
					"type": "qemu",
					"vmid": 101,
				}, {
					"id":      "storage/local-lvm",
					"node":    "pve-1",
					"type":    "storage",
					"storage": "local-lvm",
				}},
			}})
		case r.URL.Path == "/api2/json/pools" && r.Method == http.MethodGet:
			writeEnvelope(t, w, []map[string]any{{
				"poolid":  "platform",
				"comment": "Managed by Terraform",
			}})
		case r.URL.Path == "/api2/json/access/groups" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{
				"groupid": {"devs"},
				"comment": {"Developer access"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/access/groups/devs" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{
				"groupid": {"devs"},
				"comment": {"Developers"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/access/groups/devs" && r.Method == http.MethodGet:
			writeEnvelope(t, w, map[string]any{
				"comment": "Developers",
				"members": []string{"zoe@pve"},
			})
		case r.URL.Path == "/api2/json/access/groups/devs" && r.Method == http.MethodDelete:
			assertFormValues(t, r, url.Values{
				"groupid": {"devs"},
			})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewClient(ctx, ClientConfig{
		Endpoint:       server.URL,
		APITokenID:     "terraform@pve!provider",
		APITokenSecret: "token-secret",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}

	comment := "Developer access"
	if err := client.CreateGroup(ctx, "devs", &comment); err != nil {
		t.Fatalf("CreateGroup() unexpected error: %v", err)
	}
	updatedComment := "Developers"
	if err := client.UpdateGroup(ctx, "devs", &updatedComment); err != nil {
		t.Fatalf("UpdateGroup() unexpected error: %v", err)
	}
	group, err := client.GetGroup(ctx, "devs")
	if err != nil {
		t.Fatalf("GetGroup() unexpected error: %v", err)
	}
	if group.Comment != "Developers" {
		t.Fatalf("unexpected group comment: %q", group.Comment)
	}
	if err := client.DeleteGroup(ctx, "devs"); err != nil {
		t.Fatalf("DeleteGroup() unexpected error: %v", err)
	}

	pool, err := client.GetPool(ctx, "platform")
	if err != nil {
		t.Fatalf("GetPool() unexpected error: %v", err)
	}
	if pool.PoolID != "platform" {
		t.Fatalf("unexpected pool id: %q", pool.PoolID)
	}

	pools, err := client.Pools(ctx)
	if err != nil {
		t.Fatalf("Pools() unexpected error: %v", err)
	}
	if len(pools) != 1 || pools[0].PoolID != "platform" {
		t.Fatalf("unexpected pools payload: %#v", pools)
	}
}

func TestClientGetGroupNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), ClientConfig{
		Endpoint:       server.URL,
		APITokenID:     "terraform@pve!provider",
		APITokenSecret: "token-secret",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}

	_, err = client.GetGroup(context.Background(), "missing")
	if !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}

func TestClientDecodeAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(t, w, map[string]any{"errors": map[string]string{"comment": "invalid comment"}})
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), ClientConfig{
		Endpoint:       server.URL,
		APITokenID:     "terraform@pve!provider",
		APITokenSecret: "token-secret",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}

	err = client.DeleteGroup(context.Background(), "broken")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T (%v)", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected status code: %d", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Error(), "invalid comment") {
		t.Fatalf("unexpected error text: %s", apiErr.Error())
	}
}

func TestClientNodeAndClusterInventoryMethods(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)

		switch r.URL.Path {
		case "/api2/json/nodes/pve-1/dns":
			writeEnvelope(t, w, map[string]any{
				"dns1":   "1.1.1.1",
				"dns2":   "8.8.8.8",
				"dns3":   "9.9.9.9",
				"search": "example.internal",
			})
		case "/api2/json/nodes/pve-1/time":
			writeEnvelope(t, w, map[string]any{
				"localtime": 1710000100,
				"time":      1710000000,
				"timezone":  "Asia/Hong_Kong",
			})
		case "/api2/json/cluster/metrics/server":
			writeEnvelope(t, w, []map[string]any{{
				"disable": false,
				"id":      "influx-primary",
				"port":    8089,
				"server":  "influxdb.internal",
				"type":    "influxdb",
			}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewClient(ctx, ClientConfig{
		Endpoint:       server.URL,
		APITokenID:     "terraform@pve!provider",
		APITokenSecret: "token-secret",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}

	dns, err := client.NodeDNS(ctx, "pve-1")
	if err != nil {
		t.Fatalf("NodeDNS() unexpected error: %v", err)
	}
	if dns.DNS1 != "1.1.1.1" || dns.Search != "example.internal" {
		t.Fatalf("unexpected dns payload: %#v", dns)
	}

	nodeTime, err := client.NodeTime(ctx, "pve-1")
	if err != nil {
		t.Fatalf("NodeTime() unexpected error: %v", err)
	}
	if nodeTime.Timezone != "Asia/Hong_Kong" || nodeTime.Time != 1710000000 {
		t.Fatalf("unexpected time payload: %#v", nodeTime)
	}

	servers, err := client.ClusterMetricsServers(ctx)
	if err != nil {
		t.Fatalf("ClusterMetricsServers() unexpected error: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("unexpected metrics server count: %d", len(servers))
	}
	if servers[0].ID != "influx-primary" || servers[0].Port == nil || *servers[0].Port != 8089 {
		t.Fatalf("unexpected metrics server payload: %#v", servers[0])
	}
}

func assertTokenAuth(t *testing.T, r *http.Request) {
	t.Helper()
	want := "PVEAPIToken=terraform@pve!provider=token-secret"
	if got := r.Header.Get("Authorization"); got != want {
		t.Fatalf("unexpected authorization header: got %q want %q", got, want)
	}
	if cookie := r.Header.Get("Cookie"); cookie != "" {
		t.Fatalf("expected no auth cookie with token auth, got %q", cookie)
	}
}

func assertCookie(t *testing.T, r *http.Request, name, value string) {
	t.Helper()
	cookie, err := r.Cookie(name)
	if err != nil {
		t.Fatalf("missing cookie %q: %v", name, err)
	}
	if cookie.Value != value {
		t.Fatalf("unexpected cookie value for %s: got %q want %q", name, cookie.Value, value)
	}
}

func assertFormValues(t *testing.T, r *http.Request, want url.Values) {
	t.Helper()

	var got url.Values
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() unexpected error: %v", err)
		}
		got = r.Form
	default:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() unexpected error: %v", err)
		}
		got, err = url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("ParseQuery() unexpected error: %v", err)
		}
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected form values: got %#v want %#v", got, want)
	}
}

func writeEnvelope(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()
	writeJSON(t, w, map[string]any{"data": data})
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("Encode() unexpected error: %v", err)
	}
}
