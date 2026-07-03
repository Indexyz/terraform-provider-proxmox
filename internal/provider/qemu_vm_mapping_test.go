// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestQemuVMStateFromAPI(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{
		Name:        "api-vm",
		Description: "Managed by Terraform",
		Tags:        "prod,terraform",
		Template:    proxmoxOptionalBool{value: boolPtr(false)},
		Pool:        "platform",
		OnBoot:      proxmoxOptionalBool{value: boolPtr(true)},
		Protection:  proxmoxOptionalBool{value: boolPtr(true)},
		SCSIHW:      "virtio-scsi-pci",
		Tablet:      proxmoxOptionalBool{value: boolPtr(true)},
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
		Hotplug:     "network,disk,usb,cloudinit",
		CIUser:      "ubuntu",
		CIType:      "nocloud",
		CIUpgrade:   proxmoxOptionalBool{value: boolPtr(true)},
		SSHKeys:     "ssh-ed25519 AAAA... user@example",
		IPConfig: map[string]string{
			"ipconfig0": "ip=dhcp,ip6=auto",
		},
		Network: map[string]string{
			"net0": "virtio=BC:24:11:AA:BB:CC,bridge=vmbr0,tag=20,firewall=1,link_down=0,mtu=1400,queues=4,rate=5",
			"net1": "e1000=BC:24:11:AA:BB:DD,bridge=vmbr1,trunks=10;20",
		},
		Disk: map[string]string{
			"ide0":    "local-lvm:vm-101-disk-0,backup=1,shared=0,snapshot=1,serial=ide-disk,iops=100,mbps=1.5",
			"sata0":   "local-lvm:vm-101-disk-1,backup=0,shared=1,snapshot=0,serial=sata-disk,iops_max=200,mbps_max=2.5",
			"scsi0":   "local-lvm:vm-101-disk-2,cache=writeback,discard=on,iothread=1,media=disk,replicate=0,backup=1,shared=0,snapshot=1,serial=scsi-disk,size=32G,ssd=1,iops_rd=300,iops_rd_max=400,mbps_rd=3.5,mbps_rd_max=4.5",
			"virtio0": "local-lvm:vm-101-disk-3,backup=1,shared=1,snapshot=0,serial=virtio-disk,iops_wr=500,iops_wr_max=600,mbps_wr=5.5,mbps_wr_max=6.5",
			"scsi1":   "local-lvm:vm-101-disk-4,wwn=needs-raw",
		},
		ExtraConfig: map[string]string{
			"hostpci0": "0000:00:1f.0",
		},
	}, QemuVMStatus{Status: "running", Uptime: proxmoxOptionalInt64{value: intPtr64(300)}}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}

	if state.ID.ValueString() != "pve-1/101" || state.Node.ValueString() != "pve-1" || state.VMID.ValueInt64() != 101 {
		t.Fatalf("unexpected identity state: %#v", state)
	}
	if !state.OnBoot.ValueBool() || !state.Protection.ValueBool() || state.Template.ValueBool() {
		t.Fatalf("unexpected bool mapping: %#v", state)
	}
	if !state.Tablet.ValueBool() {
		t.Fatalf("expected tablet=true state, got %#v", state.Tablet)
	}
	if state.Cores.ValueInt64() != 4 || state.Uptime.ValueInt64() != 300 {
		t.Fatalf("unexpected integer mapping: %#v", state)
	}
	if state.SCSIHW.ValueString() != "virtio-scsi-pci" {
		t.Fatalf("expected scsihw state, got %#v", state.SCSIHW)
	}

	common := decodeQemuVMCommon(t, state.Common)
	if common.Hotplug.ValueString() != "network,disk,usb,cloudinit" {
		t.Fatalf("unexpected common block: %#v", common)
	}

	cloudInit := decodeQemuVMCloudInit(t, state.CloudInit)
	if cloudInit.CIUser.ValueString() != "ubuntu" || !cloudInit.CIUpgrade.ValueBool() {
		t.Fatalf("unexpected cloud_init block: %#v", cloudInit)
	}
	ipConfig := decodeQemuVMIPConfigMap(t, cloudInit.IPConfig)
	if entry := ipConfig["ipconfig0"]; entry.IPv4.ValueString() != "dhcp" || entry.IPv6.ValueString() != "auto" {
		t.Fatalf("unexpected ipconfig mapping: %#v", ipConfig)
	}

	network := decodeQemuVMNetworkMap(t, state.Network)
	if got := network["net0"]; got.Model.ValueString() != "virtio" || got.Bridge.ValueString() != "vmbr0" || got.Tag.ValueInt64() != 20 || !got.Firewall.ValueBool() || got.Rate.ValueFloat64() != 5 {
		t.Fatalf("unexpected network mapping: %#v", network)
	}
	if got := network["net1"]; got.Model.ValueString() != "e1000" || got.Bridge.ValueString() != "vmbr1" || got.Trunks.ValueString() != "10;20" {
		t.Fatalf("unexpected network mapping for trunks support: %#v", network)
	}

	disks := decodeQemuVMDiskMap(t, state.Disk)
	if got := disks["ide0"]; got.Storage.ValueString() != "local-lvm" || got.Volume.ValueString() != "local-lvm:vm-101-disk-0" || !got.Backup.ValueBool() || got.Shared.ValueBool() || !got.Snapshot.ValueBool() || got.Serial.ValueString() != "ide-disk" || got.IOPS.ValueInt64() != 100 || got.MBPS.ValueFloat64() != 1.5 {
		t.Fatalf("unexpected ide disk mapping: %#v", got)
	}
	if got := disks["sata0"]; got.Storage.ValueString() != "local-lvm" || got.Volume.ValueString() != "local-lvm:vm-101-disk-1" || got.Backup.ValueBool() || !got.Shared.ValueBool() || got.Snapshot.ValueBool() || got.Serial.ValueString() != "sata-disk" || got.IOPSMax.ValueInt64() != 200 || got.MBPSMax.ValueFloat64() != 2.5 {
		t.Fatalf("unexpected sata disk mapping: %#v", got)
	}
	if got := disks["scsi0"]; got.Storage.ValueString() != "local-lvm" || got.Volume.ValueString() != "local-lvm:vm-101-disk-2" || got.Size.ValueString() != "32G" || !got.Iothread.ValueBool() || got.Replicate.ValueBool() || !got.Backup.ValueBool() || got.Shared.ValueBool() || !got.Snapshot.ValueBool() || got.Serial.ValueString() != "scsi-disk" || got.IOPSRd.ValueInt64() != 300 || got.IOPSRdMax.ValueInt64() != 400 || got.MBPSRd.ValueFloat64() != 3.5 || got.MBPSRdMax.ValueFloat64() != 4.5 {
		t.Fatalf("unexpected disk mapping: %#v", disks)
	}
	if got := disks["virtio0"]; got.Storage.ValueString() != "local-lvm" || got.Volume.ValueString() != "local-lvm:vm-101-disk-3" || !got.Backup.ValueBool() || !got.Shared.ValueBool() || got.Snapshot.ValueBool() || got.Serial.ValueString() != "virtio-disk" || got.IOPSWr.ValueInt64() != 500 || got.IOPSWrMax.ValueInt64() != 600 || got.MBPSWr.ValueFloat64() != 5.5 || got.MBPSWrMax.ValueFloat64() != 6.5 {
		t.Fatalf("unexpected virtio disk mapping: %#v", got)
	}
	if _, ok := disks["scsi1"]; ok {
		t.Fatalf("expected unsupported scsi1 config to remain raw, got %#v", disks["scsi1"])
	}

	raw := decodeQemuVMRaw(t, state.Raw)
	gotRaw := decodeStringMap(t, raw.ExtraConfig)
	wantRaw := map[string]string{
		"hostpci0": "0000:00:1f.0",
		"scsi1":    "local-lvm:vm-101-disk-4,wwn=needs-raw",
	}
	if !reflect.DeepEqual(gotRaw, wantRaw) {
		t.Fatalf("unexpected raw extra_config: got %#v want %#v", gotRaw, wantRaw)
	}
}

