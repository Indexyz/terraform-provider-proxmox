// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestQemuVMDeviceParsingRoundTrip(t *testing.T) {
	hostPci, ok := parseQemuVMHostPci("0000:00:1f.2,mdev=i915-GVTg_V5_4,pcie=1,rombar=0,driver=keep")
	if !ok || hostPci.Host.ValueString() != "0000:00:1f.2" || hostPci.Mdev.ValueString() != "i915-GVTg_V5_4" || !hostPci.PCIE.ValueBool() || hostPci.ROMBar.ValueBool() || hostPci.Driver.ValueString() != "keep" {
		t.Fatalf("unexpected hostpci parse: %#v (%v)", hostPci, ok)
	}
	if encoded := encodeQemuVMHostPci(hostPci); encoded != "0000:00:1f.2,mdev=i915-GVTg_V5_4,pcie=1,rombar=0,driver=keep" {
		t.Fatalf("hostpci round trip = %q", encoded)
	}
	mapped, ok := parseQemuVMHostPci("mapping=gpu1,pcie=1")
	if !ok || mapped.Mapping.ValueString() != "gpu1" {
		t.Fatalf("unexpected mapping parse: %#v (%v)", mapped, ok)
	}
	if encoded := encodeQemuVMHostPci(mapped); encoded != "mapping=gpu1,pcie=1" {
		t.Fatalf("mapping round trip = %q", encoded)
	}

	usb, ok := parseQemuVMUSB("1-2.3,usb3=1")
	if !ok || usb.Host.ValueString() != "1-2.3" || !usb.USB3.ValueBool() {
		t.Fatalf("unexpected usb parse: %#v (%v)", usb, ok)
	}
	if encoded := encodeQemuVMUSB(usb); encoded != "1-2.3,usb3=1" {
		t.Fatalf("usb round trip = %q", encoded)
	}

	rng, ok := parseQemuVMRNG("/dev/hwrng,max_bytes=0,period=500")
	if !ok || rng.Source.ValueString() != "/dev/hwrng" || rng.MaxBytes.ValueInt64() != 0 || rng.Period.ValueInt64() != 500 {
		t.Fatalf("unexpected rng parse: %#v (%v)", rng, ok)
	}
	if encoded := encodeQemuVMRNG(rng); encoded != "/dev/hwrng,max_bytes=0,period=500" {
		t.Fatalf("rng round trip = %q", encoded)
	}

	virtiofs, ok := parseQemuVMVirtiofs("share1,cache=always,direct-io=1,expose-xattr=1,expose-acl=0")
	if !ok || virtiofs.DirID.ValueString() != "share1" || virtiofs.Cache.ValueString() != "always" || !virtiofs.DirectIO.ValueBool() || !virtiofs.ExposeXAttr.ValueBool() || virtiofs.ExposeACL.ValueBool() {
		t.Fatalf("unexpected virtiofs parse: %#v (%v)", virtiofs, ok)
	}
	if encoded := encodeQemuVMVirtiofs(virtiofs); encoded != "share1,cache=always,direct-io=1,expose-xattr=1,expose-acl=0" {
		t.Fatalf("virtiofs round trip = %q", encoded)
	}

	if _, ok := parseQemuVMHostPci("0000:00:1f.2,romfile=foo.bin"); ok {
		t.Fatal("unsupported hostpci key must fall back")
	}
	if _, ok := parseQemuVMUSB("spice,unknown=1"); ok {
		t.Fatal("unsupported usb key must fall back")
	}
}

