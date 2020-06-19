// Copyright 2020 The LUCI Authors.
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

// Package buildifier implements processing of Starlark files via buildifier.
//
// Buildifier is primarily intended for Bazel files. We try to disable as much
// of Bazel-specific logic as possible, keeping only generally useful
// Starlark rules.
package buildifier

import (
	"runtime"
	"sync"

	"github.com/bazelbuild/buildtools/build"
	"github.com/bazelbuild/buildtools/tables"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/starlark/interpreter"
)

// Visitor processes a parsed Starlark file, returning all errors encountered
// when processing it.
type Visitor func(path string, body []byte, f *build.File) errors.MultiError

// Visit parses Starlark files using Buildifier and calls the callback for each
// parsed file, in parallel.
//
// Collects all errors from all callbacks in a single joint multi-error.
func Visit(loader interpreter.Loader, paths []string, v Visitor) errors.MultiError {
	initTables()

	m := sync.Mutex{}
	perPath := make(map[string]errors.MultiError, len(paths))

	parallel.WorkPool(runtime.NumCPU(), func(tasks chan<- func() error) {
		for _, path := range paths {
			path := path
			tasks <- func() error {
				var errs []error
				switch body, f, err := parseFile(loader, path); {
				case err != nil:
					errs = []error{err}
				case f != nil:
					errs = v(path, body, f)
				}
				m.Lock()
				perPath[path] = errs
				m.Unlock()
				return nil
			}
		}
	})

	// Assemble errors in original order.
	var errs errors.MultiError
	for _, path := range paths {
		errs = append(errs, perPath[path]...)
	}
	return errs
}

var tablesOnce sync.Once

// initTables tweaks Buildifier to forget as much as possible about Bazel rules.
func initTables() {
	tablesOnce.Do(func() {
		tables.OverrideTables(nil, nil, nil, nil, nil, nil, nil, false, false)
	})
}

// parseFile parses a Starlark module using the buildifier parser.
//
// Returns (nil, nil, nil) if the module is a native Go module.
func parseFile(loader interpreter.Loader, path string) ([]byte, *build.File, error) {
	switch dict, src, err := loader(path); {
	case err != nil:
		return nil, nil, err
	case dict != nil:
		return nil, nil, nil
	default:
		body := []byte(src)
		f, err := build.ParseDefault(path, body)
		if f != nil {
			f.Type = build.TypeDefault // always generic Starlark file, not a BUILD
			f.Label = path             // lucicfg loader paths ~= map to Bazel labels
		}
		return body, f, err
	}
}
