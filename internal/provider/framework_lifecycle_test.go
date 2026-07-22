// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func testDataSourceConfig(t *testing.T, schema datasource.SchemaResponse, configured map[string]any) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	terraformType, ok := schema.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("unexpected data source Terraform type %T", schema.Schema.Type().TerraformType(ctx))
	}
	values := make(map[string]tftypes.Value, len(terraformType.AttributeTypes))
	for name, attributeType := range terraformType.AttributeTypes {
		value := configured[name]
		values[name] = tftypes.NewValue(attributeType, value)
	}
	return tfsdk.Config{Schema: schema.Schema, Raw: tftypes.NewValue(terraformType, values)}
}

func testProviderConfig(t *testing.T, schema provider.SchemaResponse, model any) tfsdk.Config {
	t.Helper()
	plan := tfsdk.Plan{Schema: schema.Schema}
	if diags := plan.Set(context.Background(), model); diags.HasError() {
		t.Fatalf("set provider config: %v", diags)
	}
	return tfsdk.Config{Schema: schema.Schema, Raw: plan.Raw}
}

func testResourceSchema(t *testing.T, res resource.Resource) resource.SchemaResponse {
	t.Helper()
	var resp resource.SchemaResponse
	res.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	return resp
}

func testResourcePlan(t *testing.T, schema resource.SchemaResponse, model any) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{Schema: schema.Schema}
	if diags := plan.Set(context.Background(), model); diags.HasError() {
		t.Fatalf("set resource plan: %v", diags)
	}
	return plan
}

func testResourceState(t *testing.T, schema resource.SchemaResponse, model any) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: schema.Schema}
	if diags := state.Set(context.Background(), model); diags.HasError() {
		t.Fatalf("set resource state: %v", diags)
	}
	return state
}

func testResourceConfig(t *testing.T, schema resource.SchemaResponse, model any) tfsdk.Config {
	t.Helper()
	plan := testResourcePlan(t, schema, model)
	return tfsdk.Config{Schema: schema.Schema, Raw: plan.Raw}
}

func testResourceConfigValues(t *testing.T, schema resource.SchemaResponse, configured map[string]any) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	terraformType, ok := schema.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("unexpected resource Terraform type %T", schema.Schema.Type().TerraformType(ctx))
	}
	values := make(map[string]tftypes.Value, len(terraformType.AttributeTypes))
	for name, attributeType := range terraformType.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, configured[name])
	}
	return tfsdk.Config{Schema: schema.Schema, Raw: tftypes.NewValue(terraformType, values)}
}

func testLifecycleClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := NewClient(context.Background(), ClientConfig{
		Endpoint:       server.URL,
		APITokenID:     "terraform@pve!provider",
		APITokenSecret: "token-secret",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}
	return client
}

func TestProviderAndRegisteredTypeSchemasAndConfigure(t *testing.T) {
	for _, name := range []string{envEndpoint, envUsername, envPassword, envOTP, envAPITokenID, envAPITokenSecret, envInsecure, envTimeout} {
		t.Setenv(name, "")
	}

	ctx := context.Background()
	p := &ProxmoxProvider{version: "unit"}
	var metadata provider.MetadataResponse
	p.Metadata(ctx, provider.MetadataRequest{}, &metadata)
	if metadata.TypeName != "proxmox" || metadata.Version != "unit" {
		t.Fatalf("unexpected provider metadata: %#v", metadata)
	}
	var providerSchema provider.SchemaResponse
	p.Schema(ctx, provider.SchemaRequest{}, &providerSchema)
	if len(providerSchema.Schema.Attributes) != 9 {
		t.Fatalf("unexpected provider schema attribute count: %d", len(providerSchema.Schema.Attributes))
	}
	config := ProxmoxProviderModel{
		Endpoint:       types.StringValue("http://127.0.0.1:1"),
		APITokenID:     types.StringValue("terraform@pve!unit"),
		APITokenSecret: types.StringValue("secret"),
	}
	var configured provider.ConfigureResponse
	p.Configure(ctx, provider.ConfigureRequest{Config: testProviderConfig(t, providerSchema, config)}, &configured)
	if configured.Diagnostics.HasError() || configured.ResourceData == nil || configured.ResourceData != configured.DataSourceData {
		t.Fatalf("unexpected provider configure response: data=%T diagnostics=%v", configured.ResourceData, configured.Diagnostics)
	}

	invalidConfig := ProxmoxProviderModel{Endpoint: types.StringValue("http://127.0.0.1:1")}
	var invalid provider.ConfigureResponse
	p.Configure(ctx, provider.ConfigureRequest{Config: testProviderConfig(t, providerSchema, invalidConfig)}, &invalid)
	if !invalid.Diagnostics.HasError() || invalid.ResourceData != nil || invalid.DataSourceData != nil {
		t.Fatalf("expected invalid provider authentication diagnostics, got data=%T diagnostics=%v", invalid.ResourceData, invalid.Diagnostics)
	}

	for _, factory := range p.Resources(ctx) {
		res := factory()
		var schemaResp resource.SchemaResponse
		res.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
		if len(schemaResp.Schema.GetAttributes()) == 0 {
			t.Fatalf("%T returned an empty schema", res)
		}
		configurable, ok := res.(resource.ResourceWithConfigure)
		if !ok {
			t.Fatalf("%T does not implement ResourceWithConfigure", res)
		}
		var nilResp resource.ConfigureResponse
		configurable.Configure(ctx, resource.ConfigureRequest{}, &nilResp)
		if nilResp.Diagnostics.HasError() {
			t.Fatalf("%T nil configure diagnostics: %v", res, nilResp.Diagnostics)
		}
		var okResp resource.ConfigureResponse
		configurable.Configure(ctx, resource.ConfigureRequest{ProviderData: configured.ResourceData}, &okResp)
		if okResp.Diagnostics.HasError() {
			t.Fatalf("%T configure diagnostics: %v", res, okResp.Diagnostics)
		}
		var badResp resource.ConfigureResponse
		configurable.Configure(ctx, resource.ConfigureRequest{ProviderData: "wrong"}, &badResp)
		if !badResp.Diagnostics.HasError() {
			t.Fatalf("%T expected wrong provider data diagnostic", res)
		}
	}

	for _, factory := range p.DataSources(ctx) {
		ds := factory()
		var schemaResp datasource.SchemaResponse
		ds.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
		if len(schemaResp.Schema.GetAttributes()) == 0 {
			t.Fatalf("%T returned an empty schema", ds)
		}
		configurable, ok := ds.(datasource.DataSourceWithConfigure)
		if !ok {
			t.Fatalf("%T does not implement DataSourceWithConfigure", ds)
		}
		var nilResp datasource.ConfigureResponse
		configurable.Configure(ctx, datasource.ConfigureRequest{}, &nilResp)
		if nilResp.Diagnostics.HasError() {
			t.Fatalf("%T nil configure diagnostics: %v", ds, nilResp.Diagnostics)
		}
		var okResp datasource.ConfigureResponse
		configurable.Configure(ctx, datasource.ConfigureRequest{ProviderData: configured.DataSourceData}, &okResp)
		if okResp.Diagnostics.HasError() {
			t.Fatalf("%T configure diagnostics: %v", ds, okResp.Diagnostics)
		}
		var badResp datasource.ConfigureResponse
		configurable.Configure(ctx, datasource.ConfigureRequest{ProviderData: "wrong"}, &badResp)
		if !badResp.Diagnostics.HasError() {
			t.Fatalf("%T expected wrong provider data diagnostic", ds)
		}
	}
}
