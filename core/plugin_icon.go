package core

import "strings"

const defaultPluginIconURL = "https://api.iconify.design/lucide:apple.svg"

func pluginIconOrDefault(icon string) string {
	icon = strings.TrimSpace(icon)
	if icon == "" {
		return defaultPluginIconURL
	}
	return icon
}
