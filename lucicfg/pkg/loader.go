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

package pkg

import (
	"context"
	"path"
	"strings"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/interpreter"

	"go.chromium.org/luci/lucicfg/fileset"
)

// GenericLoaderParams is parameters for GenericLoader.
//
// All paths are slash-separated and relative to the package root.
type GenericLoaderParams struct {
	// Package is the package being loaded for error messages.
	Package string
	// Resources is a set of resource files in the package.
	Resources *fileset.Set
	// IsVisible checks if the given directory actually belongs to the package.
	IsVisible func(ctx context.Context, dir string) (bool, error)
	// Fetch fetches a single file.
	Fetch func(ctx context.Context, path string) ([]byte, error)
}

// GenericLoader implements interpreter.Loader on top of given callbacks.
func GenericLoader(params GenericLoaderParams) interpreter.Loader {
	return func(ctx context.Context, p string) (dict starlark.StringDict, src string, err error) {
		p = path.Clean(p)
		if p == "." || p == ".." || strings.HasPrefix(p, "../") {
			return nil, "", errors.New("outside the package root")
		}

		if params.IsVisible != nil {
			switch visible, err := params.IsVisible(ctx, path.Dir(p)); {
			case err != nil:
				return nil, "", errors.Annotate(err, "checking visibility of %q in %q", p, params.Package).Err()
			case !visible:
				return nil, "", errors.Reason("directory %q belongs to a different (nested) package and files from it cannot be loaded directly by %q", path.Dir(p), params.Package).Err()
			}
		}

		if !strings.HasSuffix(p, ".star") {
			switch loadable, err := params.Resources.Contains(p); {
			case err != nil:
				return nil, "", errors.Annotate(err, "checking %q against pkg.resources(...) patterns in %q", p, params.Package).Err()
			case !loadable:
				return nil, "", errors.Reason(
					"this non-starlark file is not declared as a resource in "+
						"pkg.resources(...) in PACKAGE.star of %q and cannot be loaded", params.Package).Err()
			}
		}

		blob, err := params.Fetch(ctx, p)
		return nil, string(blob), err
	}
}
