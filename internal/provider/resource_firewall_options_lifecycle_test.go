// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestClusterFirewallOptionsResourceLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	options := map[string]any{}
	var forms []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.EscapedPath() != "/api2/json/cluster/firewall/options" {
			handler.fail(w, "unexpected cluster options path: %s", r.URL.EscapedPath())
			return
		}
		switch r.Method {
		case http.MethodGet:
			handler.envelope(w, options)
		case http.MethodPut:
			if err := r.ParseForm(); err != nil {
				handler.fail(w, "parse cluster options: %v", err)
				return
			}
			forms = append(forms, r.Form.Encode())
			for key, values := range r.Form {
				if key != "delete" {
					options[key] = values[0]
				}
			}
			for _, key := range splitProxmoxList(r.Form.Get("delete")) {
				delete(options, key)
			}
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected cluster options method: %s", r.Method)
		}
	}))
	defer server.Close()
	res := &ClusterFirewallOptionsResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := clusterFirewallOptionsModel{Enable: types.BoolValue(false), Ebtables: types.BoolValue(false), LogRateLimit: types.StringValue("enable=1,burst=0"), PolicyIn: types.StringValue("DROP")}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("cluster options create: %v", createResp.Diagnostics)
	}
	var created clusterFirewallOptionsModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() || created.Enable.ValueBool() || created.Ebtables.ValueBool() {
		t.Fatalf("explicit false options lost: %#v diagnostics=%v", created, diags)
	}

	updated := initial
	updated.Enable = types.BoolValue(true)
	updated.Ebtables = types.BoolNull()
	updated.LogRateLimit = types.StringNull()
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Config: testResourceConfig(t, schema, updated), State: createResp.State}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("cluster options update: %v", updateResp.Diagnostics)
	}
	assertStateString(t, updateResp.State, path.Root("policy_in"), "DROP")
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("cluster options delete: %v", deleteResp.Diagnostics)
	}
	handler.assert(t)
	want := []string{
		"ebtables=0&enable=0&log_ratelimit=enable%3D1%2Cburst%3D0&policy_in=DROP",
		"delete=ebtables%2Clog_ratelimit&enable=1&policy_in=DROP",
		"delete=ebtables%2Cenable%2Clog_ratelimit%2Cpolicy_forward%2Cpolicy_in%2Cpolicy_out",
	}
	if !reflect.DeepEqual(forms, want) {
		t.Fatalf("unexpected cluster option forms: got %v want %v", forms, want)
	}
	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, clusterFirewallOptionsModel{ID: types.StringNull()})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "cluster"}, &importResp)
	assertStateString(t, importResp.State, path.Root("id"), "cluster")
}

func TestNodeFirewallOptionsResourceLifecycle(t *testing.T) {
	handler := &lifecycleHandler{}
	options := map[string]any{}
	var forms []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.EscapedPath() != "/api2/json/nodes/pve%20one/firewall/options" {
			handler.fail(w, "unexpected node options path: %s", r.URL.EscapedPath())
			return
		}
		switch r.Method {
		case http.MethodGet:
			handler.envelope(w, options)
		case http.MethodPut:
			if err := r.ParseForm(); err != nil {
				handler.fail(w, "parse node options: %v", err)
				return
			}
			forms = append(forms, r.Form.Encode())
			for key, values := range r.Form {
				if key != "delete" {
					options[key] = values[0]
				}
			}
			for _, key := range splitProxmoxList(r.Form.Get("delete")) {
				delete(options, key)
			}
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected node options method: %s", r.Method)
		}
	}))
	defer server.Close()
	res := &NodeFirewallOptionsResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := nodeFirewallOptionsModel{Node: types.StringValue("pve one"), Enable: types.BoolValue(false), NFConntrackMax: types.Int64Value(0), LogLevelIn: types.StringValue("info"), TCPFlags: types.BoolValue(false), Nftables: types.BoolValue(false)}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("node options create: %v", createResp.Diagnostics)
	}
	var created nodeFirewallOptionsModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() || created.Enable.ValueBool() || created.NFConntrackMax.ValueInt64() != 0 || created.TCPFlags.ValueBool() {
		t.Fatalf("explicit false/zero node options lost: %#v diagnostics=%v", created, diags)
	}
	updated := initial
	updated.Enable = types.BoolValue(true)
	updated.LogLevelIn = types.StringNull()
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, updated), State: createResp.State}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("node options update: %v", updateResp.Diagnostics)
	}
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("node options delete: %v", deleteResp.Diagnostics)
	}
	handler.assert(t)
	if forms[0] != "enable=0&log_level_in=info&nf_conntrack_max=0&nftables=0&tcpflags=0" || forms[1] != "delete=log_level_in&enable=1&nf_conntrack_max=0&nftables=0&tcpflags=0" {
		t.Fatalf("unexpected node create/update forms: %v", forms[:2])
	}
	reset := "delete=enable%2Clog_level_forward%2Clog_level_in%2Clog_level_out%2Clog_nf_conntrack%2Cndp%2Cnf_conntrack_allow_invalid%2Cnf_conntrack_helpers%2Cnf_conntrack_max%2Cnf_conntrack_tcp_timeout_established%2Cnf_conntrack_tcp_timeout_syn_recv%2Cnftables%2Cnosmurfs%2Cprotection_synflood%2Cprotection_synflood_burst%2Cprotection_synflood_rate%2Csmurf_log_level%2Ctcp_flags_log_level%2Ctcpflags"
	if forms[2] != reset {
		t.Fatalf("unexpected node reset form: got %s want %s", forms[2], reset)
	}
	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, nodeFirewallOptionsModel{ID: types.StringNull(), Node: types.StringNull()})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "pve one"}, &importResp)
	assertStateString(t, importResp.State, path.Root("node"), "pve one")
}

