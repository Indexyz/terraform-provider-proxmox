// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSplitProxmoxList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
		{
			name:  "trims and sorts",
			input: " bob@pve,alice@pve , , carol@pve ",
			want:  []string{"alice@pve", "bob@pve", "carol@pve"},
		},
		{
			name:  "single value",
			input: "ops",
			want:  []string{"ops"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := splitProxmoxList(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("splitProxmoxList(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestPoolDataSourceHelpers(t *testing.T) {
	t.Parallel()

	pool := Pool{
		PoolID:  "platform",
		Comment: "Managed by Terraform",
		Members: []PoolMember{
			{ID: "storage/local-zfs", Node: "pve-2", Storage: "local-zfs", Type: "storage"},
			{ID: "qemu/102", Node: "pve-2", Type: "qemu", VMID: int64Pointer(102)},
			{ID: "qemu/101", Node: "pve-1", Type: "qemu", VMID: int64Pointer(101)},
		},
	}

	vmIDs, storageIDs, diags := poolDataSourceValues(context.Background(), pool)
	if diags.HasError() {
		t.Fatalf("poolDataSourceValues() unexpected diagnostics: %v", diags)
	}

	var gotVMIDs []int64
	if diags := vmIDs.ElementsAs(context.Background(), &gotVMIDs, false); diags.HasError() {
		t.Fatalf("unable to decode vm_ids set: %v", diags)
	}
	if want := []int64{101, 102}; !reflect.DeepEqual(gotVMIDs, want) {
		t.Fatalf("unexpected vm_ids: got %v want %v", gotVMIDs, want)
	}

	var gotStorageIDs []string
	if diags := storageIDs.ElementsAs(context.Background(), &gotStorageIDs, false); diags.HasError() {
		t.Fatalf("unable to decode storage_ids set: %v", diags)
	}
	if want := []string{"local-zfs"}; !reflect.DeepEqual(gotStorageIDs, want) {
		t.Fatalf("unexpected storage_ids: got %v want %v", gotStorageIDs, want)
	}

	gotMembers := poolDataSourceMembers(pool)
	wantMembers := []PoolDataSourceMemberRow{
		{
			ID:        types.StringValue("storage/local-zfs"),
			Node:      types.StringValue("pve-2"),
			StorageID: types.StringValue("local-zfs"),
			Type:      types.StringValue("storage"),
			VMID:      types.Int64Null(),
		},
		{
			ID:        types.StringValue("qemu/102"),
			Node:      types.StringValue("pve-2"),
			StorageID: types.StringNull(),
			Type:      types.StringValue("qemu"),
			VMID:      types.Int64Value(102),
		},
		{
			ID:        types.StringValue("qemu/101"),
			Node:      types.StringValue("pve-1"),
			StorageID: types.StringNull(),
			Type:      types.StringValue("qemu"),
			VMID:      types.Int64Value(101),
		},
	}

	if !reflect.DeepEqual(gotMembers, wantMembers) {
		t.Fatalf("unexpected pool members:\n got: %#v\nwant: %#v", gotMembers, wantMembers)
	}
}

func TestGroupResourceReadGroupState(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)

		switch r.URL.Path {
		case "/api2/json/access/groups/ops":
			writeEnvelope(t, w, map[string]any{
				"comment": "Operations",
				"members": []string{"zoe@pve", "alice@pve"},
			})
		case "/api2/json/access/groups/missing":
			http.NotFound(w, r)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
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

	resource := &GroupResource{client: client}

	state, diags := resource.readGroupState(context.Background(), "ops")
	if diags.HasError() {
		t.Fatalf("readGroupState() unexpected diagnostics: %v", diags)
	}

	if state.ID.ValueString() != "ops" || state.GroupID.ValueString() != "ops" {
		t.Fatalf("unexpected ids in state: %#v", state)
	}
	if state.Comment.ValueString() != "Operations" {
		t.Fatalf("unexpected comment: %q", state.Comment.ValueString())
	}

	var gotMembers []string
	if diags := state.Members.ElementsAs(context.Background(), &gotMembers, false); diags.HasError() {
		t.Fatalf("unable to decode members list: %v", diags)
	}
	if want := []string{"alice@pve", "zoe@pve"}; !reflect.DeepEqual(gotMembers, want) {
		t.Fatalf("unexpected members: got %v want %v", gotMembers, want)
	}

	missingState, diags := resource.readGroupState(context.Background(), "missing")
	if diags.HasError() {
		t.Fatalf("readGroupState(missing) unexpected diagnostics: %v", diags)
	}
	if !missingState.ID.IsNull() {
		t.Fatalf("expected null id for missing group, got %#v", missingState)
	}
}

func TestGroupResourceReadGroupStateReportsAPIErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTokenAuth(t, r)
		w.WriteHeader(http.StatusBadGateway)
		writeJSON(t, w, map[string]any{"errors": map[string]string{"group": "upstream unavailable"}})
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

	resource := &GroupResource{client: client}
	_, diags := resource.readGroupState(context.Background(), "ops")
	if !diags.HasError() {
		t.Fatal("expected diagnostics for upstream error")
	}

	if got := diags[0].Summary(); got != "Unable to Read Proxmox Group" {
		t.Fatalf("unexpected diagnostic summary: %q", got)
	}

	if detail := diags[0].Detail(); detail == "" || !strings.Contains(detail, "502") {
		t.Fatalf("expected diagnostic detail to mention upstream error, got %q", detail)
	}
}

func int64Pointer(v int64) *int64 {
	return &v
}
