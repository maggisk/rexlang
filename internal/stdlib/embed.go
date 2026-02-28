package stdlib

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed rexfiles/*.rex
var stdlibFS embed.FS

// Source returns the source code for a stdlib module by name (e.g. "Prelude").
func Source(name string) (string, error) {
	data, err := fs.ReadFile(stdlibFS, "rexfiles/"+name+".rex")
	if err != nil {
		return "", fmt.Errorf("unknown stdlib module: %s", name)
	}
	return string(data), nil
}
