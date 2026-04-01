package provider

import "testing"

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:     "base URL appends api path",
			input:    "https://pve.example.com:8006",
			expected: "https://pve.example.com:8006/api2/json",
		},
		{
			name:     "existing api path preserved",
			input:    "https://pve.example.com:8006/api2/json",
			expected: "https://pve.example.com:8006/api2/json",
		},
		{
			name:        "query string rejected",
			input:       "https://pve.example.com:8006?foo=bar",
			expectError: true,
		},
		{
			name:        "scheme required",
			input:       "pve.example.com:8006",
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			endpoint, err := normalizeEndpoint(test.input)
			if test.expectError {
				if err == nil {
					t.Fatalf("expected an error for %q", test.input)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error for %q: %v", test.input, err)
			}

			if got := endpoint.String(); got != test.expected {
				t.Fatalf("unexpected normalized endpoint: got %q want %q", got, test.expected)
			}
		})
	}
}
