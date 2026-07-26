package core

import "testing"

func TestCompiledAndCurrentAppVersion(t *testing.T) {
	old := compiled_at
	t.Cleanup(func() {
		compiled_at = old
	})

	compiled_at = "v0.1.6"
	if got := compiledAppVersion(); got != "v0.1.6" {
		t.Fatalf("compiledAppVersion() = %q, want %q", got, "v0.1.6")
	}
	if got := currentAppVersion(); got != "0.1.6" {
		t.Fatalf("currentAppVersion() = %q, want %q", got, "0.1.6")
	}

	compiled_at = "dev-abcdef0"
	if got := compiledAppVersion(); got != "dev-abcdef0" {
		t.Fatalf("compiledAppVersion() = %q, want %q", got, "dev-abcdef0")
	}
	if got := currentAppVersion(); got != "dev-abcdef0" {
		t.Fatalf("currentAppVersion() = %q, want %q", got, "dev-abcdef0")
	}
}
