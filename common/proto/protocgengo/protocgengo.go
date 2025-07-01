// Package protocgengo allows to bootstrap a relatively hermetic protoc-gen-go.
//
// Does not support Windows due to reliance on #!shebang.
package protocgengo

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Note we need to use *.template extension to avoid checking in go.mod file.
// This file will be recognized by Go as an indicator of a nested module, with
// following bad properties:
//   1. "go:embed" doesn't work across module boundaries.
//   2. It agitates various linters and makes life unnecessarily complicated.

//go:embed template/*.template
var content embed.FS

// Bootstrap creates a temporary directory with `protoc-gen-go` executable.
//
// Remove it after use.
//
// The executable is expected to be called in an environment that has go1.24 or
// newer in PATH.
func Bootstrap() (string, error) {
	if runtime.GOOS == "windows" {
		return "", fmt.Errorf("not supported on Windows because this relies on #!shebang")
	}

	// Note we pick GOBIN because we set a+x on the script there and we better
	// be sure the corresponding mounted disk allows executable scripts (not all
	// temp dirs do).
	tmpRoot := os.Getenv("GOBIN")
	if tmpRoot == "" {
		tmpRoot = os.TempDir()
	}
	binRoot, err := os.MkdirTemp(tmpRoot, ".cproto")
	if err != nil {
		return "", err
	}

	put := func(src, dst string, perm os.FileMode) {
		if err == nil {
			blob, rerr := content.ReadFile(filepath.Join("template", src))
			if rerr != nil {
				panic(rerr) // impossible, the file is embedded
			}
			err = os.WriteFile(filepath.Join(binRoot, dst), blob, perm)
		}
	}

	put("go.mod.template", "go.mod", 0666)
	put("go.sum.template", "go.sum", 0666)
	put("protoc-gen-go.template", "protoc-gen-go", 0777)

	if err != nil {
		_ = os.RemoveAll(binRoot)
	}
	return binRoot, err
}