func TestQemuVMDeviceValidation(t *testing.T) {
	hostPci := func() qemuVMHostPciModel {
		return qemuVMHostPciModel{Host: types.StringValue("0000:00:1f.2")}
	}
	usb := func() qemuVMUSBModel {
		return qemuVMUSBModel{Host: types.StringValue("1-2")}
	}
	if diags := validateQemuVMHostPciItem("hostpci0", hostPci()); diags.HasError() {
		t.Fatalf("valid hostpci diagnostics: %v", diags)
	}
	if diags := validateQemuVMUSBItem("usb0", usb()); diags.HasError() {
		t.Fatalf("valid usb diagnostics: %v", diags)
	}
	if diags := validateQemuVMRNGItem("rng0", qemuVMRNGModel{Source: types.StringValue("/dev/urandom")}); diags.HasError() {
		t.Fatalf("valid rng diagnostics: %v", diags)
	}
	if diags := validateQemuVMVirtiofsItem("virtiofs0", qemuVMVirtiofsModel{DirID: types.StringValue("share1")}); diags.HasError() {
		t.Fatalf("valid virtiofs diagnostics: %v", diags)
	}

	invalid := map[string]func() (string, string, diagFn){
		"hostpci without host": func() (string, string, diagFn) {
			return "hostpci0", "hostpci", func() bool { return validateQemuVMHostPciItem("hostpci0", qemuVMHostPciModel{}).HasError() }
		},
		"hostpci host and mapping": func() (string, string, diagFn) {
			item := hostPci()
			item.Mapping = types.StringValue("gpu1")
			return "hostpci0", "hostpci", func() bool { return validateQemuVMHostPciItem("hostpci0", item).HasError() }
		},
		"hostpci bad host": func() (string, string, diagFn) {
			return "hostpci0", "hostpci", func() bool {
				return validateQemuVMHostPciItem("hostpci0", qemuVMHostPciModel{Host: types.StringValue("xyz")}).HasError()
			}
		},
		"hostpci bad driver": func() (string, string, diagFn) {
			item := hostPci()
			item.Driver = types.StringValue("nvidia")
			return "hostpci0", "hostpci", func() bool { return validateQemuVMHostPciItem("hostpci0", item).HasError() }
		},
		"usb bad host": func() (string, string, diagFn) {
			return "usb0", "usb", func() bool {
				return validateQemuVMUSBItem("usb0", qemuVMUSBModel{Host: types.StringValue("keyboard")}).HasError()
			}
		},
		"rng without source": func() (string, string, diagFn) {
			return "rng0", "rng", func() bool { return validateQemuVMRNGItem("rng0", qemuVMRNGModel{}).HasError() }
		},
		"rng bad source": func() (string, string, diagFn) {
			return "rng0", "rng", func() bool {
				return validateQemuVMRNGItem("rng0", qemuVMRNGModel{Source: types.StringValue("/dev/zero")}).HasError()
			}
		},
		"virtiofs without dirid": func() (string, string, diagFn) {
			return "virtiofs0", "virtiofs", func() bool { return validateQemuVMVirtiofsItem("virtiofs0", qemuVMVirtiofsModel{}).HasError() }
		},
		"virtiofs bad cache": func() (string, string, diagFn) {
			return "virtiofs0", "virtiofs", func() bool {
				return validateQemuVMVirtiofsItem("virtiofs0", qemuVMVirtiofsModel{DirID: types.StringValue("share1"), Cache: types.StringValue("sometimes")}).HasError()
			}
		},
	}
	for name, build := range invalid {
		_, _, hasError := build()
		if !hasError() {
			t.Fatalf("%s: expected diagnostics", name)
		}
	}
}

type diagFn func() bool

func TestQemuVMDeviceSlotRanges(t *testing.T) {
	valid := map[string]func(string) bool{
		"hostpci":  isQemuVMHostPciKey,
		"usb":      isQemuVMUSBKey,
		"rng":      isQemuVMRNGKey,
		"virtiofs": isQemuVMVirtiofsKey,
	}
	cases := []struct {
		slot  string
		check func(string) bool
		ok    bool
	}{
		{"hostpci0", valid["hostpci"], true},
		{"hostpci15", valid["hostpci"], true},
		{"hostpci16", valid["hostpci"], false},
		{"usb0", valid["usb"], true},
		{"usb13", valid["usb"], true},
		{"usb14", valid["usb"], false},
		{"rng0", valid["rng"], true},
		{"rng1", valid["rng"], false},
		{"virtiofs0", valid["virtiofs"], true},
		{"virtiofs9", valid["virtiofs"], true},
		{"virtiofs10", valid["virtiofs"], false},
	}
	for _, test := range cases {
		if got := test.check(test.slot); got != test.ok {
			t.Fatalf("slot %q: got %v want %v", test.slot, got, test.ok)
		}
	}
}

