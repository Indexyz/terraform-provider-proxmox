// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"
)

func TestNodeCertificatesInfoDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/certificates/info": []map[string]any{
			{"filename": "pveproxy-ssl.pem", "subject": "CN=pve-1", "issuer": "CN=pve-1", "notafter": 20, "notbefore": 10, "fingerprint": "aa", "san": []string{"DNS:pve-1"}, "public-key-bits": 2048, "public-key-type": "RSA"},
		},
	}
	readResp := runSDNDataSource(t, NewNodeCertificatesInfoDataSource(), responses, map[string]any{"node": "pve-1"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state nodeCertificatesInfoModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() || len(state.Certificates) != 1 {
		t.Fatalf("unexpected projection: %#v (%v)", state.Certificates, diags)
	}
	entry := state.Certificates[0]
	if entry.Filename.ValueString() != "pveproxy-ssl.pem" || entry.NotAfter.ValueInt64() != 20 || len(entry.SAN.Elements()) != 1 {
		t.Fatalf("unexpected certificate: %#v", entry)
	}
}

func TestNodeSubscriptionDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/subscription": map[string]any{"status": "notfound", "message": "There is no subscription key", "serverid": "abc", "url": "https://proxmox.com"},
	}
	readResp := runSDNDataSource(t, NewNodeSubscriptionDataSource(), responses, map[string]any{"node": "pve-1"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state nodeSubscriptionModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() || state.Status.ValueString() != "notfound" || state.ServerID.ValueString() != "abc" {
		t.Fatalf("unexpected projection: %#v (%v)", state, diags)
	}
}
