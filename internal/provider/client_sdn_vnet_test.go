// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestClientSDNVnetCRUDUsesLockTokenAndPendingView(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/vnets":
			if got := r.URL.Query().Get("pending"); got != "1" {
				t.Fatalf("expected pending=1 query, got %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"data":[` + sdnVnetJSON + `]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/vnets":
			assertFormValues(t, r, url.Values{
				"alias":         {"app network"},
				"isolate-ports": {"1"},
				"lock-token":    {testSDNLockToken},
				"tag":           {"100"},
				"type":          {"vnet"},
				"vlanaware":     {"1"},
				"vnet":          {"vn1"},
				"zone":          {"vz1"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/sdn/vnets/vn1":
			assertFormValues(t, r, url.Values{
				"delete":     {"isolate-ports,vlanaware"},
				"digest":     {"vnet-digest"},
				"lock-token": {testSDNLockToken},
				"type":       {"vnet"},
				"vnet":       {"vn1"},
				"zone":       {"vz2"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/vnets/vn1":
			assertFormValues(t, r, url.Values{"lock-token": {testSDNLockToken}})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	vnets, err := client.SDNVnets(ctx)
	if err != nil {
		t.Fatalf("SDNVnets() unexpected error: %v", err)
	}
	if len(vnets) != 1 || vnets[0].State != "changed" || vnets[0].Pending["alias"] == nil {
		t.Fatalf("pending state was not decoded: %#v", vnets)
	}
	if vnets[0].Tag.Ptr() == nil || *vnets[0].Tag.Ptr() != 100 || vnets[0].VLANAware.Ptr() == nil || !*vnets[0].VLANAware.Ptr() {
		t.Fatalf("scalar values were not decoded: %#v", vnets[0])
	}

	if err := client.CreateSDNVnet(ctx, "vn1", SDNVnetRequest{
		Zone:         "vz1",
		Alias:        stringPtr("app network"),
		Tag:          intPtr64(100),
		VLANAware:    boolPtr(true),
		IsolatePorts: boolPtr(true),
		LockToken:    testSDNLockToken,
	}); err != nil {
		t.Fatalf("CreateSDNVnet() unexpected error: %v", err)
	}
	if err := client.UpdateSDNVnet(ctx, "vn1", SDNVnetRequest{
		Zone:      "vz2",
		LockToken: testSDNLockToken,
		Digest:    stringPtr("vnet-digest"),
		Delete:    []string{"vlanaware", "isolate-ports"},
	}); err != nil {
		t.Fatalf("UpdateSDNVnet() unexpected error: %v", err)
	}
	if err := client.DeleteSDNVnet(ctx, "vn1", testSDNLockToken); err != nil {
		t.Fatalf("DeleteSDNVnet() unexpected error: %v", err)
	}
}

func TestClientSDNVnetMissingUsesCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, []any{})
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	if _, err := client.GetSDNVnet(context.Background(), "gone"); !errors.Is(err, errNotFound) {
		t.Fatalf("GetSDNVnet() error = %v, want errNotFound", err)
	}
}

const sdnVnetJSON = `{"vnet":"vn1","zone":"vz1","alias":"old alias","tag":100,"vlanaware":1,"state":"changed","digest":"vnet-digest","pending":{"alias":"app network"}}`