func TestGuestFirewallOptionsResourceLifecycleAndValidation(t *testing.T) {
	handler := &lifecycleHandler{}
	options := map[string]any{}
	var forms []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if r.URL.EscapedPath() != "/api2/json/nodes/pve%20one/qemu/101/firewall/options" {
			handler.fail(w, "unexpected guest options path: %s", r.URL.EscapedPath())
			return
		}
		switch r.Method {
		case http.MethodGet:
			handler.envelope(w, options)
		case http.MethodPut:
			if err := r.ParseForm(); err != nil {
				handler.fail(w, "parse guest options: %v", err)
				return
			}
			forms = append(forms, r.Form.Encode())
			for key, values := range r.Form {
				if key != "delete" {
					options[key] = values[0]
				}
			}
			for _, key := range splitProxmoxList(r.Form.Get("delete")) {
				delete(options, key)
			}
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected guest options method: %s", r.Method)
		}
	}))
	defer server.Close()
	res := &GuestFirewallOptionsResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	initial := guestFirewallOptionsModel{Node: types.StringValue("pve one"), VMID: types.Int64Value(101), GuestType: types.StringValue("qemu"), Enable: types.BoolValue(false), DHCP: types.BoolValue(false), IPFilter: types.BoolValue(false), MACFilter: types.BoolValue(false), PolicyIn: types.StringValue("DROP"), NDP: types.BoolValue(false), RADV: types.BoolValue(false)}
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, initial)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("guest options create: %v", createResp.Diagnostics)
	}
	var created guestFirewallOptionsModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() || created.Enable.ValueBool() || created.DHCP.ValueBool() || created.MACFilter.ValueBool() {
		t.Fatalf("explicit false guest options lost: %#v diagnostics=%v", created, diags)
	}
	updated := initial
	updated.Enable = types.BoolValue(true)
	updated.PolicyIn = types.StringValue("ACCEPT")
	updateResp := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Update(context.Background(), resource.UpdateRequest{Plan: testResourcePlan(t, schema, updated), State: createResp.State}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("guest options update: %v", updateResp.Diagnostics)
	}
	assertStateString(t, updateResp.State, path.Root("policy_in"), "ACCEPT")
	readResp := resource.ReadResponse{State: updateResp.State}
	res.Read(context.Background(), resource.ReadRequest{State: updateResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("guest options read: %v", readResp.Diagnostics)
	}
	var deleteResp resource.DeleteResponse
	res.Delete(context.Background(), resource.DeleteRequest{State: readResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("guest options delete: %v", deleteResp.Diagnostics)
	}
	handler.assert(t)
	wantCreate := "dhcp=0&enable=0&ipfilter=0&macfilter=0&ndp=0&policy_in=DROP&radv=0"
	wantUpdate := "dhcp=0&enable=1&ipfilter=0&macfilter=0&ndp=0&policy_in=ACCEPT&radv=0"
	wantReset := "delete=dhcp%2Cenable%2Cipfilter%2Clog_level_in%2Clog_level_out%2Cmacfilter%2Cndp%2Cpolicy_in%2Cpolicy_out%2Cradv"
	if !reflect.DeepEqual(forms, []string{wantCreate, wantUpdate, wantReset}) {
		t.Fatalf("unexpected guest option forms: got %v", forms)
	}
	importResp := resource.ImportStateResponse{State: testResourceState(t, schema, guestFirewallOptionsModel{ID: types.StringNull(), Node: types.StringNull(), VMID: types.Int64Null(), GuestType: types.StringNull()})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "pve one/202/lxc"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("guest options import: %v", importResp.Diagnostics)
	}
	assertStateString(t, importResp.State, path.Root("guest_type"), "lxc")
	var importedVMID types.Int64
	if diags := importResp.State.GetAttribute(context.Background(), path.Root("vm_id"), &importedVMID); diags.HasError() || importedVMID.ValueInt64() != 202 {
		t.Fatalf("unexpected imported vm_id: %v diagnostics=%v", importedVMID, diags)
	}
	invalidImportResp := resource.ImportStateResponse{State: testResourceState(t, schema, guestFirewallOptionsModel{ID: types.StringNull(), Node: types.StringNull(), VMID: types.Int64Null(), GuestType: types.StringNull()})}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "pve/not-a-number/qemu"}, &invalidImportResp)
	if !invalidImportResp.Diagnostics.HasError() || !containsDiagnostic(invalidImportResp.Diagnostics, "not-a-number") || !containsDiagnostic(invalidImportResp.Diagnostics, "invalid syntax") {
		t.Fatalf("expected invalid VMID context and parser detail: %v", invalidImportResp.Diagnostics)
	}
	invalidGuestTypeState := testResourceState(t, schema, guestFirewallOptionsModel{ID: types.StringValue("existing/999/qemu"), Node: types.StringValue("existing"), VMID: types.Int64Value(999), GuestType: types.StringValue("qemu")})
	invalidGuestTypeImportResp := resource.ImportStateResponse{State: invalidGuestTypeState}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "pve/202/openvz"}, &invalidGuestTypeImportResp)
	if !invalidGuestTypeImportResp.Diagnostics.HasError() || !containsDiagnostic(invalidGuestTypeImportResp.Diagnostics, "guest_type must be 'qemu' or 'lxc'") {
		t.Fatalf("expected invalid guest import type diagnostic: %v", invalidGuestTypeImportResp.Diagnostics)
	}
	if !invalidGuestTypeImportResp.State.Raw.Equal(invalidGuestTypeState.Raw) {
		t.Fatalf("invalid guest import type partially mutated state: %v", invalidGuestTypeImportResp.State.Raw)
	}
	invalid := initial
	invalid.GuestType = types.StringValue("openvz")
	invalidResp := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: testResourcePlan(t, schema, invalid)}, &invalidResp)
	if !invalidResp.Diagnostics.HasError() || !containsDiagnostic(invalidResp.Diagnostics, "guest_type must be 'qemu' or 'lxc'") {
		t.Fatalf("expected guest type validation diagnostic: %v", invalidResp.Diagnostics)
	}
}

func TestFirewallOptionRequestMappingNullVersusExplicitValues(t *testing.T) {
	cluster := clusterFirewallOptionsRequestFromModel(clusterFirewallOptionsModel{Enable: types.BoolValue(false), Ebtables: types.BoolNull()})
	if cluster.Enable == nil || *cluster.Enable || cluster.Ebtables != nil {
		t.Fatalf("cluster null/false mapping failed: %#v", cluster)
	}
	node := firewallOptionsRequestFromModel(nodeFirewallOptionsModel{Enable: types.BoolValue(false), NFConntrackMax: types.Int64Value(0), ProtectionSynfloodRate: types.Int64Null()})
	if node.Enable == nil || *node.Enable || node.NFConntrackMax == nil || *node.NFConntrackMax != 0 || node.ProtectionSynfloodRate != nil {
		t.Fatalf("node null/false/zero mapping failed: %#v", node)
	}
	guest := guestFWRequestFromModel(guestFirewallOptionsModel{Enable: types.BoolValue(false), DHCP: types.BoolNull()})
	if guest.Enable == nil || *guest.Enable || guest.DHCP != nil {
		t.Fatalf("guest null/false mapping failed: %#v", guest)
	}
}
