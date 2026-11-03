// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClientHARuleCRUDUsesCollectionDigestAndExactFields(t *testing.T) {
	ctx := context.Background()
	var collectionReads atomic.Int64
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		methods = append(methods, r.Method)
		switch {
		case r.URL.Path == "/api2/json/cluster/ha/rules" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{
				"affinity":  {"negative"},
				"comment":   {"avoid backups"},
				"disable":   {"1"},
				"nodes":     {"node1:2,node2"},
				"resources": {"ct:201,vm:120"},
				"rule":      {"batch-off-backup"},
				"strict":    {"1"},
				"type":      {"node-affinity"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/ha/rules" && r.Method == http.MethodGet:
			collectionReads.Add(1)
			writeEnvelope(t, w, []map[string]any{
				{
					"affinity":  "positive",
					"comment":   "database pair",
					"digest":    "rule-digest",
					"disable":   0,
					"order":     1,
					"resources": "ct:201,vm:120",
					"rule":      "db-together",
					"strict":    1,
					"type":      "node-affinity",
					"nodes":     "node1:2,node2",
					"errors":    map[string]string{"nodes": "node1 is offline"},
				},
				{
					"affinity":  "negative",
					"digest":    "rule-digest",
					"order":     "2",
					"resources": "ct:201,vm:120",
					"rule":      "db-apart",
					"type":      "resource-affinity",
				},
			})
		case r.URL.Path == "/api2/json/cluster/ha/rules/db-together" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{
				"affinity":  {"positive"},
				"comment":   {"database pair"},
				"delete":    {"disable,strict"},
				"digest":    {"rule-digest"},
				"nodes":     {"node1:2,node2"},
				"resources": {"ct:201,vm:120"},
				"rule":      {"db-together"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/ha/rules/db-together" && r.Method == http.MethodDelete:
			assertFormValues(t, r, url.Values{})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	if err := client.CreateHARule(ctx, "batch-off-backup", HARuleRequest{
		Type:      haRuleTypeNodeAffinity,
		Resources: []string{"vm:120", "ct:201"},
		Nodes:     []HANode{{Node: "node2"}, {Node: "node1", Priority: 2}},
		Affinity:  stringPtr("negative"),
		Strict:    boolPtr(true),
		Disable:   boolPtr(true),
		Comment:   stringPtr("avoid backups"),
	}); err != nil {
		t.Fatalf("CreateHARule() unexpected error: %v", err)
	}

	rule, err := client.GetHARule(ctx, "db-together")
	if err != nil {
		t.Fatalf("GetHARule() unexpected error: %v", err)
	}
	if rule.Digest != "rule-digest" || rule.Resources != "ct:201,vm:120" || rule.Nodes != "node1:2,node2" {
		t.Fatalf("unexpected HA rule payload: %#v", rule)
	}
	if rule.Order.Ptr() == nil || *rule.Order.Ptr() != 1 || rule.Strict.Ptr() == nil || !*rule.Strict.Ptr() || rule.Disable.Ptr() == nil || *rule.Disable.Ptr() {
		t.Fatalf("unexpected HA rule scalars: %#v", rule)
	}
	if rule.Errors["nodes"] != "node1 is offline" {
		t.Fatalf("HA rule feasibility errors were not decoded: %#v", rule.Errors)
	}
	apart, err := client.GetHARule(ctx, "db-apart")
	if err != nil {
		t.Fatalf("GetHARule() unexpected error: %v", err)
	}
	if apart.Affinity != "negative" || apart.Order.Ptr() == nil || *apart.Order.Ptr() != 2 || apart.Nodes != "" || apart.Strict.Ptr() != nil {
		t.Fatalf("unexpected resource-affinity payload: %#v", apart)
	}

	if err := client.UpdateHARule(ctx, "db-together", HARuleRequest{
		Rule:      "db-together",
		Resources: []string{"ct:201", "vm:120"},
		Nodes:     []HANode{{Node: "node1", Priority: 2}, {Node: "node2"}},
		Affinity:  stringPtr("positive"),
		Comment:   stringPtr("database pair"),
		Digest:    stringPtr(rule.Digest),
		Delete:    []string{"strict", "disable"},
	}); err != nil {
		t.Fatalf("UpdateHARule() unexpected error: %v", err)
	}
	if err := client.DeleteHARule(ctx, "db-together"); err != nil {
		t.Fatalf("DeleteHARule() unexpected error: %v", err)
	}
	if got, want := collectionReads.Load(), int64(2); got != want {
		t.Fatalf("unexpected collection read count: got %d want %d", got, want)
	}
	if got, want := strings.Join(methods, ","), "POST,GET,GET,PUT,DELETE"; got != want {
		t.Fatalf("unexpected request sequence: got %s want %s", got, want)
	}
}

func TestClientHARuleMissingUsesCollectionAndDeleteIsIdempotent(t *testing.T) {
	ctx := context.Background()
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.URL.Path != "/api2/json/cluster/ha/rules" && r.URL.Path != "/api2/json/cluster/ha/rules/gone" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if r.Method == http.MethodGet {
			writeEnvelope(t, w, []any{})
			return
		}
		// Proxmox answers a missing rule DELETE with success.
		writeEnvelope(t, w, nil)
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	if _, err := client.GetHARule(ctx, "gone"); !errors.Is(err, errNotFound) {
		t.Fatalf("GetHARule() error = %v, want errNotFound", err)
	}
	if err := client.DeleteHARule(ctx, "gone"); err != nil {
		t.Fatalf("DeleteHARule() unexpected error: %v", err)
	}
	if got, want := strings.Join(methods, ","), "GET,DELETE"; got != want {
		t.Fatalf("unexpected request sequence: got %s want %s", got, want)
	}
}

func TestClientHARulePreservesFeasibilityError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":{"resources":"resource 'vm:120' is already used in another node affinity rule"},"data":null}`))
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	err := client.CreateHARule(context.Background(), "db-together", HARuleRequest{
		Type:      haRuleTypeNodeAffinity,
		Resources: []string{"vm:120"},
		Nodes:     []HANode{{Node: "node1"}},
	})
	if err == nil || !strings.Contains(err.Error(), "status 400") || !strings.Contains(err.Error(), "already used in another node affinity rule") {
		t.Fatalf("CreateHARule() did not preserve the feasibility error: %v", err)
	}
}
