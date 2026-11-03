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
	"testing"
)

const testSDNLockToken = "12345678-1234-1234-1234-123456789012"

func TestClientSDNLockUnlockExactForms(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/lock":
			assertFormValues(t, r, url.Values{"allow-pending": {"1"}})
			writeEnvelope(t, w, testSDNLockToken)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/lock":
			assertFormValues(t, r, url.Values{"lock-token": {testSDNLockToken}})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	token, err := client.LockSDN(ctx)
	if err != nil || token != testSDNLockToken {
		t.Fatalf("LockSDN() = %q, %v", token, err)
	}
	if err := client.UnlockSDN(ctx, token); err != nil {
		t.Fatalf("UnlockSDN() unexpected error: %v", err)
	}
}

func TestClientSDNZoneCRUDUsesLockTokenAndPendingView(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/zones":
			if got := r.URL.Query().Get("pending"); got != "1" {
				t.Fatalf("expected pending=1 query, got %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"data":[` + sdnZoneJSON + `]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/zones":
			assertFormValues(t, r, url.Values{
				"bridge":                      {"vmbr0"},
				"bridge-disable-mac-learning": {"1"},
				"dns":                         {"dns1"},
				"ipam":                        {"pve"},
				"lock-token":                  {testSDNLockToken},
				"nodes":                       {"node1,node2"},
				"type":                        {"vlan"},
				"zone":                        {"vz1"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/sdn/zones/vz1":
			assertFormValues(t, r, url.Values{
				"bridge":     {"vmbr1"},
				"delete":     {"bridge-disable-mac-learning,dnsserver"},
				"digest":     {"sdn-digest"},
				"lock-token": {testSDNLockToken},
				"type":       {"vlan"},
				"zone":       {"vz1"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/zones/vz1":
			assertFormValues(t, r, url.Values{"lock-token": {testSDNLockToken}})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	zones, err := client.SDNZones(ctx)
	if err != nil {
		t.Fatalf("SDNZones() unexpected error: %v", err)
	}
	if len(zones) != 1 || zones[0].State != "changed" || zones[0].Pending["bridge"] == nil {
		t.Fatalf("pending state was not decoded: %#v", zones)
	}
	if zones[0].MTU.Ptr() == nil || *zones[0].MTU.Ptr() != 9000 {
		t.Fatalf("numeric value was not decoded: %#v", zones[0])
	}
	if staged := sdnPendingMap(zones[0].Pending); staged["dns"] != "deleted" || staged["bridge"] != "vmbr1" {
		t.Fatalf("unexpected staged map: %#v", staged)
	}

	if err := client.CreateSDNZone(ctx, "vz1", SDNZoneRequest{
		Type:                    sdnZoneTypeVLAN,
		Nodes:                   []string{"node2", "node1"},
		Bridge:                  stringPtr("vmbr0"),
		DNS:                     stringPtr("dns1"),
		IPAM:                    stringPtr("pve"),
		BridgeDisableMACLearned: boolPtr(true),
		LockToken:               testSDNLockToken,
	}); err != nil {
		t.Fatalf("CreateSDNZone() unexpected error: %v", err)
	}
	if err := client.UpdateSDNZone(ctx, "vz1", SDNZoneRequest{
		Type:      sdnZoneTypeVLAN,
		Bridge:    stringPtr("vmbr1"),
		LockToken: testSDNLockToken,
		Digest:    stringPtr("sdn-digest"),
		Delete:    []string{"dnsserver", "bridge-disable-mac-learning"},
	}); err != nil {
		t.Fatalf("UpdateSDNZone() unexpected error: %v", err)
	}
	if err := client.DeleteSDNZone(ctx, "vz1", testSDNLockToken); err != nil {
		t.Fatalf("DeleteSDNZone() unexpected error: %v", err)
	}
}

func TestClientSDNZoneMissingUsesCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, []any{})
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	if _, err := client.GetSDNZone(context.Background(), "gone"); !errors.Is(err, errNotFound) {
		t.Fatalf("GetSDNZone() error = %v, want errNotFound", err)
	}
}

func TestClientApplySDNLocksCommitsAndReleases(t *testing.T) {
	ctx := context.Background()
	upid := "UPID:node1:00001234:00000000:00000000:reloadnetworkall::root@pam:"
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/lock":
			writeEnvelope(t, w, testSDNLockToken)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/sdn":
			assertFormValues(t, r, url.Values{"lock-token": {testSDNLockToken}, "release-lock": {"1"}})
			writeEnvelope(t, w, upid)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	got, err := client.ApplySDN(ctx)
	if err != nil || got != upid {
		t.Fatalf("ApplySDN() = %q, %v", got, err)
	}
	if want := "POST /api2/json/cluster/sdn/lock,PUT /api2/json/cluster/sdn"; strings.Join(methods, ",") != want {
		t.Fatalf("unexpected request sequence: got %v want %s", methods, want)
	}
}

func TestClientApplySDNUnlocksOnFailureAndRejectsEmptyUPID(t *testing.T) {
	ctx := context.Background()

	t.Run("commit failure unlocks", func(t *testing.T) {
		var unlocked bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				writeEnvelope(t, w, testSDNLockToken)
			case http.MethodPut:
				writeAPIError(t, w, http.StatusInternalServerError, "commit failed")
			case http.MethodDelete:
				assertFormValues(t, r, url.Values{"lock-token": {testSDNLockToken}})
				unlocked = true
				writeEnvelope(t, w, nil)
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
			}
		}))
		defer server.Close()
		if _, err := newHAResourceTestClient(t, server.URL).ApplySDN(ctx); err == nil || !strings.Contains(err.Error(), "commit failed") {
			t.Fatalf("ApplySDN() error = %v", err)
		}
		if !unlocked {
			t.Fatal("failed commit did not release the lock")
		}
	})

	t.Run("empty upid", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				writeEnvelope(t, w, testSDNLockToken)
				return
			}
			writeEnvelope(t, w, nil)
		}))
		defer server.Close()
		if _, err := newHAResourceTestClient(t, server.URL).ApplySDN(ctx); err == nil || !strings.Contains(err.Error(), "no UPID") {
			t.Fatalf("ApplySDN() with empty UPID error = %v", err)
		}
	})
}

const sdnZoneJSON = `{"zone":"vz1","type":"vlan","nodes":"node1,node2","mtu":"9000","bridge":"vmbr0","state":"changed","digest":"sdn-digest","pending":{"bridge":"vmbr1","dns":"deleted"}}`
