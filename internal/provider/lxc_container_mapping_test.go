// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestLXCContainerStateFromAPI(t *testing.T) {
	ctx := context.Background()
	prior := lxcContainerModel{
		OSTemplate: types.StringValue("local:vztmpl/debian-12.tar.zst"),
		RootFS:     types.StringValue("local-lvm:8"),
	}

	state, diags := lxcContainerStateFromAPI(ctx, "pve-1", 101, LXCContainerConfig{
		Hostname:     "ct-101",
		Description:  "Managed by Terraform",
		Tags:         "prod,terraform",
		Arch:         "amd64",
		Startup:      "order=2",
		Features:     "nesting=1",
		OSType:       "debian",
		RootFS:       "local-lvm:vm-101-disk-0,size=8G",
		Nameserver:   "1.1.1.1",
		Searchdomain: "example.internal",
		Timezone:     "host",
		Cores:        proxmoxOptionalInt64{value: intPtr64(2)},
		Memory:       proxmoxOptionalInt64{value: intPtr64(512)},
		Swap:         proxmoxOptionalInt64{value: intPtr64(128)},
		Network: map[string]string{
			"net0": "name=eth0,bridge=vmbr0,ip=dhcp",
		},
		MountPoint: map[string]string{
			"mp0": "local-lvm:1,mp=/data",
		},
		ExtraConfig: map[string]string{
			"lxc.apparmor.profile": "unconfined",
		},
	}, LXCContainerStatus{Status: "running", Uptime: proxmoxOptionalInt64{value: intPtr64(300)}}, &prior)
	assertNoDiags(t, diags)

	if state.ID.ValueString() != "pve-1/101" || state.Node.ValueString() != "pve-1" || state.VMID.ValueInt64() != 101 {
		t.Fatalf("unexpected identity fields: %#v", state)
	}
	if state.OSTemplate.ValueString() != "local:vztmpl/debian-12.tar.zst" {
		t.Fatalf("expected prior ostemplate to be preserved, got %q", state.OSTemplate.ValueString())
	}
	if state.RootFS.ValueString() != "local-lvm:8" {
		t.Fatalf("expected prior rootfs to be preserved, got %q", state.RootFS.ValueString())
	}
	if state.Hostname.ValueString() != "ct-101" || state.Description.ValueString() != "Managed by Terraform" || state.Tags.ValueString() != "prod,terraform" {
		t.Fatalf("unexpected basic fields: %#v", state)
	}
	if state.Arch.ValueString() != "amd64" || state.Startup.ValueString() != "order=2" || state.Features.ValueString() != "nesting=1" || state.OSType.ValueString() != "debian" {
		t.Fatalf("unexpected string fields: %#v", state)
	}
	if state.Cores.ValueInt64() != 2 || state.Memory.ValueInt64() != 512 || state.Swap.ValueInt64() != 128 {
		t.Fatalf("unexpected integer fields: %#v", state)
	}
	if state.OnBoot.IsNull() || state.OnBoot.ValueBool() || state.Protection.IsNull() || state.Protection.ValueBool() || state.Unprivileged.IsNull() || state.Unprivileged.ValueBool() {
		t.Fatalf("expected omitted bool defaults to false, got onboot=%#v protection=%#v unprivileged=%#v", state.OnBoot, state.Protection, state.Unprivileged)
	}
	if state.Nameserver.ValueString() != "1.1.1.1" || state.Searchdomain.ValueString() != "example.internal" || state.Timezone.ValueString() != "host" {
		t.Fatalf("unexpected dns/time fields: %#v", state)
	}
	network := decodeLXCContainerNetworkMap(t, state.Network)
	if got := network["net0"]; got.Name.ValueString() != "eth0" || got.Bridge.ValueString() != "vmbr0" || got.IP.ValueString() != "dhcp" {
		t.Fatalf("unexpected network mapping: %#v", network)
	}
	mountPoint := decodeLXCContainerMountPointMap(t, state.MountPoint)
	if got := mountPoint["mp0"]; got.Volume.ValueString() != "local-lvm:1" || got.MountPoint.ValueString() != "/data" {
		t.Fatalf("unexpected mount_point mapping: %#v", mountPoint)
	}
	raw := decodeLXCContainerRaw(t, state.Raw)
	assertStringMapValue(t, raw.ExtraConfig, map[string]string{"lxc.apparmor.profile": "unconfined"})
	if state.Status.ValueString() != "running" || state.Uptime.ValueInt64() != 300 {
		t.Fatalf("unexpected runtime state: %#v", state)
	}
}

