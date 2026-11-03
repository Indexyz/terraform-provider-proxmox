// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestNodeHardwarePCIDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/hardware/pci": []map[string]any{
			{"id": "0000:00:02.0", "vendor": "0x1af4", "device": "0x1000", "class": "0x020000", "iommugroup": -1, "mdev": 1},
			{"id": "0000:00:01.0", "vendor": "0x1234", "device": "0x1111", "vendor_name": "QEMU"},
		},
	}
	readResp := runSDNDataSource(t, NewNodeHardwarePCIDataSource(), responses, map[string]any{"node": "pve-1"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state nodeHardwarePCIModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() || len(state.Devices) != 2 || state.Devices[0].ID.ValueString() != "0000:00:01.0" || !state.Devices[1].MDev.ValueBool() {
		t.Fatalf("unexpected projection: %#v (%v)", state.Devices, diags)
	}
}

func TestNodeHardwareUSBDataSourceSortsByBus(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/hardware/usb": []map[string]any{
			{"busnum": 2, "devnum": 1, "port": 1, "vendid": "2222", "prodid": "0002"},
			{"busnum": 1, "devnum": 2, "port": 2, "vendid": "1111", "prodid": "0001", "product": "hub"},
		},
	}
	readResp := runSDNDataSource(t, NewNodeHardwareUSBDataSource(), responses, map[string]any{"node": "pve-1"})
	var state nodeHardwareUSBModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() || len(state.Devices) != 2 || state.Devices[0].VendorID.ValueString() != "1111" {
		t.Fatalf("unexpected projection: %#v (%v)", state.Devices, diags)
	}
}

func TestNodeAPTVersionsDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/apt/versions": []map[string]any{
			{"Package": "zlib", "Version": "2"},
			{"Package": "base-files", "Version": "13", "OldVersion": "12", "Origin": "Debian"},
		},
	}
	readResp := runSDNDataSource(t, NewNodeAPTVersionsDataSource(), responses, map[string]any{"node": "pve-1"})
	var state nodeAPTVersionsModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() || len(state.Packages) != 2 || state.Packages[0].Package.ValueString() != "base-files" || state.Packages[0].OldVersion.ValueString() != "12" {
		t.Fatalf("unexpected projection: %#v (%v)", state.Packages, diags)
	}
}

func TestNodeServicesDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/services": []map[string]any{
			{"service": "zfs", "state": "inactive", "active-state": "inactive"},
			{"service": "pveproxy", "state": "running", "active-state": "active"},
		},
	}
	readResp := runSDNDataSource(t, NewNodeServicesDataSource(), responses, map[string]any{"node": "pve-1"})
	var state nodeServicesModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() || len(state.Services) != 2 || state.Services[0].Service.ValueString() != "pveproxy" {
		t.Fatalf("unexpected projection: %#v (%v)", state.Services, diags)
	}
}

func TestUserTFAUnlockLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.Path != "/api2/json/access/users/root@pam/unlock-tfa" || r.Method != http.MethodPut {
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.Path)
			return
		}
		handler.envelope(w, 1)
	}))
	defer server.Close()

	ctx := context.Background()
	res := &UserTFAUnlockResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := userTFAUnlockModel{ID: types.StringNull(), UserID: types.StringValue("root@pam"), Triggers: testStringMap(t, map[string]string{"reason": "locked"}), Unlocked: types.BoolNull()}
	createResp := resource.CreateResponse{State: testResourceState(t, schema, config)}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("unlock diagnostics: %v", createResp.Diagnostics)
	}
	var state userTFAUnlockModel
	if diags := createResp.State.Get(ctx, &state); diags.HasError() || !state.Unlocked.ValueBool() || state.ID.ValueString() != "root@pam" {
		t.Fatalf("unexpected state: %#v (%v)", state, diags)
	}
	deleteResp := resource.DeleteResponse{State: createResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: createResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete must be a no-op: %v", deleteResp.Diagnostics)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

func TestUserTFAUnlockValidation(t *testing.T) {
	valid := userTFAUnlockModel{UserID: types.StringValue("root@pam"), Triggers: testStringMap(t, map[string]string{"x": "1"})}
	var resp resource.ValidateConfigResponse
	(&UserTFAUnlockResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &UserTFAUnlockResource{}), valid)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("valid diagnostics: %v", resp.Diagnostics)
	}
	invalid := userTFAUnlockModel{UserID: types.StringValue("root"), Triggers: testStringMap(t, map[string]string{})}
	resp = resource.ValidateConfigResponse{}
	(&UserTFAUnlockResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &UserTFAUnlockResource{}), invalid)}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("invalid config accepted: %v", resp.Diagnostics)
	}
	_ = fmt.Sprint
}
