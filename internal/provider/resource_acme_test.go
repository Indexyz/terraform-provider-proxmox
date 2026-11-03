// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const testCertificatePEM = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----"
const testCertificateKey = "-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----"

func TestClientNodeCertificate(t *testing.T) {
	ctx := context.Background()
	installed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/certificates/custom" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{"certificates": {testCertificatePEM}, "key": {testCertificateKey}, "force": {"1"}, "restart": {"1"}})
			installed = true
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/nodes/pve-1/certificates/info" && r.Method == http.MethodGet:
			certificates := []map[string]any{{"filename": "pve-ssl.pem", "fingerprint": "aa"}}
			if installed {
				certificates = append(certificates, map[string]any{"filename": "pveproxy-ssl.pem", "fingerprint": "bb", "issuer": "CN=ca", "subject": "CN=host", "san": "host.example.test,DNS:host.example.test", "notafter": "2030-01-01", "notbefore": "2026-01-01", "public-key-type": "RSA", "public-key-bits": 2048})
			}
			writeEnvelope(t, w, certificates)
		case r.URL.Path == "/api2/json/nodes/pve-1/certificates/custom" && r.Method == http.MethodDelete:
			assertFormValues(t, r, url.Values{"restart": {"1"}})
			installed = false
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)

	if err := client.UploadNodeCertificate(ctx, "pve-1", testCertificatePEM, testCertificateKey, true, true); err != nil {
		t.Fatalf("UploadNodeCertificate() unexpected error: %v", err)
	}
	certificate, err := client.GetNodeCertificate(ctx, "pve-1", nodeCustomCertificateFile)
	if err != nil || certificate.Fingerprint != "bb" || certificate.PublicKeyBits != 2048 {
		t.Fatalf("unexpected certificate: %#v (%v)", certificate, err)
	}
	if err := client.DeleteNodeCertificate(ctx, "pve-1", true); err != nil {
		t.Fatalf("DeleteNodeCertificate() unexpected error: %v", err)
	}
	if _, err := client.GetNodeCertificate(ctx, "pve-1", nodeCustomCertificateFile); err == nil {
		t.Fatal("deleted certificate must be missing")
	}
}

