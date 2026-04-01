package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func clientFromProviderData(data any) (*Client, error) {
	client, ok := data.(*Client)
	if !ok {
		return nil, fmt.Errorf("expected provider data type *provider.Client, got %T", data)
	}
	return client, nil
}

func stringOrNull(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

func boolOrNull(value *bool) types.Bool {
	if value == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*value)
}

func int64OrNull(value *int64) types.Int64 {
	if value == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*value)
}

func float64OrNull(value *float64) types.Float64 {
	if value == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*value)
}

func stringListValue(ctx context.Context, values []string) (types.List, diag.Diagnostics) {
	if values == nil {
		values = []string{}
	}
	return types.ListValueFrom(ctx, types.StringType, values)
}

func int64SetValue(ctx context.Context, values []int64) (types.Set, diag.Diagnostics) {
	if values == nil {
		values = []int64{}
	}
	return types.SetValueFrom(ctx, types.Int64Type, sortedInt64s(values))
}

func stringSetValue(ctx context.Context, values []string) (types.Set, diag.Diagnostics) {
	if values == nil {
		values = []string{}
	}
	return types.SetValueFrom(ctx, types.StringType, sortedStrings(values))
}

func expandInt64Set(ctx context.Context, value types.Set) ([]int64, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}

	var result []int64
	diags := value.ElementsAs(ctx, &result, false)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, diags
}

func expandStringSet(ctx context.Context, value types.Set) ([]string, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}

	var result []string
	diags := value.ElementsAs(ctx, &result, false)
	sort.Strings(result)
	return result, diags
}

func valuesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func int64ValuesEqual(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func diffStrings(current, desired []string) (add []string, remove []string) {
	currentSet := make(map[string]struct{}, len(current))
	desiredSet := make(map[string]struct{}, len(desired))

	for _, value := range current {
		currentSet[value] = struct{}{}
	}
	for _, value := range desired {
		desiredSet[value] = struct{}{}
		if _, exists := currentSet[value]; !exists {
			add = append(add, value)
		}
	}
	for _, value := range current {
		if _, exists := desiredSet[value]; !exists {
			remove = append(remove, value)
		}
	}

	return sortedStrings(add), sortedStrings(remove)
}

func diffInt64s(current, desired []int64) (add []int64, remove []int64) {
	currentSet := make(map[int64]struct{}, len(current))
	desiredSet := make(map[int64]struct{}, len(desired))

	for _, value := range current {
		currentSet[value] = struct{}{}
	}
	for _, value := range desired {
		desiredSet[value] = struct{}{}
		if _, exists := currentSet[value]; !exists {
			add = append(add, value)
		}
	}
	for _, value := range current {
		if _, exists := desiredSet[value]; !exists {
			remove = append(remove, value)
		}
	}

	return sortedInt64s(add), sortedInt64s(remove)
}
