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
		`ISO_VERSION="${PROXMOX_E2E_ISO_VERSION:-9.2-1}"`,
		`ISO_URL="${PROXMOX_E2E_ISO_URL:-https://enterprise.proxmox.com/iso/proxmox-ve_9.2-1.iso}"`,
		`ISO_SHA256="${PROXMOX_E2E_ISO_SHA256:-4e88fe416df9b527624a175f24c9aa07c714d3332afb1ee3dbf3879573ef2c6c}"`,
		`ISO_CHECKSUM_URL="${PROXMOX_E2E_ISO_CHECKSUM_URL:-https://enterprise.proxmox.com/iso/proxmox-ve_9.2-1.iso.sha256}"`,
		`keyboard = "en-us"`,
		`root-password = "$ROOT_PASSWORD"`,
		`reboot-on-error = false`,
		`disk-list = ['vda']`,
		"-no-reboot",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("answer template missing assistant schema field %q", required)
		}
	}

	for _, unsupported := range []string{
		"root_password",
		"reboot_on_error",
		"disk_list",
	} {
		if strings.Contains(content, unsupported) {
			t.Fatalf("answer template contains unsupported assistant schema field %q", unsupported)
		}
	}
}
