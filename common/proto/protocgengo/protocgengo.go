// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

//go:embed template/*/*.template
var content embed.FS

// Version is enumeration of versions of protoc-gen-go we know how to bootstrap.
//
// Note we can't just use any version, since we need to check in corresponding
// go.sum first.
type Version string

const (
	// AncientVersion indicates to use protoc-gen-go bundled with protobuf v1.
	AncientVersion Version = "ancient"
	// ModernVersion indicates to use protoc-gen-go bundled with protobuf v2.
	ModernVersion Version = "modern"
)

// Bootstrap creates a temporary directory with `protoc-gen-go` executable.
//
// Remove it after use.
//
// The executable is expected to be called in an environment that has go1.24 or
// newer in PATH.
func Bootstrap(ver Version) (string, error) {
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
			blob, rerr := content.ReadFile(filepath.Join("template", string(ver), src))
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