func TestQemuVMStateFromAPIDefaultsOmittedProtectionToFalse(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{Name: "api-vm"}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if state.Protection.IsNull() || state.Protection.IsUnknown() || state.Protection.ValueBool() {
		t.Fatalf("expected omitted protection to read as false, got %#v", state.Protection)
	}
}

func TestQemuVMStateFromAPIDefaultsOmittedTabletToFalse(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{Name: "api-vm"}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if state.Tablet.IsNull() || state.Tablet.IsUnknown() || state.Tablet.ValueBool() {
		t.Fatalf("expected omitted tablet to read as false, got %#v", state.Tablet)
	}
}

func TestQemuVMStateFromAPIOmittedSCSIHWIsNull(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{Name: "api-vm"}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if !state.SCSIHW.IsNull() || state.SCSIHW.IsUnknown() {
		t.Fatalf("expected omitted scsihw to read as null, got %#v", state.SCSIHW)
	}
}

func TestQemuVMStateFromAPIPreservesCloneState(t *testing.T) {
	t.Parallel()

	clone := mustQemuVMCloneValue(t, qemuVMCloneModel{
		SourceNode:   types.StringValue("pve-template"),
		SourceVMID:   types.Int64Value(9000),
		Full:         types.BoolValue(true),
		SnapshotName: types.StringValue("golden"),
		Storage:      types.StringValue("local-lvm"),
	})

	prior := qemuVMModel{Clone: clone}
	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{Name: "api-vm"}, QemuVMStatus{Status: "running"}, &prior)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}

	got := decodeQemuVMClone(t, state.Clone)
	if got.SourceNode.ValueString() != "pve-template" || got.SourceVMID.ValueInt64() != 9000 || !got.Full.ValueBool() {
		t.Fatalf("expected clone state to be preserved, got %#v", got)
	}
}

func TestQemuVMStateFromAPINullCloneWithoutPriorState(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		prior *qemuVMModel
	}{
		{name: "nil prior"},
		{name: "prior without clone provenance", prior: &qemuVMModel{Clone: types.ObjectNull(qemuVMCloneAttrTypes())}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{Name: "api-vm"}, QemuVMStatus{Status: "running"}, tc.prior)
			if diags.HasError() {
				t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
			}
			if !state.Clone.IsNull() || state.Clone.IsUnknown() {
				t.Fatalf("expected clone to remain null without prior clone provenance, got %#v", state.Clone)
			}
		})
	}
}

func TestQemuVMStateFromAPIPreservesSlotKeyedAdvancedDomains(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{
		IPConfig: map[string]string{
			"ipconfig0": "ip=dhcp,ip6=auto",
		},
		Network: map[string]string{
			"net1": "e1000=BC:24:11:AA:BB:DD,bridge=vmbr1,trunks=10;20",
			"net0": "virtio=BC:24:11:AA:BB:CC,bridge=vmbr0,tag=20,firewall=1,link_down=0,mtu=1400,queues=4,rate=5",
		},
		Disk: map[string]string{
			"ide0":    "local-lvm:vm-101-disk-0,backup=1,shared=0,snapshot=1,serial=ide-disk,iops=100,mbps=1.5",
			"sata0":   "local-lvm:vm-101-disk-1,backup=0,shared=1,snapshot=0,serial=sata-disk,iops_max=200,mbps_max=2.5",
			"scsi0":   "local-lvm:vm-101-disk-2,cache=writeback,discard=on,iothread=1,media=disk,replicate=0,backup=1,shared=0,snapshot=1,serial=scsi-disk,size=32G,ssd=1,iops_rd=300,iops_rd_max=400,mbps_rd=3.5,mbps_rd_max=4.5",
			"virtio0": "local-lvm:vm-101-disk-3,backup=1,shared=1,snapshot=0,serial=virtio-disk,iops_wr=500,iops_wr_max=600,mbps_wr=5.5,mbps_wr_max=6.5",
			"scsi1":   "local-lvm:vm-101-disk-4,wwn=needs-raw",
			"scsi2":   "local-lvm:vm-101-disk-5,bps=1024",
			"virtio1": "local-lvm:vm-101-disk-6,iops_wr_max_length=60",
		},
	}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}

	cloudInit := decodeQemuVMCloudInit(t, state.CloudInit)
	ipConfig := decodeQemuVMIPConfigMap(t, cloudInit.IPConfig)
	if _, ok := ipConfig["ipconfig0"]; !ok || len(ipConfig) != 1 {
		t.Fatalf("expected slot-keyed ipconfig map to preserve ipconfig0, got %#v", ipConfig)
	}

	network := decodeQemuVMNetworkMap(t, state.Network)
	if _, ok := network["net0"]; !ok || len(network) != 2 {
		t.Fatalf("expected slot-keyed network map to preserve both typed network slots, got %#v", network)
	}
	if got := network["net1"]; got.Trunks.ValueString() != "10;20" {
		t.Fatalf("expected typed trunks support to preserve net1, got %#v", network)
	}

	disks := decodeQemuVMDiskMap(t, state.Disk)
	for _, key := range []string{"ide0", "sata0", "scsi0", "virtio0"} {
		if _, ok := disks[key]; !ok {
			t.Fatalf("expected slot-keyed disk map to preserve %s, got %#v", key, disks)
		}
	}
	if len(disks) != 4 {
		t.Fatalf("expected four typed disk slots, got %#v", disks)
	}

	raw := decodeQemuVMRaw(t, state.Raw)
	gotRaw := decodeStringMap(t, raw.ExtraConfig)
	wantRaw := map[string]string{
		"scsi1":   "local-lvm:vm-101-disk-4,wwn=needs-raw",
		"scsi2":   "local-lvm:vm-101-disk-5,bps=1024",
		"virtio1": "local-lvm:vm-101-disk-6,iops_wr_max_length=60",
	}
	if !reflect.DeepEqual(gotRaw, wantRaw) {
		t.Fatalf("expected unsupported slot grammar to remain in raw.extra_config, got %#v want %#v", gotRaw, wantRaw)
	}
}

func TestQemuVMStateFromAPITypesEFIDisk0(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{
		ExtraConfig: map[string]string{
			"efidisk0": "local-lvm:vm-101-disk-0,efitype=4m,format=qcow2,ms-cert=2023,pre-enrolled-keys=1,size=528K",
			"hostpci0": "0000:00:1f.0",
		},
	}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}

	efiDisk := decodeQemuVMEFIDisk(t, state.EFIDisk)
	if efiDisk.Storage.ValueString() != "local-lvm" || efiDisk.Volume.ValueString() != "local-lvm:vm-101-disk-0" {
		t.Fatalf("unexpected typed efi_disk volume/storage: %#v", efiDisk)
	}
	if efiDisk.EFIType.ValueString() != "4m" || efiDisk.Format.ValueString() != "qcow2" || efiDisk.MSCert.ValueString() != "2023" || !efiDisk.PreEnrolledKeys.ValueBool() || efiDisk.Size.ValueString() != "528K" {
		t.Fatalf("unexpected typed efi_disk mapping: %#v", efiDisk)
	}

	raw := decodeQemuVMRaw(t, state.Raw)
	gotRaw := decodeStringMap(t, raw.ExtraConfig)
	wantRaw := map[string]string{"hostpci0": "0000:00:1f.0"}
	if !reflect.DeepEqual(gotRaw, wantRaw) {
		t.Fatalf("expected typed efidisk0 to be removed from raw.extra_config, got %#v want %#v", gotRaw, wantRaw)
	}
}

