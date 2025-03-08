// Copyright 2019 The LUCI Authors.
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

// Command docgen is the documentation generator.
//
// It is used by go:generate in "docs" directory. Not really supposed to be
// used standalone.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/starlark/docgen"
)

func main() {
	templates := flag.String("templates", "", "a directory with input *.mdt templates")
	out := flag.String("out", "", "where to put generated files")
	starlark := flag.String("starlark", "", "directory with the starlark source code")

	flag.Parse()

	if err := run(*templates, *starlark, *out); err != nil {
		fmt.Fprintf(os.Stderr, "docgen: %s\n", err)
		os.Exit(1)
	}
}

func run(templates, starlark, outDir string) error {
	// All input templates with paths relative to 'templates' dir.
	var files []string
	err := filepath.WalkDir(templates, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(path, ".mdt") {
			var rel string
			if rel, err = filepath.Rel(templates, path); err == nil {
				files = append(files, rel)
			}
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to walk %q - %w", templates, err)
	}

	// Prepare the generator that can load starlark modules to extract docs from
	// them.
	gen := docgen.Generator{
		Normalize: func(p, s string) (string, error) { return s, nil },
		Starlark:  sourceProvider(starlark),
	}

	// For each input template spit out the corresponding generated *.md file.
	haveErrs := false
	for _, f := range files {
		in := filepath.Join(templates, f)
		out := filepath.Join(outDir, f)

		// Replace ".mdt" with ".md".
		out = out[:len(out)-1]

		fmt.Printf("Rendering %s\n", f)
		if err := generate(&gen, in, out); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			haveErrs = true
		}
	}

	if haveErrs {
		return fmt.Errorf("failed")
	}
	return nil
}

func generate(gen *docgen.Generator, in, out string) error {
	body, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	output, err := gen.Render(string(body))
	if err != nil {
		return err
	}
	return os.WriteFile(out, output, 0666)
}

func sourceProvider(root string) func(string) (string, error) {
	return func(module string) (src string, err error) {
		// 'module' here is something like "@stdlib//path".
		chunks := strings.SplitN(module, "//", 2)
		if len(chunks) != 2 || !strings.HasPrefix(chunks[0], "@") {
			return "", fmt.Errorf("unrecognized module path %s", module)
		}
		pkg := chunks[0][1:]
		path := chunks[1]

		// @proto package is not explorable for now. Can potentially auto-generate
		// it from proto descriptors, if necessary.
		if pkg == "proto" {
			return "", nil
		}

		fmt.Printf("  loading %q\n", module)
		body, err := os.ReadFile(filepath.Join(root, pkg, path))
		if err != nil {
			return "", err
		}
		return string(body), nil
	}
}
