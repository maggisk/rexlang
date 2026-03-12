package stdlib

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed all:rexfiles
var stdlibFS embed.FS

// Source returns the source code for a stdlib module by name (e.g. "Prelude", "Json.Decode").
func Source(name string) (string, error) {
	path := strings.ReplaceAll(name, ".", "/")
	data, err := fs.ReadFile(stdlibFS, "rexfiles/"+path+".rex")
	if err != nil {
		return "", fmt.Errorf("unknown stdlib module: %s", name)
	}
	return string(data), nil
}

// SourceForTarget returns the source code for a stdlib module, with target-specific
// overlay merged if available. For target "" or "native", returns the base module.
// For other targets (e.g. "browser"), loads Module.rex then overlays Module.browser.rex.
// If only the overlay exists (no base), returns just the overlay.
func SourceForTarget(name, target string) (string, error) {
	if target == "" || target == "native" {
		return Source(name)
	}
	path := strings.ReplaceAll(name, ".", "/")
	basePath := "rexfiles/" + path + ".rex"
	overlayPath := "rexfiles/" + path + "." + target + ".rex"

	baseData, baseErr := fs.ReadFile(stdlibFS, basePath)
	overlayData, overlayErr := fs.ReadFile(stdlibFS, overlayPath)

	if baseErr != nil && overlayErr != nil {
		return "", fmt.Errorf("unknown stdlib module: %s", name)
	}
	if baseErr != nil {
		// Overlay-only module (e.g. Js.browser.rex with no Js.rex)
		return string(overlayData), nil
	}
	if overlayErr != nil {
		// No overlay for this target — return base only
		return string(baseData), nil
	}
	// Both exist — concatenate base + overlay
	return string(baseData) + "\n" + string(overlayData), nil
}