func TestQemuVMStateFromAPIPreservesUnsupportedEFIDisk0InRaw(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{
		ExtraConfig: map[string]string{
			"efidisk0": "local-lvm:vm-101-disk-0,iothread=1",
		},
	}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if !state.EFIDisk.IsNull() || state.EFIDisk.IsUnknown() {
		t.Fatalf("expected unsupported efidisk0 grammar to remain untyped, got %#v", state.EFIDisk)
	}

	raw := decodeQemuVMRaw(t, state.Raw)
	gotRaw := decodeStringMap(t, raw.ExtraConfig)
	wantRaw := map[string]string{"efidisk0": "local-lvm:vm-101-disk-0,iothread=1"}
	if !reflect.DeepEqual(gotRaw, wantRaw) {
		t.Fatalf("expected unsupported efidisk0 grammar to remain in raw.extra_config, got %#v want %#v", gotRaw, wantRaw)
	}
}

func TestQemuVMStateFromAPITypesTPMState0(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{
		ExtraConfig: map[string]string{
			"tpmstate0": "local-lvm:vm-101-disk-9,format=raw,size=4M,version=v2.0",
			"hostpci0":  "0000:00:1f.0",
		},
	}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}

	tpmState := decodeQemuVMTPMState(t, state.TPMState)
	if tpmState.Storage.ValueString() != "local-lvm" || tpmState.Volume.ValueString() != "local-lvm:vm-101-disk-9" {
		t.Fatalf("unexpected typed tpm_state volume/storage: %#v", tpmState)
	}
	if tpmState.Format.ValueString() != "raw" || tpmState.Size.ValueString() != "4M" || tpmState.Version.ValueString() != "v2.0" {
		t.Fatalf("unexpected typed tpm_state mapping: %#v", tpmState)
	}

	raw := decodeQemuVMRaw(t, state.Raw)
	gotRaw := decodeStringMap(t, raw.ExtraConfig)
	wantRaw := map[string]string{"hostpci0": "0000:00:1f.0"}
	if !reflect.DeepEqual(gotRaw, wantRaw) {
		t.Fatalf("expected typed tpmstate0 to be removed from raw.extra_config, got %#v want %#v", gotRaw, wantRaw)
	}
}

func TestQemuVMStateFromAPIPreservesUnsupportedTPMState0InRaw(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{
		ExtraConfig: map[string]string{
			"tpmstate0": "local-lvm:vm-101-disk-9,iothread=1",
		},
	}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if !state.TPMState.IsNull() || state.TPMState.IsUnknown() {
		t.Fatalf("expected unsupported tpmstate0 grammar to remain untyped, got %#v", state.TPMState)
	}

	raw := decodeQemuVMRaw(t, state.Raw)
	gotRaw := decodeStringMap(t, raw.ExtraConfig)
	wantRaw := map[string]string{"tpmstate0": "local-lvm:vm-101-disk-9,iothread=1"}
	if !reflect.DeepEqual(gotRaw, wantRaw) {
		t.Fatalf("expected unsupported tpmstate0 grammar to remain in raw.extra_config, got %#v want %#v", gotRaw, wantRaw)
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
		Protection:  types.BoolValue(false),
		SCSIHW:      types.StringValue("virtio-scsi-pci"),
		Tablet:      types.BoolValue(false),
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
		Common: mustQemuVMCommonValue(t, qemuVMCommonModel{
			Hotplug: types.StringValue("network,disk,usb,cloudinit"),
		}),
		CloudInit: mustQemuVMCloudInitValue(t, qemuVMCloudInitModel{
			CIUser:    types.StringValue("ubuntu"),
			CIType:    types.StringValue("nocloud"),
			CIUpgrade: types.BoolValue(true),
			SSHKeys:   types.StringValue("ssh-ed25519 AAAA... user@example"),
			IPConfig: mustQemuVMIPConfigMapValue(t, map[string]qemuVMIPConfigModel{
				"ipconfig0": {
					IPv4:     types.StringValue("dhcp"),
					Gateway:  types.StringNull(),
					IPv6:     types.StringValue("auto"),
					Gateway6: types.StringNull(),
				},
			}),
		}),
		Network: mustQemuVMNetworkMapValue(t, map[string]qemuVMNetworkModel{
			"net0": {
				Model:    types.StringValue("virtio"),
				Bridge:   types.StringValue("vmbr0"),
				MACAddr:  types.StringValue("BC:24:11:AA:BB:CC"),
				Tag:      types.Int64Value(20),
				Trunks:   types.StringValue("10;20"),
				Firewall: types.BoolValue(true),
				LinkDown: types.BoolValue(false),
				MTU:      types.Int64Value(1400),
				Queues:   types.Int64Value(4),
				Rate:     types.Float64Value(5),
			},
		}),
		Disk: mustQemuVMDiskMapValue(t, map[string]qemuVMDiskModel{
			"scsi0": {
				Storage:   types.StringValue("local-lvm"),
				Size:      types.StringValue("32G"),
				Media:     types.StringValue("disk"),
				Cache:     types.StringValue("writeback"),
				Discard:   types.StringValue("on"),
				Iothread:  types.BoolValue(true),
				SSD:       types.BoolValue(true),
				Replicate: types.BoolValue(false),
				Backup:    types.BoolValue(true),
				Shared:    types.BoolValue(false),
				Snapshot:  types.BoolValue(true),
				Serial:    types.StringValue("scsi-disk"),
				IOPS:      types.Int64Value(100),
				IOPSMax:   types.Int64Value(200),
				IOPSRd:    types.Int64Value(300),
				IOPSRdMax: types.Int64Value(400),
				IOPSWr:    types.Int64Value(500),
				IOPSWrMax: types.Int64Value(600),
				MBPS:      types.Float64Value(1.5),
				MBPSMax:   types.Float64Value(2.5),
				MBPSRd:    types.Float64Value(3.5),
				MBPSRdMax: types.Float64Value(4.5),
				MBPSWr:    types.Float64Value(5.5),
				MBPSWrMax: types.Float64Value(6.5),
			},
		}),
		Raw: mustQemuVMRawValue(t, qemuVMRawModel{
			ExtraConfig: mustStringMapValue(t, map[string]string{"hostpci0": "0000:00:1f.0"}),
		}),
		Clone: mustQemuVMCloneValue(t, qemuVMCloneModel{
			SourceNode:   types.StringValue("pve-template"),
			SourceVMID:   types.Int64Value(9000),
			Full:         types.BoolValue(true),
			SnapshotName: types.StringValue("golden"),
			Storage:      types.StringValue("local-lvm"),
			BWLimit:      types.Int64Value(2048),
		}),
	}

	createReq, diags := qemuVMCreateRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if createReq.VMID != 101 || createReq.Name == nil || *createReq.Name != "api-vm" {
		t.Fatalf("unexpected create request: %#v", createReq)
	}
	if createReq.Hotplug == nil || *createReq.Hotplug != "network,disk,usb,cloudinit" {
		t.Fatalf("expected hotplug in create request, got %#v", createReq)
	}
	if createReq.Protection == nil || *createReq.Protection {
		t.Fatalf("expected protection=false in create request, got %#v", createReq.Protection)
	}
	if createReq.SCSIHW == nil || *createReq.SCSIHW != "virtio-scsi-pci" {
		t.Fatalf("expected scsihw in create request, got %#v", createReq.SCSIHW)
	}
	if createReq.Tablet == nil || *createReq.Tablet {
		t.Fatalf("expected tablet=false in create request, got %#v", createReq.Tablet)
	}
	if got := createReq.IPConfig["ipconfig0"]; got != "ip=dhcp,ip6=auto" {
		t.Fatalf("unexpected ipconfig encoding: %#v", createReq.IPConfig)
	}
	if got := createReq.Network["net0"]; got != "virtio=BC:24:11:AA:BB:CC,bridge=vmbr0,tag=20,trunks=10;20,firewall=1,link_down=0,mtu=1400,queues=4,rate=5" {
		t.Fatalf("unexpected network encoding: %#v", createReq.Network)
	}
	if got := createReq.Disk["scsi0"]; got != "local-lvm:32G,media=disk,cache=writeback,discard=on,iothread=1,replicate=0,ssd=1,backup=1,shared=0,snapshot=1,serial=scsi-disk,iops=100,iops_max=200,iops_rd=300,iops_rd_max=400,iops_wr=500,iops_wr_max=600,mbps=1.5,mbps_max=2.5,mbps_rd=3.5,mbps_rd_max=4.5,mbps_wr=5.5,mbps_wr_max=6.5" {
		t.Fatalf("unexpected disk encoding: %#v", createReq.Disk)
	}
	if got := createReq.ExtraConfig["hostpci0"]; got != "0000:00:1f.0" {
		t.Fatalf("unexpected raw encoding: %#v", createReq.ExtraConfig)
	}

	cloneReq, diags := qemuVMCloneRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if cloneReq.SourceNode != "pve-template" || cloneReq.SourceVMID != 9000 || cloneReq.TargetNode != "" {
		t.Fatalf("unexpected clone request core values: %#v", cloneReq)
	}
	if cloneReq.NewID != 101 || cloneReq.Storage == nil || *cloneReq.Storage != "local-lvm" || cloneReq.BWLimit == nil || *cloneReq.BWLimit != 2048 {
		t.Fatalf("unexpected clone request: %#v", cloneReq)
	}

	updateReq, diags := qemuVMUpdateRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if updateReq.OnBoot == nil || !*updateReq.OnBoot || updateReq.Protection == nil || *updateReq.Protection || updateReq.SCSIHW == nil || *updateReq.SCSIHW != "virtio-scsi-pci" || updateReq.Tablet == nil || *updateReq.Tablet || updateReq.Memory == nil || *updateReq.Memory != 8192 {
		t.Fatalf("unexpected update request: %#v", updateReq)
	}
	if got, want := reflect.ValueOf(updateReq).NumField(), reflect.ValueOf(UpdateQemuVMRequest{}).NumField(); got != want {
		t.Fatalf("unexpected update request field count: got %d want %d", got, want)
	}
}

