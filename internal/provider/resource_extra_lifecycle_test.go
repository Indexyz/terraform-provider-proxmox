// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestHAResourceActionLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.EscapedPath() != "/api2/json/cluster/ha/resources/vm:100/migrate" {
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
			return
		}
		if !handler.form(w, r, url.Values{"node": {"pve2"}}) {
			return
		}
		handler.envelope(w, map[string]any{"sid": "vm:100", "requested-node": "pve2"})
	}))
	defer server.Close()

	res := &HAResourceActionResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := haResourceActionModel{SID: types.StringValue("vm:100"), Action: types.StringValue("migrate"), Node: types.StringValue("pve2"), Triggers: testStringMap(t, map[string]string{"gen": "1"})}
	createResp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created haResourceActionModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if created.ID.ValueString() != "vm:100/migrate" || created.Result.ValueString() == "" {
		t.Fatalf("unexpected state: %#v", created)
	}
	deleteResp := resource.DeleteResponse{State: createResp.State}
	res.Delete(context.Background(), resource.DeleteRequest{State: createResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", deleteResp.Diagnostics)
	}
	handler.assert(t)
}

func TestCephHealthMuteLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	listed := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/cluster/ceph/health-mute/OSD_NEAR_FULL":
			if !handler.form(w, r, url.Values{"value": {"1"}, "ttl": {"1h"}, "sticky": {"1"}}) {
				return
			}
			handler.envelope(w, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/ceph/health-mute":
			if listed {
				handler.envelope(w, []map[string]any{{"code": "OSD_NEAR_FULL", "summary": "near full"}})
			} else {
				handler.envelope(w, []map[string]any{})
			}
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &CephHealthMuteResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := cephHealthMuteModel{Code: types.StringValue("OSD_NEAR_FULL"), Value: types.BoolValue(true), TTL: types.StringValue("1h"), Sticky: types.BoolValue(true)}
	createResp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created cephHealthMuteModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() || created.ID.ValueString() != "OSD_NEAR_FULL" {
		t.Fatalf("decode state: %#v %v", created, diags)
	}
	listed = false
	readResp := resource.ReadResponse{State: createResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: createResp.State}, &readResp)
	if readResp.Diagnostics.HasError() || !readResp.State.Raw.IsNull() {
		t.Fatalf("expected resource removal, raw=%v diags=%v", readResp.State.Raw, readResp.Diagnostics)
	}
	handler.assert(t)
}

func TestStorageDownloadURLLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve/storage/local/download-url":
			if !handler.form(w, r, url.Values{"url": {"https://example.invalid/x.iso"}, "content": {"iso"}, "filename": {"x.iso"}}) {
				return
			}
			handler.envelope(w, "UPID:pve:download")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/tasks/UPID:pve:download/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api2/json/nodes/pve/storage/local/content/local:iso%2Fx.iso":
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &StorageDownloadURLResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	model := storageDownloadURLModel{Node: types.StringValue("pve"), Storage: types.StringValue("local"), URL: types.StringValue("https://example.invalid/x.iso"), Content: types.StringValue("iso"), Filename: types.StringValue("x.iso"), VerifyCertificates: types.BoolValue(true), Triggers: testStringMap(t, map[string]string{"gen": "1"})}
	createResp := testResourceCreateResponse(t, schema)
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, model)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created storageDownloadURLModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() || created.VolumeID.ValueString() != "local:iso/x.iso" {
		t.Fatalf("decode state: %#v %v", created, diags)
	}
	deleteResp := resource.DeleteResponse{State: createResp.State}
	res.Delete(context.Background(), resource.DeleteRequest{State: createResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", deleteResp.Diagnostics)
	}
	handler.assert(t)
}

