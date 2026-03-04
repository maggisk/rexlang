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
