package provider

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestExpandSetHelpers(t *testing.T) {
	t.Parallel()

	intSet, diags := types.SetValueFrom(context.Background(), types.Int64Type, []int64{9, 1, 5})
	if diags.HasError() {
		t.Fatalf("types.SetValueFrom(int) unexpected diagnostics: %v", diags)
	}
	gotInts, diags := expandInt64Set(context.Background(), intSet)
	if diags.HasError() {
		t.Fatalf("expandInt64Set() unexpected diagnostics: %v", diags)
	}
	if want := []int64{1, 5, 9}; !reflect.DeepEqual(gotInts, want) {
		t.Fatalf("unexpected expanded ints: got %v want %v", gotInts, want)
	}

	stringSet, diags := types.SetValueFrom(context.Background(), types.StringType, []string{"zeta", "alpha", "beta"})
	if diags.HasError() {
		t.Fatalf("types.SetValueFrom(string) unexpected diagnostics: %v", diags)
	}
	gotStrings, diags := expandStringSet(context.Background(), stringSet)
	if diags.HasError() {
		t.Fatalf("expandStringSet() unexpected diagnostics: %v", diags)
	}
	if want := []string{"alpha", "beta", "zeta"}; !reflect.DeepEqual(gotStrings, want) {
		t.Fatalf("unexpected expanded strings: got %v want %v", gotStrings, want)
	}

	gotNilInts, diags := expandInt64Set(context.Background(), types.SetNull(types.Int64Type))
	if diags.HasError() {
		t.Fatalf("expandInt64Set(null) unexpected diagnostics: %v", diags)
	}
	if gotNilInts != nil {
		t.Fatalf("expected nil ints for null set, got %v", gotNilInts)
	}

	gotNilStrings, diags := expandStringSet(context.Background(), types.SetNull(types.StringType))
	if diags.HasError() {
		t.Fatalf("expandStringSet(null) unexpected diagnostics: %v", diags)
	}
	if gotNilStrings != nil {
		t.Fatalf("expected nil strings for null set, got %v", gotNilStrings)
	}
}

func TestDiffHelpers(t *testing.T) {
	t.Parallel()

	addStrings, removeStrings := diffStrings(
		[]string{"b", "a", "shared"},
		[]string{"c", "shared", "a"},
	)
	if want := []string{"c"}; !reflect.DeepEqual(addStrings, want) {
		t.Fatalf("unexpected string additions: got %v want %v", addStrings, want)
	}
	if want := []string{"b"}; !reflect.DeepEqual(removeStrings, want) {
		t.Fatalf("unexpected string removals: got %v want %v", removeStrings, want)
	}

	addInts, removeInts := diffInt64s(
		[]int64{3, 1, 2},
		[]int64{2, 4, 1},
	)
	if want := []int64{4}; !reflect.DeepEqual(addInts, want) {
		t.Fatalf("unexpected int additions: got %v want %v", addInts, want)
	}
	if want := []int64{3}; !reflect.DeepEqual(removeInts, want) {
		t.Fatalf("unexpected int removals: got %v want %v", removeInts, want)
	}
}

func TestFlattenPoolMembers(t *testing.T) {
	t.Parallel()

	pool := Pool{
		Members: []PoolMember{
			{ID: "qemu/300", Node: "pve-3", Type: "qemu", VMID: int64Pointer(300)},
			{ID: "storage/ceph", Node: "pve-2", Type: "storage", Storage: "ceph"},
			{ID: "lxc/101", Node: "pve-1", Type: "lxc", VMID: int64Pointer(101)},
		},
	}

	vmIDs, storageIDs, members := flattenPoolMembers(pool)
	if want := []int64{101, 300}; !reflect.DeepEqual(vmIDs, want) {
		t.Fatalf("unexpected flattened vm ids: got %v want %v", vmIDs, want)
	}
	if want := []string{"ceph"}; !reflect.DeepEqual(storageIDs, want) {
		t.Fatalf("unexpected flattened storage ids: got %v want %v", storageIDs, want)
	}
	if len(members) != 3 {
		t.Fatalf("unexpected member count: %d", len(members))
	}
	if members[0].ID.ValueString() != "qemu/300" || members[1].StorageID.ValueString() != "ceph" {
		t.Fatalf("unexpected member projection: %#v", members)
	}
}

func TestValueConversionHelpers(t *testing.T) {
	t.Parallel()

	if got := stringPointer(types.StringNull()); got != nil {
		t.Fatalf("expected nil pointer for null string, got %v", *got)
	}
	if got := stringPointer(types.StringValue("ops")); got == nil || *got != "ops" {
		t.Fatalf("unexpected pointer conversion result: %v", got)
	}

	if boolValueFromType(types.BoolNull()) {
		t.Fatal("expected false for null bool")
	}
	if !boolValueFromType(types.BoolValue(true)) {
		t.Fatal("expected true for bool value")
	}

	if !commentChanged(types.StringNull(), "existing") {
		t.Fatal("expected null desired comment to differ from existing comment")
	}
	if commentChanged(types.StringValue("same"), "same") {
		t.Fatal("expected identical comments to be unchanged")
	}
	if !commentChanged(types.StringValue("next"), "prev") {
		t.Fatal("expected changed comments to be detected")
	}
}
