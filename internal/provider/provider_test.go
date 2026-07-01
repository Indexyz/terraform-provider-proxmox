// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories is used to instantiate a provider during acceptance testing.
// The factory function is called for each Terraform CLI command to create a provider
// server that the CLI can connect to and interact with.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"proxmox": providerserver.NewProtocol6WithError(New("test")()),
}

var (
	_ = testAccProtoV6ProviderFactories
	_ = testAccPreCheck
)

func testAccPreCheck(t *testing.T) {
	t.Helper()

	if os.Getenv(envEndpoint) == "" {
		t.Fatalf("%s must be set for acceptance tests", envEndpoint)
	}

	passwordAuthConfigured := os.Getenv(envUsername) != "" && os.Getenv(envPassword) != ""
	tokenAuthConfigured := os.Getenv(envAPITokenID) != "" && os.Getenv(envAPITokenSecret) != ""
	if !passwordAuthConfigured && !tokenAuthConfigured {
		t.Fatalf("set either %s/%s or %s/%s for acceptance tests", envUsername, envPassword, envAPITokenID, envAPITokenSecret)
	}
}