func TestClientACMEEndpoints(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/cluster/acme/account" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{"contact": {"mailto:a@b.test"}, "name": {"le"}, "directory": {"https://ca/dir"}, "eab-kid": {"kid"}, "eab-hmac-key": {"hmac"}})
			writeEnvelope(t, w, "UPID:pve-1:0000:0000:0000:acmeregister::root:")
		case r.URL.Path == "/api2/json/cluster/acme/account" && r.Method == http.MethodGet:
			writeEnvelope(t, w, []map[string]any{{"name": "le", "contact": "mailto:a@b.test", "location": "https://ca/acct/1", "status": "valid"}})
		case r.URL.Path == "/api2/json/cluster/acme/account/le" && r.Method == http.MethodGet:
			writeEnvelope(t, w, map[string]any{"account": map[string]any{"contact": []string{"mailto:a@b.test"}, "status": "valid"}, "directory": "https://ca/dir", "location": "https://ca/acct/1"})
		case r.URL.Path == "/api2/json/cluster/acme/account/le" && r.Method == http.MethodPut:
			assertFormValues(t, r, url.Values{"contact": {"mailto:c@d.test"}, "name": {"le"}})
			writeEnvelope(t, w, "UPID:pve-1:0000:0000:0000:acmeupdate::root:")
		case r.URL.Path == "/api2/json/cluster/acme/account/le" && r.Method == http.MethodDelete:
			writeEnvelope(t, w, "UPID:pve-1:0000:0000:0000:acmeremove::root:")
		case r.URL.Path == "/api2/json/cluster/acme/plugins" && r.Method == http.MethodPost:
			assertFormValues(t, r, url.Values{"id": {"cf"}, "type": {"dns"}, "api": {"cloudflare"}, "data": {"a1b2"}})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/acme/plugins" && r.Method == http.MethodGet:
			writeEnvelope(t, w, []map[string]any{{"plugin": "cf", "type": "dns", "api": "cloudflare", "data": "a1b2", "digest": "d1"}})
		case r.URL.Path == "/api2/json/cluster/acme/plugins/cf" && r.Method == http.MethodPut:
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/acme/plugins/cf" && r.Method == http.MethodDelete:
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/nodes/pve-1/certificates/acme/certificate" && r.Method == http.MethodPost:
			writeEnvelope(t, w, "UPID:pve-1:0000:0000:0000:acme:order:root@pam:")
		case r.URL.Path == "/api2/json/nodes/pve-1/certificates/acme/certificate" && r.Method == http.MethodDelete:
			writeEnvelope(t, w, "UPID:pve-1:0000:0000:0000:acme:revoke:root@pam:")
		case r.URL.Path == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:0000:0000:0000:acme:order:root@pam:/status",
			r.URL.Path == "/api2/json/nodes/pve-1/tasks/UPID:pve-1:0000:0000:0000:acme:revoke:root@pam:/status":
			writeEnvelope(t, w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)

	if _, err := client.CreateACMEAccount(ctx, ACMEAccount{Name: "le", Directory: "https://ca/dir", Details: ACMEAccountDetail{Contact: []string{"mailto:a@b.test"}}}, "kid", "hmac"); err != nil {
		t.Fatalf("CreateACMEAccount() unexpected error: %v", err)
	}
	account, err := client.GetACMEAccount(ctx, "le")
	if err != nil || account.Location != "https://ca/acct/1" || strings.Join(account.Details.Contact, ",") != "mailto:a@b.test" {
		t.Fatalf("unexpected account: %#v (%v)", account, err)
	}
	if _, err := client.UpdateACMEAccount(ctx, "le", "mailto:c@d.test"); err != nil {
		t.Fatalf("UpdateACMEAccount() unexpected error: %v", err)
	}
	if _, err := client.DeleteACMEAccount(ctx, "le"); err != nil {
		t.Fatalf("DeleteACMEAccount() unexpected error: %v", err)
	}
	if err := client.CreateACMEPlugin(ctx, ACMEPlugin{ID: "cf", Type: "dns", API: "cloudflare", Data: "a1b2"}, nil, nil); err != nil {
		t.Fatalf("CreateACMEPlugin() unexpected error: %v", err)
	}
	plugin, err := client.GetACMEPlugin(ctx, "cf")
	if err != nil || plugin.Digest != "d1" {
		t.Fatalf("unexpected plugin: %#v (%v)", plugin, err)
	}
	if err := client.UpdateACMEPlugin(ctx, "cf", plugin, nil, nil, nil, plugin.Digest); err != nil {
		t.Fatalf("UpdateACMEPlugin() unexpected error: %v", err)
	}
	if err := client.DeleteACMEPlugin(ctx, "cf"); err != nil {
		t.Fatalf("DeleteACMEPlugin() unexpected error: %v", err)
	}
	upid, err := client.OrderNodeACMECertificate(ctx, "pve-1", true)
	if err != nil || upid == "" {
		t.Fatalf("OrderNodeACMECertificate() = %q, %v", upid, err)
	}
	if _, err := client.RevokeNodeACMECertificate(ctx, "pve-1"); err != nil {
		t.Fatalf("RevokeNodeACMECertificate() unexpected error: %v", err)
	}
}

func TestNodeCertificateLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	installed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if err := r.ParseForm(); err != nil {
			handler.fail(w, "parse form: %v", err)
			return
		}
		switch {
		case r.URL.Path == "/api2/json/nodes/pve-1/certificates/custom" && r.Method == http.MethodPost:
			installed = true
			handler.envelope(w, nil)
		case r.URL.Path == "/api2/json/nodes/pve-1/certificates/info":
			certificates := []any{}
			if installed {
				certificates = append(certificates, map[string]any{"filename": nodeCustomCertificateFile, "fingerprint": "bb", "san": "host.example.test"})
			}
			handler.envelope(w, certificates)
		case r.URL.Path == "/api2/json/nodes/pve-1/certificates/custom" && r.Method == http.MethodDelete:
			installed = false
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &NodeCertificateResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := nodeCertificateModel{ID: types.StringNull(), Node: types.StringValue("pve-1"), Certificates: types.StringValue(testCertificatePEM), Key: types.StringValue(testCertificateKey), Force: types.BoolValue(true), Restart: types.BoolValue(true), Fingerprint: types.StringNull(), Issuer: types.StringNull(), Subject: types.StringNull(), SAN: types.ListNull(types.StringType), NotAfter: types.StringNull(), NotBefore: types.StringNull(), PublicKeyType: types.StringNull(), PublicKeyBits: types.Int64Null()}
	createResp := resource.CreateResponse{State: testResourceState(t, schema, config)}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created nodeCertificateModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() || created.Fingerprint.ValueString() != "bb" {
		t.Fatalf("unexpected state: %#v (%v)", created, diags)
	}
	if created.Key.ValueString() != testCertificateKey {
		t.Fatalf("key was not preserved: %#v", created)
	}
	deleteResp := resource.DeleteResponse{State: createResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: createResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() || installed {
		t.Fatalf("delete failed: %v installed=%v", deleteResp.Diagnostics, installed)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

func TestACMEPluginDataRoundTrip(t *testing.T) {
	value := types.MapValueMust(types.StringType, map[string]attr.Value{"CF_API_TOKEN": types.StringValue("token"), "CF_ZONE": types.StringValue("zone")})
	encoded, diags := acmePluginDataFromModel(value)
	if diags.HasError() || encoded == "" {
		t.Fatalf("encode failed: %v", diags)
	}
	decoded, decodeDiags := acmePluginDataToModel(context.Background(), encoded)
	if decodeDiags.HasError() {
		t.Fatalf("decode failed: %v", decodeDiags)
	}
	items := map[string]string{}
	decoded.ElementsAs(context.Background(), &items, false)
	if items["CF_API_TOKEN"] != "token" || items["CF_ZONE"] != "zone" {
		t.Fatalf("round trip mismatch: %#v", items)
	}
}

func TestACMEValidation(t *testing.T) {
	account := acmeAccountModel{ID: types.StringNull(), Name: types.StringValue("le"), Contact: types.StringValue("mailto:a@b.test"), Directory: types.StringNull(), TOS: types.StringNull(), EABKID: types.StringNull(), EABHMAC: types.StringNull(), Location: types.StringNull()}
	var resp resource.ValidateConfigResponse
	(&ACMEAccountResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &ACMEAccountResource{}), account)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("valid account diagnostics: %v", resp.Diagnostics)
	}
	account.Contact = types.StringValue("a@b.test")
	account.EABKID = types.StringValue("kid")
	resp = resource.ValidateConfigResponse{}
	(&ACMEAccountResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &ACMEAccountResource{}), account)}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("invalid account accepted")
	}

	plugin := acmePluginModel{ID: types.StringNull(), Plugin: types.StringValue("cf"), Type: types.StringValue("dns"), API: types.StringNull(), Data: types.MapNull(types.StringType), Nodes: types.SetNull(types.StringType), ValidationDelay: types.Int64Null(), Disable: types.BoolNull()}
	resp = resource.ValidateConfigResponse{}
	(&ACMEPluginResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &ACMEPluginResource{}), plugin)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("valid plugin diagnostics: %v", resp.Diagnostics)
	}
	plugin.Type = types.StringValue("xen")
	plugin.ValidationDelay = types.Int64Value(-1)
	resp = resource.ValidateConfigResponse{}
	(&ACMEPluginResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &ACMEPluginResource{}), plugin)}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("invalid plugin accepted")
	}
}