func TestQemuVMRequestFromModelMapsProtectionTrue(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		VMID:       types.Int64Value(101),
		Protection: types.BoolValue(true),
	}

	createReq, diags := qemuVMCreateRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if createReq.Protection == nil || !*createReq.Protection {
		t.Fatalf("expected protection=true in create request, got %#v", createReq.Protection)
	}

	updateReq, diags := qemuVMUpdateRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if updateReq.Protection == nil || !*updateReq.Protection {
		t.Fatalf("expected protection=true in update request, got %#v", updateReq.Protection)
	}
}

func TestQemuVMRequestFromModelMapsTabletTrue(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		VMID:   types.Int64Value(101),
		Tablet: types.BoolValue(true),
	}

	createReq, diags := qemuVMCreateRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if createReq.Tablet == nil || !*createReq.Tablet {
		t.Fatalf("expected tablet=true in create request, got %#v", createReq.Tablet)
	}

	updateReq, diags := qemuVMUpdateRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if updateReq.Tablet == nil || !*updateReq.Tablet {
		t.Fatalf("expected tablet=true in update request, got %#v", updateReq.Tablet)
	}
}

func TestValidateQemuVMRawConflicts(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		Network: mustQemuVMNetworkMapValue(t, map[string]qemuVMNetworkModel{
			"net0": {
				Model:  types.StringValue("virtio"),
				Bridge: types.StringValue("vmbr0"),
			},
		}),
		Raw: mustQemuVMRawValue(t, qemuVMRawModel{
			ExtraConfig: mustStringMapValue(t, map[string]string{
				"net0": "virtio=BC:24:11:AA:BB:CC,bridge=vmbr0",
			}),
		}),
	}

	diags := validateQemuVMRawConflicts(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected raw-vs-typed conflict diagnostics")
	}
	if got := diags[0].Summary(); got != "Conflicting raw.extra_config entry" {
		t.Fatalf("unexpected diagnostic summary: %q", got)
	}
}

func TestValidateQemuVMRawConflictsIncludesEFIDisk0(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		EFIDisk: mustQemuVMEFIDiskValue(t, qemuVMEFIDiskModel{
			Storage:         types.StringValue("local-lvm"),
			Size:            types.StringValue("4M"),
			EFIType:         types.StringValue("4m"),
			PreEnrolledKeys: types.BoolValue(true),
		}),
		Raw: mustQemuVMRawValue(t, qemuVMRawModel{
			ExtraConfig: mustStringMapValue(t, map[string]string{
				"efidisk0": "local-lvm:4M,efitype=4m,pre-enrolled-keys=1",
			}),
		}),
	}

	diags := validateQemuVMRawConflicts(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected raw-vs-typed conflict diagnostics for efidisk0")
	}
	if got := diags[0].Summary(); got != "Conflicting raw.extra_config entry" {
		t.Fatalf("unexpected diagnostic summary: %q", got)
	}
}

func TestValidateQemuVMRawConflictsIncludesTPMState0(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		TPMState: mustQemuVMTPMStateValue(t, qemuVMTPMStateModel{
			Storage: types.StringValue("local-lvm"),
			Size:    types.StringValue("4M"),
			Format:  types.StringValue("raw"),
			Version: types.StringValue("v2.0"),
		}),
		Raw: mustQemuVMRawValue(t, qemuVMRawModel{
			ExtraConfig: mustStringMapValue(t, map[string]string{
				"tpmstate0": "local-lvm:4M,format=raw,version=v2.0",
			}),
		}),
	}

	diags := validateQemuVMRawConflicts(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected raw-vs-typed conflict diagnostics for tpmstate0")
	}
	if got := diags[0].Summary(); got != "Conflicting raw.extra_config entry" {
		t.Fatalf("unexpected diagnostic summary: %q", got)
	}
}

func TestValidateQemuVMRawConflictsReservesProtection(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		Raw: mustQemuVMRawValue(t, qemuVMRawModel{
			ExtraConfig: mustStringMapValue(t, map[string]string{
				"protection": "1",
			}),
		}),
	}

	diags := validateQemuVMRawConflicts(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected raw-vs-typed conflict diagnostics for protection")
	}
	if got := diags[0].Summary(); got != "Conflicting raw.extra_config entry" {
		t.Fatalf("unexpected diagnostic summary: %q", got)
	}
}

func TestValidateQemuVMRawConflictsReservesSCSIHW(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		Raw: mustQemuVMRawValue(t, qemuVMRawModel{
			ExtraConfig: mustStringMapValue(t, map[string]string{
				"scsihw": "virtio-scsi-pci",
			}),
		}),
	}

	diags := validateQemuVMRawConflicts(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected raw-vs-typed conflict diagnostics for scsihw")
	}
	if got := diags[0].Summary(); got != "Conflicting raw.extra_config entry" {
		t.Fatalf("unexpected diagnostic summary: %q", got)
	}
}

func TestValidateQemuVMRawConflictsReservesTablet(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		Raw: mustQemuVMRawValue(t, qemuVMRawModel{
			ExtraConfig: mustStringMapValue(t, map[string]string{
				"tablet": "1",
			}),
		}),
	}

	diags := validateQemuVMRawConflicts(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected raw-vs-typed conflict diagnostics for tablet")
	}
	if got := diags[0].Summary(); got != "Conflicting raw.extra_config entry" {
		t.Fatalf("unexpected diagnostic summary: %q", got)
	}
}

