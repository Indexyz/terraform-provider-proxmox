// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func clusterMappingSet(t *testing.T, kind string, entries ...map[string]attr.Value) types.Set {
	t.Helper()
	objectType := types.ObjectType{AttrTypes: clusterMappingAttrTypes(kind)}
	values := make([]attr.Value, 0, len(entries))
	for _, entry := range entries {
		values = append(values, types.ObjectValueMust(objectType.AttrTypes, entry))
	}
	return types.SetValueMust(objectType, values)
}

func TestClientClusterMappingForms(t *testing.T) {
	ctx := context.Background()
	var created, updated url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/cluster/mapping/pci" && r.Method == http.MethodPost:
			_ = r.ParseForm()
			created = r.PostForm
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/mapping/pci/gpu" && r.Method == http.MethodPut:
			_ = r.ParseForm()
			updated = r.PostForm
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/mapping/pci" && r.Method == http.MethodGet:
			writeEnvelope(t, w, []map[string]any{{"id": "gpu", "map": []string{"node=pve-1,path=0000:01:00.0,id=10de:1eb8"}, "digest": "d1"}})
		case r.URL.Path == "/api2/json/cluster/mapping/pci/gpu" && r.Method == http.MethodDelete:
			assertFormValues(t, r, url.Values{})
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)

	entry := ClusterMapping{Map: []string{"node=pve-1,path=0000:01:00.0,id=10de:1eb8"}, Description: "gpu"}
	if err := client.CreateClusterMapping(ctx, "pci", "gpu", entry); err != nil {
		t.Fatalf("CreateClusterMapping() unexpected error: %v", err)
	}
	if got := created["map"]; len(got) != 1 || got[0] != "node=pve-1,path=0000:01:00.0,id=10de:1eb8" {
		t.Fatalf("unexpected map form: %#v", created)
	}
	if got := created.Get("description"); got != "gpu" {
		t.Fatalf("unexpected description: %q", got)
	}
	current, err := client.GetClusterMapping(ctx, "pci", "gpu")
	if err != nil || current.Digest != "d1" || len(current.Map) != 1 {
		t.Fatalf("unexpected mapping: %#v (%v)", current, err)
	}
	current.Map = []string{"node=pve-1,path=0000:02:00.0,id=10de:1eb8"}
	if err := client.UpdateClusterMapping(ctx, "pci", "gpu", current, []string{"description"}); err != nil {
		t.Fatalf("UpdateClusterMapping() unexpected error: %v", err)
	}
	if updated.Get("digest") != "d1" || updated.Get("delete") != "description" {
		t.Fatalf("unexpected update form: %#v", updated)
	}
	if err := client.DeleteClusterMapping(ctx, "pci", "gpu"); err != nil {
		t.Fatalf("DeleteClusterMapping() unexpected error: %v", err)
	}
}

func TestClusterMappingEntryEncoding(t *testing.T) {
	encoded := encodeClusterMappingEntry(map[string]string{"node": "pve-1", "path": "0000:01:00.0", "id": "10de:1eb8", "iommugroup": "12", "description": "gpu"})
	if encoded != "description=gpu,id=10de:1eb8,iommugroup=12,node=pve-1,path=0000:01:00.0" {
		t.Fatalf("unexpected encoding: %q", encoded)
	}
	state, diags := clusterMappingStateMap("pci", []string{encoded})
	if diags.HasError() || len(state.Elements()) != 1 {
		t.Fatalf("unexpected decode: %#v (%v)", state, diags)
	}
}

func TestClusterMappingValidation(t *testing.T) {
	valid := clusterMappingModel{
		ID:  types.StringValue("gpu"),
		Map: clusterMappingSet(t, "pci", map[string]attr.Value{"node": types.StringValue("pve-1"), "path": types.StringValue("0000:01:00.0"), "id": types.StringValue("10de:1eb8"), "subsystem_id": types.StringNull(), "iommugroup": types.Int64Null(), "description": types.StringNull()}),
	}
	if diags := validateClusterMapping("pci", valid); diags.HasError() {
		t.Fatalf("valid mapping diagnostics: %v", diags)
	}
	invalid := map[string]struct {
		kind   string
		config clusterMappingModel
	}{
		"bad id":       {"pci", clusterMappingModel{ID: types.StringValue("1bad"), Map: valid.Map}},
		"bad node":     {"pci", clusterMappingModel{ID: types.StringValue("gpu"), Map: clusterMappingSet(t, "pci", map[string]attr.Value{"node": types.StringValue("pve_1"), "path": types.StringValue("0000:01:00.0"), "id": types.StringValue("10de:1eb8"), "subsystem_id": types.StringNull(), "iommugroup": types.Int64Null(), "description": types.StringNull()})}},
		"bad pci path": {"pci", clusterMappingModel{ID: types.StringValue("gpu"), Map: clusterMappingSet(t, "pci", map[string]attr.Value{"node": types.StringValue("pve-1"), "path": types.StringValue("01:00"), "id": types.StringValue("10de:1eb8"), "subsystem_id": types.StringNull(), "iommugroup": types.Int64Null(), "description": types.StringNull()})}},
		"bad pci id":   {"pci", clusterMappingModel{ID: types.StringValue("gpu"), Map: clusterMappingSet(t, "pci", map[string]attr.Value{"node": types.StringValue("pve-1"), "path": types.StringValue("0000:01:00.0"), "id": types.StringValue("10de"), "subsystem_id": types.StringNull(), "iommugroup": types.Int64Null(), "description": types.StringNull()})}},
		"duplicate node": {"pci", clusterMappingModel{ID: types.StringValue("gpu"), Map: clusterMappingSet(t, "pci",
			map[string]attr.Value{"node": types.StringValue("pve-1"), "path": types.StringValue("0000:01:00.0"), "id": types.StringValue("10de:1eb8"), "subsystem_id": types.StringNull(), "iommugroup": types.Int64Null(), "description": types.StringNull()},
			map[string]attr.Value{"node": types.StringValue("pve-1"), "path": types.StringValue("0000:02:00.0"), "id": types.StringValue("10de:1eb8"), "subsystem_id": types.StringNull(), "iommugroup": types.Int64Null(), "description": types.StringNull()})}},
		"usb without path or id": {"usb", clusterMappingModel{ID: types.StringValue("stick"), Map: clusterMappingSet(t, "usb", map[string]attr.Value{"node": types.StringValue("pve-1"), "path": types.StringValue(""), "id": types.StringNull()})}},
		"relative dir":           {"dir", clusterMappingModel{ID: types.StringValue("share"), Map: clusterMappingSet(t, "dir", map[string]attr.Value{"node": types.StringValue("pve-1"), "path": types.StringValue("srv/share")})}},
	}
	for name, testCase := range invalid {
		if diags := validateClusterMapping(testCase.kind, testCase.config); !diags.HasError() {
			t.Fatalf("%s: expected diagnostics", name)
		}
	}
	if diags := validateClusterMapping("pci", clusterMappingModel{ID: types.StringUnknown(), Map: types.SetUnknown(types.ObjectType{AttrTypes: clusterMappingAttrTypes("pci")})}); diags.HasError() {
		t.Fatalf("unknown values must defer: %v", diags)
	}
}

func TestClusterMappingLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	entries := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if err := r.ParseForm(); err != nil {
			handler.fail(w, "parse form: %v", err)
			return
		}
		switch {
		case r.URL.Path == "/api2/json/cluster/mapping/usb" && r.Method == http.MethodGet:
			list := []map[string]any{}
			for _, entry := range entries {
				list = append(list, entry)
			}
			handler.envelope(w, list)
		case r.URL.Path == "/api2/json/cluster/mapping/usb" && r.Method == http.MethodPost:
			entries["stick"] = map[string]any{"id": "stick", "map": r.Form["map"], "digest": "d1"}
			handler.envelope(w, nil)
		case r.URL.Path == "/api2/json/cluster/mapping/usb/stick" && r.Method == http.MethodPut:
			entries["stick"]["map"] = r.Form["map"]
			handler.envelope(w, nil)
		case r.URL.Path == "/api2/json/cluster/mapping/usb/stick" && r.Method == http.MethodDelete:
			delete(entries, "stick")
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &ClusterMappingUSBResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := clusterMappingModel{
		ID:  types.StringValue("stick"),
		Map: clusterMappingSet(t, "usb", map[string]attr.Value{"node": types.StringValue("pve-1"), "path": types.StringValue("1-2"), "id": types.StringNull()}),
	}
	createResp := resource.CreateResponse{State: testResourceState(t, schema, clusterMappingModel{ID: types.StringNull(), Map: types.SetNull(types.ObjectType{AttrTypes: clusterMappingAttrTypes("usb")}), Description: types.StringNull(), LiveMigration: types.BoolNull()})}
	initializeResourcePrivate(t, &createResp)
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	if got := entries["stick"]["map"]; got == nil {
		t.Fatalf("mapping was not created: %#v", entries)
	} else if list, ok := got.([]string); !ok || len(list) != 1 || list[0] != "node=pve-1,path=1-2" {
		t.Fatalf("unexpected map wire value: %#v", got)
	}
	var created clusterMappingModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() || created.ID.ValueString() != "stick" {
		t.Fatalf("unexpected created state: %#v (%v)", created, diags)
	}
	deleteResp := resource.DeleteResponse{State: createResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: createResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() || len(entries) != 0 {
		t.Fatalf("delete failed: %v %#v", deleteResp.Diagnostics, entries)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

func TestClusterMappingImportAndSchema(t *testing.T) {
	for index, res := range []resource.Resource{NewClusterMappingPCIResource(), NewClusterMappingUSBResource(), NewClusterMappingDirResource()} {
		kind := []string{clusterMappingKindPCI, clusterMappingKindUSB, clusterMappingKindDir}[index]
		schemaResp := testResourceSchema(t, res)
		importer, ok := res.(resource.ResourceWithImportState)
		if !ok {
			t.Fatalf("%T lacks import", res)
		}
		var resp resource.ImportStateResponse
		resp.State = testResourceState(t, schemaResp, clusterMappingModel{ID: types.StringNull(), Description: types.StringNull(), Map: types.SetNull(types.ObjectType{AttrTypes: clusterMappingAttrTypes(kind)}), LiveMigration: types.BoolNull()})
		importer.ImportState(context.Background(), resource.ImportStateRequest{ID: "gpu"}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("import diagnostics: %v", resp.Diagnostics)
		}
		var state clusterMappingModel
		if diags := resp.State.Get(context.Background(), &state); diags.HasError() || state.ID.ValueString() != "gpu" {
			t.Fatalf("unexpected import state: %#v (%v)", state, diags)
		}
		id, ok := schemaResp.Schema.Attributes["id"].(resourceschema.StringAttribute)
		if !ok || !id.IsRequired() || len(id.PlanModifiers) == 0 {
			t.Fatal("id must be required and replace")
		}
	}
	bad := resource.ImportStateResponse{}
	badImporter, ok := NewClusterMappingPCIResource().(resource.ResourceWithImportState)
	if !ok {
		t.Fatal("missing importer")
	}
	badImporter.ImportState(context.Background(), resource.ImportStateRequest{ID: "1bad"}, &bad)
	if !bad.Diagnostics.HasError() {
		t.Fatal("invalid import id accepted")
	}
	_ = strings.TrimSpace
}
