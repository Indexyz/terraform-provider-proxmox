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

func TestClientSDNSubnetCRUDUsesCIDROnCreateAndInternalIDAfter(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/sdn/vnets/vn1/subnets":
			if got := r.URL.Query().Get("pending"); got != "1" {
				t.Fatalf("expected pending=1 query, got %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"data":[` + sdnSubnetJSON + `]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/cluster/sdn/vnets/vn1/subnets":
			assertFormValues(t, r, url.Values{
				"dhcp-range":    {"start-address=10.1.0.10,end-address=10.1.0.20", "start-address=10.1.0.30,end-address=10.1.0.40"},
				"dnszoneprefix": {"adm"},
				"gateway":       {"10.1.0.1"},
				"lock-token":    {testSDNLockToken},
				"snat":          {"1"},
				"subnet":        {"10.1.0.0/24"},
				"type":          {"subnet"},
				"vnet":          {"vn1"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/sdn/vnets/vn1/subnets/vz1-10.1.0.0-24":
			assertFormValues(t, r, url.Values{
				"delete":     {"dhcp-range,dnszoneprefix"},
				"digest":     {"subnet-digest"},
				"lock-token": {testSDNLockToken},
				"subnet":     {"vz1-10.1.0.0-24"},
				"type":       {"subnet"},
				"vnet":       {"vn1"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/cluster/sdn/vnets/vn1/subnets/vz1-10.1.0.0-24":
			assertFormValues(t, r, url.Values{"lock-token": {testSDNLockToken}})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	subnets, err := client.SDNSubnets(ctx, "vn1")
	if err != nil {
		t.Fatalf("SDNSubnets() unexpected error: %v", err)
	}
	if len(subnets) != 1 || subnets[0].CIDR != "10.1.0.0/24" || subnets[0].Subnet != "vz1-10.1.0.0-24" {
		t.Fatalf("unexpected subnets: %#v", subnets)
	}
	if len(subnets[0].DHCPRanges) != 1 || subnets[0].DHCPRanges[0].StartAddress != "10.1.0.10" {
		t.Fatalf("dhcp ranges were not decoded: %#v", subnets[0].DHCPRanges)
	}
	if err := client.CreateSDNSubnet(ctx, "vn1", "10.1.0.0/24", SDNSubnetRequest{
		Vnet:          "vn1",
		Gateway:       stringPtr("10.1.0.1"),
		SNAT:          boolPtr(true),
		DNSZonePrefix: stringPtr("adm"),
		DHCPRanges: []SDNDHCPRange{
			{StartAddress: "10.1.0.10", EndAddress: "10.1.0.20"},
			{StartAddress: "10.1.0.30", EndAddress: "10.1.0.40"},
		},
		LockToken: testSDNLockToken,
	}); err != nil {
		t.Fatalf("CreateSDNSubnet() unexpected error: %v", err)
	}
	if err := client.UpdateSDNSubnet(ctx, "vn1", "vz1-10.1.0.0-24", SDNSubnetRequest{
		Vnet:      "vn1",
		LockToken: testSDNLockToken,
		Digest:    stringPtr("subnet-digest"),
		Delete:    []string{"dnszoneprefix", "dhcp-range"},
	}); err != nil {
		t.Fatalf("UpdateSDNSubnet() unexpected error: %v", err)
	}
	if err := client.DeleteSDNSubnet(ctx, "vn1", "vz1-10.1.0.0-24", testSDNLockToken); err != nil {
		t.Fatalf("DeleteSDNSubnet() unexpected error: %v", err)
	}
}

func TestClientSDNSubnetMissingUsesCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, []any{})
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	if _, err := client.GetSDNSubnet(context.Background(), "vn1", "10.9.0.0/24"); !errors.Is(err, errNotFound) {
		t.Fatalf("GetSDNSubnet() error = %v, want errNotFound", err)
	}
}

const sdnSubnetJSON = `{"subnet":"vz1-10.1.0.0-24","cidr":"10.1.0.0/24","vnet":"vn1","zone":"vz1","gateway":"10.1.0.1","snat":1,"state":"changed","digest":"subnet-digest","dhcp-range":[{"start-address":"10.1.0.10","end-address":"10.1.0.20"}]}`
