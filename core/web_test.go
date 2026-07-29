package core

import "testing"

func TestNormalizeHTTPPort(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int
	}{
		{name: "plain", value: "8080", want: 8080},
		{name: "stored int", value: "d:8081", want: 8081},
		{name: "stored float", value: "f:8080.000000", want: 8080},
		{name: "invalid host", value: "f:8080.000000:bad", want: 8080},
		{name: "empty default", value: "", want: 8080},
		{name: "too high", value: "70000", want: 8080},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeHTTPPort(tt.value); got != tt.want {
				t.Fatalf("normalizeHTTPPort(%q) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestCanonicalHTTPPortValue(t *testing.T) {
	for _, value := range []string{"8080", "d:8080", "f:8080.000000"} {
		port, stored := canonicalHTTPPortValue(value)
		if port != 8080 || stored != "8080" {
			t.Fatalf("canonicalHTTPPortValue(%q) = (%d, %q), want (8080, 8080)", value, port, stored)
		}
	}
}
