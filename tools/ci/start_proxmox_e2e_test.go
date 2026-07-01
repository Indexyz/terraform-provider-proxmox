// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package ci

import (
	"os"
	"strings"
	"testing"
)

func TestStartProxmoxAcceptsUnauthorizedVersionResponseAsReady(t *testing.T) {
	script, err := os.ReadFile("start-proxmox-e2e.sh")
	if err != nil {
		t.Fatalf("read start script: %v", err)
	}

	content := string(script)
	for _, required := range []string{
		"--write-out '%{http_code}'",
		`"$http_code" == "401"`,
		"Proxmox API is ready",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("start script must include %q to treat Proxmox unauthenticated API responses as ready", required)
		}
	}
}