func TestGuestExtraValidation(t *testing.T) {
	statusRes := &GuestStatusActionResource{}
	schema := testResourceSchema(t, statusRes)
	valid := guestStatusActionModel{Type: types.StringValue("qemu"), Action: types.StringValue("reset"), Node: types.StringValue("pve"), VMID: types.Int64Value(100), Triggers: testStringMap(t, map[string]string{"gen": "1"})}
	var ok resource.ValidateConfigResponse
	statusRes.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, schema, valid)}, &ok)
	if ok.Diagnostics.HasError() {
		t.Fatalf("valid diagnostics: %v", ok.Diagnostics)
	}
	for name, model := range map[string]guestStatusActionModel{
		"lxc reset": {Type: types.StringValue("lxc"), Action: types.StringValue("reset"), Node: types.StringValue("pve"), VMID: types.Int64Value(100), Triggers: valid.Triggers},
		"bad type":  {Type: types.StringValue("vm"), Action: types.StringValue("start"), Node: types.StringValue("pve"), VMID: types.Int64Value(100), Triggers: valid.Triggers},
	} {
		var resp resource.ValidateConfigResponse
		statusRes.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, schema, model)}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("%s: expected diagnostics", name)
		}
	}
	var unknown resource.ValidateConfigResponse
	valid.Type = types.StringUnknown()
	valid.Action = types.StringUnknown()
	statusRes.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, schema, valid)}, &unknown)
	if unknown.Diagnostics.HasError() {
		t.Fatalf("unknown diagnostics: %v", unknown.Diagnostics)
	}

	haRes := &HAResourceActionResource{}
	haSchema := testResourceSchema(t, haRes)
	haModel := haResourceActionModel{SID: types.StringValue("vm:100"), Action: types.StringValue("bogus"), Node: types.StringValue("pve"), Triggers: valid.Triggers}
	var haResp resource.ValidateConfigResponse
	haRes.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, haSchema, haModel)}, &haResp)
	if !haResp.Diagnostics.HasError() {
		t.Fatal("expected invalid HA action diagnostics")
	}

	upload := &StorageUploadResource{}
	uploadSchema := testResourceSchema(t, upload)
	var uploadResp resource.ValidateConfigResponse
	upload.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, uploadSchema, storageUploadModel{Node: types.StringValue("pve"), Storage: types.StringValue("local"), Source: types.StringValue("/tmp/x"), Content: types.StringValue("backup"), Triggers: valid.Triggers})}, &uploadResp)
	if !uploadResp.Diagnostics.HasError() {
		t.Fatal("expected invalid storage content diagnostics")
	}
}

