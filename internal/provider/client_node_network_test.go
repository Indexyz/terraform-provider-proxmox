// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClientNodeNetworkStagedCRUDUsesCollectionAndPendingChanges(t *testing.T) {
	ctx := context.Background()
	var collectionReads atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/node1/network":
			collectionReads.Add(1)
			_, _ = w.Write([]byte(`{"data":[` + nodeNetworkBridgeJSON + `],"changes":["+ vmbr1: bridge_ports eth9"]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/node1/network":
			assertFormValues(t, r, url.Values{
				"address":         {"10.0.0.2"},
				"autostart":       {"1"},
				"bond-primary":    {"eth1"},
				"bond_mode":       {"active-backup"},
				"bridge_ports":    {"eth1 eth2"},
				"iface":           {"vmbr1"},
				"netmask":         {"255.255.255.0"},
				"type":            {"bridge"},
				"vlan-id":         {"100"},
				"vlan-raw-device": {"eth0"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/nodes/node1/network/vmbr1":
			assertFormValues(t, r, url.Values{
				"bridge_ports": {"eth1"},
				"delete":       {"address,bond-primary,netmask,vlan-id,vlan-raw-device"},
				"mtu":          {"9000"},
				"type":         {"bridge"},
			})
			writeEnvelope(t, w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/nodes/node1/network/vmbr1":
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	ifaces, changes, err := client.NodeNetworkInterfaces(ctx, "node1")
	if err != nil {
		t.Fatalf("NodeNetworkInterfaces() unexpected error: %v", err)
	}
	if len(ifaces) != 1 || ifaces[0].Iface != "vmbr1" || ifaces[0].Autostart.Ptr() == nil || !*ifaces[0].Autostart.Ptr() {
		t.Fatalf("unexpected interfaces: %#v", ifaces)
	}
	if ifaces[0].VLANID.Ptr() == nil || *ifaces[0].VLANID.Ptr() != 100 || ifaces[0].BondPrimary != "eth1" || ifaces[0].VLANRawDevice != "eth0" {
		t.Fatalf("hyphenated keys were not decoded: %#v", ifaces[0])
	}
	if !reflect.DeepEqual(changes, []string{"+ vmbr1: bridge_ports eth9"}) {
		t.Fatalf("pending changes were not captured: %#v", changes)
	}

	if err := client.CreateNodeNetworkInterface(ctx, "node1", "vmbr1", NodeNetworkInterfaceRequest{
		Type:          nodeNetworkTypeBridge,
		Autostart:     boolPtr(true),
		Address:       stringPtr("10.0.0.2"),
		Netmask:       stringPtr("255.255.255.0"),
		BridgePorts:   []string{"eth2", "eth1"},
		BondPrimary:   stringPtr("eth1"),
		VLANID:        intPtr64(100),
		VLANRawDevice: stringPtr("eth0"),
		BondMode:      stringPtr("active-backup"),
	}); err != nil {
		t.Fatalf("CreateNodeNetworkInterface() unexpected error: %v", err)
	}
	if err := client.UpdateNodeNetworkInterface(ctx, "node1", "vmbr1", NodeNetworkInterfaceRequest{
		Type:        nodeNetworkTypeBridge,
		MTU:         intPtr64(9000),
		BridgePorts: []string{"eth1"},
		Delete:      []string{"netmask", "vlan-raw-device", "vlan-id", "bond-primary", "address"},
	}); err != nil {
		t.Fatalf("UpdateNodeNetworkInterface() unexpected error: %v", err)
	}
	if err := client.DeleteNodeNetworkInterface(ctx, "node1", "vmbr1"); err != nil {
		t.Fatalf("DeleteNodeNetworkInterface() unexpected error: %v", err)
	}
	if got, want := collectionReads.Load(), int64(2); got != want {
		t.Fatalf("unexpected collection read count: got %d want %d", got, want)
	}
}

func TestClientNodeNetworkDeleteIsIdempotentAndMissingMapsToNotFound(t *testing.T) {
	ctx := context.Background()
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Method == http.MethodGet {
			writeEnvelope(t, w, []any{})
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
	}))
	defer server.Close()

	client := newHAResourceTestClient(t, server.URL)
	if _, err := client.GetNodeNetworkInterface(ctx, "node1", "vmbr9"); !errors.Is(err, errNotFound) {
		t.Fatalf("GetNodeNetworkInterface() error = %v, want errNotFound", err)
	}
	if err := client.DeleteNodeNetworkInterface(ctx, "node1", "vmbr9"); err != nil {
		t.Fatalf("DeleteNodeNetworkInterface() unexpected error: %v", err)
	}
	if got, want := strings.Join(methods, ","), "GET,GET"; got != want {
		t.Fatalf("unexpected request sequence: got %s want %s", got, want)
	}
}

func TestClientNodeNetworkApplyRequiresAValidUPID(t *testing.T) {
	ctx := context.Background()
	upids := []string{"", "garbage", "UPID:"}
	for _, upid := range upids {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPut || r.URL.Path != "/api2/json/nodes/node1/network" {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
			}
			if upid == "" {
				writeEnvelope(t, w, nil)
				return
			}
			writeEnvelope(t, w, upid)
		}))
		if _, err := newHAResourceTestClient(t, server.URL).ApplyNodeNetwork(ctx, "node1", nil); err == nil {
			t.Fatalf("ApplyNodeNetwork() with UPID %q returned no error", upid)
		}
		server.Close()
	}

	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		got = r.Form
		writeEnvelope(t, w, "UPID:node1:00001234:00000000:00000000:srvreload:networking:root@pam:")
	}))
	defer server.Close()
	upid, err := newHAResourceTestClient(t, server.URL).ApplyNodeNetwork(ctx, "node1", boolPtr(true))
	if err != nil {
		t.Fatalf("ApplyNodeNetwork() unexpected error: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:node1:") {
		t.Fatalf("unexpected UPID: %q", upid)
	}
	if !reflect.DeepEqual(got, url.Values{"regenerate-frr": {"1"}}) {
		t.Fatalf("unexpected apply form: %#v", got)
	}
}

const nodeNetworkBridgeJSON = `{"iface":"vmbr1","type":"bridge","autostart":1,"bridge_ports":"eth1 eth2","vlan-id":100,"vlan-raw-device":"eth0","bond-primary":"eth1"}`
