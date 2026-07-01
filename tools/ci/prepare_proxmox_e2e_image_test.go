// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package ci

import (
	"os"
	"strings"
	"testing"
)

func TestPrepareProxmoxE2EAnswerTemplateMatchesAssistantSchema(t *testing.T) {
	script, err := os.ReadFile("prepare-proxmox-e2e-image.sh")
	if err != nil {
		t.Fatalf("read prepare script: %v", err)
	}
	content := string(script)

	for _, required := range []string{
		`keyboard = "en-us"`,
		`root_password = "$ROOT_PASSWORD"`,
		`reboot_on_error = false`,
		`disk_list = ['vda']`,
		"-no-reboot",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("answer template missing assistant schema field %q", required)
		}
	}

	for _, unsupported := range []string{
		"root-password",
		"reboot-mode",
		"disk-list",
	} {
		if strings.Contains(content, unsupported) {
			t.Fatalf("answer template contains unsupported assistant schema field %q", unsupported)
		}
	}
}
