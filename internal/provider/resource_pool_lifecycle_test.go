// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func testInt64Set(t *testing.T, values ...int64) types.Set {
	t.Helper()
	value, diags := types.SetValueFrom(context.Background(), types.Int64Type, values)
	if diags.HasError() {
		t.Fatalf("create int set: %v", diags)
	}
	return value
}

func testStringSet(t *testing.T, values ...string) types.Set {
	t.Helper()
	value, diags := types.SetValueFrom(context.Background(), types.StringType, values)
	if diags.HasError() {
		t.Fatalf("create string set: %v", diags)
	}
	return value
}

func TestPoolResourceLifecycleOrdersMembershipChanges(t *testing.T) {
	comment := "created"
	vmIDs := []int64{}
	vmTypes := map[int64]string{100: "qemu", 101: "qemu", 200: "lxc"}
	storageIDs := []string{}
	exists := true
	handler := &lifecycleHandler{}
	var operations []string
	var updateForms []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/pools":
			if !handler.form(w, r, url.Values{"poolid": {"unit"}, "comment": {"created"}}) {
				return
			}
			operations = append(operations, "create")
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/pools":
			if r.URL.Query().Get("poolid") != "unit" {
				handler.fail(w, "unexpected pool query: %s", r.URL.RawQuery)
				return
			}
			if !exists {
				handler.envelope(w, []any{})
				return
			}
			members := make([]map[string]any, 0, len(vmIDs)+len(storageIDs))
			for _, vmID := range vmIDs {
				kind := vmTypes[vmID]
				members = append(members, map[string]any{"id": kind + "/" + strconv.FormatInt(vmID, 10), "node": "pve", "type": kind, "vmid": vmID})
			}
			for _, storageID := range storageIDs {
				members = append(members, map[string]any{"id": "storage/" + storageID, "storage": storageID, "type": "storage"})
			}
			operations = append(operations, "get")
			handler.envelope(w, []map[string]any{{"poolid": "unit", "comment": comment, "members": members}})
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/pools":
			if err := r.ParseForm(); err != nil {
				handler.fail(w, "parse pool form: %v", err)
				return
			}
			if r.Form.Get("poolid") != "unit" {
				handler.fail(w, "unexpected pool form: %v", r.Form)
				return
			}
			updateForms = append(updateForms, r.Form.Encode())
			switch {
			case r.Form.Get("comment") != "":
				comment = r.Form.Get("comment")
				operations = append(operations, "comment")
			case r.Form.Get("delete") == "1":
				vmIDs = removeTestInts(vmIDs, r.Form.Get("vms"))
				storageIDs = removeTestStrings(storageIDs, r.Form.Get("storage"))
				operations = append(operations, "remove")
			default:
				if r.Form.Get("allow-move") != "1" {
					handler.fail(w, "pool additions must enable allow-move: %v", r.Form)
					return
				}
				vmIDs = appendTestInts(vmIDs, r.Form.Get("vms"))
				storageIDs = append(storageIDs, splitProxmoxList(r.Form.Get("storage"))...)
				operations = append(operations, "add")
			}
			handler.envelope(w, nil)
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/pools/unit":
			if !exists {
				writeAPIError(t, w, http.StatusNotFound, "missing pool")
				return
			}
			if len(vmIDs) != 0 || len(storageIDs) != 0 {
				handler.fail(w, "pool deleted before it was emptied: vms=%v storages=%v", vmIDs, storageIDs)
				return
			}
			exists = false
			operations = append(operations, "delete")
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &PoolResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	memberType := types.ObjectType{AttrTypes: poolResourceMemberAttrTypes()}
	initial := PoolResourceModel{PoolID: types.StringValue("unit"), Comment: types.StringValue("created"), AllowMove: types.BoolValue(true), VMIDs: testInt64Set(t, 100, 200), StorageIDs: testStringSet(t, "local"), Members: types.ListNull(memberType)}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("pool create diagnostics: %v", createResp.Diagnostics)
	}
	assertStateString(t, createResp.State, path.Root("members").AtListIndex(2).AtName("storage_id"), "local")

	updated := initial
	updated.Comment = types.StringValue("updated")
	updated.VMIDs = testInt64Set(t, 101, 200)
	updated.StorageIDs = testStringSet(t, "backup")
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, updated), State: createResp.State}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("pool update diagnostics: %v", updateResp.Diagnostics)
	}
	assertStateString(t, updateResp.State, path.Root("comment"), "updated")
	var updatedState PoolResourceModel
	if diags := updateResp.State.Get(context.Background(), &updatedState); diags.HasError() {
		t.Fatalf("decode updated pool state: %v", diags)
	}
	var gotVMIDs []int64
	if diags := updatedState.VMIDs.ElementsAs(context.Background(), &gotVMIDs, false); diags.HasError() {
		t.Fatalf("decode updated vm_ids: %v", diags)
	}
	if got, want := gotVMIDs, []int64{101, 200}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected updated vm_ids: got %v want %v", got, want)
	}
	var gotStorageIDs []string
	if diags := updatedState.StorageIDs.ElementsAs(context.Background(), &gotStorageIDs, false); diags.HasError() {
		t.Fatalf("decode updated storage_ids: %v", diags)
	}
	if got, want := gotStorageIDs, []string{"backup"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected updated storage_ids: got %v want %v", got, want)
	}
	var gotMembers []PoolResourceMemberItem
	if diags := updatedState.Members.ElementsAs(context.Background(), &gotMembers, false); diags.HasError() {
		t.Fatalf("decode updated computed members: %v", diags)
	}
	if len(gotMembers) != 3 || gotMembers[0].Type.ValueString() != "lxc" || gotMembers[0].VMID.ValueInt64() != 200 || gotMembers[1].Type.ValueString() != "qemu" || gotMembers[1].VMID.ValueInt64() != 101 || gotMembers[2].Type.ValueString() != "storage" || gotMembers[2].StorageID.ValueString() != "backup" {
		t.Fatalf("unexpected typed computed members: %#v", gotMembers)
	}

	readResp := resource.ReadResponse{State: updateResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: updateResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("pool read diagnostics: %v", readResp.Diagnostics)
	}
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("pool delete diagnostics: %v", deleteResp.Diagnostics)
	}
	missingResp := resource.ReadResponse{State: readResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: readResp.State}, &missingResp)
	if missingResp.Diagnostics.HasError() || !missingResp.State.Raw.IsNull() {
		t.Fatalf("missing pool was not removed from state: diagnostics=%v raw=%v", missingResp.Diagnostics, missingResp.State.Raw)
	}
	var idempotentDeleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &idempotentDeleteResp)
	if idempotentDeleteResp.Diagnostics.HasError() {
		t.Fatalf("idempotent pool delete diagnostics: %v", idempotentDeleteResp.Diagnostics)
	}
	handler.assert(t)
	want := []string{"create", "add", "get", "get", "comment", "remove", "add", "get", "get", "get", "remove", "delete"}
	if !reflect.DeepEqual(operations, want) {
		t.Fatalf("unexpected pool operation order: got %v want %v", operations, want)
	}
	wantForms := []string{
		"allow-move=1&poolid=unit&storage=local&vms=100%2C200",
		"comment=updated&poolid=unit",
		"delete=1&poolid=unit&storage=local&vms=100",
		"allow-move=1&poolid=unit&storage=backup&vms=101",
		"delete=1&poolid=unit&storage=backup&vms=101%2C200",
	}
	if !reflect.DeepEqual(updateForms, wantForms) {
		t.Fatalf("unexpected pool update forms: got %v want %v", updateForms, wantForms)
	}

	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, PoolResourceModel{ID: types.StringNull(), PoolID: types.StringNull(), Comment: types.StringNull(), AllowMove: types.BoolNull(), VMIDs: types.SetNull(types.Int64Type), StorageIDs: types.SetNull(types.StringType), Members: types.ListNull(memberType)})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "unit"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("pool import diagnostics: %v", importResp.Diagnostics)
	}
	assertStateString(t, importResp.State, path.Root("pool_id"), "unit")
}

func TestPoolResourceReadPreservesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeAPIError(t, w, http.StatusInternalServerError, "cluster filesystem unavailable")
	}))
	defer server.Close()
	res := &PoolResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	memberType := types.ObjectType{AttrTypes: poolResourceMemberAttrTypes()}
	state := testResourceState(t, schema, PoolResourceModel{ID: types.StringValue("unit"), PoolID: types.StringValue("unit"), AllowMove: types.BoolValue(false), VMIDs: testInt64Set(t), StorageIDs: testStringSet(t), Members: types.ListNull(memberType)})
	resp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if !resp.Diagnostics.HasError() || !containsDiagnostic(resp.Diagnostics, "status 500") || !containsDiagnostic(resp.Diagnostics, "cluster filesystem unavailable") {
		t.Fatalf("expected preserved pool API error, got %v", resp.Diagnostics)
	}
	if !resp.State.Raw.Equal(state.Raw) {
		t.Fatalf("pool API error unexpectedly mutated state: %v", resp.State.Raw)
	}
}

func appendTestInts(current []int64, values string) []int64 {
	for _, value := range splitProxmoxList(values) {
		parsed, _ := strconv.ParseInt(value, 10, 64)
		current = append(current, parsed)
	}
	return current
}

func removeTestInts(current []int64, values string) []int64 {
	removed := map[int64]bool{}
	for _, value := range splitProxmoxList(values) {
		parsed, _ := strconv.ParseInt(value, 10, 64)
		removed[parsed] = true
	}
	result := current[:0]
	for _, value := range current {
		if !removed[value] {
			result = append(result, value)
		}
	}
	return result
}

func removeTestStrings(current []string, values string) []string {
	removed := map[string]bool{}
	for _, value := range strings.Split(values, ",") {
		removed[value] = true
	}
	result := current[:0]
	for _, value := range current {
		if !removed[value] {
			result = append(result, value)
		}
	}
	return result
}