func TestParseAndEncodeQemuVMNetworkTrunks(t *testing.T) {
	t.Parallel()

	parsed, ok := parseQemuVMNetwork("e1000=BC:24:11:AA:BB:DD,bridge=vmbr1,trunks=10;20")
	if !ok {
		t.Fatal("expected network config with trunks to parse")
	}
	if parsed.Model.ValueString() != "e1000" || parsed.Bridge.ValueString() != "vmbr1" || parsed.MACAddr.ValueString() != "BC:24:11:AA:BB:DD" || parsed.Trunks.ValueString() != "10;20" {
		t.Fatalf("unexpected parsed network config: %#v", parsed)
	}

	encoded := encodeQemuVMNetwork(qemuVMNetworkModel{
		Model:   types.StringValue("e1000"),
		MACAddr: types.StringValue("BC:24:11:AA:BB:DD"),
		Bridge:  types.StringValue("vmbr1"),
		Trunks:  types.StringValue("10;20"),
	})
	if encoded != "e1000=BC:24:11:AA:BB:DD,bridge=vmbr1,trunks=10;20" {
		t.Fatalf("unexpected encoded network config: %q", encoded)
	}
}

func TestParseAndEncodeQemuVMDiskP1cQoSFields(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name            string
		slot            string
		raw             string
		assertParsedQoS func(*testing.T, qemuVMDiskModel)
	}{
		{
			name: "ide",
			slot: "ide0",
			raw:  "local-lvm:vm-101-disk-0,backup=1,shared=0,snapshot=1,serial=ide-disk,iops=100,mbps=1.5",
			assertParsedQoS: func(t *testing.T, disk qemuVMDiskModel) {
				t.Helper()
				if disk.IOPS.ValueInt64() != 100 || disk.MBPS.ValueFloat64() != 1.5 {
					t.Fatalf("expected ide QoS fields to parse, got %#v", disk)
				}
			},
		},
		{
			name: "sata",
			slot: "sata0",
			raw:  "local-lvm:vm-101-disk-1,backup=0,shared=1,snapshot=0,serial=sata-disk,iops_max=200,mbps_max=2.5",
			assertParsedQoS: func(t *testing.T, disk qemuVMDiskModel) {
				t.Helper()
				if disk.IOPSMax.ValueInt64() != 200 || disk.MBPSMax.ValueFloat64() != 2.5 {
					t.Fatalf("expected sata QoS fields to parse, got %#v", disk)
				}
			},
		},
		{
			name: "scsi",
			slot: "scsi0",
			raw:  "local-lvm:vm-101-disk-2,backup=1,shared=0,snapshot=1,serial=scsi-disk,iops_rd=300,iops_rd_max=400,mbps_rd=3.5,mbps_rd_max=4.5",
			assertParsedQoS: func(t *testing.T, disk qemuVMDiskModel) {
				t.Helper()
				if disk.IOPSRd.ValueInt64() != 300 || disk.IOPSRdMax.ValueInt64() != 400 || disk.MBPSRd.ValueFloat64() != 3.5 || disk.MBPSRdMax.ValueFloat64() != 4.5 {
					t.Fatalf("expected scsi QoS fields to parse, got %#v", disk)
				}
			},
		},
		{
			name: "virtio",
			slot: "virtio0",
			raw:  "local-lvm:vm-101-disk-3,backup=1,shared=1,snapshot=0,serial=virtio-disk,iops_wr=500,iops_wr_max=600,mbps_wr=5.5,mbps_wr_max=6.5",
			assertParsedQoS: func(t *testing.T, disk qemuVMDiskModel) {
				t.Helper()
				if disk.IOPSWr.ValueInt64() != 500 || disk.IOPSWrMax.ValueInt64() != 600 || disk.MBPSWr.ValueFloat64() != 5.5 || disk.MBPSWrMax.ValueFloat64() != 6.5 {
					t.Fatalf("expected virtio QoS fields to parse, got %#v", disk)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parsed, ok := parseQemuVMDisk(tc.raw)
			if !ok {
				t.Fatalf("expected %s config to parse", tc.slot)
			}
			if parsed.Volume.ValueString() == "" || parsed.Storage.ValueString() != "local-lvm" {
				t.Fatalf("expected %s volume/storage to remain typed, got %#v", tc.slot, parsed)
			}
			if parsed.Serial.ValueString() == "" || parsed.Backup.IsNull() || parsed.Shared.IsNull() || parsed.Snapshot.IsNull() {
				t.Fatalf("expected %s P1b fields to remain typed, got %#v", tc.slot, parsed)
			}
			tc.assertParsedQoS(t, parsed)

			if encoded := encodeQemuVMDisk(parsed); encoded != tc.raw {
				t.Fatalf("unexpected encoded %s config: got %q want %q", tc.slot, encoded, tc.raw)
			}
		})
	}
}

func TestParseQemuVMDiskRejectsUnsupportedQoSBurstGrammar(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"local-lvm:vm-101-disk-4,bps=1024",
		"local-lvm:vm-101-disk-5,iops_wr_max_length=60",
	} {
		if _, ok := parseQemuVMDisk(raw); ok {
			t.Fatalf("expected unsupported QoS grammar %q to remain untyped", raw)
		}
	}
}

func TestQemuVMRequestFromModelEncodesEFIDisk0(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		EFIDisk: mustQemuVMEFIDiskValue(t, qemuVMEFIDiskModel{
			Storage:         types.StringValue("local-lvm"),
			Size:            types.StringValue("4M"),
			EFIType:         types.StringValue("4m"),
			Format:          types.StringValue("qcow2"),
			MSCert:          types.StringValue("2023"),
			PreEnrolledKeys: types.BoolValue(true),
		}),
		Raw: mustQemuVMRawValue(t, qemuVMRawModel{
			ExtraConfig: mustStringMapValue(t, map[string]string{
				"hostpci0": "0000:00:1f.0",
			}),
		}),
	}

	createReq, diags := qemuVMCreateRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if got := createReq.ExtraConfig["efidisk0"]; got != "local-lvm:4M,efitype=4m,format=qcow2,ms-cert=2023,pre-enrolled-keys=1" {
		t.Fatalf("unexpected efidisk0 encoding: %#v", createReq.ExtraConfig)
	}
	if got := createReq.ExtraConfig["hostpci0"]; got != "0000:00:1f.0" {
		t.Fatalf("expected raw.extra_config entries to remain merged with efidisk0, got %#v", createReq.ExtraConfig)
	}
}

func TestQemuVMRequestFromModelEncodesTPMState0(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		TPMState: mustQemuVMTPMStateValue(t, qemuVMTPMStateModel{
			Storage: types.StringValue("local-lvm"),
			Size:    types.StringValue("4M"),
			Format:  types.StringValue("raw"),
			Version: types.StringValue("v2.0"),
		}),
		Raw: mustQemuVMRawValue(t, qemuVMRawModel{
			ExtraConfig: mustStringMapValue(t, map[string]string{
				"hostpci0": "0000:00:1f.0",
			}),
		}),
	}

	createReq, diags := qemuVMCreateRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if got := createReq.ExtraConfig["tpmstate0"]; got != "local-lvm:4M,format=raw,version=v2.0" {
		t.Fatalf("unexpected tpmstate0 encoding: %#v", createReq.ExtraConfig)
	}
	if got := createReq.ExtraConfig["hostpci0"]; got != "0000:00:1f.0" {
		t.Fatalf("expected raw.extra_config entries to remain merged with tpmstate0, got %#v", createReq.ExtraConfig)
	}
}

