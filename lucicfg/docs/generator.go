// Copyright 2018 The LUCI Authors.
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

//go:generate go run generator.go

package main

import (
	"fmt"
	"os"
	"strings"

	"go.chromium.org/luci/starlark/docgen"

	generated "go.chromium.org/luci/lucicfg/starlark"
)

func main() {
	os.Exit(func() int {
		loader := docgen.Loader{
			Source: sourceLoader,
		}
		builtins, err := loader.Load("@stdlib//builtins.star")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load stdlib - %s", err)
			return 1
		}
		visit("", builtins)
		return 0
	}())
}

func visit(indent string, symbols docgen.Symbols) {
	for _, sym := range symbols {
		if strings.HasPrefix(sym.Name, "_") {
			continue
		}
		if sym.Nested != nil {
			fmt.Printf("%s%s = {\n", indent, sym.Name)
			visit(indent+"  ", sym.Nested)
			fmt.Printf("%s}\n", indent)
		} else {
			fmt.Printf("%s%s\n", indent, sym)
		}
	}
}

// sourceLoader returns source code of a given module.
//
// The module is "@<mod>//<path>", where we recognize stdlib and proto modules.
func sourceLoader(module string) (string, error) {
	chunks := strings.Split(module, "//")
	if len(chunks) != 2 {
		return "", fmt.Errorf("bad module name %q, expecting @<mod>//<path>", module)
	}
	switch chunks[0] {
	case "@stdlib":
		return stdlibSource(chunks[1])
	case "@proto":
		return protoSource(chunks[1])
	}
	return "", fmt.Errorf("unknown module %q", module)
}

// stdlibSource returns source code of a stdlib module.
func stdlibSource(path string) (string, error) {
	src, ok := generated.Assets()["stdlib/"+path]
	if !ok {
		return "", fmt.Errorf("unknown stdlib module %q", path)
	}
	return src, nil
}

// protoSource returns auto-generated source code of a proto module.
func protoSource(path string) (string, error) {
	return "", nil // not supported for now
}