func TestLXCContainerStateFromAPIUsesAPIRootFSWithoutPrior(t *testing.T) {
	state, diags := lxcContainerStateFromAPI(context.Background(), "pve-1", 101, LXCContainerConfig{RootFS: "local-lvm:vm-101-disk-0,size=8G"}, LXCContainerStatus{}, nil)
	assertNoDiags(t, diags)
	if state.RootFS.ValueString() != "local-lvm:vm-101-disk-0,size=8G" {
		t.Fatalf("expected API rootfs without prior state, got %q", state.RootFS.ValueString())
	}
}

func TestLXCContainerRequestFromModel(t *testing.T) {
	ctx := context.Background()
	model := lxcContainerModel{
		Node:         types.StringValue("pve-1"),
		VMID:         types.Int64Value(101),
		OSTemplate:   types.StringValue("local:vztmpl/debian-12.tar.zst"),
		Hostname:     types.StringValue("ct-101"),
		Description:  types.StringValue("Managed by Terraform"),
		Tags:         types.StringValue("prod,terraform"),
		Arch:         types.StringValue("amd64"),
		Cores:        types.Int64Value(2),
		Memory:       types.Int64Value(512),
		Swap:         types.Int64Value(128),
		OnBoot:       types.BoolValue(true),
		Protection:   types.BoolValue(true),
		Startup:      types.StringValue("order=2"),
		Unprivileged: types.BoolValue(true),
		Features:     types.StringValue("nesting=1"),
		OSType:       types.StringValue("debian"),
		RootFS:       types.StringValue("local-lvm:8"),
		Nameserver:   types.StringValue("1.1.1.1"),
		Searchdomain: types.StringValue("example.internal"),
		Timezone:     types.StringValue("host"),
		Network:      mustLXCContainerNetworkMapValue(t, map[string]lxcContainerNetworkModel{"net0": {Name: types.StringValue("eth0"), Bridge: types.StringValue("vmbr0"), IP: types.StringValue("dhcp")}}),
		MountPoint:   mustLXCContainerMountPointMapValue(t, map[string]lxcContainerMountPointModel{"mp0": {Volume: types.StringValue("local-lvm:1"), MountPoint: types.StringValue("/data")}}),
		Raw:          mustLXCContainerRawValue(t, lxcContainerRawModel{ExtraConfig: mustStringMapValue(t, map[string]string{"lxc.apparmor.profile": "unconfined"})}),
	}

	createReq, diags := lxcContainerCreateRequestFromModel(ctx, model)
	assertNoDiags(t, diags)
	if createReq.VMID != 101 || createReq.OSTemplate == nil || *createReq.OSTemplate != "local:vztmpl/debian-12.tar.zst" {
		t.Fatalf("unexpected create identity/template fields: %#v", createReq)
	}
	if createReq.RootFS == nil || *createReq.RootFS != "local-lvm:8" || createReq.Arch == nil || *createReq.Arch != "amd64" || createReq.Unprivileged == nil || !*createReq.Unprivileged {
		t.Fatalf("expected replacement fields in create request: %#v", createReq)
	}
	if createReq.Hostname == nil || *createReq.Hostname != "ct-101" || createReq.Memory == nil || *createReq.Memory != 512 || createReq.OnBoot == nil || !*createReq.OnBoot {
		t.Fatalf("expected typed fields in create request: %#v", createReq)
	}
	if got := createReq.Network["net0"]; got != "name=eth0,bridge=vmbr0,ip=dhcp" {
		t.Fatalf("unexpected create network map: %#v", createReq.Network)
	}
	if got := createReq.MountPoint["mp0"]; got != "local-lvm:1,mp=/data" {
		t.Fatalf("unexpected create mount map: %#v", createReq.MountPoint)
	}
	if got := createReq.ExtraConfig["lxc.apparmor.profile"]; got != "unconfined" {
		t.Fatalf("unexpected create raw map: %#v", createReq.ExtraConfig)
	}

	updateReq, diags := lxcContainerUpdateRequestFromModel(ctx, model, model)
	assertNoDiags(t, diags)
	if updateReq.RootFS != nil || updateReq.Arch != nil || updateReq.Unprivileged != nil {
		t.Fatalf("did not expect replacement fields in update request: %#v", updateReq)
	}
	if updateReq.Hostname == nil || *updateReq.Hostname != "ct-101" || updateReq.Memory == nil || *updateReq.Memory != 512 || updateReq.OnBoot == nil || !*updateReq.OnBoot {
		t.Fatalf("expected typed fields in update request: %#v", updateReq)
	}
	if got := updateReq.Network["net0"]; got != "name=eth0,bridge=vmbr0,ip=dhcp" {
		t.Fatalf("unexpected update network map: %#v", updateReq.Network)
	}
	if got := updateReq.MountPoint["mp0"]; got != "local-lvm:1,mp=/data" {
		t.Fatalf("unexpected update mount map: %#v", updateReq.MountPoint)
	}
	if got := updateReq.ExtraConfig["lxc.apparmor.profile"]; got != "unconfined" {
		t.Fatalf("unexpected update raw map: %#v", updateReq.ExtraConfig)
	}
}