func TestParseAndEncodeQemuVMEFIDisk(t *testing.T) {
	t.Parallel()

	parsed, ok := parseQemuVMEFIDisk("local-lvm:vm-101-disk-0,efitype=4m,format=raw,ms-cert=2011,pre-enrolled-keys=0,size=528K")
	if !ok {
		t.Fatal("expected efidisk0 config to parse")
	}
	if parsed.Storage.ValueString() != "local-lvm" || parsed.Volume.ValueString() != "local-lvm:vm-101-disk-0" || parsed.EFIType.ValueString() != "4m" || parsed.Format.ValueString() != "raw" || parsed.MSCert.ValueString() != "2011" || parsed.PreEnrolledKeys.ValueBool() || parsed.Size.ValueString() != "528K" {
		t.Fatalf("unexpected parsed efidisk0 config: %#v", parsed)
	}
	if encoded := encodeQemuVMEFIDisk(parsed); encoded != "local-lvm:vm-101-disk-0,efitype=4m,format=raw,ms-cert=2011,pre-enrolled-keys=0,size=528K" {
		t.Fatalf("unexpected encoded efidisk0 config: %q", encoded)
	}
}

func TestParseQemuVMEFIDiskRejectsUnsupportedGrammar(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"local-lvm:vm-101-disk-0,serial=efi",
		"local-lvm:vm-101-disk-0,pre-enrolled-keys=maybe",
	} {
		if _, ok := parseQemuVMEFIDisk(raw); ok {
			t.Fatalf("expected unsupported efidisk0 grammar %q to remain untyped", raw)
		}
	}
}

func TestParseAndEncodeQemuVMTPMState(t *testing.T) {
	t.Parallel()

	parsed, ok := parseQemuVMTPMState("local-lvm:vm-101-disk-9,format=raw,size=4M,version=v2.0")
	if !ok {
		t.Fatal("expected tpmstate0 config to parse")
	}
	if parsed.Storage.ValueString() != "local-lvm" || parsed.Volume.ValueString() != "local-lvm:vm-101-disk-9" || parsed.Format.ValueString() != "raw" || parsed.Size.ValueString() != "4M" || parsed.Version.ValueString() != "v2.0" {
		t.Fatalf("unexpected parsed tpmstate0 config: %#v", parsed)
	}
	if encoded := encodeQemuVMTPMState(parsed); encoded != "local-lvm:vm-101-disk-9,format=raw,size=4M,version=v2.0" {
		t.Fatalf("unexpected encoded tpmstate0 config: %q", encoded)
	}
}

func TestParseQemuVMTPMStateRejectsUnsupportedGrammar(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"local-lvm:vm-101-disk-9,pre-enrolled-keys=1",
		"local-lvm:vm-101-disk-9,version=",
	} {
		if _, ok := parseQemuVMTPMState(raw); ok {
			t.Fatalf("expected unsupported tpmstate0 grammar %q to remain untyped", raw)
		}
	}
}

func decodeQemuVMTPMState(t *testing.T, value types.Object) qemuVMTPMStateModel {
	t.Helper()
	if value.IsNull() || value.IsUnknown() {
		t.Fatalf("expected known tpm_state object, got %#v", value)
	}
	var result qemuVMTPMStateModel
	assertNoDiags(t, value.As(context.Background(), &result, qemuObjectAsOptions()))
	return result
}

func mustQemuVMTPMStateValue(t *testing.T, value qemuVMTPMStateModel) types.Object {
	t.Helper()
	result, diags := types.ObjectValueFrom(context.Background(), qemuVMTPMStateAttrTypes(), value)
	assertNoDiags(t, diags)
	return result
}

func decodeQemuVMEFIDisk(t *testing.T, value types.Object) qemuVMEFIDiskModel {
	t.Helper()
	if value.IsNull() || value.IsUnknown() {
		t.Fatalf("expected known efi_disk object, got %#v", value)
	}
	var result qemuVMEFIDiskModel
	assertNoDiags(t, value.As(context.Background(), &result, qemuObjectAsOptions()))
	return result
}

func mustQemuVMEFIDiskValue(t *testing.T, value qemuVMEFIDiskModel) types.Object {
	t.Helper()
	result, diags := types.ObjectValueFrom(context.Background(), qemuVMEFIDiskAttrTypes(), value)
	assertNoDiags(t, diags)
	return result
}

func decodeQemuVMVGA(t *testing.T, value types.Object) qemuVMVGAModel {
	t.Helper()
	if value.IsNull() || value.IsUnknown() {
		t.Fatalf("expected known vga object, got %#v", value)
	}
	var result qemuVMVGAModel
	assertNoDiags(t, value.As(context.Background(), &result, qemuObjectAsOptions()))
	return result
}

func mustQemuVMVGAValue(t *testing.T, value qemuVMVGAModel) types.Object {
	t.Helper()
	result, diags := types.ObjectValueFrom(context.Background(), qemuVMVGAAttrTypes(), value)
	assertNoDiags(t, diags)
	return result
}

func decodeQemuVMCommon(t *testing.T, value types.Object) qemuVMCommonModel {
	t.Helper()
	if value.IsNull() || value.IsUnknown() {
		t.Fatalf("expected known common object, got %#v", value)
	}
	var result qemuVMCommonModel
	assertNoDiags(t, value.As(context.Background(), &result, qemuObjectAsOptions()))
	return result
}

func decodeQemuVMCloudInit(t *testing.T, value types.Object) qemuVMCloudInitModel {
	t.Helper()
	if value.IsNull() || value.IsUnknown() {
		t.Fatalf("expected known cloud_init object, got %#v", value)
	}
	var result qemuVMCloudInitModel
	assertNoDiags(t, value.As(context.Background(), &result, qemuObjectAsOptions()))
	return result
}

func decodeQemuVMRaw(t *testing.T, value types.Object) qemuVMRawModel {
	t.Helper()
	if value.IsNull() || value.IsUnknown() {
		t.Fatalf("expected known raw object, got %#v", value)
	}
	var result qemuVMRawModel
	assertNoDiags(t, value.As(context.Background(), &result, qemuObjectAsOptions()))
	return result
}

func decodeQemuVMClone(t *testing.T, value types.Object) qemuVMCloneModel {
	t.Helper()
	if value.IsNull() || value.IsUnknown() {
		t.Fatalf("expected known clone object, got %#v", value)
	}
	var result qemuVMCloneModel
	assertNoDiags(t, value.As(context.Background(), &result, qemuObjectAsOptions()))
	return result
}

func decodeQemuVMIPConfigMap(t *testing.T, value types.Map) map[string]qemuVMIPConfigModel {
	t.Helper()
	var result map[string]qemuVMIPConfigModel
	assertNoDiags(t, value.ElementsAs(context.Background(), &result, false))
	return result
}

func decodeQemuVMNetworkMap(t *testing.T, value types.Map) map[string]qemuVMNetworkModel {
	t.Helper()
	var result map[string]qemuVMNetworkModel
	assertNoDiags(t, value.ElementsAs(context.Background(), &result, false))
	return result
}

func decodeQemuVMDiskMap(t *testing.T, value types.Map) map[string]qemuVMDiskModel {
	t.Helper()
	var result map[string]qemuVMDiskModel
	assertNoDiags(t, value.ElementsAs(context.Background(), &result, false))
	return result
}

func decodeStringMap(t *testing.T, value types.Map) map[string]string {
	t.Helper()
	var result map[string]string
	assertNoDiags(t, value.ElementsAs(context.Background(), &result, false))
	return result
}

func mustQemuVMCommonValue(t *testing.T, value qemuVMCommonModel) types.Object {
	t.Helper()
	result, diags := types.ObjectValueFrom(context.Background(), qemuVMCommonAttrTypes(), value)
	assertNoDiags(t, diags)
	return result
}

