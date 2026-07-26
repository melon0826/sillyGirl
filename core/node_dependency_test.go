package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureNodePackageJSONRepairsInvalidDependencyFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")
	if err := os.WriteFile(path, []byte(`{"name":"bad","version":"1.0.0","dependencies":{"ipp":"^2.0.1"},"devDependencies":[]}`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ensureNodePackageJSON(dir, "bad"); err != nil {
		t.Fatalf("ensureNodePackageJSON returned error: %v", err)
	}

	deps, err := readNodeDependencies(nodeDependencyPlugin{Name: "bad", Title: "bad", File: "main.js", Path: dir})
	if err != nil {
		t.Fatalf("readNodeDependencies returned error: %v", err)
	}
	if len(deps) != len(nodeSillygirlRuntimeDependencies)+1 {
		t.Fatalf("unexpected dependencies: %#v", deps)
	}
	names := map[string]bool{}
	for _, dep := range deps {
		names[dep.Name] = true
	}
	for name := range nodeSillygirlRuntimeDependencies {
		if !names[name] {
			t.Fatalf("missing dependency %s in %#v", name, deps)
		}
	}
	if !names["ipp"] {
		t.Fatalf("missing dependency ipp in %#v", deps)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	manifest := map[string]interface{}{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if _, ok := manifest["pnpm"]; ok {
		t.Fatalf("unexpected deprecated pnpm settings in %s", string(data))
	}
	workspace, err := os.ReadFile(filepath.Join(dir, "pnpm-workspace.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(workspace), "allowBuilds:\n  protobufjs: true\n") {
		t.Fatalf("missing protobufjs allowBuilds in %s", string(workspace))
	}
}

func TestEnsureNodePackageJSONCreatesPnpmBuildAllowlist(t *testing.T) {
	dir := t.TempDir()
	if err := ensureNodePackageJSON(dir, "new-plugin"); err != nil {
		t.Fatalf("ensureNodePackageJSON returned error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := map[string]interface{}{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if _, ok := manifest["pnpm"]; ok {
		t.Fatalf("unexpected deprecated pnpm settings in %s", string(data))
	}
	workspace, err := os.ReadFile(filepath.Join(dir, "pnpm-workspace.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(workspace), "allowBuilds:\n  protobufjs: true\n") {
		t.Fatalf("missing protobufjs allowBuilds in %s", string(workspace))
	}
}

func TestNodeRuntimeDependenciesIncludeExpress(t *testing.T) {
	for _, name := range []string{"@grpc/grpc-js", "express", "google-protobuf"} {
		if _, ok := nodeSillygirlRuntimeDependencies[name]; !ok {
			t.Fatalf("missing runtime dependency %s", name)
		}
	}
}

func TestEnsureNodeSillygirlModuleWritesRuntimeFiles(t *testing.T) {
	dir := t.TempDir()
	if err := ensureNodeSillygirlModule(dir); err != nil {
		t.Fatalf("ensureNodeSillygirlModule returned error: %v", err)
	}
	for _, name := range []string{
		filepath.Join("node_modules", "sillygirl", "index.js"),
		filepath.Join("node_modules", "sillygirl", "srpc.js"),
		filepath.Join("node_modules", "sillygirl", "sillygirl.d.ts"),
		filepath.Join("node_modules", "sillygirl", "package.json"),
		filepath.Join("node_modules", "sillygirl.d.ts"),
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("expected runtime file %s: %v", name, err)
		}
	}
}

func TestNormalizeNodeScriptFileName(t *testing.T) {
	tests := []struct {
		name string
		want string
		ok   bool
	}{
		{name: "daily-sign", want: "daily-sign.js", ok: true},
		{name: "daily-sign.js", want: "daily-sign.js", ok: true},
		{name: "bad.ts", ok: false},
		{name: "../bad.js", ok: false},
		{name: "bad/name.js", ok: false},
	}
	for _, tt := range tests {
		got, err := normalizeNodeScriptFileName(tt.name)
		if tt.ok && err != nil {
			t.Fatalf("normalizeNodeScriptFileName(%q) returned error: %v", tt.name, err)
		}
		if !tt.ok && err == nil {
			t.Fatalf("normalizeNodeScriptFileName(%q) expected error, got %q", tt.name, got)
		}
		if tt.ok && got != tt.want {
			t.Fatalf("normalizeNodeScriptFileName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestNormalizePythonDependencyName(t *testing.T) {
	tests := map[string]string{
		"requests==2.32.0":   "requests",
		"pydantic[email]":    "pydantic",
		"beautiful_soup4":    "beautiful-soup4",
		"urllib.parse":       "",
		"../bad":             "",
		"https://bad/pkg.py": "",
	}
	for input, want := range tests {
		if got := normalizePythonDependencyName(input); got != want {
			t.Fatalf("normalizePythonDependencyName(%q) = %q; want %q", input, got, want)
		}
	}
}

func TestNormalizePipxRegistryDefault(t *testing.T) {
	got, err := normalizePipxRegistry("")
	if err != nil {
		t.Fatalf("normalizePipxRegistry returned error: %v", err)
	}
	if got != defaultPipxRegistry {
		t.Fatalf("normalizePipxRegistry(\"\") = %q; want %q", got, defaultPipxRegistry)
	}
}
