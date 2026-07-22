// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sync"
	"testing"
)

type lifecycleHandler struct {
	mu       sync.Mutex
	failures []string
}

func (h *lifecycleHandler) fail(w http.ResponseWriter, format string, args ...any) bool {
	message := fmt.Sprintf(format, args...)
	h.mu.Lock()
	h.failures = append(h.failures, message)
	h.mu.Unlock()
	http.Error(w, message, http.StatusInternalServerError)
	return false
}

func (h *lifecycleHandler) auth(w http.ResponseWriter, r *http.Request) bool {
	want := "PVEAPIToken=terraform@pve!provider=token-secret"
	if got := r.Header.Get("Authorization"); got != want {
		return h.fail(w, "unexpected authorization header: got %q want %q", got, want)
	}
	if cookie := r.Header.Get("Cookie"); cookie != "" {
		return h.fail(w, "expected no auth cookie, got %q", cookie)
	}
	return true
}

func (h *lifecycleHandler) form(w http.ResponseWriter, r *http.Request, want url.Values) bool {
	var got url.Values
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
		if err := r.ParseForm(); err != nil {
			return h.fail(w, "parse form: %v", err)
		}
		got = r.Form
	} else {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return h.fail(w, "read form: %v", err)
		}
		got, err = url.ParseQuery(string(body))
		if err != nil {
			return h.fail(w, "parse form: %v", err)
		}
	}
	if !reflect.DeepEqual(got, want) {
		return h.fail(w, "unexpected form: got %#v want %#v", got, want)
	}
	return true
}

func (h *lifecycleHandler) envelope(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"data": data}); err != nil {
		h.mu.Lock()
		h.failures = append(h.failures, fmt.Sprintf("encode response: %v", err))
		h.mu.Unlock()
	}
}

func (h *lifecycleHandler) assert(t *testing.T) {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.failures) > 0 {
		t.Fatalf("HTTP handler failures: %v", h.failures)
	}
}