func TestLXCContainerUpdateRequestDeletesRemovedKeys(t *testing.T) {
	ctx := context.Background()
	prior := lxcContainerModel{
		Hostname:   types.StringValue("ct-101"),
		Tags:       types.StringValue("prod,terraform"),
		Network:    mustLXCContainerNetworkMapValue(t, map[string]lxcContainerNetworkModel{"net0": {Name: types.StringValue("eth0"), Bridge: types.StringValue("vmbr0"), IP: types.StringValue("dhcp")}, "net1": {Name: types.StringValue("eth1"), Bridge: types.StringValue("vmbr1"), IP: types.StringValue("dhcp")}}),
		MountPoint: mustLXCContainerMountPointMapValue(t, map[string]lxcContainerMountPointModel{"mp0": {Volume: types.StringValue("local-lvm:1"), MountPoint: types.StringValue("/data")}}),
		Raw: mustLXCContainerRawValue(t, lxcContainerRawModel{ExtraConfig: mustStringMapValue(t, map[string]string{
			"lxc.apparmor.profile": "unconfined",
			"lxc.keep":             "1",
		})}),
	}
	plan := lxcContainerModel{
		Hostname:   types.StringNull(),
		Tags:       types.StringNull(),
		Network:    mustLXCContainerNetworkMapValue(t, map[string]lxcContainerNetworkModel{"net1": {Name: types.StringValue("eth1"), Bridge: types.StringValue("vmbr1"), IP: types.StringValue("dhcp")}}),
		MountPoint: types.MapNull(types.ObjectType{AttrTypes: lxcContainerMountPointAttrTypes()}),
		Raw:        mustLXCContainerRawValue(t, lxcContainerRawModel{ExtraConfig: mustStringMapValue(t, map[string]string{"lxc.keep": "1"})}),
	}

	updateReq, diags := lxcContainerUpdateRequestFromModel(ctx, plan, prior)
	assertNoDiags(t, diags)
	expected := []string{"hostname", "lxc.apparmor.profile", "mp0", "net0", "tags"}
	if !reflect.DeepEqual(updateReq.Delete, expected) {
		t.Fatalf("unexpected delete keys: got %#v want %#v", updateReq.Delete, expected)
	}
}

func TestValidateLXCContainerRawConflictsReservesTypedKeys(t *testing.T) {
	model := lxcContainerModel{
		Raw: mustLXCContainerRawValue(t, lxcContainerRawModel{ExtraConfig: mustStringMapValue(t, map[string]string{
			"rootfs":   "local-lvm:8",
			"hostname": "ct-101",
			"net0":     "name=eth0,bridge=vmbr0,ip=dhcp",
			"mp0":      "local-lvm:1,mp=/data",
		})}),
	}

	diags := validateLXCContainerRawConflicts(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected raw conflict diagnostics")
	}
	for _, key := range []string{"rootfs", "hostname", "net0", "mp0"} {
		found := false
		for _, diag := range diags {
			if strings.Contains(diag.Detail(), key) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected conflict diagnostic for %q, got %v", key, diags)
		}
	}
}

