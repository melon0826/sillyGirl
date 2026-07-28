package core

import "testing"

func TestPluginParseDefaultIcon(t *testing.T) {
	fn, _ := pluginParse(`/**
 * @title demo
 * @rule ^demo$
 */`, "demo")
	if fn.Icon != defaultPluginIconURL {
		t.Fatalf("Icon = %q; want %q", fn.Icon, defaultPluginIconURL)
	}
}

func TestPluginParseKeepsDeclaredIcon(t *testing.T) {
	const icon = "https://example.com/custom.png"
	fn, _ := pluginParse(`/**
 * @title demo
 * @rule ^demo$
 * @icon https://example.com/custom.png
 */`, "demo")
	if fn.Icon != icon {
		t.Fatalf("Icon = %q; want %q", fn.Icon, icon)
	}
}
