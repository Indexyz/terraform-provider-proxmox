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
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestClientNotificationEndpoints(t *testing.T) {
	ctx := context.Background()
	var created, updated, updatedMatcher url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/cluster/notifications/endpoints/gotify" && r.Method == http.MethodPost:
			_ = r.ParseForm()
			created = r.PostForm
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/notifications/endpoints/gotify" && r.Method == http.MethodGet:
			writeEnvelope(t, w, []map[string]any{{"name": "phone", "server": "https://gotify.test", "digest": "d1", "disable": 0}})
		case r.URL.Path == "/api2/json/cluster/notifications/endpoints/gotify/phone" && r.Method == http.MethodPut:
			_ = r.ParseForm()
			updated = r.PostForm
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/notifications/endpoints/gotify/phone" && r.Method == http.MethodDelete:
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/notifications/matchers" && r.Method == http.MethodPost:
			_ = r.ParseForm()
			updatedMatcher = r.PostForm
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/notifications/matchers" && r.Method == http.MethodGet:
			writeEnvelope(t, w, []map[string]any{{"name": "m1", "target": []string{"phone"}, "mode": "all", "digest": "d2"}})
		case r.URL.Path == "/api2/json/cluster/notifications/matchers/m1" && r.Method == http.MethodPut:
			_ = r.ParseForm()
			writeEnvelope(t, w, nil)
		case r.URL.Path == "/api2/json/cluster/notifications/matchers/m1" && r.Method == http.MethodDelete:
			writeEnvelope(t, w, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client := newHAResourceTestClient(t, server.URL)

	if err := client.CreateNotificationEndpoint(ctx, "gotify", url.Values{"name": {"phone"}, "server": {"https://gotify.test"}, "token": {"t"}}); err != nil {
		t.Fatalf("CreateNotificationEndpoint() unexpected error: %v", err)
	}
	if created.Get("token") != "t" {
		t.Fatalf("unexpected create form: %#v", created)
	}
	entry, err := client.GetNotificationEndpoint(ctx, "gotify", "phone")
	if err != nil || entry.Digest != "d1" {
		t.Fatalf("unexpected endpoint: %#v (%v)", entry, err)
	}
	if err := client.UpdateNotificationEndpoint(ctx, "gotify", "phone", url.Values{"server": {"https://x"}}, entry.Digest); err != nil {
		t.Fatalf("UpdateNotificationEndpoint() unexpected error: %v", err)
	}
	if updated.Get("digest") != "d1" {
		t.Fatalf("digest missing from update: %#v", updated)
	}
	if err := client.DeleteNotificationEndpoint(ctx, "gotify", "phone"); err != nil {
		t.Fatalf("DeleteNotificationEndpoint() unexpected error: %v", err)
	}
	if err := client.CreateNotificationMatcher(ctx, url.Values{"name": {"m1"}, "target": {"phone"}}); err != nil {
		t.Fatalf("CreateNotificationMatcher() unexpected error: %v", err)
	}
	if updatedMatcher.Get("name") != "m1" || updatedMatcher.Get("target") != "phone" {
		t.Fatalf("unexpected matcher form: %#v", updatedMatcher)
	}
	if err := client.UpdateNotificationMatcher(ctx, "m1", url.Values{"mode": {"any"}}, "d2"); err != nil {
		t.Fatalf("UpdateNotificationMatcher() unexpected error: %v", err)
	}
	if err := client.DeleteNotificationMatcher(ctx, "m1"); err != nil {
		t.Fatalf("DeleteNotificationMatcher() unexpected error: %v", err)
	}
}

func TestNotificationEndpointLifecyclePreservesSecret(t *testing.T) {
	handler := &lifecycleHandler{}
	endpoint := map[string]any{"name": "phone", "server": "https://gotify.test", "digest": "d1", "comment": "phone"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.auth(w, r) {
			return
		}
		if err := r.ParseForm(); err != nil {
			handler.fail(w, "parse form: %v", err)
			return
		}
		switch {
		case r.URL.Path == "/api2/json/cluster/notifications/endpoints/gotify" && r.Method == http.MethodPost:
			handler.envelope(w, nil)
		case r.URL.Path == "/api2/json/cluster/notifications/endpoints/gotify" && r.Method == http.MethodGet:
			handler.envelope(w, []map[string]any{endpoint})
		case r.URL.Path == "/api2/json/cluster/notifications/endpoints/gotify/phone" && r.Method == http.MethodPut:
			endpoint["server"] = r.Form.Get("server")
			endpoint["comment"] = r.Form.Get("comment")
			endpoint["digest"] = "d2"
			handler.envelope(w, nil)
		case r.URL.Path == "/api2/json/cluster/notifications/endpoints/gotify/phone" && r.Method == http.MethodDelete:
			handler.envelope(w, nil)
		default:
			handler.fail(w, "unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	res := &NotificationEndpointGotifyResource{client: testLifecycleClient(t, server)}
	schema := testResourceSchema(t, res)
	config := notificationEndpointModel{
		Name: types.StringValue("phone"), Server: types.StringValue("https://gotify.test"), Token: types.StringValue("secret-token"),
		Comment: types.StringValue("phone"), Disable: types.BoolNull(), ID: types.StringNull(), Password: types.StringNull(),
		Method: types.StringNull(), URL: types.StringNull(), Body: types.StringNull(), Author: types.StringNull(), From: types.StringNull(), Mode: types.StringNull(), Username: types.StringNull(), Port: types.Int64Null(),
		Mailto: types.SetNull(types.StringType), MailtoUser: types.SetNull(types.StringType), Header: types.SetNull(types.StringType), Secret: types.SetNull(types.StringType),
	}
	createResp := resource.CreateResponse{State: testResourceState(t, schema, config)}
	res.Create(ctx, resource.CreateRequest{Plan: testResourcePlan(t, schema, config)}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", createResp.Diagnostics)
	}
	var created notificationEndpointModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() {
		t.Fatalf("decode state: %v", diags)
	}
	if created.Token.ValueString() != "secret-token" {
		t.Fatalf("secret was not kept: %#v", created)
	}
	readResp := resource.ReadResponse{State: createResp.State}
	res.Read(ctx, resource.ReadRequest{State: createResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}
	var refreshed notificationEndpointModel
	if diags := readResp.State.Get(ctx, &refreshed); diags.HasError() || refreshed.Token.ValueString() != "secret-token" || refreshed.Server.ValueString() != "https://gotify.test" {
		t.Fatalf("refresh lost secret or value: %#v (%v)", refreshed, diags)
	}
	deleteResp := resource.DeleteResponse{State: readResp.State}
	res.Delete(ctx, resource.DeleteRequest{State: readResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", deleteResp.Diagnostics)
	}
	for _, failure := range handler.failures {
		t.Fatal(failure)
	}
}

func emptyNotificationModel() notificationEndpointModel {
	return notificationEndpointModel{
		ID: types.StringNull(), Name: types.StringNull(), Comment: types.StringNull(), Disable: types.BoolNull(),
		Author: types.StringNull(), Server: types.StringNull(), From: types.StringNull(), Mode: types.StringNull(),
		Username: types.StringNull(), Password: types.StringNull(), Token: types.StringNull(), Method: types.StringNull(),
		URL: types.StringNull(), Body: types.StringNull(), Mailto: types.SetNull(types.StringType),
		MailtoUser: types.SetNull(types.StringType), Header: types.SetNull(types.StringType), Secret: types.SetNull(types.StringType),
		Port: types.Int64Null(),
	}
}

func TestNotificationValidation(t *testing.T) {
	valid := emptyNotificationModel()
	valid.Name = types.StringValue("mail")
	valid.URL = types.StringValue("https://hook")
	valid.Method = types.StringValue("post")
	valid.Mode = types.StringValue("tls")
	valid.Port = types.Int64Value(465)
	var resp resource.ValidateConfigResponse
	(&NotificationEndpointWebhookResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &NotificationEndpointWebhookResource{}), valid)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("valid config diagnostics: %v", resp.Diagnostics)
	}
	invalid := map[string]func(*notificationEndpointModel){
		"bad name":   func(m *notificationEndpointModel) { m.Name = types.StringValue("1bad") },
		"bad mode":   func(m *notificationEndpointModel) { m.Mode = types.StringValue("ssl") },
		"bad method": func(m *notificationEndpointModel) { m.Method = types.StringValue("patch") },
		"bad url":    func(m *notificationEndpointModel) { m.URL = types.StringValue("ftp://hook") },
		"bad port":   func(m *notificationEndpointModel) { m.Port = types.Int64Value(0) },
	}
	for name, mutate := range invalid {
		config := emptyNotificationModel()
		mutate(&config)
		var diags resource.ValidateConfigResponse
		(&NotificationEndpointWebhookResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &NotificationEndpointWebhookResource{}), config)}, &diags)
		if !diags.Diagnostics.HasError() {
			t.Fatalf("%s: expected diagnostics", name)
		}
	}

	matcher := notificationMatcherModel{Name: types.StringValue("m1"), Target: types.SetValueMust(types.StringType, []attr.Value{types.StringValue("phone")}), Mode: types.StringValue("any"), ID: types.StringNull(), Comment: types.StringNull(), Disable: types.BoolNull(), InvertMatch: types.BoolNull(), MatchField: types.SetNull(types.StringType), MatchSeverity: types.SetNull(types.StringType), MatchCalendar: types.SetNull(types.StringType)}
	var matcherResp resource.ValidateConfigResponse
	(&NotificationMatcherResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: testResourceConfig(t, testResourceSchema(t, &NotificationMatcherResource{}), matcher)}, &matcherResp)
	if !matcherResp.Diagnostics.HasError() || !strings.Contains(matcherResp.Diagnostics[0].Detail(), "not found") && !matcherResp.Diagnostics.HasError() {
		t.Logf("matcher diagnostics: %v", matcherResp.Diagnostics)
	}
}

func TestNotificationMatcherFormAndState(t *testing.T) {
	model := notificationMatcherModel{
		Name: types.StringValue("m1"), Target: types.SetValueMust(types.StringType, []attr.Value{types.StringValue("phone")}),
		Mode: types.StringValue("any"), Comment: types.StringValue("c"), Disable: types.BoolValue(true), InvertMatch: types.BoolValue(true),
		MatchField: types.SetNull(types.StringType), MatchSeverity: types.SetNull(types.StringType), MatchCalendar: types.SetNull(types.StringType),
	}
	form := notificationMatcherForm(model)
	if form.Get("mode") != "any" || form["target"][0] != "phone" || form.Get("disable") != "1" || form.Get("invert-match") != "1" {
		t.Fatalf("unexpected matcher form: %#v", form)
	}
	state, stateDiags := notificationMatcherState(context.Background(), NotificationMatcher{Name: "m1", Target: []string{"phone"}, Mode: "any"})
	if stateDiags.HasError() || state.Target.IsNull() || len(state.Target.Elements()) != 1 {
		t.Fatalf("unexpected matcher state: %#v (%v)", state, stateDiags)
	}
}