func mustQemuVMCloudInitValue(t *testing.T, value qemuVMCloudInitModel) types.Object {
	t.Helper()
	result, diags := types.ObjectValueFrom(context.Background(), qemuVMCloudInitAttrTypes(), value)
	assertNoDiags(t, diags)
	return result
}

func mustQemuVMRawValue(t *testing.T, value qemuVMRawModel) types.Object {
	t.Helper()
	result, diags := types.ObjectValueFrom(context.Background(), qemuVMRawAttrTypes(), value)
	assertNoDiags(t, diags)
	return result
}

func mustQemuVMCloneValue(t *testing.T, value qemuVMCloneModel) types.Object {
	t.Helper()
	result, diags := types.ObjectValueFrom(context.Background(), qemuVMCloneAttrTypes(), value)
	assertNoDiags(t, diags)
	return result
}

func mustQemuVMIPConfigMapValue(t *testing.T, value map[string]qemuVMIPConfigModel) types.Map {
	t.Helper()
	result, diags := types.MapValueFrom(context.Background(), types.ObjectType{AttrTypes: qemuVMIPConfigAttrTypes()}, value)
	assertNoDiags(t, diags)
	return result
}

func mustQemuVMNetworkMapValue(t *testing.T, value map[string]qemuVMNetworkModel) types.Map {
	t.Helper()
	result, diags := types.MapValueFrom(context.Background(), types.ObjectType{AttrTypes: qemuVMNetworkAttrTypes()}, value)
	assertNoDiags(t, diags)
	return result
}

func mustQemuVMDiskMapValue(t *testing.T, value map[string]qemuVMDiskModel) types.Map {
	t.Helper()
	result, diags := types.MapValueFrom(context.Background(), types.ObjectType{AttrTypes: qemuVMDiskAttrTypes()}, value)
	assertNoDiags(t, diags)
	return result
}

func mustStringMapValue(t *testing.T, value map[string]string) types.Map {
	t.Helper()
	result, diags := types.MapValueFrom(context.Background(), types.StringType, value)
	assertNoDiags(t, diags)
	return result
}

func assertNoDiags(t *testing.T, diags diag.Diagnostics) {
	t.Helper()
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestParseAndEncodeQemuVMVGA(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		raw    string
		parsed qemuVMVGAModel
	}{
		{name: "type only", raw: "std", parsed: qemuVMVGAModel{Type: types.StringValue("std")}},
		{name: "type and memory", raw: "std,memory=128", parsed: qemuVMVGAModel{Type: types.StringValue("std"), Memory: types.Int64Value(128)}},
		{name: "type memory clipboard", raw: "qxl,memory=256,clipboard=vnc", parsed: qemuVMVGAModel{Type: types.StringValue("qxl"), Memory: types.Int64Value(256), Clipboard: types.StringValue("vnc")}},
		{name: "serial terminal", raw: "serial0", parsed: qemuVMVGAModel{Type: types.StringValue("serial0")}},
		{name: "type and clipboard", raw: "virtio,clipboard=vnc", parsed: qemuVMVGAModel{Type: types.StringValue("virtio"), Clipboard: types.StringValue("vnc")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseQemuVMVGA(tc.raw)
			if !ok {
				t.Fatalf("parseQemuVMVGA(%q) expected ok, got false", tc.raw)
			}
			if got.Type != tc.parsed.Type || got.Memory != tc.parsed.Memory || got.Clipboard != tc.parsed.Clipboard {
				t.Fatalf("parseQemuVMVGA(%q) = %#v, want %#v", tc.raw, got, tc.parsed)
			}
			encoded := encodeQemuVMVGA(got)
			if encoded != tc.raw {
				t.Fatalf("encodeQemuVMVGA round-trip = %q, want %q", encoded, tc.raw)
			}
		})
	}
}

func TestParseQemuVMVGARejectsKeyedFirstSegment(t *testing.T) {
	t.Parallel()
	if _, ok := parseQemuVMVGA("memory=128"); ok {
		t.Fatal("expected keyed-only vga value (no positional type) to be unparseable so it stays raw")
	}
}

func TestParseQemuVMVGARejectsBadMemory(t *testing.T) {
	t.Parallel()
	if _, ok := parseQemuVMVGA("std,memory=big"); ok {
		t.Fatal("expected non-integer memory to be unparseable so it stays raw")
	}
}

func TestParseQemuVMVGARejectsUnknownKey(t *testing.T) {
	t.Parallel()
	if _, ok := parseQemuVMVGA("std,resolution=1024"); ok {
		t.Fatal("expected unknown vga key to be unparseable so it stays raw")
	}
}

func TestEncodeQemuVMVGARequiresType(t *testing.T) {
	t.Parallel()
	if encoded := encodeQemuVMVGA(qemuVMVGAModel{Memory: types.Int64Value(128)}); encoded != "" {
		t.Fatalf("expected empty vga encoding when type is null, got %q", encoded)
	}
}

func TestQemuVMStateFromAPIParsesVGA(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{
		Name: "api-vm",
		ExtraConfig: map[string]string{
			"vga":      "std,memory=128,clipboard=vnc",
			"hostpci0": "0000:00:1f.0",
		},
	}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	vga := decodeQemuVMVGA(t, state.VGA)
	if vga.Type.ValueString() != "std" || vga.Memory.ValueInt64() != 128 || vga.Clipboard.ValueString() != "vnc" {
		t.Fatalf("unexpected vga state: %#v", vga)
	}
	raw := decodeQemuVMRaw(t, state.Raw)
	gotRaw := decodeStringMap(t, raw.ExtraConfig)
	if _, ok := gotRaw["vga"]; ok {
		t.Fatalf("expected typed vga to be removed from raw.extra_config, got %#v", gotRaw)
	}
	if gotRaw["hostpci0"] != "0000:00:1f.0" {
		t.Fatalf("expected unrelated raw key preserved, got %#v", gotRaw)
	}
}

func TestQemuVMStateFromAPIUnparseableVGASaysRaw(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{
		Name:        "api-vm",
		ExtraConfig: map[string]string{"vga": "std,resolution=1024"},
	}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if !state.VGA.IsNull() {
		t.Fatalf("expected vga block null for unparseable value, got %#v", state.VGA)
	}
	raw := decodeQemuVMRaw(t, state.Raw)
	gotRaw := decodeStringMap(t, raw.ExtraConfig)
	if gotRaw["vga"] != "std,resolution=1024" {
		t.Fatalf("expected unparseable vga preserved in raw, got %#v", gotRaw)
	}
}

func TestQemuVMStateFromAPIAbsentVGANull(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{Name: "api-vm"}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if !state.VGA.IsNull() {
		t.Fatalf("expected absent vga block null, got %#v", state.VGA)
	}
}

func TestQemuVMRequestFromModelEncodesVGA(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		VMID: types.Int64Value(101),
		VGA:  mustQemuVMVGAValue(t, qemuVMVGAModel{Type: types.StringValue("virtio"), Memory: types.Int64Value(256)}),
	}
	createReq, diags := qemuVMCreateRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if got := createReq.ExtraConfig["vga"]; got != "virtio,memory=256" {
		t.Fatalf("expected vga encoded into extra_config, got %#v", createReq.ExtraConfig)
	}
}

func TestQemuVMRequestFromModelOmitsVGAWhenTypeNull(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		VMID: types.Int64Value(101),
		VGA:  mustQemuVMVGAValue(t, qemuVMVGAModel{Memory: types.Int64Value(128)}),
	}
	createReq, diags := qemuVMCreateRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if _, ok := createReq.ExtraConfig["vga"]; ok {
		t.Fatalf("expected no vga in extra_config when type null, got %#v", createReq.ExtraConfig)
	}
}