func TestGuestExtraResourceLifecycles(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/nodes/pve/qemu/100/unlink":
			if !handler.form(w, r, url.Values{"idlist": {"scsi0"}}) {
				return
			}
			handler.envelope(w, nil)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/nodes/pve/qemu/100/cloudinit":
			handler.envelope(w, nil)
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve/qemu/100/template":
			if !handler.form(w, r, url.Values{"disk": {"scsi0"}}) {
				return
			}
			handler.envelope(w, nil)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/nodes/pve/lxc/101/resize":
			if !handler.form(w, r, url.Values{"disk": {"rootfs"}, "size": {"+1G"}}) {
				return
			}
			handler.envelope(w, nil)
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve/qemu/100/status/reboot":
			if !handler.form(w, r, url.Values{"timeout": {"30"}}) {
				return
			}
			handler.envelope(w, "UPID:pve:reboot")
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api2/json/nodes/pve/tasks/UPID:pve:reboot/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := testLifecycleClient(t, server)
	ctx := context.Background()

	unlink := &GuestUnlinkResource{client: client}
	unlinkSchema := testResourceSchema(t, unlink)
	unlinkResp := testResourceCreateResponse(t, unlinkSchema)
	unlink.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, unlinkSchema, guestUnlinkModel{Node: types.StringValue("pve"), VMID: types.Int64Value(100), IDList: stringListMust(t, []string{"scsi0"}), Force: types.BoolValue(false), Triggers: testStringMap(t, map[string]string{"g": "1"})})}, &unlinkResp)
	if unlinkResp.Diagnostics.HasError() {
		t.Fatalf("unlink diagnostics: %v", unlinkResp.Diagnostics)
	}

	cloudinit := &GuestCloudinitResource{client: client}
	ciSchema := testResourceSchema(t, cloudinit)
	ciResp := testResourceCreateResponse(t, ciSchema)
	cloudinit.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, ciSchema, guestCloudinitModel{Node: types.StringValue("pve"), VMID: types.Int64Value(100), Triggers: testStringMap(t, map[string]string{"g": "1"})})}, &ciResp)
	if ciResp.Diagnostics.HasError() {
		t.Fatalf("cloudinit diagnostics: %v", ciResp.Diagnostics)
	}

	tmpl := &GuestTemplateResource{client: client}
	tmplSchema := testResourceSchema(t, tmpl)
	tmplResp := testResourceCreateResponse(t, tmplSchema)
	tmpl.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, tmplSchema, guestTemplateModel{Type: types.StringValue("qemu"), Node: types.StringValue("pve"), VMID: types.Int64Value(100), Disks: stringListMust(t, []string{"scsi0"}), Triggers: testStringMap(t, map[string]string{"g": "1"})})}, &tmplResp)
	if tmplResp.Diagnostics.HasError() {
		t.Fatalf("template diagnostics: %v", tmplResp.Diagnostics)
	}

	resize := &LXCResizeResource{client: client}
	resizeSchema := testResourceSchema(t, resize)
	resizeResp := testResourceCreateResponse(t, resizeSchema)
	resize.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, resizeSchema, lxcResizeModel{Node: types.StringValue("pve"), VMID: types.Int64Value(101), Disk: types.StringValue("rootfs"), Size: types.StringValue("+1G"), Triggers: testStringMap(t, map[string]string{"g": "1"})})}, &resizeResp)
	if resizeResp.Diagnostics.HasError() {
		t.Fatalf("resize diagnostics: %v", resizeResp.Diagnostics)
	}

	action := &GuestStatusActionResource{client: client}
	actionSchema := testResourceSchema(t, action)
	actionResp := testResourceCreateResponse(t, actionSchema)
	action.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, actionSchema, guestStatusActionModel{Type: types.StringValue("qemu"), Node: types.StringValue("pve"), VMID: types.Int64Value(100), Action: types.StringValue("reboot"), Timeout: types.Int64Value(30), Triggers: testStringMap(t, map[string]string{"g": "1"})})}, &actionResp)
	if actionResp.Diagnostics.HasError() {
		t.Fatalf("action diagnostics: %v", actionResp.Diagnostics)
	}
	var actionState guestStatusActionModel
	if diags := actionResp.State.Get(ctx, &actionState); diags.HasError() || actionState.UPID.ValueString() != "UPID:pve:reboot" {
		t.Fatalf("action state: %#v %v", actionState, diags)
	}
	handler.assert(t)
}

func TestCephServiceLifecycles(t *testing.T) {
	handler := &lifecycleHandler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve/ceph/start", "/api2/json/nodes/pve/ceph/stop":
			if !handler.form(w, r, url.Values{"service": {"osd.0"}}) {
				return
			}
			handler.envelope(w, "UPID:pve:svc")
		case "/api2/json/nodes/pve/tasks/UPID:pve:svc/status":
			handler.envelope(w, map[string]any{"status": "stopped", "exitstatus": "OK"})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := testLifecycleClient(t, server)

	for _, test := range []struct {
		res   resource.Resource
		model cephServiceModel
	}{
		{&CephServiceStartResource{client: client}, cephServiceModel{Node: types.StringValue("pve"), Service: types.StringValue("osd.0"), Triggers: testStringMap(t, map[string]string{"g": "1"})}},
		{&CephServiceStopResource{client: client}, cephServiceModel{Node: types.StringValue("pve"), Service: types.StringValue("osd.0"), Triggers: testStringMap(t, map[string]string{"g": "1"})}},
	} {
		if test.model.Node.IsNull() {
			test.model = cephServiceModel{Node: types.StringValue("pve"), Service: types.StringValue("osd.0"), Triggers: testStringMap(t, map[string]string{"g": "1"})}
		}
		schema := testResourceSchema(t, test.res)
		resp := testResourceCreateResponse(t, schema)
		switch typed := test.res.(type) {
		case *CephServiceStartResource:
			typed.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, test.model)}, &resp)
		case *CephServiceStopResource:
			typed.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, test.model)}, &resp)
		}
		if resp.Diagnostics.HasError() {
			t.Fatalf("service diagnostics: %v", resp.Diagnostics)
		}
	}
	handler.assert(t)
}

func stringListMust(t *testing.T, values []string) types.List {
	t.Helper()
	list, diags := types.ListValueFrom(context.Background(), types.StringType, values)
	if diags.HasError() {
		t.Fatalf("list diagnostics: %v", diags)
	}
	return list
}
