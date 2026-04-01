// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestQemuVMStateFromAPI(t *testing.T) {
	t.Parallel()

	state := qemuVMStateFromAPI("pve-1", 101, QemuVMConfig{
		Name:        "api-vm",
		Description: "Managed by Terraform",
		Tags:        "prod,terraform",
		Template:    proxmoxOptionalBool{value: boolPtr(false)},
		Pool:        "platform",
		OnBoot:      proxmoxOptionalBool{value: boolPtr(true)},
		Startup:     "order=2",
		Bios:        "ovmf",
		Machine:     "q35",
		Agent:       "enabled=1",
		Cores:       proxmoxOptionalInt64{value: intPtr64(4)},
		Sockets:     proxmoxOptionalInt64{value: intPtr64(2)},
		Memory:      proxmoxOptionalInt64{value: intPtr64(8192)},
		CPU:         "host",
		OSType:      "l26",
		Boot:        "order=scsi0;net0",
	}, QemuVMStatus{Status: "running", Uptime: proxmoxOptionalInt64{value: intPtr64(300)}})

	if state.ID.ValueString() != "pve-1/101" || state.Node.ValueString() != "pve-1" || state.VMID.ValueInt64() != 101 {
		t.Fatalf("unexpected identity state: %#v", state)
	}
	if !state.OnBoot.ValueBool() || state.Template.ValueBool() {
		t.Fatalf("unexpected bool mapping: %#v", state)
	}
	if state.Cores.ValueInt64() != 4 || state.Uptime.ValueInt64() != 300 {
		t.Fatalf("unexpected integer mapping: %#v", state)
	}
}

func TestParseQemuVMImportID(t *testing.T) {
	t.Parallel()

	node, vmID, err := parseQemuVMImportID("pve-1/101")
	if err != nil {
		t.Fatalf("parseQemuVMImportID() unexpected error: %v", err)
	}
	if node != "pve-1" || vmID != 101 {
		t.Fatalf("unexpected parsed values: node=%q vmID=%d", node, vmID)
	}

	if _, _, err := parseQemuVMImportID("missing-slash"); err == nil {
		t.Fatal("expected error for malformed import identifier")
	}
}

func TestQemuVMRequestFromModel(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		VMID:        types.Int64Value(101),
		Name:        types.StringValue("api-vm"),
		Description: types.StringValue("Managed by Terraform"),
		Tags:        types.StringValue("prod,terraform"),
		Pool:        types.StringValue("platform"),
		OnBoot:      types.BoolValue(true),
		Startup:     types.StringValue("order=2"),
		Bios:        types.StringValue("ovmf"),
		Machine:     types.StringValue("q35"),
		Agent:       types.StringValue("enabled=1"),
		Cores:       types.Int64Value(4),
		Sockets:     types.Int64Value(2),
		Memory:      types.Int64Value(8192),
		CPU:         types.StringValue("host"),
		OSType:      types.StringValue("l26"),
		Boot:        types.StringValue("order=scsi0;net0"),
	}

	createReq := qemuVMCreateRequestFromModel(model)
	if createReq.VMID != 101 || createReq.Name == nil || *createReq.Name != "api-vm" {
		t.Fatalf("unexpected create request: %#v", createReq)
	}

	updateReq := qemuVMUpdateRequestFromModel(model)
	if updateReq.OnBoot == nil || !*updateReq.OnBoot || updateReq.Memory == nil || *updateReq.Memory != 8192 {
		t.Fatalf("unexpected update request: %#v", updateReq)
	}

	if got, want := reflect.ValueOf(updateReq).NumField(), reflect.ValueOf(UpdateQemuVMRequest{}).NumField(); got != want {
		t.Fatalf("unexpected update request field count: got %d want %d", got, want)
	}
}
