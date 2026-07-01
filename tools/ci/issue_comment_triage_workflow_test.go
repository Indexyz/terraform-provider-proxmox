// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package ci

import (
	"os"
	"strings"
	"testing"
)

func TestIssueCommentTriageSkipsMissingWaitingResponseLabel(t *testing.T) {
	workflow, err := os.ReadFile("../../.github/workflows/issue-comment-triage.yml")
	if err != nil {
		t.Fatalf("read issue comment triage workflow: %v", err)
	}

	content := string(workflow)
	guard := "if: contains(github.event.issue.labels.*.name, 'waiting-response')"
	if !strings.Contains(content, guard) {
		t.Fatalf("workflow must guard waiting-response removal with %q", guard)
	}
}
