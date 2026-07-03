// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestClientACLMethods(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		switch {
		case r.URL.Path == "/api2/json/access/acl" && r.Method == http.MethodPut && r.FormValue("delete") != "1":
			assertFormValues(t, r, url.Values{
				"path":      {"/vms/101"},
				"roles":     {"TerraformManage,Auditor"},
				"users":     {"admin@pam"},
				"groups":    {"admins"},
				"propagate": {"1"},
			})
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/access/acl" && r.Method == http.MethodGet:
			writeEnvelope(t, w, []map[string]any{
				{"path": "/vms/101", "propagate": 1, "roleid": "TerraformManage", "type": "user", "ugid": "admin@pam"},
				{"path": "/vms/101", "propagate": 1, "roleid": "Auditor", "type": "group", "ugid": "admins"},
				{"path": "/vms/102", "propagate": 1, "roleid": "Administrator", "type": "user", "ugid": "root@pam"},
			})
		case r.URL.Path == "/api2/json/access/acl" && r.Method == http.MethodPut && r.FormValue("delete") == "1":
			assertFormValues(t, r, url.Values{
				"path":      {"/vms/101"},
				"roles":     {"TerraformManage,Auditor"},
				"users":     {"admin@pam"},
				"groups":    {"admins"},
				"propagate": {"1"},
				"delete":    {"1"},
			})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s form=%v", r.Method, r.URL.String(), r.Form)
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

	prop := true
	if err := client.SetACL(ctx, ACLRequest{
		Path:      "/vms/101",
		Roles:     "TerraformManage,Auditor",
		Users:     "admin@pam",
		Groups:    "admins",
		Propagate: &prop,
	}); err != nil {
		t.Fatalf("SetACL() unexpected error: %v", err)
	}

	entries, err := client.GetACL(ctx)
	if err != nil {
		t.Fatalf("GetACL() unexpected error: %v", err)
	}
	var pathEntries []ACLEntry
	for _, e := range entries {
		if e.Path == "/vms/101" {
			pathEntries = append(pathEntries, e)
		}
	}
	if len(pathEntries) != 2 {
		t.Fatalf("expected 2 ACL entries for /vms/101, got %d", len(pathEntries))
	}

	if err := client.SetACL(ctx, ACLRequest{
		Path:      "/vms/101",
		Roles:     "TerraformManage,Auditor",
		Users:     "admin@pam",
		Groups:    "admins",
		Propagate: &prop,
		Delete:    true,
	}); err != nil {
		t.Fatalf("SetACL delete unexpected error: %v", err)
	}
}