func TestValidateQemuVMRawConflictsReservesVGA(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		VGA: mustQemuVMVGAValue(t, qemuVMVGAModel{Type: types.StringValue("std")}),
		Raw: mustQemuVMRawValue(t, qemuVMRawModel{
			ExtraConfig: mustStringMapValue(t, map[string]string{"vga": "std"}),
		}),
	}
	diags := validateQemuVMRawConflicts(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected raw-vs-typed conflict diagnostics for vga")
	}
	if got := diags[0].Summary(); got != "Conflicting raw.extra_config entry" {
		t.Fatalf("unexpected diagnostic summary: %q", got)
	}
}

func TestQemuVMStateFromAPIParsesSerial(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{
		Name: "api-vm",
		Serial: map[string]string{
			"serial0": "socket",
			"serial1": "/dev/ttyS0",
		},
		ExtraConfig: map[string]string{"hostpci0": "0000:00:1f.0"},
	}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	serial := decodeStringMap(t, state.Serial)
	if serial["serial0"] != "socket" || serial["serial1"] != "/dev/ttyS0" {
		t.Fatalf("unexpected serial state: %#v", serial)
	}
	raw := decodeQemuVMRaw(t, state.Raw)
	gotRaw := decodeStringMap(t, raw.ExtraConfig)
	if gotRaw["hostpci0"] != "0000:00:1f.0" {
		t.Fatalf("expected unrelated raw key preserved, got %#v", gotRaw)
	}
}

func TestQemuVMStateFromAPIAbsentSerialNull(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{Name: "api-vm"}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if !state.Serial.IsNull() {
		t.Fatalf("expected absent serial map null, got %#v", state.Serial)
	}
}

func TestQemuVMRequestFromModelEncodesSerial(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		VMID:   types.Int64Value(101),
		Serial: mustStringMapValue(t, map[string]string{"serial0": "socket"}),
	}
	createReq, diags := qemuVMCreateRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if got := createReq.Serial["serial0"]; got != "socket" {
		t.Fatalf("expected serial0 in create request, got %#v", createReq.Serial)
	}
}

func TestValidateQemuVMRawConflictsReservesSerial(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		Serial: mustStringMapValue(t, map[string]string{"serial0": "socket"}),
		Raw: mustQemuVMRawValue(t, qemuVMRawModel{
			ExtraConfig: mustStringMapValue(t, map[string]string{"serial0": "socket"}),
		}),
	}
	diags := validateQemuVMRawConflicts(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected raw-vs-typed conflict diagnostics for serial0")
	}
	if got := diags[0].Summary(); got != "Conflicting raw.extra_config entry" {
		t.Fatalf("unexpected diagnostic summary: %q", got)
	}
}

func TestQemuVMStateFromAPIDefaultsCPUFieldsOmitted(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{Name: "api-vm"}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if !state.NUMA.IsNull() || state.NUMA.IsUnknown() {
		t.Fatalf("expected omitted numa to read as null, got %#v", state.NUMA)
	}
	if !state.VCPUs.IsNull() {
		t.Fatalf("expected omitted vcpus to read as null, got %#v", state.VCPUs)
	}
	if !state.CPUUnits.IsNull() {
		t.Fatalf("expected omitted cpuunits to read as null, got %#v", state.CPUUnits)
	}
	if !state.CPULimit.IsNull() {
		t.Fatalf("expected omitted cpulimit to read as null, got %#v", state.CPULimit)
	}
}

func TestQemuVMRequestFromModelMapsCPUFields(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		VMID:     types.Int64Value(101),
		NUMA:     types.BoolValue(true),
		VCPUs:    types.Int64Value(4),
		CPUUnits: types.Int64Value(2048),
		CPULimit: types.Float64Value(2.0),
	}
	createReq, diags := qemuVMCreateRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if createReq.NUMA == nil || !*createReq.NUMA {
		t.Fatalf("expected numa=true in create request, got %#v", createReq.NUMA)
	}
	if createReq.VCPUs == nil || *createReq.VCPUs != 4 {
		t.Fatalf("expected vcpus=4 in create request, got %#v", createReq.VCPUs)
	}
	if createReq.CPUUnits == nil || *createReq.CPUUnits != 2048 {
		t.Fatalf("expected cpuunits=2048 in create request, got %#v", createReq.CPUUnits)
	}
	if createReq.CPULimit == nil || *createReq.CPULimit != 2.0 {
		t.Fatalf("expected cpulimit=2.0 in create request, got %#v", createReq.CPULimit)
	}
}

func TestValidateQemuVMRawConflictsReservesCPUFields(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"numa", "vcpus", "cpuunits", "cpulimit"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			model := qemuVMModel{
				Raw: mustQemuVMRawValue(t, qemuVMRawModel{
					ExtraConfig: mustStringMapValue(t, map[string]string{key: "1"}),
				}),
			}
			diags := validateQemuVMRawConflicts(context.Background(), model)
			if !diags.HasError() {
				t.Fatalf("expected raw-vs-typed conflict diagnostics for %s", key)
			}
			if got := diags[0].Summary(); got != "Conflicting raw.extra_config entry" {
				t.Fatalf("unexpected diagnostic summary: %q", got)
			}
		})
	}
}

func TestQemuVMStateFromAPIDefaultsMemoryFieldsOmitted(t *testing.T) {
	t.Parallel()

	state, diags := qemuVMStateFromAPI(context.Background(), "pve-1", 101, QemuVMConfig{Name: "api-vm"}, QemuVMStatus{}, nil)
	if diags.HasError() {
		t.Fatalf("qemuVMStateFromAPI() unexpected diagnostics: %v", diags)
	}
	if !state.Balloon.IsNull() {
		t.Fatalf("expected omitted balloon to read as null, got %#v", state.Balloon)
	}
	if !state.Shares.IsNull() {
		t.Fatalf("expected omitted shares to read as null, got %#v", state.Shares)
	}
	if !state.Hugepages.IsNull() {
		t.Fatalf("expected omitted hugepages to read as null, got %#v", state.Hugepages)
	}
}

func TestQemuVMRequestFromModelMapsMemoryFields(t *testing.T) {
	t.Parallel()

	model := qemuVMModel{
		VMID:      types.Int64Value(101),
		Balloon:   types.Int64Value(1024),
		Shares:    types.Int64Value(500),
		Hugepages: types.StringValue("1024"),
	}
	createReq, diags := qemuVMCreateRequestFromModel(context.Background(), model)
	assertNoDiags(t, diags)
	if createReq.Balloon == nil || *createReq.Balloon != 1024 {
		t.Fatalf("expected balloon=1024 in create request, got %#v", createReq.Balloon)
	}
	if createReq.Shares == nil || *createReq.Shares != 500 {
		t.Fatalf("expected shares=500 in create request, got %#v", createReq.Shares)
	}
	if createReq.Hugepages == nil || *createReq.Hugepages != "1024" {
		t.Fatalf("expected hugepages=1024 in create request, got %#v", createReq.Hugepages)
	}
}

func TestValidateQemuVMRawConflictsReservesMemoryFields(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"balloon", "shares", "hugepages"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			model := qemuVMModel{
				Raw: mustQemuVMRawValue(t, qemuVMRawModel{
					ExtraConfig: mustStringMapValue(t, map[string]string{key: "2"}),
				}),
			}
			diags := validateQemuVMRawConflicts(context.Background(), model)
			if !diags.HasError() {
				t.Fatalf("expected raw-vs-typed conflict diagnostics for %s", key)
			}
			if got := diags[0].Summary(); got != "Conflicting raw.extra_config entry" {
				t.Fatalf("unexpected diagnostic summary: %q", got)
			}
		})
	}
}