func TestValidateLXCContainerMapKeysRejectsInvalidKeys(t *testing.T) {
	model := lxcContainerModel{
		Network:    mustStringMapValue(t, map[string]string{"hostname": "ct-101"}),
		MountPoint: mustStringMapValue(t, map[string]string{"rootfs": "local-lvm:8"}),
	}

	diags := validateLXCContainerMapKeys(context.Background(), model)
	if len(diags) != 2 {
		t.Fatalf("expected two invalid map key diagnostics, got %v", diags)
	}
	if got := diags[0].Summary(); got != "Invalid LXC network key" {
		t.Fatalf("unexpected first diagnostic summary: %q", got)
	}
	if got := diags[1].Summary(); got != "Invalid LXC mount_point key" {
		t.Fatalf("unexpected second diagnostic summary: %q", got)
	}

	_, requestDiags := lxcContainerCreateRequestFromModel(context.Background(), model)
	if !requestDiags.HasError() {
		t.Fatal("expected request mapping to reject invalid map keys")
	}
}

func TestParseLXCContainerImportID(t *testing.T) {
	node, vmID, err := parseLXCContainerImportID("pve-1/101")
	if err != nil {
		t.Fatalf("parseLXCContainerImportID() unexpected error: %v", err)
	}
	if node != "pve-1" || vmID != 101 {
		t.Fatalf("unexpected parsed import id: node=%q vmID=%d", node, vmID)
	}

	for _, value := range []string{"missing-slash", "/101", "pve-1/", "pve-1/not-an-int"} {
		if _, _, err := parseLXCContainerImportID(value); err == nil {
			t.Fatalf("expected error for import identifier %q", value)
		}
	}
}

func decodeLXCContainerRaw(t *testing.T, value types.Object) lxcContainerRawModel {
	t.Helper()
	if value.IsNull() || value.IsUnknown() {
		t.Fatalf("expected known raw object, got %#v", value)
	}
	var result lxcContainerRawModel
	assertNoDiags(t, value.As(context.Background(), &result, qemuObjectAsOptions()))
	return result
}

func mustLXCContainerRawValue(t *testing.T, value lxcContainerRawModel) types.Object {
	t.Helper()
	result, diags := types.ObjectValueFrom(context.Background(), lxcContainerRawAttrTypes(), value)
	assertNoDiags(t, diags)
	return result
}

func assertStringMapValue(t *testing.T, value types.Map, expected map[string]string) {
	t.Helper()
	if value.IsNull() || value.IsUnknown() {
		t.Fatalf("expected known map, got %#v", value)
	}
	var actual map[string]string
	assertNoDiags(t, value.ElementsAs(context.Background(), &actual, false))
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("unexpected map value: got %#v want %#v", actual, expected)
	}
}

func mustLXCContainerCloneValue(t *testing.T, value lxcContainerCloneModel) types.Object {
	t.Helper()
	result, diags := types.ObjectValueFrom(context.Background(), lxcContainerCloneAttrTypes(), value)
	assertNoDiags(t, diags)
	return result
}

func TestLXCContainerCloneRequestFromModel(t *testing.T) {
	t.Parallel()

	model := lxcContainerModel{
		Node:     types.StringValue("pve-1"),
		VMID:     types.Int64Value(200),
		Hostname: types.StringValue("cloned-ct"),
		Clone: mustLXCContainerCloneValue(t, lxcContainerCloneModel{
			SourceVMID: types.Int64Value(9000),
			Full:       types.BoolValue(true),
			Storage:    types.StringValue("local-lvm"),
		}),
	}

	cloneReq, diags := lxcContainerCloneRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if cloneReq.SourceNode != "pve-1" || cloneReq.SourceVMID != 9000 || cloneReq.NewID != 200 {
		t.Fatalf("unexpected clone core values: %#v", cloneReq)
	}
	if cloneReq.Hostname == nil || *cloneReq.Hostname != "cloned-ct" {
		t.Fatalf("unexpected hostname: %#v", cloneReq.Hostname)
	}
	if cloneReq.Full == nil || !*cloneReq.Full {
		t.Fatalf("expected full=true, got %#v", cloneReq.Full)
	}
	if cloneReq.Storage == nil || *cloneReq.Storage != "local-lvm" {
		t.Fatalf("unexpected storage: %#v", cloneReq.Storage)
	}
}