func TestQemuVMDeviceStateKeepsUnparsableSlotsInRaw(t *testing.T) {
	state, unsupported, diags := qemuVMHostPciStateValue(context.Background(), map[string]string{
		"hostpci0": "0000:00:1f.2",
		"hostpci1": "0000:00:1f.3,romfile=foo.bin",
	})
	if diags.HasError() {
		t.Fatalf("state diagnostics: %v", diags)
	}
	if len(state.Elements()) != 1 || unsupported["hostpci1"] == "" {
		t.Fatalf("unexpected state: %#v / %#v", state, unsupported)
	}
	if _, ok := state.Elements()["hostpci1"]; ok {
		t.Fatal("unparsable slot must not be typed")
	}
}

func TestQemuVMDeviceRawConflict(t *testing.T) {
	model := minimalQemuVMModel("pve", 100)
	model.HostPCI = types.MapValueMust(types.ObjectType{AttrTypes: qemuVMHostPciAttrTypes()}, map[string]attr.Value{
		"hostpci0": types.ObjectValueMust(qemuVMHostPciAttrTypes(), map[string]attr.Value{
			"host":    types.StringValue("0000:00:1f.2"),
			"mapping": types.StringNull(),
			"mdev":    types.StringNull(),
			"pcie":    types.BoolNull(),
			"x_vga":   types.BoolNull(),
			"rombar":  types.BoolNull(),
			"driver":  types.StringNull(),
		}),
	})
	model.Raw = types.ObjectValueMust(qemuVMRawAttrTypes(), map[string]attr.Value{
		"extra_config": types.MapValueMust(types.StringType, map[string]attr.Value{
			"hostpci0": types.StringValue("0000:00:1f.2"),
		}),
	})
	diags := validateQemuVMRawConflicts(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("typed slot overlapping raw must conflict")
	}
}

func TestQemuVMDeviceUpdateDeletesRemovedSlots(t *testing.T) {
	ctx := context.Background()
	handler := &lifecycleHandler{}
	var updateForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/qemu/200/config":
			handler.envelope(w, map[string]any{
				"name": "device-vm", "memory": 512, "cores": 1,
				"usb0": "1-2", "hostpci0": "0000:00:1f.2",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/qemu/200/status/current":
			handler.envelope(w, map[string]any{"status": "stopped", "uptime": 0})
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/nodes/pve/qemu/200/config":
			if err := r.ParseForm(); err != nil {
				handler.fail(w, "parse form: %v", err)
				return
			}
			updateForm = r.Form
			handler.envelope(w, nil)
		case r.Method == http.MethodGet:
			handler.envelope(w, map[string]any{"name": "device-vm", "memory": 512, "cores": 1})
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	res := &QemuVMResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	prior := minimalQemuVMModel("pve", 200)
	prior.Name = types.StringValue("device-vm")
	prior.Memory = types.Int64Value(512)
	prior.Cores = types.Int64Value(1)
	prior.USB = types.MapValueMust(types.ObjectType{AttrTypes: qemuVMUSBAttrTypes()}, map[string]attr.Value{
		"usb0": types.ObjectValueMust(qemuVMUSBAttrTypes(), map[string]attr.Value{
			"host":    types.StringValue("1-2"),
			"mapping": types.StringNull(),
			"usb3":    types.BoolNull(),
		}),
	})
	stateResp := resource.ReadResponse{State: testResourceState(t, schema, prior)}
	res.Read(ctx, resource.ReadRequest{State: stateResp.State}, &stateResp)
	if stateResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", stateResp.Diagnostics)
	}
	var refreshed qemuVMModel
	if diags := stateResp.State.Get(ctx, &refreshed); diags.HasError() {
		t.Fatalf("decode refreshed state: %v", diags)
	}

	// Drop both compound devices, keep the scalars.
	plan := refreshed
	plan.USB = types.MapNull(types.ObjectType{AttrTypes: qemuVMUSBAttrTypes()})
	plan.HostPCI = types.MapNull(types.ObjectType{AttrTypes: qemuVMHostPciAttrTypes()})
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}, Private: stateResp.Private}
	res.Update(ctx, resource.UpdateRequest{
		Config:  testResourceConfig(t, schema, plan),
		Plan:    testResourcePlan(t, schema, plan),
		State:   stateResp.State,
		Private: stateResp.Private,
	}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", updateResp.Diagnostics)
	}
	if updateForm == nil {
		t.Fatal("update did not reach Proxmox")
	}
	if got := updateForm.Get("delete"); got != "hostpci0,usb0" {
		t.Fatalf("removed compound slots were not deleted: %q", got)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}
