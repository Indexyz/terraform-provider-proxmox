// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"testing"
)

func TestNodeRRDDataDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/rrddata?cf=MAX&timeframe=hour": []map[string]any{
			{"time": 10, "cpu": 0.1},
			{"time": 20, "cpu": 0.2},
		},
	}
	readResp := runSDNDataSource(t, NewNodeRRDDataDataSource(), responses, map[string]any{"node": "pve-1", "timeframe": "hour", "cf": "MAX"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state nodeRRDDataModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if !strings.Contains(state.Data.ValueString(), `"cpu":0.2`) || state.CF.ValueString() != "MAX" {
		t.Fatalf("unexpected data: %#v", state)
	}
}

func TestQemuRRDDataDataSource(t *testing.T) {
	responses := map[string]any{
		"/api2/json/nodes/pve-1/qemu/320/rrddata?timeframe=day": []map[string]any{{"time": 10, "mem": 512}},
	}
	readResp := runSDNDataSource(t, NewQemuRRDDataDataSource(), responses, map[string]any{"node": "pve-1", "vmid": int64(320), "timeframe": "day"})
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var state qemuRRDDataModel
	if diags := readResp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if !strings.Contains(state.Data.ValueString(), `"mem":512`) || state.VMID.ValueInt64() != 320 {
		t.Fatalf("unexpected data: %#v", state)
	}
}
