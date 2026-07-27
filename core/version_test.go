package core

import "testing"

func TestCompiledAndCurrentAppVersion(t *testing.T) {
	old := compiled_at
	t.Cleanup(func() {
		compiled_at = old
	})

	compiled_at = "v0.1.9"
	if got := compiledAppVersion(); got != "v0.1.9" {
		t.Fatalf("compiledAppVersion() = %q, want %q", got, "v0.1.9")
	}
	if got := currentAppVersion(); got != "0.1.9" {
		t.Fatalf("currentAppVersion() = %q, want %q", got, "0.1.9")
	}

	compiled_at = "dev-abcdef0"
	if got := compiledAppVersion(); got != "dev-abcdef0" {
		t.Fatalf("compiledAppVersion() = %q, want %q", got, "dev-abcdef0")
	}
	if got := currentAppVersion(); got != "dev-abcdef0" {
		t.Fatalf("currentAppVersion() = %q, want %q", got, "dev-abcdef0")
	}
}

func TestBackendVersionStorageKeysAreReserved(t *testing.T) {
	for _, key := range []string{"compiled_at", "latest_version", "remote_version", "version"} {
		if !isBackendVersionStorageKey("sillyGirl", key) {
			t.Fatalf("expected sillyGirl.%s to be reserved", key)
		}
		if !isBackendVersionStorageKey("app", key) {
			t.Fatalf("expected app.%s alias to be reserved", key)
		}
	}
	if isBackendVersionStorageKey("plugins", "version") {
		t.Fatalf("plugin version metadata must not be reserved")
	}
	keys := filterBackendVersionStorageKeys("sillyGirl", []string{"name", "version", "remote_version", "port"})
	if len(keys) != 2 || keys[0] != "name" || keys[1] != "port" {
		t.Fatalf("filtered keys = %#v; want name, port", keys)
	}
}