func TestLXCContainerCloneRequestDefaultsSourceNode(t *testing.T) {
	t.Parallel()

	model := lxcContainerModel{
		Node: types.StringValue("pve-2"),
		VMID: types.Int64Value(201),
		Clone: mustLXCContainerCloneValue(t, lxcContainerCloneModel{
			SourceVMID: types.Int64Value(9001),
		}),
	}

	cloneReq, diags := lxcContainerCloneRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if cloneReq.SourceNode != "pve-2" {
		t.Fatalf("expected source_node to default to managed node pve-2, got %q", cloneReq.SourceNode)
	}
}

func mustLXCContainerNetworkMapValue(t *testing.T, items map[string]lxcContainerNetworkModel) types.Map {
	t.Helper()
	if len(items) == 0 {
		return types.MapNull(types.ObjectType{AttrTypes: lxcContainerNetworkAttrTypes()})
	}
	result, diags := types.MapValueFrom(context.Background(), types.ObjectType{AttrTypes: lxcContainerNetworkAttrTypes()}, items)
	assertNoDiags(t, diags)
	return result
}

func mustLXCContainerMountPointMapValue(t *testing.T, items map[string]lxcContainerMountPointModel) types.Map {
	t.Helper()
	if len(items) == 0 {
		return types.MapNull(types.ObjectType{AttrTypes: lxcContainerMountPointAttrTypes()})
	}
	result, diags := types.MapValueFrom(context.Background(), types.ObjectType{AttrTypes: lxcContainerMountPointAttrTypes()}, items)
	assertNoDiags(t, diags)
	return result
}

func decodeLXCContainerNetworkMap(t *testing.T, value types.Map) map[string]lxcContainerNetworkModel {
	t.Helper()
	if value.IsNull() || value.IsUnknown() {
		t.Fatalf("expected known network map, got %#v", value)
	}
	var result map[string]lxcContainerNetworkModel
	assertNoDiags(t, value.ElementsAs(context.Background(), &result, false))
	return result
}

func decodeLXCContainerMountPointMap(t *testing.T, value types.Map) map[string]lxcContainerMountPointModel {
	t.Helper()
	if value.IsNull() || value.IsUnknown() {
		t.Fatalf("expected known mount_point map, got %#v", value)
	}
	var result map[string]lxcContainerMountPointModel
	assertNoDiags(t, value.ElementsAs(context.Background(), &result, false))
	return result
}

func TestParseAndEncodeLXCContainerNetwork(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		raw    string
		parsed lxcContainerNetworkModel
	}{
		{name: "name bridge ip", raw: "name=eth0,bridge=vmbr0,ip=dhcp", parsed: lxcContainerNetworkModel{Name: types.StringValue("eth0"), Bridge: types.StringValue("vmbr0"), IP: types.StringValue("dhcp")}},
		{name: "with firewall and mtu", raw: "name=eth0,bridge=vmbr0,mtu=1400,firewall=1", parsed: lxcContainerNetworkModel{Name: types.StringValue("eth0"), Bridge: types.StringValue("vmbr0"), Firewall: types.BoolValue(true), MTU: types.Int64Value(1400)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parsed, ok := parseLXCContainerNetwork(tc.raw)
			if !ok {
				t.Fatalf("parseLXCContainerNetwork(%q) expected ok", tc.raw)
			}
			encoded := encodeLXCContainerNetwork(parsed)
			if encoded != tc.raw {
				t.Fatalf("encode round-trip = %q, want %q", encoded, tc.raw)
			}
		})
	}
}

func TestParseLXCContainerMountPointRequiresVolumeAndPath(t *testing.T) {
	t.Parallel()
	if _, ok := parseLXCContainerMountPoint("local-lvm:1"); ok {
		t.Fatal("expected mount point without mp= to be unparseable so it stays raw")
	}
	if _, ok := parseLXCContainerMountPoint("local-lvm:1,mp=/data"); ok {
		// ok expected
	} else {
		t.Fatal("expected mount point with volume + mp to parse")
	}
}
